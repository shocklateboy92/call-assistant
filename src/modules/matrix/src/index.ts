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
  ApplyConfigRequest,
  ApplyConfigResponse,
  GetCurrentConfigResponse,
  ValidateConfigRequest,
  ValidateConfigResponse,
  GetConfigSchemaResponse,
} from "call-assistant-protos/services/config";
import { ModuleState, HealthStatus } from "call-assistant-protos/common";
import { Empty } from "call-assistant-protos/google/protobuf/empty";
import {
  ListEntitiesRequest,
  ListEntitiesResponse,
  Protocol,
} from "call-assistant-protos/entities";
import type { CallContext } from "nice-grpc-common";
import { createClient as createMatrixClient, ClientEvent } from "matrix-js-sdk";
import { MatrixProtocol } from "./matrix-protocol";
import { MatrixConfiguration } from "./configuration";


class MatrixModule
  implements
    ModuleServiceImplementation,
    ConfigurableModuleServiceImplementation
{
  private configuration = new MatrixConfiguration();
  private matrixProtocol?: MatrixProtocol;

  async healthCheck(
    request: HealthCheckRequest,
    context: CallContext
  ): Promise<HealthCheckResponse> {
    console.log("[Matrix Module] HealthCheck called");

    const isConfigured = this.configuration.isConfigured();

    return {
      status: {
        state: !isConfigured
          ? ModuleState.MODULE_STATE_WAITING_FOR_CONFIG
          : ModuleState.MODULE_STATE_READY,
        health: HealthStatus.HEALTH_STATUS_HEALTHY,
        error_message: "",
        // TODO: This should be tracked by the orchestrator
        //       instead of being sent by the module
        last_heartbeat: new Date(),
      },
    };
  }

  async shutdown(
    request: ShutdownRequest,
    context: CallContext
  ): Promise<ShutdownResponse> {
    console.log("[Matrix Module] Shutdown called with:", request);
    const response: ShutdownResponse = {}

    // Clean up Matrix client
    if (this.matrixProtocol) {
      try {
        this.matrixProtocol.shutdown();
        console.log("[Matrix Module] Matrix client stopped");
      } catch (error) {
        console.error("[Matrix Module] Error stopping Matrix client:", error);
        response.error_message = `Failed to stop Matrix client: ${String(error)}`;
        // Swallowing the error since we're shutting down anyway
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
    _request: Empty,
    _context: CallContext
  ): Promise<GetConfigSchemaResponse> {
    return this.configuration.getConfigSchema(_request, _context);
  }

  async applyConfig(
    request: ApplyConfigRequest,
    context: CallContext
  ): Promise<ApplyConfigResponse> {
    const result = await this.configuration.applyConfig(request, context);
    
    if (result.success) {
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
    
    return result;
  }

  async getCurrentConfig(
    _request: Empty,
    _context: CallContext
  ): Promise<GetCurrentConfigResponse> {
    return this.configuration.getCurrentConfig(_request, _context);
  }

  async validateConfig(
    request: ValidateConfigRequest,
    _context: CallContext
  ): Promise<ValidateConfigResponse> {
    return this.configuration.validateConfig(request, _context);
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

  private async initializeMatrixProtocol(): Promise<void> {
    const config = this.configuration.currentConfig;
    if (!config) {
      throw new Error("Matrix configuration not provided");
    }

    console.log(
      `[Matrix Module] Initializing Matrix client for ${config.userId}`
    );

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
    matrixClient.on(ClientEvent.Sync, (state: unknown) => {
      console.log(`[Matrix Module] Sync state: ${JSON.stringify(state)}`);
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
}

// Main execution
async function main() {
  const port = parseInt(process.env.GRPC_PORT || "50051");
  console.log(`[Matrix Module] Starting on port ${port}`);

  const server = createServer();
  const matrixModule = new MatrixModule();
  server.add(ModuleServiceDefinition, matrixModule);
  server.add(ConfigurableModuleServiceDefinition, matrixModule);

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
