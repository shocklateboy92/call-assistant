# gRPC Protocol Design

## Communication Patterns

- **Service ↔ Orchestrator**: Direct calls / event streams
- **Orchestrator ↔ Module**: gRPC calls (entity management)  
- **Service ↔ Module**: Direct gRPC (specialized operations)
- **Module → Orchestrator**: Events, status updates

## Core Services

### ModuleService (each module)
- `HealthCheck()` - Status reporting
- `Shutdown()` - Graceful shutdown

### EntityService (each module)  
- `CreateEntity()` - Create entity instances
- `ConfigureEntity()` - Update configuration
- `GetEntityStatus()` - Query state
- `DestroyEntity()` - Cleanup resources
- `ListEntities()` - List active entities

### PipelineService (each module)
- `ConnectEntities()` - Establish connections  
- `StartFlow()` - Begin media flow
- `StopFlow()` - Stop media flow
- `GetFlowStatus()` - Query flow state

### EventService (orchestrator)
- `ReportEvent()` - Event notifications
- `ReportStatus()` - Status updates
- `ReportMetrics()` - Performance data

## Key Message Types

```protobuf
message ModuleInfo {
  string id = 1;
  string name = 2;
  string grpc_address = 3;  // "localhost:50051"
  ModuleStatus status = 4;
}

message EntityCapabilities {
  repeated string supported_protocols = 1;  // rtsp, webrtc, hls
  repeated string supported_codecs = 2;     // h264, opus, aac
  MediaType media_type = 3;                 // AUDIO, VIDEO
}

message Connection {
  string id = 1;
  string source_entity_id = 2;
  string target_entity_id = 3;
  MediaType media_type = 4;
  string transport_protocol = 5;
}
```

## Entity Types

- **MediaSource**: RTSP cameras, USB webcams, files
- **MediaSink**: Chromecast, displays, files  
- **Protocol**: Matrix, XMPP, WebRTC endpoints
- **Converter**: go2rtc, FFmpeg, format converters

Each entity type has separate audio/video entities for flexible routing.