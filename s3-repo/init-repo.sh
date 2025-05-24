#!/bin/bash

# Variables configurables
BUCKET_NAME="filesystem-daemon-repo"
REGION="us-east-1"

# Verificar que el bucket existe
if ! aws s3api head-bucket --bucket "$BUCKET_NAME" 2>/dev/null; then
    echo "Creando bucket S3..."
    aws s3 mb s3://$BUCKET_NAME --region $REGION
fi

# Configurar políticas de bucket
aws s3api put-bucket-policy --bucket $BUCKET_NAME --policy '{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "PublicReadGetObject",
            "Effect": "Allow",
            "Principal": "*",
            "Action": "s3:GetObject",
            "Resource": "arn:aws:s3:::'$BUCKET_NAME'/*"
        }
    ]
}'

# Generar claves GPG si no existen
if [ ! -f "repo-key.gpg" ]; then
    echo "Generando claves GPG..."
    gpg --batch --gen-key <<EOF
    Key-Type: RSA
    Key-Length: 4096
    Subkey-Type: RSA
    Subkey-Length: 4096
    Name-Real: Filesystem Daemon Repo
    Name-Email: admin@filesystem-daemon-repo.com
    Expire-Date: 0
    %no-protection
    %commit
EOF

    # Exportar claves
    gpg --export -a "Filesystem Daemon Repo" > repo-key.gpg
    gpg --export-secret-key -a "Filesystem Daemon Repo" > repo-key-secret.gpg
fi

# Subir claves al bucket
aws s3 cp repo-key.gpg s3://$BUCKET_NAME/apt/ --acl public-read
aws s3 cp repo-key-secret.gpg s3://$BUCKET_NAME/apt/ --acl private

# Generar archivo de configuración APT
mkdir -p apt/conf

cat > apt/conf/apt.conf <<EOL
Acquire::http::Proxy "";

Dir::Etc::SourceList "/etc/apt/sources.list.d/filesystem-daemon.list";
Dir::Etc::SourceParts "/etc/apt/sources.list.d";

APT::Get::List-Cleanup "0";
APT::Get::AllowUnauthenticated "0";

Acquire::https::Verify-Peer "true";
Acquire::https::Verify-Host "true";

Acquire::Languages "none";
EOL

# Subir configuración
aws s3 cp --recursive apt/ s3://$BUCKET_NAME/apt/ --acl public-read

# Configurar website hosting
aws s3 website s3://$BUCKET_NAME --index-document index.html --error-document error.html

# Mostrar información de configuración
echo "Repositorio inicializado exitosamente"
echo "URL del repositorio: https://$BUCKET_NAME.s3.$REGION.amazonaws.com"
echo "Clave GPG pública: https://$BUCKET_NAME.s3.$REGION.amazonaws.com/apt/repo-key.gpg"
