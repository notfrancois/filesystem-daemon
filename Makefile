.PHONY: all clean proto build run test

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
	  ./main.go

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
