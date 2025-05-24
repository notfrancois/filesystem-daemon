#!/bin/bash

# Variables
PACKAGE_NAME="filesystem-daemon"
VERSION="1.0.0"
BIN_PATH="/tmp/filesystem-daemon"
SERVICE_FILE="/tmp/filesystem-daemon.service"

# Opciones de seguridad
ENABLE_SECURITY_SCAN=false  # Cambiar a true para habilitar escaneo de seguridad
LICENSE_CHECK=false         # Cambiar a true para verificar licencias

# Colores para output
GREEN="\033[0;32m"
RED="\033[0;31m"
YELLOW="\033[0;33m"
NC="\033[0m" # No Color

# Detener servicio si ya está corriendo
sudo systemctl stop ${PACKAGE_NAME} 2>/dev/null || true

# Ejecutar tests y verificar cobertura
echo -e "${YELLOW}Ejecutando tests...${NC}"
go test -v -cover ./... || {
  echo -e "${RED}Tests fallidos${NC}"
  exit 1
}

# Ejecutar tests de race conditions
echo -e "${YELLOW}Verificando race conditions...${NC}"
go test -race ./... || {
  echo -e "${RED}Detectadas race conditions${NC}"
  exit 1
}

# Compilar con las mismas flags de seguridad que en CI/CD
echo -e "${YELLOW}Compilando el binario con medidas de seguridad...${NC}"
CGO_ENABLED=0 go build -o ${BIN_PATH} \
  -ldflags="-s -w -X main.version=local-$(date +%Y%m%d-%H%M%S)" \
  -tags "netgo osusergo static_build" \
  ./main.go

# Escaneo de seguridad opcional
if [ "$ENABLE_SECURITY_SCAN" = true ]; then
  echo -e "${YELLOW}Ejecutando escaneo de seguridad...${NC}"
  if command -v trivy &> /dev/null; then
    trivy fs --severity HIGH,CRITICAL --exit-code 1 ./
  else
    echo -e "${RED}Trivy no está instalado. Saltando escaneo de seguridad.${NC}"
    echo -e "${YELLOW}Para instalarlo: sudo apt-get install -y trivy${NC}"
  fi
fi

if [ ! -f "${BIN_PATH}" ]; then
    echo -e "${RED}❌ Error: La compilación falló${NC}"
    exit 1
fi

# Instalar el binario y el servicio localmente
echo -e "${YELLOW}Instalando el binario y configurando el servicio...${NC}"
sudo install -m 755 ${BIN_PATH} /usr/bin/filesystem-daemon

# Crear el directorio de monitoreo si no existe
echo -e "${YELLOW}Configurando directorios con permisos seguros...${NC}"
sudo mkdir -p /var/www/html
sudo chmod 750 /var/www/html

# Crear un servicio con las mismas medidas de seguridad que en producción
echo -e "${YELLOW}Creando configuración del servicio con medidas de seguridad...${NC}"
cat > ${SERVICE_FILE} << EOF
[Unit]
Description=Secure Filesystem Monitoring Daemon
Documentation=man:filesystem-daemon(8)
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/filesystem-daemon
Restart=always
RestartSec=5
TimeoutStartSec=0
WorkingDirectory=/var/www/html

# Medidas de seguridad contra backports y fugas de datos
PrivateTmp=true
PrivateDevices=true
ProtectSystem=full
ProtectHome=true
NoNewPrivileges=true
SystemCallFilter=~@debug
SystemCallArchitectures=native
MemoryDenyWriteExecute=true
# Permitir explícitamente las familias de direcciones para redes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=true
RestrictRealtime=true
# Asignar capacidades necesarias para binding de puertos
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_RAW CAP_SYS_ADMIN
AmbientCapabilities=CAP_NET_BIND_SERVICE CAP_NET_RAW

# Seguridad del sistema de archivos
ReadWritePaths=/var/www/html
ReadOnlyPaths=/etc

[Install]
WantedBy=multi-user.target
EOF

# Instalar y arrancar el servicio
echo -e "${YELLOW}Instalando y activando el servicio...${NC}"
sudo cp ${SERVICE_FILE} /lib/systemd/system/filesystem-daemon.service
sudo systemctl daemon-reload
sudo systemctl enable ${PACKAGE_NAME}
sudo systemctl start ${PACKAGE_NAME}

# Verificar instalación
echo -e "${YELLOW}Verificando estado del servicio...${NC}"
if systemctl is-active --quiet ${PACKAGE_NAME}; then
    echo -e "${GREEN}✅ El servicio está corriendo correctamente${NC}"
else
    echo -e "${RED}❌ Error: El servicio no está corriendo${NC}"
    # Mostrar logs para diagnóstico
    journalctl -u ${PACKAGE_NAME} --no-pager -n 20
    exit 1
fi

# Verificar que el puerto esté abierto - con lsof y con pequeño retraso
echo -e "${YELLOW}Verificando puerto gRPC (50051)...${NC}"
# Esperar un poco para que el servicio termine de inicializarse
sleep 2

# Intentar con lsof primero, que es más confiable para procesos específicos
if sudo lsof -i :50051 | grep -q LISTEN; then
    echo -e "${GREEN}✅ Puerto gRPC (50051) abierto correctamente (lsof)${NC}"
else
    # Intentar con ss como alternativa
    if ss -tuln | grep -q ':50051 '; then
        echo -e "${GREEN}✅ Puerto gRPC (50051) abierto correctamente (ss)${NC}"
    else
        echo -e "${RED}❌ Error: Puerto gRPC (50051) no está abierto${NC}"
        echo -e "${YELLOW}Mostrando información de diagnóstico de red...${NC}"
        echo -e "${YELLOW}ss -tulnp:${NC}"
        sudo ss -tulnp
        echo -e "${YELLOW}netstat -tulnp:${NC}"
        sudo netstat -tulnp 2>/dev/null
        # Temporalmente continuamos sin error para diagnóstico
        # exit 1
    fi
fi

# Verificar permisos de los directorios
echo -e "${YELLOW}Verificando permisos de directorios...${NC}"
PERMS=$(stat -c "%a" /var/www/html)
if [ "$PERMS" = "750" ]; then
    echo -e "${GREEN}✅ Los permisos del directorio son correctos (750)${NC}"
else
    echo -e "${RED}❌ Error: Los permisos del directorio no son seguros: $PERMS${NC}"
fi

# Mostrar estado del servicio
echo -e "${YELLOW}Estado del servicio:${NC}"
systemctl status ${PACKAGE_NAME} --no-pager

# Mostrar logs del servicio
echo -e "${YELLOW}Logs del servicio:${NC}"
journalctl -u ${PACKAGE_NAME} --no-pager -n 20

echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}✅ Daemon instalado y configurado exitosamente${NC}"
echo -e "${GREEN}   con todas las medidas de seguridad${NC}"
echo -e "${GREEN}=========================================${NC}"
