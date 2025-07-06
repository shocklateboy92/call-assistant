# Project Structure

```
call-assistant/
├── src/
│   ├── cmd/orchestrator/       # Main Go orchestrator
│   ├── internal/               # Private orchestrator code
│   │   ├── discovery.go        # Module discovery and management
│   │   ├── lifecycle.go        # Module process lifecycle
│   │   ├── pipeline.go         # Pipeline/graph management
│   │   └── grpc.go             # gRPC server implementation
│   ├── pkg/                    # Reusable Go packages
│   ├── services/               # In-process services
│   │   ├── config/
│   │   └── webui/
│   ├── modules/                # Multi-language modules
│   │   ├── go2rtc/             # Go module (separate go.mod)
│   │   ├── matrix/             # TypeScript module
│   │   └── chromecast/         # Python module
│   ├── api/proto/              # gRPC protobuf definitions
│   └── generated/              # Generated gRPC code (gitignored)
│       ├── go/
│       ├── typescript/
│       └── python/
├── config/                     # Configuration files
├── scripts/                    # Build scripts
├── docs/                       # Documentation
└── go.mod                      # Root Go module
```

## Key Points

- **Orchestrator**: Main Go application in `src/cmd/orchestrator/`
- **Modules**: Independent processes in `src/modules/`, each with own `module.yaml`
- **Protobuf**: Centralized definitions in `src/api/proto/`, generated code in `src/generated/`
- **Services**: In-process services that can be migrated to separate processes later
- **Multi-language**: Each module uses its language's conventions (go.mod, package.json, requirements.txt)