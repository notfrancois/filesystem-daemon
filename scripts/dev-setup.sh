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
    
    # Create development directories
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
        
        chmod 644 "$PROJECT_ROOT/dev-certs/server.key" "$PROJECT_ROOT/dev-certs/server.crt"
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
    
    # Set proper permissions
    find "$PROJECT_ROOT/dev-assets" -type f -exec chmod 664 {} \;
    find "$PROJECT_ROOT/dev-assets" -type d -exec chmod 775 {} \;
    
    log "Development environment setup complete!"
}

start_dev_services() {
    log "Starting development services..."
    
    cd "$PROJECT_ROOT"
    
    # Load development environment
    export $(cat .env.dev | grep -v '^#' | xargs)
    
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