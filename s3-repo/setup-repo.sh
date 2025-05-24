#!/bin/bash

# Variables configurables
BUCKET_NAME="filesystem-daemon-repo"
REGION="us-east-1"

# Crear bucket
aws s3 mb s3://$BUCKET_NAME --region $REGION

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

# Configurar CORS
aws s3api put-bucket-cors --bucket $BUCKET_NAME --cors-configuration '{
    "CORSRules": [
        {
            "AllowedHeaders": ["*"],
            "AllowedMethods": ["GET", "HEAD"],
            "AllowedOrigins": ["*"],
            "MaxAgeSeconds": 3600
        }
    ]
}'

# Configurar website hosting (opcional)
aws s3 website s3://$BUCKET_NAME --index-document index.html --error-document error.html

# Crear estructura de directorios
aws s3 cp --recursive --acl public-read dist/ s3://$BUCKET_NAME/

# Generar clave GPG para firmar paquetes
if [ ! -f "repo-key.gpg" ]; then
    gpg --gen-key --batch <<EOF
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
    
    # Exportar la clave
    gpg --export -a "Filesystem Daemon Repo" > repo-key.gpg
    gpg --export-secret-key -a "Filesystem Daemon Repo" > repo-key-secret.gpg
fi

# Mostrar información de la clave
KEY_ID=$(gpg --list-secret-keys --with-colons | grep ^sec | cut -d: -f5)
echo "Clave GPG generada con ID: $KEY_ID"
echo "Actualizar el repo-config.yaml con este ID de clave"

# Configurar el bucket como punto de distribución
aws s3api put-bucket-versioning --bucket $BUCKET_NAME --versioning-configuration Status=Enabled
aws s3api put-bucket-encryption --bucket $BUCKET_NAME --server-side-encryption-configuration '{
    "Rules": [
        {
            "ApplyServerSideEncryptionByDefault": {
                "SSEAlgorithm": "AES256"
            }
        }
    ]
}'
