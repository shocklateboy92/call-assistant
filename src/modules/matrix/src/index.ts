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
} from "call-assistant-protos/entities";
import type { CallContext } from "nice-grpc-common";
import { MatrixProtocol } from "./matrix-protocol";
import { MatrixConfiguration } from "./configuration";
import { eventDispatch } from "./event-dispatch";

class MatrixModule
  implements
    ModuleServiceImplementation,
    ConfigurableModuleServiceImplementation
{
  private configuration = new MatrixConfiguration();
  // Store protocols by their unique ID
  private protocols: Record<string, MatrixProtocol> = {};

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
    const response: ShutdownResponse = {};

    // Clean up Matrix client
    for (const protocol of Object.values(this.protocols)) {
      try {
        protocol.shutdown();
        console.log("[Matrix Module] Matrix client stopped");
      } catch (error) {
        console.error("[Matrix Module] Error stopping Matrix client:", error);
        response.error_message += `Error stopping Matrix client: ${String(
          error
        )}`;
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

  getConfigSchema(
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

    this.initializeMatrixProtocol();

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

    return {
      success: true,
      error_message: "",
      media_sources: [],
      media_sinks: [],
      protocols: Object.values(this.protocols).filter(
        (p) =>
          request.state_filter === undefined ||
          p.status?.state === request.state_filter
      ),
      converters: [],
    };
  }

  private initializeMatrixProtocol() {
    const config = this.configuration.currentConfig;
    if (!config) {
      console.warn(
        "[Matrix Module] Configuration has not applied successfully, skipping Matrix client initialization"
      );
      return;
    }

    console.log(
      `[Matrix Module] Initializing Matrix client for ${config.userId}`
    );

    // Create the protocol wrapper
    const protocol = new MatrixProtocol(config);
    this.protocols[protocol.id] = protocol;

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

  eventDispatch.sendEvent({
    $case: "module_started",
    module_started: {
      module_version: "1.0.0",
      process_id: process.pid.toString(),
      grpc_port: port,
      startup_duration_ms: Math.floor(performance.now()),
    },
  });

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
