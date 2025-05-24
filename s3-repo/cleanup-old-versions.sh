#!/bin/bash

# Variables configurables
BUCKET_NAME="filesystem-daemon-repo"
DISTRIBUTION="stable"
COMPONENT="main"
ARCH="amd64"
MAX_VERSIONS=3

# Obtener lista de paquetes ordenados por versión
packages=$(aws s3 ls "s3://$BUCKET_NAME/pool/$COMPONENT/" --recursive | grep ".deb$" | sort -k4)

# Contar versiones por paquete
while read -r line; do
    filename=$(echo "$line" | awk '{print $4}')
    package=$(basename "$filename" | cut -d'_' -f1)
    version=$(basename "$filename" | cut -d'_' -f2)

    # Agregar a la lista si es una nueva versión
    if [ -z "${versions[$package]}" ]; then
        versions[$package]="$version"
    else
        # Comparar versiones
        if dpkg --compare-versions "$version" gt "${versions[$package]}"; then
            # Si la nueva versión es mayor, guardar la anterior para borrar
            old_versions[$package]+=" ${versions[$package]}"
            versions[$package]="$version"
        else
            old_versions[$package]+=" $version"
        fi
    fi
done <<< "$packages"

# Eliminar versiones antiguas
for package in "${!versions[@]}"; do
    old_versions_list=(${old_versions[$package]})
    if [ ${#old_versions_list[@]} -gt $((MAX_VERSIONS - 1)) ]; then
        # Ordenar versiones antiguas
        IFS=$'\n' sorted_versions=($(sort <<< "${old_versions_list[*]}"))
        unset IFS
        
        # Eliminar las versiones más antiguas
        for version in "${sorted_versions[@]:0:$((MAX_VERSIONS - 1))}"; do
            filename="$package_$version"*.deb
            echo "Eliminando versión antigua: $filename"
            aws s3 rm "s3://$BUCKET_NAME/pool/$COMPONENT/$filename"
        done
    fi
done

# Actualizar índices de paquetes
aws s3 cp "s3://$BUCKET_NAME/dists/$DISTRIBUTION/$COMPONENT/binary-$ARCH/Packages" "/tmp/Packages"
apt-ftparchive packages "/tmp/Packages" > "/tmp/Packages.new"
gpg --clearsign -o "/tmp/Packages" "/tmp/Packages.new"
aws s3 cp "/tmp/Packages" "s3://$BUCKET_NAME/dists/$DISTRIBUTION/$COMPONENT/binary-$ARCH/Packages" --acl public-read

# Limpiar archivos temporales
rm -f "/tmp/Packages" "/tmp/Packages.new"
