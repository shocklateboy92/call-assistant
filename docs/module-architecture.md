# Module Architecture

## Overview

The Call Assistant uses a **module-based architecture** where the orchestrator discovers and manages independent processes that provide entities to the system. Modules are language-agnostic and communicate with the orchestrator via gRPC.

## Module Discovery

### Directory Structure

The orchestrator scans for modules using this directory structure:

```
call-assistant/
├── modules/
│   ├── go2rtc/
│   │   ├── module.yaml
│   │   └── go2rtc-module          # Binary executable
│   ├── matrix/
│   │   ├── module.yaml
│   │   ├── package.json
│   │   └── src/
│   │       └── index.ts
│   ├── chromecast/
│   │   ├── module.yaml
│   │   ├── requirements.txt
│   │   └── chromecast_module.py
│   └── onvif-discovery/
│       ├── module.yaml
│       └── onvif-scanner.go
```

### Module Manifest (module.yaml)

Each module directory contains a minimal `module.yaml` manifest:

```yaml
# modules/go2rtc/module.yaml
name: "go2rtc-bridge"
version: "1.0.0"
description: "Provides RTSP to WebRTC conversion using go2rtc"

# Production execution command
command: "./go2rtc-module"

# Development execution command (compile, watch mode, etc.)
dev_command: "go run . --dev"

# Modules that must be registered before this module starts
dependencies:
  - "device-discovery"
  - "config-manager"
```

```yaml
# modules/matrix/module.yaml  
name: "matrix-protocol"
version: "1.0.0"
description: "Matrix calling protocol implementation"

command: "npm start"
dev_command: "npm run dev"

dependencies: []
```

```yaml
# modules/chromecast/module.yaml
name: "chromecast-bridge"
version: "1.0.0" 
description: "Chromecast device control and casting"

command: "python chromecast_module.py"
dev_command: "python chromecast_module.py --dev"

dependencies:
  - "device-discovery"
```

## Orchestrator-Driven Lifecycle

### 1. Module Discovery Phase

The orchestrator follows this startup sequence:

```
Startup → Scan modules/ directory → Parse module.yaml files → Build dependency graph → Start modules in order
```

**Discovery Process:**
1. **Directory Scan**: Recursively scan `modules/` directory for `module.yaml` files
2. **Manifest Parsing**: Load and validate each manifest
3. **Dependency Resolution**: Build dependency graph and determine startup order
4. **Environment Preparation**: Assign gRPC ports and prepare execution environment

### 2. Module Startup Sequence

For each module in dependency order:

```
1. Assign available gRPC port
2. Set GRPC_PORT environment variable  
3. Execute command in module directory
4. Capture stdout/stderr for centralized logging
5. Wait for module to start gRPC server on assigned port
6. Call RegisterModule() RPC to discover entities and capabilities
7. Module reports STARTING state, then WAITING_FOR_CONFIG if it needs configuration
8. Services (via orchestrator) provide configuration if needed
9. Module reports READY state and proceed to next module
```

### 3. Process Management

**Environment Variables:**
- `GRPC_PORT`: Assigned port for the module's gRPC server (e.g. "50051")
- Standard environment variables are inherited from orchestrator

**Logging Integration:**
- All module stdout/stderr is captured and tagged with module name
- Structured logging with consistent format across all modules
- Centralized log aggregation and rotation

**Process Monitoring:**
- Regular gRPC health checks to each module
- Automatic restart on process failure
- Graceful shutdown with SIGTERM/SIGKILL escalation

## What is a Module?

### Conceptual Definition

A **module** is an **independent process** that provides one or more **entities** to the call assistant system. Modules are domain experts that handle specific technologies:

- **go2rtc module**: Expert at streaming protocol conversion (RTSP ↔ WebRTC)
- **matrix module**: Expert at Matrix calling protocol and signaling
- **chromecast module**: Expert at casting to Chromecast devices  
- **onvif-discovery module**: Expert at discovering IP cameras on the network

## What is a Service?

### Conceptual Definition

A **service** is a **shared capability** that provides common functionality to multiple modules and the orchestrator. Services handle cross-cutting concerns and shared infrastructure:

- **config service**: Centralized configuration management for all modules
- **web UI service**: Dashboard and control interface for the entire system

### Services vs Modules

| Aspect | **Services** | **Modules** |
|--------|-------------|-------------|
| **Purpose** | Cross-cutting concerns, shared infrastructure | Domain-specific entity management |
| **Consumers** | Multiple modules + orchestrator | Only orchestrator |
| **Execution** | In-process with orchestrator (migration-ready) | Separate processes |
| **Examples** | Config, Web UI | go2rtc, Matrix, Chromecast |
| **Communication** | Direct function calls (in-process) | gRPC (across processes) |
| **Lifecycle** | Managed by orchestrator startup | Managed by orchestrator discovery |
| **Dependencies** | **Services sit in front of orchestrator** | **Cannot depend on services** |

### Service Architecture

**In-Process Design (Current):**
- Services run within the orchestrator process
- Share the same gRPC port as the orchestrator
- Direct function calls for maximum performance
- Single deployment unit

**Migration-Ready Design:**
- Each service has its own proto definitions
- Business logic is process-agnostic
- Can be moved to separate processes when scaling demands it
- Same service implementation works both in-process and remote

### Service Responsibilities

**1. Cross-Cutting Functionality:**
Services provide shared capabilities that multiple modules need:

- **Config Service**: "Store/retrieve configuration values for any module"
- **Web UI Service**: "Provide dashboard interface for all system components"

**2. Shared Infrastructure:**
Services manage common resources and provide unified interfaces:
- **Centralized Configuration**: Single source of truth for all settings
- **User Interface**: Unified control plane for the entire system

**3. Migration Flexibility:**
Services are designed to be moved to separate processes when needed:
- **In-Process**: Direct function calls for maximum performance
- **Remote**: gRPC calls when scaling or isolation is required
- **Hybrid**: Mix of in-process and remote based on requirements

### Module Responsibilities

**1. Entity Provision:**
Modules register what entity types they can provide, then create/manage entity instances on demand:

- Module advertises: "I can provide converter and video_source entities"
- Orchestrator requests: "Create a converter entity for this RTSP stream"
- Module responds: "Created converter entity 'go2rtc_conv_123' with WebRTC output"

**2. Entity Lifecycle Management:**
- **Create**: Set up new entity instances (configure new camera stream)
- **Configure**: Modify entity settings (change stream quality)  
- **Monitor**: Report entity health and performance metrics
- **Destroy**: Clean up entity resources when no longer needed

**3. Protocol Translation:**
Modules bridge between the orchestrator's abstract entity model and real-world systems:
- **go2rtc module**: "Create converter" → go2rtc HTTP API calls
- **matrix module**: "Join call" → matrix-js-sdk method calls
- **chromecast module**: "Start casting" → pychromecast commands

### Module Independence

**Process Isolation:**
- Each module runs in its own process space
- Module crashes don't affect other modules or the orchestrator
- Modules can be written in any language
- Modules can be restarted independently

**Communication Boundaries:**
- **Service ↔ Orchestrator**: Direct function calls (in-process) or gRPC (when migrated)
- **Orchestrator ↔ Module**: gRPC for control plane (entity management, configuration)
- **Module ↔ External Systems**: Native protocols (HTTP, Matrix, Cast protocol, etc.)
- **Module ↔ Module**: Never directly - always coordinated through orchestrator
- **Module ↔ Service**: **FORBIDDEN** - modules cannot depend on services

**Communication Flow:**
- **gRPC Method Calls**: Services → Orchestrator → Modules
- **Events**: Modules → Orchestrator → Services

**Resource Management:**
- Modules manage their own external dependencies (processes, client connections)
- Orchestrator manages module processes (lifecycle, logging, monitoring)
- Clear separation of concerns and responsibilities

## Data Flow Example

### Camera to Matrix Call Flow

```
1. User Request: "Start camera call to Matrix room"
   ↓
2. Orchestrator: Query entity registry
   - Source: "reolink_front_door" (go2rtc module)
   - Target: "matrix_main" (matrix module)
   - Path: Camera → go2rtc converter → Matrix protocol
   ↓
3. Orchestrator: Send gRPC requests to modules
   - go2rtc module: "Create converter entity for RTSP stream"
   - matrix module: "Join room and accept WebRTC stream"
   ↓
4. Modules: Execute using native APIs
   - go2rtc module → go2rtc HTTP API → RTSP camera
   - matrix module → matrix-js-sdk → Matrix homeserver
   ↓
5. Media Flow: Camera → go2rtc → WebRTC → Matrix → Remote participant
```

### Module Perspective

**From go2rtc module's view:**
1. "Orchestrator started me on port 50051"
2. "I registered my converter and video_source capabilities"
3. "Orchestrator asked me to create converter for rtsp://camera/stream"
4. "I configured go2rtc via HTTP API and created entity 'conv_123'"
5. "I'm monitoring the stream and reporting metrics back"

**From matrix module's view:**  
1. "Orchestrator started me on port 50052"
2. "I registered my calling_protocol capability for Matrix"
3. "Orchestrator asked me to join room with WebRTC stream URL"
4. "I used matrix-js-sdk to authenticate and join the call"
5. "I'm handling call signaling and reporting call status"

## Benefits

**For Module Developers:**
- **Language Freedom**: Use the best language/libraries for each domain
- **Simple Contract**: Just implement the gRPC service interface
- **Independent Development**: Develop and test modules in isolation
- **Native APIs**: Use each system's preferred SDK without translation layers

**For System Operators:**
- **Centralized Management**: Single point for process lifecycle and logging
- **Simple Deployment**: Drop module directory and restart orchestrator
- **Clear Dependencies**: Explicit dependency declarations in manifests
- **Easy Debugging**: Process isolation makes issues easier to isolate

**For System Architecture:**
- **Fault Tolerance**: Module failures are contained and recoverable  
- **Horizontal Scaling**: Modules can run on different machines if needed
- **Extensibility**: Add new capabilities without changing core system
- **Maintainability**: Clean separation of concerns and responsibilities

This architecture enables building a complex multi-protocol video calling system from simple, focused, language-agnostic modules that can be developed and maintained independently.