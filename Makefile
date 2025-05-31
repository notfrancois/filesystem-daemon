.PHONY: all clean proto build test docker-build docker-run docker-stop docker-dev docker-prod docker-clean dev-setup

all: proto build

# Install required protobuf tools
tools:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate Go code from proto definition
proto:
	mkdir -p proto/gen
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       proto/filesystem.proto

# Build the daemon
build:
	CGO_ENABLED=0 go build -o cmd/filesystem-daemon \
	  -ldflags="-s -w -X main.version=dev-$(shell date +%Y%m%d-%H%M%S)" \
	  -tags "netgo osusergo static_build" \
	  ./cmd/daemon

# Run the daemon (for development)
run: build
	sudo ./cmd/filesystem-daemon

# Test
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f cmd/filesystem-daemon
	rm -rf proto/gen

# Docker targets
docker-build:
	docker build -t filesystem-daemon:$(shell git rev-parse --short HEAD) .
	docker tag filesystem-daemon:$(shell git rev-parse --short HEAD) filesystem-daemon:latest

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

docker-logs:
	docker compose logs -f filesystem-daemon

# Development environment setup
dev-setup:
	./scripts/dev-setup.sh setup

# Development with Docker (no TLS, local volumes)
docker-dev: dev-setup
	./scripts/dev-setup.sh start

# Stop development environment
docker-dev-stop:
	./scripts/dev-setup.sh stop

# Production deployment
docker-prod:
	ENVIRONMENT=production docker compose up -d

# Clean Docker artifacts
docker-clean:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml down -v --rmi local || true

# Security scan
security-scan:
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
		-v $(PWD):/workspace aquasec/trivy:latest \
		filesystem-daemon:latest
