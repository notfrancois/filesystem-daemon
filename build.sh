#!/bin/bash

# Colores para output
GREEN="\033[0;32m"
RED="\033[0;31m"
YELLOW="\033[0;33m"
NC="\033[0m" # No Color

# Directorios
BIN_DIR="/usr/bin"
DAEMON_BIN="filesystem-daemon"
CLI_BIN="fsdaemon"

# Compila el daemon
echo -e "${YELLOW}Compilando el daemon...${NC}"
CGO_ENABLED=0 go build -o ${DAEMON_BIN} \
  -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty || echo 'dev')" \
  -tags "netgo osusergo static_build" \
  ./cmd/daemon/main.go

if [ $? -ne 0 ]; then
  echo -e "${RED}❌ Error al compilar el daemon${NC}"
  exit 1
fi

# Compila el CLI
echo -e "${YELLOW}Compilando el CLI...${NC}"
CGO_ENABLED=0 go build -o ${CLI_BIN} \
  -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty || echo 'dev')" \
  -tags "netgo osusergo static_build" \
  ./cmd/cli/main.go

if [ $? -ne 0 ]; then
  echo -e "${RED}❌ Error al compilar el CLI${NC}"
  exit 1
fi

echo -e "${GREEN}✅ Compilación completada exitosamente${NC}"
echo -e "${YELLOW}Daemon: ${GREEN}${DAEMON_BIN}${NC}"
echo -e "${YELLOW}CLI:    ${GREEN}${CLI_BIN}${NC}"

# Verificar si queremos instalar
if [ "$1" == "install" ]; then
  echo -e "${YELLOW}Instalando binarios...${NC}"
  sudo install -m 755 ${DAEMON_BIN} ${BIN_DIR}/${DAEMON_BIN}
  sudo install -m 755 ${CLI_BIN} ${BIN_DIR}/${CLI_BIN}
  echo -e "${GREEN}✅ Instalación completada${NC}"
  echo -e "${YELLOW}Los binarios están disponibles en:${NC}"
  echo -e "  ${BIN_DIR}/${DAEMON_BIN}"
  echo -e "  ${BIN_DIR}/${CLI_BIN}"
fi
