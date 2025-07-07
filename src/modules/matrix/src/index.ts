#!/usr/bin/env node

import { createServer } from "nice-grpc";
import {
  ModuleServiceImplementation,
  ModuleServiceDefinition,
  HealthCheckRequest,
  HealthCheckResponse,
  ShutdownRequest,
  ShutdownResponse,
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
import {
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

} from "call-assistant-protos/module";
import { ModuleState, HealthStatus, MetricValue } from "call-assistant-protos/common";
import { Empty } from "call-assistant-protos/google/protobuf/empty";
import {
  ListEntitiesRequest,
  ListEntitiesResponse,
  Protocol,
  GetEntityStatusRequest,
  GetEntityStatusResponse,
  ConnectionStatus,
  Connection,
  MediaType,
  QualityProfile,
  EntityState,
} from "call-assistant-protos/entities";
import type { CallContext } from "nice-grpc-common";
import { createClient as createMatrixClient, ClientEvent, CallEvent, MatrixClient, MatrixCall } from "matrix-js-sdk";
import { createChannel, createClient } from "nice-grpc";
import {
  EventServiceDefinition,
  Event,
  EventSeverity,
  ReportEventRequest,
} from "call-assistant-protos/events";
import { MatrixProtocol } from "./matrix-protocol";
import { MatrixModuleConfiguration, MatrixModuleConfig } from "./configuration";

class MatrixModule
  implements
    ModuleServiceImplementation,
    ConfigurableModuleServiceImplementation,
    PipelineServiceImplementation
{
  private configuration = new MatrixModuleConfiguration();
  private matrixProtocol?: MatrixProtocol;
  private isMatrixSynced: boolean = false;
  private activeConnections: Map<string, {
    sourceEntityId: string;
    targetEntityId: string;
    mediaType: MediaType;
    quality: QualityProfile;
    connectionConfig: { [key: string]: string };
    createdAt: Date;
    status: ConnectionStatus;
    startedAt?: Date;
    matrixCall?: MatrixCall; // Matrix call object from matrix-js-sdk
    targetRoom?: string;
    callType?: string;
  }> = new Map();


  async healthCheck(
    request: HealthCheckRequest,
    context: CallContext
  ): Promise<HealthCheckResponse> {
    console.log("[Matrix Module] HealthCheck called");

    const isConfigured = this.configuration.currentConfig !== undefined;
    const isConnected = this.matrixProtocol !== undefined;

    // Only report as ready when Matrix client is fully synced
    let state = ModuleState.MODULE_STATE_UNSPECIFIED;
    if (isConfigured && isConnected && this.isMatrixSynced) {
      state = ModuleState.MODULE_STATE_READY;
    } else if (isConfigured && isConnected) {
      state = ModuleState.MODULE_STATE_STARTING;
    } else if (isConfigured) {
      state = ModuleState.MODULE_STATE_WAITING_FOR_CONFIG;
    }

    return {
      status: {
        state,
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
    console.log("[Matrix Module] Shutdown called with:", request);

    const response: ShutdownResponse = {
      success: true,
      error_message: "",
    };

    // Clean up Matrix client
    if (this.matrixProtocol) {
      try {
        this.matrixProtocol.shutdown();
        console.log("[Matrix Module] Matrix client stopped");
      } catch (error) {
        console.error("[Matrix Module] Error stopping Matrix client:", error);
      }
    }

    // Gracefully shutdown after sending response
    setTimeout(() => {
      console.log("[Matrix Module] Shutting down gracefully");
      process.exit(0);
    }, 1000);

    return response;
  }

  async getConfigSchema(
    request: Empty,
    context: CallContext
  ): Promise<GetConfigSchemaResponse> {
    return this.configuration.getConfigSchema(request, context);
  }

  async applyConfig(
    request: ApplyConfigRequest,
    context: CallContext
  ): Promise<ApplyConfigResponse> {
    console.log(
      "[Matrix Module] ApplyConfig called with:",
      request.config_json
    );

    const response = await this.configuration.applyConfig(request, context);
    
    // If configuration was applied successfully, initialize Matrix client
    if (response.success) {
      try {
        await this.initializeMatrixProtocol();
        console.log(
          "[Matrix Module] Configuration applied and Matrix client initialized successfully"
        );
      } catch (error) {
        console.error(
          "[Matrix Module] Error initializing Matrix client:",
          error
        );
        return {
          success: false,
          error_message: `Failed to initialize Matrix client: ${
            error instanceof Error ? error.message : String(error)
          }`,
          validation_errors: [],
          applied_config_version: "",
        };
      }
    }

    return response;
  }

  async getCurrentConfig(
    request: Empty,
    context: CallContext
  ): Promise<GetCurrentConfigResponse> {
    return this.configuration.getCurrentConfig(request, context);
  }

  async validateConfig(
    request: ValidateConfigRequest,
    context: CallContext
  ): Promise<ValidateConfigResponse> {
    return this.configuration.validateConfig(request, context);
  }

  async listEntities(
    request: ListEntitiesRequest,
    context: CallContext
  ): Promise<ListEntitiesResponse> {
    console.log("[Matrix Module] ListEntities called");

    try {
      const protocols: Protocol[] = [];

      // If we have a configured matrix protocol, include it
      if (this.matrixProtocol) {
        // Apply filters if specified
        const passesTypeFilter =
          !request.entity_type_filter ||
          request.entity_type_filter === "protocol";
        const passesStateFilter =
          !request.state_filter ||
          this.matrixProtocol.status?.state === request.state_filter;

        if (passesTypeFilter && passesStateFilter) {
          protocols.push(this.matrixProtocol);
        }
      }

      return {
        success: true,
        error_message: "",
        media_sources: [],
        media_sinks: [],
        protocols: protocols,
        converters: [],
      };
    } catch (error) {
      console.error("[Matrix Module] Error listing entities:", error);
      return {
        success: false,
        error_message: `Failed to list entities: ${
          error instanceof Error ? error.message : String(error)
        }`,
        media_sources: [],
        media_sinks: [],
        protocols: [],
        converters: [],
      };
    }
  }

  async getEntityStatus(
    request: GetEntityStatusRequest,
    context: CallContext
  ): Promise<GetEntityStatusResponse> {
    console.log("[Matrix Module] GetEntityStatus called for:", request.entity_id);

    try {
      // Check if we have a matrix protocol and it matches the requested entity
      if (this.matrixProtocol && this.matrixProtocol.id === request.entity_id) {
        return {
          success: true,
          error_message: "",
          status: this.matrixProtocol.status,
        };
      }

      // Entity not found
      return {
        success: false,
        error_message: `Entity not found: ${request.entity_id}`,
        status: undefined,
      };
    } catch (error) {
      console.error("[Matrix Module] Error getting entity status:", error);
      return {
        success: false,
        error_message: `Failed to get entity status: ${
          error instanceof Error ? error.message : String(error)
        }`,
        status: undefined,
      };
    }
  }

  private async initializeMatrixProtocol(): Promise<void> {
    const config = this.configuration.currentConfig;
    if (!config) {
      throw new Error("Matrix configuration not provided");
    }

    console.log(
      `[Matrix Module] Initializing Matrix client for ${config.userId}`
    );

    // Reset sync state when reinitializing
    this.isMatrixSynced = false;
    console.log(`[Matrix Module] Reset isMatrixSynced to false during initialization at ${new Date().toISOString()}`);

    // Stop existing protocol if present
    if (this.matrixProtocol) {
      try {
        this.matrixProtocol.shutdown();
      } catch (error) {
        console.warn("[Matrix Module] Error stopping previous client:", error);
      }
    }

    // Create new Matrix client
    const matrixClient = createMatrixClient({
      baseUrl: config.homeserver,
      accessToken: config.accessToken,
      userId: config.userId,
      deviceId: config.deviceId || "call-assistant-module",
    });

    // Set up event handlers
    matrixClient.on(ClientEvent.Sync, (state: string) => {
      console.log(`[Matrix Module] Sync state: ${state}`);
      
      if (state === "PREPARED") {
        console.log(`[Matrix Module] Matrix client fully synced and ready! Setting isMatrixSynced=true at ${new Date().toISOString()}`);
        this.isMatrixSynced = true;
        
        // Send status update event to orchestrator
        this.sendSyncCompleteEvent();
      } else if (state === "SYNCING") {
        console.log(`[Matrix Module] Matrix client is syncing...`);
        // Don't reset isMatrixSynced to false if it was already prepared
        // The client can go back to syncing temporarily but still be usable for calls
        if (!this.isMatrixSynced) {
          console.log(`[Matrix Module] Matrix client initial sync in progress...`);
        } else {
          console.log(`[Matrix Module] Matrix client re-syncing (keeping ready state)`);
        }
      }
    });

    matrixClient.on(ClientEvent.ClientWellKnown, (wellKnown: unknown) => {
      console.log(
        `[Matrix Module] Client well-known received: ${JSON.stringify(
          wellKnown
        )}`
      );
    });

    // Start the Matrix client
    await matrixClient.startClient();

    // Create the protocol wrapper
    this.matrixProtocol = new MatrixProtocol(matrixClient, config);

    console.log("[Matrix Module] Matrix client initialized successfully");
  }

  // PipelineService implementation
  async connectEntities(
    request: ConnectEntitiesRequest,
    context: CallContext
  ): Promise<ConnectEntitiesResponse> {
    console.log("[Matrix Module] ConnectEntities called with:", request);

    try {
      // Validate that target entity is our matrix protocol
      if (request.target_entity_id !== this.matrixProtocol?.id) {
        return {
          success: false,
          error_message: `Matrix module can only be target entity. Unknown target: ${request.target_entity_id}`,
          connection: undefined,
        };
      }

      const connectionId = `conn_matrix_${Date.now()}`;
      
      // Create connection object matching dummy module pattern
      const defaultQuality: QualityProfile = {
        resolution: { width: 1920, height: 1080 },
        bitrate: 2000000,
        framerate: 30,
        video_codec: "h264",
        audio_codec: "opus",
      };

      const connectionMetrics: { [key: string]: MetricValue } = {
        frames_received: { value: { $case: "int_value" as const, int_value: 0 } },
        bytes_transferred: { value: { $case: "int_value" as const, int_value: 0 } },
      };

      const connection: Connection = {
        id: connectionId,
        source_entity_id: request.source_entity_id,
        target_entity_id: request.target_entity_id,
        media_type: request.media_type,
        transport_protocol: "webrtc",
        quality: request.quality ?? defaultQuality,
        status: ConnectionStatus.CONNECTION_STATUS_INITIALIZING,
        metrics: connectionMetrics,
      };

      // Store connection info internally
      this.activeConnections.set(connectionId, {
        sourceEntityId: request.source_entity_id,
        targetEntityId: request.target_entity_id,
        mediaType: request.media_type,
        quality: request.quality ?? defaultQuality,
        connectionConfig: request.connection_config ?? {},
        createdAt: new Date(),
        status: ConnectionStatus.CONNECTION_STATUS_INITIALIZING,
      });

      console.log(`[Matrix Module] Connection established: ${connectionId}`);

      return {
        success: true,
        error_message: "",
        connection: connection,
      };
    } catch (error) {
      console.error("[Matrix Module] Error connecting entities:", error);
      return {
        success: false,
        error_message: `Failed to connect entities: ${
          error instanceof Error ? error.message : String(error)
        }`,
        connection: undefined,
      };
    }
  }

  async disconnectEntities(
    request: DisconnectEntitiesRequest,
    context: CallContext
  ): Promise<DisconnectEntitiesResponse> {
    console.log("[Matrix Module] DisconnectEntities called with:", request);

    try {
      const connectionId = request.connection_id;
      
      if (this.activeConnections.has(connectionId)) {
        this.activeConnections.delete(connectionId);
        console.log(`[Matrix Module] Connection disconnected: ${connectionId}`);
      }

      return {
        success: true,
        error_message: "",
      };
    } catch (error) {
      console.error("[Matrix Module] Error disconnecting entities:", error);
      return {
        success: false,
        error_message: `Failed to disconnect entities: ${
          error instanceof Error ? error.message : String(error)
        }`,
      };
    }
  }

  async startFlow(
    request: StartFlowRequest,
    context: CallContext
  ): Promise<StartFlowResponse> {
    console.log("[Matrix Module] StartFlow called with:", request);

    try {
      const connectionId = request.connection_id;
      
      // Check if connection exists
      if (!this.activeConnections.has(connectionId)) {
        return {
          success: false,
          error_message: `Connection not found: ${connectionId}`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }

      const connection = this.activeConnections.get(connectionId);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${connectionId}`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }

      const runtimeConfig = request.runtime_config ?? {};
      
      // Extract target room from runtime config
      const targetRoom = runtimeConfig.target_room;
      const callType = runtimeConfig.call_type;
      
      console.log(`[Matrix Module] Starting ${callType} call to room: ${targetRoom}`);
      
      try {
        // Initialize Matrix call
        const matrixCall = await this.initializeMatrixCall(targetRoom, callType);
        
        // Update connection status
        connection.status = ConnectionStatus.CONNECTION_STATUS_CONNECTED;
        connection.startedAt = new Date();
        connection.matrixCall = matrixCall;
        connection.targetRoom = targetRoom;
        connection.callType = callType;

        console.log(`[Matrix Module] Flow started for connection: ${connectionId}`);
        
        return {
          success: true,
          error_message: "",
          status: ConnectionStatus.CONNECTION_STATUS_CONNECTED,
        };
      } catch (callError) {
        console.error("[Matrix Module] Error initializing Matrix call:", callError);
        connection.status = ConnectionStatus.CONNECTION_STATUS_ERROR;
        
        return {
          success: false,
          error_message: `Failed to start Matrix call: ${
            callError instanceof Error ? callError.message : String(callError)
          }`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }
    } catch (error) {
      console.error("[Matrix Module] Error starting flow:", error);
      return {
        success: false,
        error_message: `Failed to start flow: ${
          error instanceof Error ? error.message : String(error)
        }`,
        status: ConnectionStatus.CONNECTION_STATUS_ERROR,
      };
    }
  }

  async stopFlow(
    request: StopFlowRequest,
    context: CallContext
  ): Promise<StopFlowResponse> {
    console.log("[Matrix Module] StopFlow called with:", request);

    try {
      const connectionId = request.connection_id;
      
      if (!this.activeConnections.has(connectionId)) {
        return {
          success: false,
          error_message: `Connection not found: ${connectionId}`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }

      const connection = this.activeConnections.get(connectionId);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${connectionId}`,
          status: ConnectionStatus.CONNECTION_STATUS_ERROR,
        };
      }
      
      // Stop Matrix call if present
      if (connection.matrixCall) {
        await this.stopMatrixCall(connection.matrixCall);
        connection.matrixCall = undefined;
      }

      // Update connection status
      connection.status = ConnectionStatus.CONNECTION_STATUS_DISCONNECTED;
      
      console.log(`[Matrix Module] Flow stopped for connection: ${connectionId}`);

      return {
        success: true,
        error_message: "",
        status: ConnectionStatus.CONNECTION_STATUS_DISCONNECTED,
      };
    } catch (error) {
      console.error("[Matrix Module] Error stopping flow:", error);
      return {
        success: false,
        error_message: `Failed to stop flow: ${
          error instanceof Error ? error.message : String(error)
        }`,
        status: ConnectionStatus.CONNECTION_STATUS_ERROR,
      };
    }
  }

  async getFlowStatus(
    request: GetFlowStatusRequest,
    context: CallContext
  ): Promise<GetFlowStatusResponse> {
    console.log("[Matrix Module] GetFlowStatus called with:", request);

    try {
      const connectionId = request.connection_id;
      
      if (!this.activeConnections.has(connectionId)) {
        return {
          success: false,
          error_message: `Connection not found: ${connectionId}`,
          connection: undefined,
        };
      }

      const connection = this.activeConnections.get(connectionId);
      if (!connection) {
        return {
          success: false,
          error_message: `Connection not found: ${connectionId}`,
          connection: undefined,
        };
      }
      
      const connectionMetrics: { [key: string]: MetricValue } = {
        target_room: { value: { $case: "string_value" as const, string_value: connection.targetRoom || "" } },
        call_type: { value: { $case: "string_value" as const, string_value: connection.callType || "" } },
        uptime_seconds: { 
          value: { 
            $case: "int_value" as const, 
            int_value: connection.startedAt ? Math.floor((Date.now() - connection.startedAt.getTime()) / 1000) : 0 
          } 
        },
      };

      const connectionResponse: Connection = {
        id: connectionId,
        source_entity_id: connection.sourceEntityId,
        target_entity_id: connection.targetEntityId,
        media_type: connection.mediaType,
        transport_protocol: "webrtc",
        quality: connection.quality,
        status: connection.status,
        metrics: connectionMetrics,
      };
      
      return {
        success: true,
        error_message: "",
        connection: connectionResponse,
      };
    } catch (error) {
      console.error("[Matrix Module] Error getting flow status:", error);
      return {
        success: false,
        error_message: `Failed to get flow status: ${
          error instanceof Error ? error.message : String(error)
        }`,
        connection: undefined,
      };
    }
  }

  // Matrix call helper methods
  private async initializeMatrixCall(targetRoom: string, callType: string): Promise<MatrixCall> {
    if (!this.matrixProtocol) {
      throw new Error("Matrix protocol not initialized");
    }

    console.log(`[Matrix Module] Initializing Matrix call to room ${targetRoom}`);
    
    const matrixClient = this.matrixProtocol.getMatrixClient();
    
    // Check our own sync state flag (more reliable than Matrix client state)
    const syncState = matrixClient.getSyncState();
    console.log(`[Matrix Module] Current sync state: ${syncState}, isMatrixSynced: ${this.isMatrixSynced} at ${new Date().toISOString()}`);
    
    // Only check our own sync flag - we'll handle Matrix client state issues in the call creation retry logic
    if (!this.isMatrixSynced) {
      throw new Error(`Matrix module not synced yet - wait for sync completion before attempting calls`);
    }
    
    console.log(`[Matrix Module] Matrix module is synced, proceeding with call creation`);
    
    
    // Create or get the room
    try {
      const room = matrixClient.getRoom(targetRoom);
      if (!room) {
        console.log(`[Matrix Module] Room ${targetRoom} not found in client state, attempting call anyway`);
        // Don't throw error - room might exist but client not fully synced
      } else {
        console.log(`[Matrix Module] Found room: ${room.name || targetRoom}, members: ${room.getJoinedMemberCount()}`);
      }

      // Create the call - using matrix-js-sdk's MatrixCall
      console.log(`[Matrix Module] Creating call object for room ${targetRoom}`);
      
      let call: MatrixCall | null = null;
      let attempts = 0;
      const maxCallAttempts = 5;
      
      // Try to create call multiple times in case of transient sync state issues
      while (!call && attempts < maxCallAttempts) {
        try {
          call = matrixClient.createCall(targetRoom);
          if (call) {
            console.log(`[Matrix Module] Successfully created call object on attempt ${attempts + 1}`);
            break;
          }
        } catch (error) {
          console.log(`[Matrix Module] Call creation attempt ${attempts + 1} failed:`, error);
        }
        
        attempts++;
        if (attempts < maxCallAttempts) {
          console.log(`[Matrix Module] Retrying call creation in 1 second... (attempt ${attempts + 1}/${maxCallAttempts})`);
          await new Promise(resolve => setTimeout(resolve, 1000));
        }
      }
      
      if (!call) {
        // Try to provide more helpful error information
        const syncState = matrixClient.getSyncState();
        const isLoggedIn = matrixClient.isLoggedIn();
        throw new Error(`Failed to create Matrix call after ${maxCallAttempts} attempts - client state: sync=${syncState}, logged_in=${isLoggedIn}, module_synced=${this.isMatrixSynced}`);
      }

      console.log(`[Matrix Module] Successfully created call object for room ${targetRoom}`);

      // Set up call event listeners using proper Matrix call events
      call.on(CallEvent.State, (state: string) => {
        console.log(`[Matrix Module] Call state changed: ${state}`);
      });

      call.on(CallEvent.Error, (error: Error) => {
        console.error("[Matrix Module] Call error:", error);
      });

      // Place the call based on type
      if (callType === "video") {
        await call.placeVideoCall();
      } else {
        await call.placeVoiceCall();
      }
      
      console.log(`[Matrix Module] Matrix ${callType} call placed successfully to room ${targetRoom}`);
      
      return call;
    } catch (error) {
      console.error("[Matrix Module] Error initializing Matrix call:", error);
      throw error;
    }
  }

  private async stopMatrixCall(matrixCall: MatrixCall): Promise<void> {
    try {
      console.log("[Matrix Module] Stopping Matrix call");
      // Use the correct hangup method - it expects CallErrorCode and boolean
      (matrixCall as any).hangup("user_hangup", false);
      console.log("[Matrix Module] Matrix call stopped");
    } catch (error) {
      console.error("[Matrix Module] Error stopping Matrix call:", error);
    }
  }

  // Event reporting methods
  private async sendSyncCompleteEvent(): Promise<void> {
    try {
      console.log("[Matrix Module] Sending sync complete event to orchestrator");
      
      const channel = createChannel("localhost:9090");
      const eventClient = createClient(EventServiceDefinition, channel);
      
      const config = this.configuration.currentConfig;
      
      const event: Event = {
        id: `matrix_sync_${Date.now()}`,
        severity: EventSeverity.EVENT_SEVERITY_INFO,
        source_module_id: "matrix",
        timestamp: new Date(),
        event_data: {
          $case: "entity_state_changed",
          entity_state_changed: {
            entity_id: this.matrixProtocol?.id || "unknown",
            old_state: EntityState.ENTITY_STATE_CREATED,
            new_state: EntityState.ENTITY_STATE_ACTIVE,
            reason: "Matrix client sync completed - module ready for calls",
          }
        }
      };

      const request: ReportEventRequest = { event };
      const response = await eventClient.reportEvent(request);
      
      if (response.success) {
        console.log("[Matrix Module] ✅ Sync complete event sent successfully");
      } else {
        console.error("[Matrix Module] ❌ Failed to send sync complete event:", response.error_message);
      }
      
      channel.close();
    } catch (error) {
      console.error("[Matrix Module] Error sending sync complete event:", error);
    }
  }
}

// Main execution
async function main() {
  const port = parseInt(process.env.GRPC_PORT || "50051");
  console.log(`[Matrix Module] Starting on port ${port}`);

  const server = createServer();
  const matrixModule = new MatrixModule();
  server.add(ModuleServiceDefinition, matrixModule);
  server.add(ConfigurableModuleServiceDefinition, matrixModule);
  server.add(PipelineServiceDefinition, matrixModule);

  await server.listen(`0.0.0.0:${port}`);
  console.log(`[Matrix Module] Server started on port ${port}`);
  console.log("[Matrix Module] Ready to receive requests");

  // Keep the process alive and handle shutdown signals
  process.on("SIGINT", () => {
    console.log("[Matrix Module] Received SIGINT, shutting down gracefully");
    server.shutdown();
    process.exit(0);
  });

  process.on("SIGTERM", () => {
    console.log("[Matrix Module] Received SIGTERM, shutting down gracefully");
    server.shutdown();
    process.exit(0);
  });
}

if (require.main === module) {
  main().catch((error) => {
    console.error("[Matrix Module] Failed to start:", error);
    process.exit(1);
  });
}
