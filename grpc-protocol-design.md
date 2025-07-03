# gRPC Protocol Design

## Core Communication Protocol Design

Based on the architecture requirements, here's the high-level design for the gRPC communication protocol:

### **Communication Pattern**
- **Orchestrator as Client**: Makes RPC calls to modules
- **Modules as Servers**: Each module runs a gRPC server on assigned port
- **Bidirectional Communication**: Control commands (orchestrator→module) + status/events (module→orchestrator)

### **Core Services**

#### **1. ModuleService** (provided by each module)
**Purpose**: Module lifecycle, registration, and health management

**Key Operations**:
- `RegisterModule()` - Initial registration with capabilities
- `HealthCheck()` - Regular health monitoring
- `Configure()` - Runtime configuration updates
- `Shutdown()` - Graceful shutdown coordination

#### **2. EntityService** (provided by each module)
**Purpose**: Entity lifecycle management within the module

**Key Operations**:
- `CreateEntity()` - Create new entity instances
- `ConfigureEntity()` - Update entity configuration
- `GetEntityStatus()` - Query entity state and metrics
- `DestroyEntity()` - Clean up entity resources
- `ListEntities()` - Enumerate active entities

#### **3. PipelineService** (provided by each module)
**Purpose**: Pipeline/graph operations and media flow management

**Key Operations**:
- `ConnectEntities()` - Establish connections between entities
- `DisconnectEntities()` - Remove connections
- `StartFlow()` - Begin media flow through pipeline
- `StopFlow()` - Stop media flow
- `GetFlowStatus()` - Query flow state and metrics
- `AdjustQuality()` - Dynamic quality/bitrate adjustments

#### **4. EventService** (provided by orchestrator)
**Purpose**: Receive events and status updates from modules

**Key Operations**:
- `ReportEvent()` - Module-to-orchestrator event notifications
- `ReportStatus()` - Periodic status updates
- `ReportMetrics()` - Performance and health metrics

## Key Interaction Patterns

### **1. Module Startup and Registration Flow**
1. Orchestrator discovers module via directory scan
2. Orchestrator starts module process with `GRPC_PORT` env var
3. Module starts gRPC server on assigned port
4. Orchestrator calls `RegisterModule()` → Module responds with `ModuleInfo` + capabilities
5. Orchestrator calls `HealthCheck()` to verify module is ready
6. Module marked as `READY` in orchestrator registry

### **2. Entity Creation Flow**
1. Orchestrator determines need for entity (e.g., RTSP source)
2. Orchestrator finds capable module via capability matching
3. Orchestrator calls `EntityService.CreateEntity()` → Request: `entity_type`, `configuration` → Response: `EntityDefinition` with assigned ID
4. Module creates entity instance and returns status
5. Orchestrator calls `ConfigureEntity()` if additional setup needed
6. Entity marked as `READY` for pipeline use

### **3. Pipeline Construction Flow**
1. Orchestrator calculates optimal path (source → converters → target)
2. For each connection in path:
   a. Orchestrator calls `PipelineService.ConnectEntities()`
   b. Module validates compatibility and creates connection
   c. Module returns `Connection` with actual parameters
3. Orchestrator calls `StartFlow()` to begin media flow
4. Modules coordinate media transport (outside gRPC)
5. Orchestrator monitors flow via `GetFlowStatus()`

### **4. Health Monitoring Flow**
1. Orchestrator periodically calls `HealthCheck()` on each module
2. Modules report `ModuleStatus` + entity health
3. Modules proactively call `EventService.ReportEvent()` for issues
4. Orchestrator correlates health data and triggers recovery actions
5. Failed modules are restarted, entities are recreated

### **5. Dynamic Configuration Flow**
1. Configuration change needed (e.g., quality adjustment)
2. Orchestrator calls `PipelineService.AdjustQuality()`
3. Module updates active flow parameters
4. Module calls `EventService.ReportStatus()` to confirm change
5. Orchestrator updates flow state and metrics

## Design Strengths

- **Independence**: Modules run as separate processes with their own gRPC servers
- **Language Agnostic**: Any language can implement the gRPC services
- **Capability-Driven**: Dynamic discovery and matching of module capabilities
- **Extensible**: Easy to add new entity types and services
- **Resilient**: Built-in health monitoring and error reporting

## Communication Flow

1. **Bootstrap**: Orchestrator discovers and starts modules
2. **Registration**: Modules advertise their capabilities
3. **Entity Management**: Dynamic creation and configuration of entities
4. **Pipeline Construction**: Connecting entities to form media flows
5. **Runtime Management**: Health monitoring, quality adjustments, error handling

## Next Steps

The protocol is designed to be implemented incrementally:
1. Start with ModuleService for basic module lifecycle
2. Add EntityService for entity management
3. Implement PipelineService for media flows
4. Add EventService for comprehensive monitoring

This design balances simplicity with the flexibility needed for a complex multi-protocol video calling system.