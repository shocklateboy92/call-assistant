# Project Structure

## Overview

The Call Assistant project uses a multi-language monorepo structure that follows Go best practices while supporting modules written in any language. The orchestrator is the primary Go application, with modules organized as independent processes that communicate via gRPC.

## Directory Structure

```
call-assistant/
├── cmd/
│   └── orchestrator/           # Main Go orchestrator binary
│       └── main.go
├── internal/                   # Private orchestrator code
│   ├── discovery.go            # Module discovery and management
│   ├── lifecycle.go            # Module process lifecycle
│   ├── pipeline.go             # Pipeline/graph management
│   └── grpc.go                 # gRPC server implementation
├── pkg/                        # Reusable Go packages
│   ├── proto/                  # Go protobuf client wrappers
│   └── config/                 # Configuration utilities
├── api/                        # Language-agnostic protobuf definitions
│   └── proto/
│       ├── module.proto
│       ├── entity.proto
│       ├── pipeline.proto
│       └── services/           # In-process services (can be used by modules)
│           ├── config.proto
│           └── webui.proto
├── services/                   # In-process services (can be used by modules)
│   ├── config/
│   │   ├── service.go          # Business logic
│   │   ├── server.go           # gRPC server implementation
│   │   ├── client.go           # In-process client wrapper
│   │   └── main.go             # Future: separate process entry point
│   └── webui/
│       ├── service.go
│       ├── server.go
│       ├── client.go
│       └── main.go
├── modules/                    # Multi-language modules (shared parent)
│   ├── go2rtc/
│   │   ├── module.yaml
│   │   ├── main.go
│   │   └── go.mod              # Separate Go module
│   ├── matrix/
│   │   ├── module.yaml
│   │   ├── package.json
│   │   └── src/
│   └── chromecast/
│       ├── module.yaml
│       ├── requirements.txt
│       └── chromecast_module.py
├── generated/                  # Generated gRPC code (gitignored)
│   ├── go/
│   ├── typescript/
│   └── python/
│       ├── pyproject.toml     # Package config for editable install
│       └── __init__.py        # Makes it a proper Python package
├── configs/                    # Configuration files
│   ├── orchestrator.yaml
│   └── modules/
├── scripts/                    # Build and deployment scripts
│   ├── build.sh
│   ├── generate-proto.sh
│   └── dev.sh
├── docs/                       # Documentation
│   ├── module-architecture.md
│   └── pipeline-graph-architecture.md
├── go.mod                      # Root Go module for orchestrator
├── go.sum
├── .golangci.yml              # Go linting configuration
├── Dockerfile
└── README.md
```

## Key Design Decisions

### 1. Go Best Practices Compliance

**`cmd/orchestrator/`**: Main application following Go standards
- Contains the orchestrator's main entry point
- Follows the standard Go project layout convention

**`internal/`**: Private orchestrator code, organized by responsibility
- `discovery.go`: Module discovery and registration logic
- `lifecycle.go`: Process management and monitoring
- `pipeline.go`: Graph-based pipeline orchestration
- `grpc.go`: gRPC server and service implementations

**`pkg/`**: Reusable packages that could be imported by modules
- `proto/`: Go client wrappers for protobuf services
- `config/`: Configuration parsing and validation utilities

**Root `go.mod`**: Orchestrator is the primary Go application
- Single Go module for the orchestrator
- Modules can have their own Go modules if needed

### 2. Multi-Language Monorepo Pattern

**`api/proto/`**: Language-agnostic protobuf definitions
- Central location for all gRPC service definitions
- Follows protobuf best practices for multi-language support
- Single source of truth for API contracts

**`generated/`**: Separate directory for generated code
- Language-specific subdirectories (go/, typescript/, python/)
- Gitignored - regenerated during build process
- Never contains hand-written code

**`modules/`**: Each module can use its own language's conventions
- Go modules: Use standard Go project structure with own `go.mod`
- TypeScript modules: Use `package.json` and standard Node.js conventions
- Python modules: Use `requirements.txt` and standard Python structure

### 3. Protobuf/gRPC Organization

**Centralized proto definitions**: All `.proto` files in `api/proto/`
- Ensures consistency across all languages
- Makes breaking changes visible across the entire system
- Enables proper versioning and evolution

**Language-specific generation**: Each language gets its own `generated/` subdirectory
- Prevents language-specific artifacts from polluting other language directories
- Allows for language-specific build optimizations
- Python generated code is packaged for editable installation
- Simplifies CI/CD pipeline configuration

**Clean separation**: Generated code never mixed with hand-written code
- Follows protobuf best practices
- Makes it clear what should/shouldn't be edited manually
- Enables safe regeneration of code

### 4. Module Independence

**Separate Go modules**: `modules/go2rtc/` has its own `go.mod` for independence
- Allows modules to use different dependency versions
- Enables independent builds and testing
- Prevents dependency conflicts with orchestrator

**Language flexibility**: TypeScript modules use `package.json`, Python uses `requirements.txt`
- Each language follows its own ecosystem conventions
- Modules can be developed by teams familiar with specific languages
- Enables use of best-in-class libraries for each domain

**Shared parent**: All modules under `modules/` directory as required
- Satisfies the constraint that modules share a parent directory
- Enables the orchestrator to discover modules through directory scanning
- Simplifies deployment and packaging

### 5. Development Workflow

**`scripts/`**: Build automation for multi-language compilation
- `generate-proto.sh`: Generates code for all supported languages
- `build.sh`: Builds orchestrator and modules
- `dev.sh`: Starts development environment with hot reloading

**`configs/`**: Environment-specific configuration
- `orchestrator.yaml`: Main orchestrator configuration
- `modules/`: Module-specific configuration overrides

**`docs/`**: Centralized documentation
- Technical architecture documents
- API documentation
- Development guides

## Build Process

### Proto Generation Script (`scripts/generate-proto.sh`)

```bash
#!/bin/bash
# Generate Go code
protoc --go_out=generated/go --go-grpc_out=generated/go api/proto/*.proto

# Generate TypeScript code  
protoc --ts_out=generated/typescript api/proto/*.proto

# Generate Python code
protoc --python_out=generated/python --grpc_python_out=generated/python api/proto/*.proto
```

### Development Script (`scripts/dev.sh`)

```bash
#!/bin/bash
# Generate protobuf code
./scripts/generate-proto.sh

# Start orchestrator in dev mode
cd cmd/orchestrator && go run . --dev &

# Orchestrator will discover and start modules based on their module.yaml dev_command
```

### Build Script (`scripts/build.sh`)

```bash
#!/bin/bash
# Generate protobuf code
./scripts/generate-proto.sh

# Build orchestrator
cd cmd/orchestrator && go build -o ../../bin/orchestrator

# Build Go modules
cd modules/go2rtc && go build -o ../../bin/go2rtc-module

# Install Node.js dependencies for TypeScript modules
cd modules/matrix && npm install && npm run build

# Install Python dependencies and generated gRPC package
cd modules/chromecast && pip install -r requirements.txt
pip install -e ../../generated/python
```

## Benefits

### Go Standards Compliance
- Follows `golang-standards/project-layout` recommendations
- Uses `internal/` for private code, `pkg/` for reusable packages
- `cmd/` contains main applications
- Compatible with standard Go tooling

### Multi-Language Support
- Each language can follow its own conventions within `modules/`
- Shared protobuf definitions enable type-safe communication
- Generated code is cleanly separated from hand-written code
- Language-specific build processes are isolated

### Monorepo Advantages
- Shared build scripts and CI/CD configuration
- Centralized dependency management where possible
- Atomic changes across orchestrator and modules
- Single repository for documentation and configuration

### Development Experience
- Clear separation of concerns between orchestrator and modules
- Easy to add new modules in any language
- Standard tooling (golangci-lint, protoc, etc.) works seamlessly
- Hot reloading support for development workflow

## Migration Path

This structure can be evolved incrementally:

1. **Phase 1**: Start with orchestrator in `cmd/orchestrator/`
2. **Phase 2**: Add protobuf definitions in `api/proto/`
3. **Phase 3**: Implement first module (go2rtc) in `modules/go2rtc/`
4. **Phase 4**: Add additional modules as needed
5. **Phase 5**: Enhance build automation and CI/CD

The structure balances Go best practices with the realities of a multi-language system, following industry standards for both monorepos and gRPC projects while maintaining the flexibility to grow and evolve over time.