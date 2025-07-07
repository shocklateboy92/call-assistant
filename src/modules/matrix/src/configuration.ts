import { createClient as createMatrixClient, MatrixClient } from "matrix-js-sdk";
import Ajv, { JSONSchemaType } from "ajv";
import {
  GetConfigSchemaResponse,
  ApplyConfigRequest,
  ApplyConfigResponse,
  GetCurrentConfigResponse,
  ValidateConfigRequest,
  ValidateConfigResponse,
} from "call-assistant-protos/services/config";
import { Empty } from "call-assistant-protos/google/protobuf/empty";
import type { CallContext } from "nice-grpc-common";

export interface MatrixModuleConfig {
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

export class MatrixModuleConfiguration {
  private config?: MatrixModuleConfig;
  private configVersion: string = "1";
  private validate = new Ajv().compile(schema);

  get currentConfig(): MatrixModuleConfig | undefined {
    return this.config;
  }

  get currentConfigVersion(): string {
    return this.configVersion;
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
      const newConfig: unknown = JSON.parse(request.config_json);
      if (!this.validate(newConfig)) {
        return {
          success: false,
          error_message: "Invalid configuration JSON",
          validation_errors: this.validate.errors?.map((error) => ({
            field_path: error.instancePath.slice(1), // Remove leading dot
            error_code: error.keyword,
            error_message: error.message ?? "Unknown validation error",
            provided_value: JSON.stringify(error.data),
            expected_constraint: JSON.stringify(error.schema),
          })) ?? [],
          applied_config_version: "",
        };
      }

      // Apply the configuration
      this.config = { ...this.config, ...newConfig };
      this.configVersion = request.config_version || new Date().toISOString();

      console.log("[Matrix Module] Configuration applied successfully");

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

    let testClient: MatrixClient | undefined;
    try {
      // Parse JSON first
      const testConfig = JSON.parse(request.config_json);
      if (!this.validate(testConfig)) {
        return {
          valid: false,
          validation_errors: this.validate.errors?.map((error) => ({
            field_path: error.instancePath.slice(1), // Remove leading dot
            error_code: error.keyword,
            error_message: error.message ?? "Unknown validation error",
            provided_value: JSON.stringify(error.data),
            expected_constraint: JSON.stringify(error.schema),
          })) ?? [],
        };
      }

      // Test connection to homeserver
      console.log(
        "[Matrix Module] Testing connection to homeserver:",
        testConfig.homeserver
      );

      try {
        testClient = createMatrixClient({
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
    } finally {
      // Clean up test client if it was created
      if (testClient) {
        try {
          testClient.stopClient();
          console.log("[Matrix Module] Test Matrix client stopped");
        } catch (error) {
          console.warn("[Matrix Module] Error stopping test client:", error);
          // swallowing the exception here because this really shouldn't be fatal
        }
      }
    }
  }
}
