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
import { ModuleState, HealthStatus } from "call-assistant-protos/common";
import { Empty } from "call-assistant-protos/google/protobuf/empty";
import type { CallContext } from "nice-grpc-common";
import { JSONSchemaType } from "ajv";
import {
  MatrixClient,
  createClient as createMatrixClient,
  ClientEvent,
} from "matrix-js-sdk";

interface MatrixModuleConfig {
  homeserver: string;
  accessToken: string;
  userId: string;
  deviceId?: string;
}

// AJV will type check this schema against the MatrixModuleConfig interface
const schema: JSONSchemaType<MatrixModuleConfig> = {
  $schema: "http://json-schema.org/draft-07/schema#",
  type: "object",
  properties: {
    homeserver: {
      type: "string",
      nullable: false,
      description: "Matrix homeserver URL (e.g., https://matrix.org)",
    },
    accessToken: {
      type: "string",
      nullable: false,
      description: "Matrix access token for authentication",
    },
    userId: {
      type: "string",
      nullable: false,
      description: "Matrix user ID (e.g., @user:matrix.org)",
    },
    deviceId: {
      type: "string",
      nullable: true,
      description: "Device ID for this Matrix client (optional)",
    },
  },
  required: ["homeserver", "accessToken", "userId"],
  additionalProperties: false,
};

class MatrixModule
  implements
    ModuleServiceImplementation,
    ConfigurableModuleServiceImplementation
{
  private config?: MatrixModuleConfig;
  private configVersion: string = "1";
  private matrixClient?: MatrixClient;

  async healthCheck(
    request: HealthCheckRequest,
    context: CallContext
  ): Promise<HealthCheckResponse> {
    console.log("[Matrix Module] HealthCheck called");

    const isConfigured = this.config !== undefined;
    const isConnected = this.matrixClient !== undefined;

    return {
      status: {
        state:
          isConfigured && isConnected
            ? ModuleState.MODULE_STATE_READY
            : isConfigured
            ? ModuleState.MODULE_STATE_WAITING_FOR_CONFIG
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
    console.log("[Matrix Module] Shutdown called with:", request);

    const response: ShutdownResponse = {
      success: true,
      error_message: "",
    };

    // Clean up Matrix client
    if (this.matrixClient) {
      try {
        this.matrixClient.stopClient();
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
    console.log("[Matrix Module] GetConfigSchema called");

    return {
      success: true,
      error_message: "",
      schema: {
        schema_version: this.configVersion,
        json_schema: JSON.stringify(schema, null, 2),
        required: true,
      },
    };
  }

  async applyConfig(
    request: ApplyConfigRequest,
    context: CallContext
  ): Promise<ApplyConfigResponse> {
    console.log(
      "[Matrix Module] ApplyConfig called with:",
      request.config_json
    );

    try {
      const newConfig = JSON.parse(request.config_json) as MatrixModuleConfig;

      // Apply the configuration
      this.config = { ...this.config, ...newConfig };
      this.configVersion = request.config_version || new Date().toISOString();

      // Initialize Matrix client with new config
      try {
        await this.initializeMatrixClient();
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

      return {
        success: true,
        error_message: "",
        validation_errors: [],
        applied_config_version: this.configVersion,
      };
    } catch (error) {
      console.error("[Matrix Module] Error applying configuration:", error);
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
    console.log("[Matrix Module] GetCurrentConfig called");

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
      "[Matrix Module] ValidateConfig called with:",
      request.config_json
    );

    try {
      // Parse JSON first
      const testConfig = JSON.parse(request.config_json) as MatrixModuleConfig;

      // Validate required fields
      if (!testConfig.homeserver) {
        return {
          valid: false,
          validation_errors: [
            {
              field_path: "homeserver",
              error_code: "MISSING_REQUIRED_FIELD",
              error_message: "homeserver is required",
              provided_value: "",
              expected_constraint: "Non-empty string",
            },
          ],
        };
      }

      if (!testConfig.accessToken) {
        return {
          valid: false,
          validation_errors: [
            {
              field_path: "accessToken",
              error_code: "MISSING_REQUIRED_FIELD",
              error_message: "accessToken is required",
              provided_value: "",
              expected_constraint: "Non-empty string",
            },
          ],
        };
      }

      if (!testConfig.userId) {
        return {
          valid: false,
          validation_errors: [
            {
              field_path: "userId",
              error_code: "MISSING_REQUIRED_FIELD",
              error_message: "userId is required",
              provided_value: "",
              expected_constraint: "Non-empty string",
            },
          ],
        };
      }

      // Test connection to homeserver
      console.log(
        "[Matrix Module] Testing connection to homeserver:",
        testConfig.homeserver
      );

      try {
        const testClient = createMatrixClient({
          baseUrl: testConfig.homeserver,
          accessToken: testConfig.accessToken,
          userId: testConfig.userId,
          deviceId: testConfig.deviceId || "call-assistant-test",
        });

        // Test the connection by calling whoami
        const whoamiResponse = await testClient.whoami();
        console.log(
          "[Matrix Module] Homeserver connection test successful:",
          whoamiResponse
        );

        // Verify the user ID matches
        if (whoamiResponse.user_id !== testConfig.userId) {
          return {
            valid: false,
            validation_errors: [
              {
                field_path: "userId",
                error_code: "INVALID_USER_ID",
                error_message: `User ID mismatch: expected ${testConfig.userId}, got ${whoamiResponse.user_id}`,
                provided_value: testConfig.userId,
                expected_constraint: `Must match authenticated user: ${whoamiResponse.user_id}`,
              },
            ],
          };
        }

        return {
          valid: true,
          validation_errors: [],
        };
      } catch (error) {
        console.error(
          "[Matrix Module] Homeserver connection test failed:",
          error
        );
        return {
          valid: false,
          validation_errors: [
            {
              field_path: "homeserver",
              error_code: "CONNECTION_FAILED",
              error_message: `Failed to connect to homeserver: ${
                error instanceof Error ? error.message : String(error)
              }`,
              provided_value: testConfig.homeserver,
              expected_constraint:
                "Valid homeserver URL with working Matrix API",
            },
          ],
        };
      }
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

  private async initializeMatrixClient(): Promise<void> {
    if (!this.config) {
      throw new Error("Matrix configuration not provided");
    }

    console.log(
      `[Matrix Module] Initializing Matrix client for ${this.config.userId}`
    );

    // Stop existing client if present
    if (this.matrixClient) {
      try {
        this.matrixClient.stopClient();
      } catch (error) {
        console.warn("[Matrix Module] Error stopping previous client:", error);
      }
    }

    // Create new Matrix client
    this.matrixClient = createMatrixClient({
      baseUrl: this.config.homeserver,
      accessToken: this.config.accessToken,
      userId: this.config.userId,
      deviceId: this.config.deviceId || "call-assistant-module",
    });

    // Set up event handlers
    this.matrixClient.on(ClientEvent.Sync, (state: unknown) => {
      console.log(`[Matrix Module] Sync state: ${JSON.stringify(state)}`);
    });

    this.matrixClient.on(ClientEvent.ClientWellKnown, (wellKnown: unknown) => {
      console.log(
        `[Matrix Module] Client well-known received: ${JSON.stringify(
          wellKnown
        )}`
      );
    });

    // Start the Matrix client
    await this.matrixClient.startClient();

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
