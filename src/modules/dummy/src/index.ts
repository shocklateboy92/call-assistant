#!/usr/bin/env node

import { createServer } from "nice-grpc";
import {
  ModuleServiceImplementation,
  ModuleServiceDefinition,
  HealthCheckRequest,
  HealthCheckResponse,
  ShutdownRequest,
  ShutdownResponse,
  PipelineServiceImplementation,
  PipelineServiceDefinition,
  ConnectEntitiesRequest,
  ConnectEntitiesResponse,
  DisconnectEntitiesRequest,
  DisconnectEntitiesResponse,
  StartFlowRequest,
  StartFlowResponse,
  StopFlowRequest,
  StopFlowResponse,
  GetFlowStatusRequest,
  GetFlowStatusResponse,
  AdjustQualityRequest,
  AdjustQualityResponse,
} from "call-assistant-protos/module";
import {
  ConfigurableModuleServiceImplementation,
  ConfigurableModuleServiceDefinition,
  GetConfigSchemaResponse,
  ApplyConfigRequest,
  ApplyConfigResponse,
  GetCurrentConfigResponse,
  ValidateConfigRequest,
  ValidateConfigResponse,
} from "call-assistant-protos/services/config";
import { ModuleState, HealthStatus } from "call-assistant-protos/common";
import { Empty } from "call-assistant-protos/google/protobuf/empty";
import {
  ListEntitiesRequest,
  ListEntitiesResponse,
  MediaSource,
  MediaSink,
  EntityState,
  EntityStatus,
  EntityCapabilities,
  MediaType,
  Connection,
  ConnectionStatus,
  QualityProfile,
  GetEntityStatusRequest,
  GetEntityStatusResponse,
} from "call-assistant-protos/entities";
import type { CallContext } from "nice-grpc-common";
import { JSONSchemaType } from "ajv";

interface DummyModuleConfig {
  test_pattern_fps: number;
  test_pattern_resolution: string;
  enable_frame_counting: boolean;
}

interface TestPatternSource {
  id: string;
  fps: number;
  resolution: { width: number; height: number };
  frameCount: number;
  isActive: boolean;
  interval?: NodeJS.Timeout;
}

interface TestSink {
  id: string;
  frameCount: number;
  isActive: boolean;
  lastFrameTime: Date;
}

interface TestConnection {
  id: string;
  sourceEntityId: string;
  targetEntityId: string;
  mediaType: MediaType;
  quality: QualityProfile;
  status: ConnectionStatus;
  createdAt: Date;
  startedAt?: Date;
  interval?: NodeJS.Timeout;
  framesSent: number;
  bytesTransferred: number;
}

// AJV will type check this schema against the DummyModuleConfig interface
const schema: JSONSchemaType<DummyModuleConfig> = {
  $schema: "http://json-schema.org/draft-07/schema#",
  type: "object",
  properties: {
    test_pattern_fps: {
      type: "number",
      nullable: false,
      description: "Frame rate for synthetic video test pattern",
      minimum: 1,
      maximum: 60,
    },
    test_pattern_resolution: {
      type: "string",
      nullable: false,
      description: "Resolution for test pattern (e.g., '1920x1080')",
      pattern: "^\\d+x\\d+$",
    },
    enable_frame_counting: {
      type: "boolean",
      nullable: false,
      description: "Whether to enable frame counting for test sinks",
    },
  },
  required: ["test_pattern_fps", "test_pattern_resolution", "enable_frame_counting"],
  additionalProperties: false,
};

class DummyModule
  implements
    ModuleServiceImplementation,
    ConfigurableModuleServiceImplementation,
    PipelineServiceImplementation
{
  private config?: DummyModuleConfig;
  private configVersion: string = "1";
  private testSources: Map<string, TestPatternSource> = new Map();
  private testSinks: Map<string, TestSink> = new Map();
  private connections: Map<string, TestConnection> = new Map();
  private nextEntityId = 1;
  private nextConnectionId = 1;

  constructor() {
    // Create some default test entities
    this.createDefaultEntities();
  }

  private createDefaultEntities(): void {
    // Create a default test pattern source
    const source = this.createTestPatternSource(new Map([
      ['fps', '30'],
      ['resolution', '1920x1080']
    ]));
    
    // Create a default test sink
    const sink = this.createTestSink(new Map());
    
    console.log(`[Dummy Module] Created default entities: source=${source.id}, sink=${sink.id}`);
  }

  async healthCheck(
    request: HealthCheckRequest,
    context: CallContext
  ): Promise<HealthCheckResponse> {
    console.log("[Dummy Module] HealthCheck called");

    return {
      status: {
        state: this.config
          ? ModuleState.MODULE_STATE_READY
          : ModuleState.MODULE_STATE_UNSPECIFIED,
        health: HealthStatus.HEALTH_STATUS_HEALTHY,
        error_message: "",
        last_heartbeat: new Date(),
      },
    };
  }

  async shutdown(
    request: ShutdownRequest,
    context: CallContext
  ): Promise<ShutdownResponse> {
    console.log("[Dummy Module] Shutdown called with:", request);

    const response: ShutdownResponse = {
      success: true,
      error_message: "",
    };

    // Gracefully shutdown after sending response
    setTimeout(() => {
      console.log("[Dummy Module] Shutting down gracefully");
      process.exit(0);
    }, 1000);

    return response;
  }

  async getConfigSchema(
    request: Empty,
    context: CallContext
  ): Promise<GetConfigSchemaResponse> {
    console.log("[Dummy Module] GetConfigSchema called");

    // Basic schema for demonstration - config service will handle validation

    return {
      success: true,
      error_message: "",
      schema: {
        schema_version: this.configVersion,
        json_schema: JSON.stringify(schema, null, 2),
        required: false,
      },
    };
  }

  async applyConfig(
    request: ApplyConfigRequest,
    context: CallContext
  ): Promise<ApplyConfigResponse> {
    console.log("[Dummy Module] ApplyConfig called with:", request.config_json);

    try {
      const newConfig = JSON.parse(request.config_json) as DummyModuleConfig;

      // Apply the configuration without validation - config service will handle that
      this.config = { ...this.config, ...newConfig };
      this.configVersion = request.config_version || new Date().toISOString();

      console.log(
        "[Dummy Module] Configuration applied successfully:",
        this.config
      );

      return {
        success: true,
        error_message: "",
        validation_errors: [],
        applied_config_version: this.configVersion,
      };
    } catch (error) {
      console.error("[Dummy Module] Error applying configuration:", error);
      return {
        success: false,
        error_message: `Failed to parse configuration: ${
          error instanceof Error ? error.message : String(error)
        }`,
        validation_errors: [],
        applied_config_version: "",
      };
    }
  }

  async getCurrentConfig(
    request: Empty,
    context: CallContext
  ): Promise<GetCurrentConfigResponse> {
    console.log("[Dummy Module] GetCurrentConfig called");

    return {
      success: true,
      error_message: "",
      config_json: JSON.stringify(this.config, null, 2),
      config_version: this.configVersion,
    };
  }

  async validateConfig(
    request: ValidateConfigRequest,
    context: CallContext
  ): Promise<ValidateConfigResponse> {
    console.log(
      "[Dummy Module] ValidateConfig called with:",
      request.config_json
    );

    try {
      // Just check if it's valid JSON - config service will handle validation
      JSON.parse(request.config_json);

      // In a real module, this is where you would validate the credentials
      // by making a connection to the external service.

      return {
        valid: true,
        validation_errors: [],
      };
    } catch (error) {
      return {
        valid: false,
        validation_errors: [
          {
            field_path: "",
            error_code: "INVALID_JSON",
            error_message: `Invalid JSON: ${
              error instanceof Error ? error.message : String(error)
            }`,
            provided_value: request.config_json,
            expected_constraint: "Valid JSON object",
          },
        ],
      };
    }
  }


  async getEntityStatus(
    request: GetEntityStatusRequest,
    context: CallContext
  ): Promise<GetEntityStatusResponse> {
    console.log(`[Dummy Module] GetEntityStatus called for: ${request.entity_id}`);

    try {
      // Check sources
      const source = this.testSources.get(request.entity_id);
      if (source) {
        return {
          success: true,
          error_message: "",
          status: {
            state: source.isActive ? EntityState.ENTITY_STATE_ACTIVE : EntityState.ENTITY_STATE_CREATED,
            health: HealthStatus.HEALTH_STATUS_HEALTHY,
            error_message: "",
            active_connections: Array.from(this.connections.values())
              .filter(conn => conn.sourceEntityId === request.entity_id)
              .map(conn => conn.id),
            metrics: {
              frames_generated: { value: { $case: "int_value", int_value: source.frameCount } },
              fps: { value: { $case: "double_value", double_value: source.fps } },
            },
            created_at: new Date(),
            last_updated: new Date(),
          }
        };
      }

      // Check sinks
      const sink = this.testSinks.get(request.entity_id);
      if (sink) {
        return {
          success: true,
          error_message: "",
          status: {
            state: sink.isActive ? EntityState.ENTITY_STATE_ACTIVE : EntityState.ENTITY_STATE_CREATED,
            health: HealthStatus.HEALTH_STATUS_HEALTHY,
            error_message: "",
            active_connections: Array.from(this.connections.values())
              .filter(conn => conn.targetEntityId === request.entity_id)
              .map(conn => conn.id),
            metrics: {
              frames_received: { value: { $case: "int_value", int_value: sink.frameCount } },
              last_frame_time: { value: { $case: "string_value", string_value: sink.lastFrameTime.toISOString() } },
            },
            created_at: new Date(),
            last_updated: new Date(),
          }
        };
      }

      return {
        success: false,
        error_message: `Entity not found: ${request.entity_id}`,
        status: undefined,
      };
    } catch (error) {
      console.error("[Dummy Module] Error getting entity status:", error);
      return {
        success: false,
        error_message: `Failed to get entity status: ${error instanceof Error ? error.message : String(error)}`,
        status: undefined,
      };
    }
  }

  async listEntities(
    request: ListEntitiesRequest,
    context: CallContext
  ): Promise<ListEntitiesResponse> {
    console.log("[Dummy Module] ListEntities called");

    try {
      const mediaSources: MediaSource[] = [];
      const mediaSinks: MediaSink[] = [];

      // Add test pattern sources
      for (const [id, source] of this.testSources.entries()) {
        const passesTypeFilter = !request.entity_type_filter || request.entity_type_filter === "media_source";
        const passesStateFilter = !request.state_filter || request.state_filter === EntityState.ENTITY_STATE_ACTIVE;
        
        if (passesTypeFilter && passesStateFilter) {
          mediaSources.push({
            id,
            name: `Test Pattern Source ${id}`,
            type: "test_pattern",
            config: {
              fps: source.fps.toString(),
              resolution: `${source.resolution.width}x${source.resolution.height}`,
              frame_count: source.frameCount.toString(),
            },
            status: {
              state: source.isActive ? EntityState.ENTITY_STATE_ACTIVE : EntityState.ENTITY_STATE_CREATED,
              health: HealthStatus.HEALTH_STATUS_HEALTHY,
              error_message: "",
              active_connections: [],
              metrics: {
                frames_generated: { value: { $case: "int_value", int_value: source.frameCount } },
                fps: { value: { $case: "double_value", double_value: source.fps } },
              },
              created_at: new Date(),
              last_updated: new Date(),
            },
            provides: {
              supported_protocols: ["synthetic"],
              supported_codecs: ["test_pattern"],
              media_type: MediaType.MEDIA_TYPE_VIDEO,
              properties: {
                synthetic: "true",
                test_pattern: "color_bars",
              },
            },
          });
        }
      }

      // Add test sinks
      for (const [id, sink] of this.testSinks.entries()) {
        const passesTypeFilter = !request.entity_type_filter || request.entity_type_filter === "media_sink";
        const passesStateFilter = !request.state_filter || request.state_filter === EntityState.ENTITY_STATE_ACTIVE;
        
        if (passesTypeFilter && passesStateFilter) {
          mediaSinks.push({
            id,
            name: `Test Sink ${id}`,
            type: "test_sink",
            config: {
              frame_count: sink.frameCount.toString(),
              last_frame_time: sink.lastFrameTime.toISOString(),
            },
            status: {
              state: sink.isActive ? EntityState.ENTITY_STATE_ACTIVE : EntityState.ENTITY_STATE_CREATED,
              health: HealthStatus.HEALTH_STATUS_HEALTHY,
              error_message: "",
              active_connections: [],
              metrics: {
                frames_received: { value: { $case: "int_value", int_value: sink.frameCount } },
                last_frame_time: { value: { $case: "string_value", string_value: sink.lastFrameTime.toISOString() } },
              },
              created_at: new Date(),
              last_updated: new Date(),
            },
            requires: {
              supported_protocols: ["synthetic"],
              supported_codecs: ["test_pattern"],
              media_type: MediaType.MEDIA_TYPE_VIDEO,
              properties: {
                discard_mode: "true",
                frame_counting: "true",
              },
            },
          });
        }
      }

      return {
        success: true,
        error_message: "",
        media_sources: mediaSources,
        media_sinks: mediaSinks,
        protocols: [],
        converters: [],
      };
    } catch (error) {
      console.error("[Dummy Module] Error listing entities:", error);
      return {
        success: false,
        error_message: `Failed to list entities: ${error instanceof Error ? error.message : String(error)}`,
        media_sources: [],
        media_sinks: [],
        protocols: [],
        converters: [],
      };
    }
  }

  private parseResolution(resolution: string): { width: number; height: number } {
    const [width, height] = resolution.split('x').map(Number);
    return { width, height };
  }

  private createTestPatternSource(config: Map<string, string>): TestPatternSource {
    const id = `test_source_${this.nextEntityId++}`;
    const fps = parseInt(config.get('fps') || '30');
    const resolution = this.parseResolution(config.get('resolution') || '1920x1080');
    
    const source: TestPatternSource = {
      id,
      fps,
      resolution,
      frameCount: 0,
      isActive: false,
    };

    this.testSources.set(id, source);
    return source;
  }

  private createTestSink(config: Map<string, string>): TestSink {
    const id = `test_sink_${this.nextEntityId++}`;
    
    const sink: TestSink = {
      id,
      frameCount: 0,
      isActive: false,
      lastFrameTime: new Date(),
    };

    this.testSinks.set(id, sink);
    return sink;
  }

  private startTestPattern(source: TestPatternSource): void {
    if (source.isActive) return;
    
    source.isActive = true;
    const intervalMs = 1000 / source.fps;
    
    source.interval = setInterval(() => {
      source.frameCount++;
      if (source.frameCount % 30 === 0) {
        console.log(`[Dummy Module] Test pattern ${source.id} generated ${source.frameCount} frames`);
      }
    }, intervalMs);
    
    console.log(`[Dummy Module] Started test pattern ${source.id} at ${source.fps} fps`);
  }

  private stopTestPattern(source: TestPatternSource): void {
    if (!source.isActive) return;
    
    source.isActive = false;
    if (source.interval) {
      clearInterval(source.interval);
      source.interval = undefined;
    }
    
    console.log(`[Dummy Module] Stopped test pattern ${source.id}`);
  }

  private simulateFrameReceived(sink: TestSink): void {
    sink.frameCount++;
    sink.lastFrameTime = new Date();
    
    if (sink.frameCount % 30 === 0) {
      console.log(`[Dummy Module] Test sink ${sink.id} received ${sink.frameCount} frames`);
    }
  }

  // PipelineService implementation
  async connectEntities(
    request: ConnectEntitiesRequest,
    context: CallContext
  ): Promise<ConnectEntitiesResponse> {
    console.log(`[Dummy Module] ConnectEntities called: ${request.source_entity_id} -> ${request.target_entity_id}`);

    try {
      // Validate source entity exists
      const source = this.testSources.get(request.source_entity_id);
      if (!source) {
        return {
          success: false,
          error_message: `Source entity not found: ${request.source_entity_id}`,
          connection: undefined,
        };
      }

      // Validate target entity exists
      const sink = this.testSinks.get(request.target_entity_id);
      if (!sink) {
        return {
          success: false,
          error_message: `Target entity not found: ${request.target_entity_id}`,
          connection: undefined,
        };
      }

      // Create connection
      const connectionId = `conn_${this.nextConnectionId++}`;
      const connection: TestConnection = {
        id: connectionId,
        sourceEntityId: request.source_entity_id,
        targetEntityId: request.target_entity_id,
        mediaType: request.media_type,
        quality: request.quality || {
          resolution: { width: 1920, height: 1080 },
          bitrate: 2000000,
          framerate: 30,
          video_codec: "test_pattern",
          audio_codec: "",
        },
        status: ConnectionStatus.CONNECTION_STATUS_INITIALIZING,
        createdAt: new Date(),
        framesSent: 0,
        bytesTransferred: 0,
      };

      this.connections.set(connectionId, connection);

      const connectionResponse: Connection = {
        id: connectionId,
        source_entity_id: request.source_entity_id,
        target_entity_id: request.target_entity_id,
        media_type: request.media_type,
        transport_protocol: "synthetic",
        quality: connection.quality,
        status: ConnectionStatus.CONNECTION_STATUS_INITIALIZING,
        metrics: {
          frames_sent: { value: { $case: "int_value", int_value: 0 } },
          bytes_transferred: { value: { $case: "int_value", int_value: 0 } },
        },
      };

      console.log(`[Dummy Module] Created connection: ${connectionId}`);
      return {
        success: true,
        error_message: "",
        connection: connectionResponse,
      };
    } catch (error) {
      console.error("[Dummy Module] Error connecting entities:", error);
      return {
        success: false,
        error_message: `Failed to connect entities: ${error instanceof Error ? error.message : String(error)}`,
        connection: undefined,
      };
    }
  }

  async disconnectEntities(
    request: DisconnectEntitiesRequest,
    context: CallContext
  ): Promise<DisconnectEntitiesResponse> {
    console.log(`[Dummy Module] DisconnectEntities called: ${request.connection_id}`);

    try {
      const connection = this.connections.get(request.connection_id);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${request.connection_id}`,
        };
      }

      // Stop any active flow
      if (connection.interval) {
        clearInterval(connection.interval);
        connection.interval = undefined;
      }

      // Remove connection
      this.connections.delete(request.connection_id);

      console.log(`[Dummy Module] Disconnected: ${request.connection_id}`);
      return {
        success: true,
        error_message: "",
      };
    } catch (error) {
      console.error("[Dummy Module] Error disconnecting entities:", error);
      return {
        success: false,
        error_message: `Failed to disconnect entities: ${error instanceof Error ? error.message : String(error)}`,
      };
    }
  }

  async startFlow(
    request: StartFlowRequest,
    context: CallContext
  ): Promise<StartFlowResponse> {
    console.log(`[Dummy Module] StartFlow called: ${request.connection_id}`);

    try {
      const connection = this.connections.get(request.connection_id);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${request.connection_id}`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }

      // Get source and sink
      const source = this.testSources.get(connection.sourceEntityId);
      const sink = this.testSinks.get(connection.targetEntityId);

      if (!source || !sink) {
        return {
          success: false,
          error_message: `Source or sink entity not found`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }

      // Start the flow simulation
      connection.status = ConnectionStatus.CONNECTION_STATUS_CONNECTED;
      connection.startedAt = new Date();
      
      // Start source if not active
      if (!source.isActive) {
        this.startTestPattern(source);
      }

      // Start sink
      sink.isActive = true;

      // Simulate data transfer from source to sink
      const transferRate = connection.quality.framerate || 30;
      connection.interval = setInterval(() => {
        connection.framesSent++;
        connection.bytesTransferred += 1024; // Simulate 1KB per frame
        
        // Forward frame to sink
        this.simulateFrameReceived(sink);

        if (connection.framesSent % 30 === 0) {
          console.log(`[Dummy Module] Connection ${connection.id}: ${connection.framesSent} frames transferred`);
        }
      }, 1000 / transferRate);

      console.log(`[Dummy Module] Started flow: ${request.connection_id}`);
      return {
        success: true,
        error_message: "",
        status: ConnectionStatus.CONNECTION_STATUS_CONNECTED,
      };
    } catch (error) {
      console.error("[Dummy Module] Error starting flow:", error);
      return {
        success: false,
        error_message: `Failed to start flow: ${error instanceof Error ? error.message : String(error)}`,
        status: ConnectionStatus.CONNECTION_STATUS_ERROR,
      };
    }
  }

  async stopFlow(
    request: StopFlowRequest,
    context: CallContext
  ): Promise<StopFlowResponse> {
    console.log(`[Dummy Module] StopFlow called: ${request.connection_id}`);

    try {
      const connection = this.connections.get(request.connection_id);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${request.connection_id}`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }

      // Stop data transfer
      if (connection.interval) {
        clearInterval(connection.interval);
        connection.interval = undefined;
      }

      connection.status = ConnectionStatus.CONNECTION_STATUS_DISCONNECTED;

      // Stop sink
      const sink = this.testSinks.get(connection.targetEntityId);
      if (sink) {
        sink.isActive = false;
      }

      console.log(`[Dummy Module] Stopped flow: ${request.connection_id}`);
      return {
        success: true,
        error_message: "",
        status: ConnectionStatus.CONNECTION_STATUS_DISCONNECTED,
      };
    } catch (error) {
      console.error("[Dummy Module] Error stopping flow:", error);
      return {
        success: false,
        error_message: `Failed to stop flow: ${error instanceof Error ? error.message : String(error)}`,
        status: ConnectionStatus.CONNECTION_STATUS_ERROR,
      };
    }
  }

  async getFlowStatus(
    request: GetFlowStatusRequest,
    context: CallContext
  ): Promise<GetFlowStatusResponse> {
    console.log(`[Dummy Module] GetFlowStatus called: ${request.connection_id}`);

    try {
      const connection = this.connections.get(request.connection_id);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${request.connection_id}`,
          connection: undefined,
        };
      }

      const connectionResponse: Connection = {
        id: connection.id,
        source_entity_id: connection.sourceEntityId,
        target_entity_id: connection.targetEntityId,
        media_type: connection.mediaType,
        transport_protocol: "synthetic",
        quality: connection.quality,
        status: connection.status,
        metrics: {
          frames_sent: { value: { $case: "int_value", int_value: connection.framesSent } },
          bytes_transferred: { value: { $case: "int_value", int_value: connection.bytesTransferred } },
          latency_ms: { value: { $case: "int_value", int_value: 10 } }, // Simulated low latency
          uptime_seconds: { 
            value: { 
              $case: "int_value", 
              int_value: connection.startedAt ? Math.floor((Date.now() - connection.startedAt.getTime()) / 1000) : 0 
            } 
          },
        },
      };

      return {
        success: true,
        error_message: "",
        connection: connectionResponse,
      };
    } catch (error) {
      console.error("[Dummy Module] Error getting flow status:", error);
      return {
        success: false,
        error_message: `Failed to get flow status: ${error instanceof Error ? error.message : String(error)}`,
        connection: undefined,
      };
    }
  }

  async adjustQuality(
    request: AdjustQualityRequest,
    context: CallContext
  ): Promise<AdjustQualityResponse> {
    console.log(`[Dummy Module] AdjustQuality called: ${request.connection_id}`);

    try {
      const connection = this.connections.get(request.connection_id);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${request.connection_id}`,
          updated_connection: undefined,
        };
      }

      // Update quality settings
      if (request.new_quality) {
        connection.quality = request.new_quality;
        
        // If flow is active, restart with new settings
        if (connection.interval && request.new_quality.framerate) {
          clearInterval(connection.interval);
          
          const transferRate = request.new_quality.framerate;
          connection.interval = setInterval(() => {
            connection.framesSent++;
            connection.bytesTransferred += Math.floor((request.new_quality?.bitrate || 1000000) / 8 / transferRate);
            
            const sink = this.testSinks.get(connection.targetEntityId);
            if (sink) {
              this.simulateFrameReceived(sink);
            }
          }, 1000 / transferRate);
        }
      }

      const connectionResponse: Connection = {
        id: connection.id,
        source_entity_id: connection.sourceEntityId,
        target_entity_id: connection.targetEntityId,
        media_type: connection.mediaType,
        transport_protocol: "synthetic",
        quality: connection.quality,
        status: connection.status,
        metrics: {
          frames_sent: { value: { $case: "int_value", int_value: connection.framesSent } },
          bytes_transferred: { value: { $case: "int_value", int_value: connection.bytesTransferred } },
        },
      };

      console.log(`[Dummy Module] Adjusted quality for: ${request.connection_id}`);
      return {
        success: true,
        error_message: "",
        updated_connection: connectionResponse,
      };
    } catch (error) {
      console.error("[Dummy Module] Error adjusting quality:", error);
      return {
        success: false,
        error_message: `Failed to adjust quality: ${error instanceof Error ? error.message : String(error)}`,
        updated_connection: undefined,
      };
    }
  }
}

// Main execution
async function main() {
  const port = parseInt(process.env.GRPC_PORT || "50051");
  console.log(`[Dummy Module] Starting on port ${port}`);

  const server = createServer();
  const dummyModule = new DummyModule();
  server.add(ModuleServiceDefinition, dummyModule);
  server.add(ConfigurableModuleServiceDefinition, dummyModule);
  server.add(PipelineServiceDefinition, dummyModule);

  await server.listen(`0.0.0.0:${port}`);
  console.log(`[Dummy Module] Server started on port ${port}`);
  console.log("[Dummy Module] Ready to receive requests");

  // Keep the process alive and handle shutdown signals
  process.on("SIGINT", () => {
    console.log("[Dummy Module] Received SIGINT, shutting down gracefully");
    server.shutdown();
    process.exit(0);
  });

  process.on("SIGTERM", () => {
    console.log("[Dummy Module] Received SIGTERM, shutting down gracefully");
    server.shutdown();
    process.exit(0);
  });
}

if (require.main === module) {
  main().catch((error) => {
    console.error("[Dummy Module] Failed to start:", error);
    process.exit(1);
  });
}
