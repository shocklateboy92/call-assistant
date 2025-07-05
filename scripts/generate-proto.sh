#!/bin/bash

set -e

echo "Generating protobuf code..."

# Create generated directories
mkdir -p generated/go generated/typescript generated/python

# Generate Go code
echo "Generating Go code..."
protoc --go_out=generated/go --go-grpc_out=generated/go \
  --proto_path=api/proto \
  api/proto/*.proto

# Generate TypeScript code (requires grpc-tools)
echo "Generating TypeScript code..."
if command -v grpc_tools_node_protoc >/dev/null 2>&1; then
  grpc_tools_node_protoc --js_out=import_style=commonjs,binary:generated/typescript \
    --grpc_out=grpc_js:generated/typescript \
    --ts_out=grpc_js:generated/typescript \
    --proto_path=api/proto \
    api/proto/*.proto
else
  echo "Warning: grpc-tools not found, skipping TypeScript generation"
  echo "Install with: npm install -g grpc-tools"
fi
