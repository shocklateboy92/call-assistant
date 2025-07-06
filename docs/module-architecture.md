# Module Architecture

## Core Concepts

**Module**: Independent process providing entities (go2rtc, matrix, chromecast)
**Service**: In-process shared capability (config, webui)

## Module Discovery & Startup

1. Orchestrator scans `src/modules/` for `module.yaml` files
2. Resolves dependencies and assigns gRPC ports
3. Starts modules: `GRPC_PORT=50051 ./command`
4. Health checks until module reports READY
5. Config service connects and provides configuration

### module.yaml Format
```yaml
name: "go2rtc-bridge"
version: "1.0.0"
description: "RTSP to WebRTC conversion"
command: "./go2rtc-module"
dev_command: "go run . --dev"
dependencies: ["device-discovery"]
```

## Communication Patterns

- **Service ↔ Orchestrator**: Direct calls (in-process)
- **Orchestrator ↔ Module**: gRPC (entity management)
- **Service ↔ Module**: Direct gRPC (configuration)
- **Module ↔ External**: Native APIs (HTTP, SDKs)
- **Module ↔ Module**: FORBIDDEN (use orchestrator)

## Module Responsibilities

1. **Entity Provision**: Create/manage entities on demand
2. **Health Reporting**: Regular status updates via HealthCheck()
3. **Protocol Translation**: Bridge orchestrator ↔ external systems

## Example Flow

```
Camera → go2rtc Converter → Matrix Protocol → Remote User
RTSP     gRPC calls        WebRTC stream    Matrix SDK
```

1. Orchestrator finds capable modules
2. Creates entities via gRPC calls
3. Modules use native APIs (go2rtc HTTP, matrix-js-sdk)
4. Media flows outside gRPC (direct protocols)