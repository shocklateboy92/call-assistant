#!/bin/bash

set -e

echo "Generating protobuf code..."

# Create generated directories
mkdir -p generated/go generated/typescript

# Generate Go code
echo "Generating Go code..."
protoc --go_out=generated/go --go-grpc_out=generated/go \
  --proto_path=api/proto \
  api/proto/*.proto

# Generate TypeScript code (requires ts-proto for nice-grpc)
echo "Generating TypeScript code..."
if command -v protoc-gen-ts_proto >/dev/null 2>&1; then
  protoc "--plugin=protoc-gen-ts_proto=$(which protoc-gen-ts_proto)" \
    --ts_proto_out=generated/typescript \
    --ts_proto_opt=outputServices=nice-grpc,outputServices=generic-definitions,useExactTypes=false \
    --proto_path=api/proto \
    api/proto/*.proto
else
  echo "Warning: ts-proto not found, skipping TypeScript generation"
  echo "Install with: npm install ts-proto"
fi

echo "Protobuf generation complete!"
echo "Generated files:"
echo "  - Go: generated/go/"
echo "  - TypeScript: generated/typescript/ (nice-grpc compatible)"
