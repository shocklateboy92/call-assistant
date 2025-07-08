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
import {
  ListEntitiesRequest,
  ListEntitiesResponse,
} from "call-assistant-protos/entities";
import type { CallContext } from "nice-grpc-common";
import { JSONSchemaType } from "ajv";

interface DummyModuleConfig {
  username: string;
  password: string;
}

// AJV will type check this schema against the DummyModuleConfig interface
const schema: JSONSchemaType<DummyModuleConfig> = {
  $schema: "http://json-schema.org/draft-07/schema#",
  type: "object",
  properties: {
    username: {
      type: "string",
      nullable: false,
      description: "Username for authentication",
    },
    password: {
      type: "string",
      nullable: false,
      description: "Password for authentication",
    },
  },
  required: ["username", "password"],
  additionalProperties: false,
};

class DummyModule
  implements
    ModuleServiceImplementation,
    ConfigurableModuleServiceImplementation
{
  private config?: DummyModuleConfig;
  private configVersion: string = "1";

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

    // Gracefully shutdown after sending response
    setTimeout(() => {
      console.log("[Dummy Module] Shutting down gracefully");
      process.exit(0);
    }, 1000);

    return {};
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

  async listEntities(
    request: ListEntitiesRequest,
    context: CallContext
  ): Promise<ListEntitiesResponse> {
    console.log("[Dummy Module] ListEntities called");

    return {
      success: true,
      error_message: "",
      media_sources: [],
      media_sinks: [],
      protocols: [],
      converters: [],
    };
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
