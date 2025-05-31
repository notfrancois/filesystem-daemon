#!/bin/bash
set -e

# Crear directorio de certificados si no existe
mkdir -p certs
chmod 750 certs

# Generar certificados auto-firmados para desarrollo
if [ ! -f certs/server.key ] || [ ! -f certs/server.crt ]; then
    openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt -days 365 -nodes -subj "/CN=filesystem-daemon"
    chmod 640 certs/server.key certs/server.crt
fi

# Crear directorio de assets si no existe
mkdir -p /var/www/html
chown -R www-data:www-data /var/www/html
chmod 755 /var/www/html 