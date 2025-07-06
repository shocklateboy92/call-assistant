# Go Development Tools & Commands

## Key Commands

- **Build**: `go build`
- **Lint**: `golangci-lint run`
- **Format**: `golines -w .`
- **Modules**: `go mod tidy`
- **Scenario Test**: `go run ./src/cmd/scenario`

## Project Tools

- **golangci-lint**: Multi-linter aggregator
- **Go Modules**: Dependency management
- **protoc**: Protocol buffer compiler
- **go generate**: Code generation

## gRPC Best Practices

- Use proto3 for new projects
- Never add required fields in protobuf
- Enable gzip compression
- Implement proper error handling
- Use streaming for performance