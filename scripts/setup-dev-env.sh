#\!/bin/bash

set -e

echo "Setting up Universal Call Assistant development environment..."

# Install Go protobuf plugins
echo "Installing Go protobuf plugins..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Install Node.js gRPC tools
echo "Installing Node.js gRPC tools..."
npm install -g grpc-tools

# Generate protobuf code
echo "Generating protobuf code..."
./scripts/generate-proto.sh

echo "Development environment setup complete\!"