#!/bin/bash

BUCKET_NAME="filesystem-daemon-repo"
REGION="us-east-1"

# Generar archivo de configuración APT
cat > apt.conf <<EOL
Acquire::http::Proxy "";

Dir::Etc::SourceList "/etc/apt/sources.list.d/filesystem-daemon.list";
Dir::Etc::SourceParts "/etc/apt/sources.list.d";

APT::Get::List-Cleanup "0";
APT::Get::AllowUnauthenticated "0";

Acquire::https::Verify-Peer "true";
Acquire::https::Verify-Host "true";

Acquire::Languages "none";
EOL

# Generar archivo de lista de fuentes
cat > filesystem-daemon.list <<EOL
deb [signed-by=/usr/share/keyrings/filesystem-daemon-archive-keyring.gpg arch=amd64] https://$BUCKET_NAME.s3.$REGION.amazonaws.com stable main
EOL

# Generar script de instalación
cat > install.sh <<EOL
#!/bin/bash

curl -fsSL "https://$BUCKET_NAME.s3.$REGION.amazonaws.com/apt.key" | sudo gpg --dearmor -o /usr/share/keyrings/filesystem-daemon-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/filesystem-daemon-archive-keyring.gpg arch=amd64] https://$BUCKET_NAME.s3.$REGION.amazonaws.com stable main" | sudo tee /etc/apt/sources.list.d/filesystem-daemon.list > /dev/null

sudo apt update
sudo apt install filesystem-daemon
EOL

chmod +x install.sh

# Subir archivos al bucket
aws s3 cp apt.conf "s3://$BUCKET_NAME/apt/conf/" --acl public-read
aws s3 cp filesystem-daemon.list "s3://$BUCKET_NAME/apt/" --acl public-read
aws s3 cp install.sh "s3://$BUCKET_NAME/apt/" --acl public-read

# Generar clave GPG para el repositorio
if [ ! -f "apt.key" ]; then
    gpg --gen-key --batch <<EOF
    Key-Type: RSA
    Key-Length: 4096
    Subkey-Type: RSA
    Subkey-Length: 4096
    Name-Real: Filesystem Daemon APT Repo
    Name-Email: apt@filesystem-daemon-repo.com
    Expire-Date: 0
    %no-protection
    %commit
EOF
    
    gpg --export -a "Filesystem Daemon APT Repo" > apt.key
fi

# Subir la clave GPG
aws s3 cp apt.key "s3://$BUCKET_NAME/apt/" --acl public-read

# Mostrar URL de instalación
echo "URL para instalar el paquete:"
echo "curl -fsSL \"https://$BUCKET_NAME.s3.$REGION.amazonaws.com/apt/install.sh\" | sudo bash"
