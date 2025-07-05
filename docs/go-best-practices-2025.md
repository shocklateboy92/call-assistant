# Go Best Practices for 2025

## Project Setup & Structure

### Start Simple
- Begin with `go.mod` and a single `main.go` file
- Evolve structure as project grows
- Don't create directories just for organization

### Standard Layout (for larger projects)
```
project/
├── cmd/           # Main applications
├── internal/      # Private code
├── pkg/          # Reusable packages
├── api/          # API specs (protobuf, OpenAPI)
├── configs/      # Configuration files
├── scripts/      # Build/CI scripts
└── go.mod
```

## Dependency Management

### Go Modules (official solution)
- Initialize: `go mod init <module-name>`
- Use semantic versioning
- Manage tool dependencies separately
- Leverage build cache for performance

## Linting & Code Quality

### golangci-lint (recommended)
- Aggregates 48+ linters
- Runs in parallel
- Configure via `.golangci.yml`
- Caches results for speed

### Configuration example
```yaml
linters:
  enable:
    - gofmt
    - golint
    - govet
    - ineffassign
    - misspell
```

## gRPC Best Practices

### Protocol Buffers
- Use proto3 for new projects
- Never add required fields
- Make all fields optional/repeated
- Don't change field types after deployment

### Implementation
- Use streaming for performance
- Enable gzip compression
- Implement proper error handling
- Use HTTPS for security

### Project Organization
```
project/
├── api/
│   └── proto/     # .proto files
├── cmd/
│   └── server/    # gRPC server
├── internal/
│   └── service/   # Service implementations
└── pkg/
    └── client/    # gRPC client
```

## Key Tools & Commands

- **Build**: `go build`
- **Test**: `go test ./...`
- **Lint**: `golangci-lint run`
- **Format**: `go fmt`
- **Generate**: `go generate` (for protobuf)
- **Modules**: `go mod tidy`

## Performance & Security

- Compile to single binary
- Use build cache
- Implement authentication/authorization
- Validate inputs
- Handle errors properly
- Use structured logging

## Key Principles

1. **Start Simple**: Begin with minimal structure and evolve as needed
2. **Use Go Modules**: Official dependency management solution
3. **Implement golangci-lint**: Comprehensive linting for code quality
4. **Structure by Responsibility**: Organize code by functional responsibilities
5. **Follow Semantic Versioning**: For dependencies and releases
6. **Use Tool Dependencies**: For development tools written in Go
7. **Test Thoroughly**: Use Go's built-in testing framework
8. **Security First**: Implement proper authentication and input validation