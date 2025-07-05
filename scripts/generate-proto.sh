#!/bin/bash

set -e

echo "Generating protobuf code..."

# Create generated directories
mkdir -p src/generated/go src/generated/typescript

# Generate Go code
echo "Generating Go code..."
protoc --go_out=src/generated/go --go-grpc_out=src/generated/go \
  --proto_path=src/api/proto \
  src/api/proto/*.proto

# Generate TypeScript code (requires ts-proto for nice-grpc)
echo "Generating TypeScript code..."
if npm --prefix src/generated/typescript run generate ; then
  echo "TypeScript generation completed successfully"
else
  echo "Warning: TypeScript generation failed"
  echo "Make sure ts-proto is installed: npm install"
fi

echo "Protobuf generation complete!"
echo "Generated files:"
echo "  - Go: src/generated/go/"
echo "  - TypeScript: src/generated/typescript/ (nice-grpc compatible)"
