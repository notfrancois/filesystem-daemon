#!/bin/bash
set -euo pipefail

# Docker initialization script for filesystem daemon
# Handles permission setup for legacy web server compatibility

ASSETS_DIR="${ASSETS_PATH:-/var/www/html}"
WEB_UID="${WEB_SERVER_UID:-33}"
WEB_GID="${WEB_SERVER_GID:-33}"
DEFAULT_FILE_MODE="${DEFAULT_FILE_MODE:-0644}"
DEFAULT_DIR_MODE="${DEFAULT_DIR_MODE:-0755}"

log() {
    echo "[$(date -Iseconds)] $*" >&2
}

# Create assets directory if it doesn't exist
if [[ ! -d "$ASSETS_DIR" ]]; then
    log "Creating assets directory: $ASSETS_DIR"
    mkdir -p "$ASSETS_DIR"
fi

# Set ownership and permissions
log "Setting ownership to $WEB_UID:$WEB_GID"
chown -R "$WEB_UID:$WEB_GID" "$ASSETS_DIR"

log "Setting directory permissions to $DEFAULT_DIR_MODE"
find "$ASSETS_DIR" -type d -exec chmod "$DEFAULT_DIR_MODE" {} \;

log "Setting file permissions to $DEFAULT_FILE_MODE"
find "$ASSETS_DIR" -type f -exec chmod "$DEFAULT_FILE_MODE" {} \;

log "Permission setup completed successfully" 