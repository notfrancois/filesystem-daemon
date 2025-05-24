#!/bin/bash

# Ensure scripts directory exists
mkdir -p $(dirname "$0")

# Go to project root
cd "$(dirname "$0")/.."

# Create proto output directory
mkdir -p proto/gen

# Add Go bin directory to PATH
export PATH=$PATH:$HOME/go/bin

# Generate Go code from proto definition
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/filesystem.proto

echo "Proto files generated successfully"
