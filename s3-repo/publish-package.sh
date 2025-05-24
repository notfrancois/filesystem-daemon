#!/bin/bash

# Variables
PACKAGE_FILE=$1
BUCKET_NAME="filesystem-daemon-repo"
DISTRIBUTION="stable"
COMPONENT="main"
ARCH="amd64"

# Verificar que el paquete existe
if [ ! -f "$PACKAGE_FILE" ]; then
    echo "Error: Paquete no encontrado"
    exit 1
fi

# Extraer información del paquete
PACKAGE_NAME=$(dpkg-deb --info "$PACKAGE_FILE" | grep Package | awk '{print $2}')
PACKAGE_VERSION=$(dpkg-deb --info "$PACKAGE_FILE" | grep Version | awk '{print $2}')

# Crear directorio de paquetes si no existe
aws s3api put-object --bucket "$BUCKET_NAME" --key "pool/$COMPONENT/"

# Subir el paquete
aws s3 cp "$PACKAGE_FILE" "s3://$BUCKET_NAME/pool/$COMPONENT/" --acl public-read

# Crear symlink simbólico para la versión más reciente
aws s3api put-object --bucket "$BUCKET_NAME" --key "pool/$COMPONENT/$PACKAGE_NAME_latest.deb" --website-redirect-location "/pool/$COMPONENT/$PACKAGE_NAME_$PACKAGE_VERSION.deb"

# Actualizar el índice de paquetes
aws s3 cp "s3://$BUCKET_NAME/dists/$DISTRIBUTION/$COMPONENT/binary-$ARCH/Packages" "/tmp/Packages"

# Generar nuevo índice
apt-ftparchive packages "/tmp/Packages" > "/tmp/Packages.new"

# Firmar el índice
gpg --clearsign -o "/tmp/Packages" "/tmp/Packages.new"

# Subir el nuevo índice
aws s3 cp "/tmp/Packages" "s3://$BUCKET_NAME/dists/$DISTRIBUTION/$COMPONENT/binary-$ARCH/Packages" --acl public-read

# Limpiar archivos temporales
rm -f "/tmp/Packages" "/tmp/Packages.new"

# Mostrar URL del paquete
PACKAGE_URL="https://$BUCKET_NAME.s3.$REGION.amazonaws.com/pool/$COMPONENT/$PACKAGE_NAME_$PACKAGE_VERSION.deb"
echo "Paquete publicado exitosamente en: $PACKAGE_URL"
