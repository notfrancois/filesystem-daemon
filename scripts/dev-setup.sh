#!/bin/bash
set -euo pipefail

# Development setup script for filesystem daemon

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

log() {
    echo "[$(date -Iseconds)] $*" >&2
}

setup_dev_environment() {
    log "Setting up development environment..."
    
    # Create development directories with proper ownership
    mkdir -p "$PROJECT_ROOT/dev-assets"
    mkdir -p "$PROJECT_ROOT/dev-certs"
    mkdir -p "$PROJECT_ROOT/logs"
    
    # Generate self-signed certificates for development
    if [[ ! -f "$PROJECT_ROOT/dev-certs/server.key" ]] || [[ ! -f "$PROJECT_ROOT/dev-certs/server.crt" ]]; then
        log "Generating development certificates..."
        openssl req -x509 -newkey rsa:4096 \
            -keyout "$PROJECT_ROOT/dev-certs/server.key" \
            -out "$PROJECT_ROOT/dev-certs/server.crt" \
            -days 365 -nodes \
            -subj "/CN=filesystem-daemon-dev/O=Development/C=US"
        
        # Set certificates permissions safely
        chmod 644 "$PROJECT_ROOT/dev-certs/server.key" "$PROJECT_ROOT/dev-certs/server.crt" 2>/dev/null || {
            log "Warning: Could not set certificate permissions, continuing..."
        }
    fi
    
    # Create sample assets for testing
    cat > "$PROJECT_ROOT/dev-assets/index.html" << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>Filesystem Daemon - Development</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .status { color: green; font-weight: bold; }
    </style>
</head>
<body>
    <h1>Filesystem Daemon Development</h1>
    <p class="status">✅ Assets directory is working!</p>
    <p>This file is served from the development assets directory.</p>
</body>
</html>
EOF
    
    # Set proper permissions safely
    find "$PROJECT_ROOT/dev-assets" -type f -exec chmod 664 {} \; 2>/dev/null || {
        log "Warning: Could not set file permissions, continuing..."
    }
    find "$PROJECT_ROOT/dev-assets" -type d -exec chmod 775 {} \; 2>/dev/null || {
        log "Warning: Could not set directory permissions, continuing..."
    }
    
    log "Development environment setup complete!"
}

start_dev_services() {
    log "Starting development services..."
    
    cd "$PROJECT_ROOT"
    
    # Create .env.dev if it doesn't exist
    if [[ ! -f .env.dev ]]; then
        log "Creating default .env.dev file..."
        cat > .env.dev << 'EOF'
# Development environment configuration
ENVIRONMENT=development
DEV_MODE=true
BUILD_VERSION=dev

# Network configuration
GRPC_PORT=50051
HEALTH_PORT=50052

# Paths for development
ASSETS_PATH=./dev-assets
CERTS_PATH=./dev-certs

# Development user - use current user's ID
DEV_UID=1000
DEV_GID=1000
WEB_SERVER_UID=1000
WEB_SERVER_GID=1000

# Relaxed permissions for development
DEFAULT_FILE_MODE=0664
DEFAULT_DIR_MODE=0775

# Security settings (relaxed for development)
TLS_ENABLED=false
TRUSTED_NETWORKS=127.0.0.1/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,0.0.0.0/0

# Asset management
MAX_FILE_SIZE=500MB
ALLOWED_EXTENSIONS=jpg,jpeg,png,gif,svg,css,js,html,txt,pdf,woff,woff2,ttf,json,xml,ico
WATCH_CHANGES=true

# Logging for development
LOG_LEVEL=debug
LOG_FORMAT=text
EOF
    fi
    
    # Export environment variables (avoid UID conflict)
    export ENVIRONMENT=development
    export DEV_MODE=true
    export BUILD_VERSION=dev
    export GRPC_PORT=50051
    export HEALTH_PORT=50052
    export ASSETS_PATH=./dev-assets
    export CERTS_PATH=./dev-certs
    export DEV_UID=$(id -u)
    export DEV_GID=$(id -g)
    export WEB_SERVER_UID=$(id -u)
    export WEB_SERVER_GID=$(id -g)
    export DEFAULT_FILE_MODE=0664
    export DEFAULT_DIR_MODE=0775
    export TLS_ENABLED=false
    export TRUSTED_NETWORKS="127.0.0.1/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,0.0.0.0/0"
    export MAX_FILE_SIZE=500MB
    export ALLOWED_EXTENSIONS="jpg,jpeg,png,gif,svg,css,js,html,txt,pdf,woff,woff2,ttf,json,xml,ico"
    export WATCH_CHANGES=true
    export LOG_LEVEL=debug
    export LOG_FORMAT=text
    
    # Start with development compose
    docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
    
    log "Development services started!"
    log "gRPC server: localhost:50051 (no TLS)"
    log "Health check: http://localhost:50052/health"
    log "Test web server: http://localhost:8080"
}

stop_dev_services() {
    log "Stopping development services..."
    
    cd "$PROJECT_ROOT"
    docker compose -f docker-compose.yml -f docker-compose.dev.yml down
    
    log "Development services stopped!"
}

show_logs() {
    cd "$PROJECT_ROOT"
    docker compose -f docker-compose.yml -f docker-compose.dev.yml logs -f
}

case "${1:-setup}" in
    "setup")
        setup_dev_environment
        ;;
    "start")
        start_dev_services
        ;;
    "stop")
        stop_dev_services
        ;;
    "restart")
        stop_dev_services
        start_dev_services
        ;;
    "logs")
        show_logs
        ;;
    *)
        echo "Usage: $0 {setup|start|stop|restart|logs}"
        exit 1
        ;;
esac 