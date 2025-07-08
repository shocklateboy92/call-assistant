import Ajv, { JSONSchemaType } from "ajv";
import { createClient as createMatrixClient } from "matrix-js-sdk";
import {
  GetConfigSchemaResponse,
  ApplyConfigResponse,
  GetCurrentConfigResponse,
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

const validate = new Ajv().compile(schema);

export class MatrixConfiguration {
  private config?: MatrixModuleConfig;
  private configVersion: string = "1";

  get currentConfig(): MatrixModuleConfig | undefined {
    return this.config;
  }

  get version(): string {
    return this.configVersion;
  }

  isConfigured(): boolean {
    return this.config !== undefined;
  }

  async getConfigSchema(
    request: Empty,
    context: CallContext
  ): Promise<GetConfigSchemaResponse> {
    console.log("[Matrix Configuration] GetConfigSchema called");

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
    request: { config_json: string; config_version?: string },
    context: CallContext
  ): Promise<ApplyConfigResponse> {
    console.log(
      "[Matrix Configuration] ApplyConfig called with:",
      request.config_json
    );

    try {
      const newConfig = JSON.parse(request.config_json) as MatrixModuleConfig;

      this.config = { ...this.config, ...newConfig };
      this.configVersion = request.config_version || new Date().toISOString();

      return {
        success: true,
        error_message: "",
        validation_errors: [],
        applied_config_version: this.configVersion,
      };
    } catch (error) {
      console.error(
        "[Matrix Configuration] Error applying configuration:",
        error
      );
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
    console.log("[Matrix Configuration] GetCurrentConfig called");

    return {
      success: true,
      error_message: "",
      config_json: JSON.stringify(this.config, null, 2),
      config_version: this.configVersion,
    };
  }

  async validateConfig(
    request: { config_json: string },
    context: CallContext
  ): Promise<ValidateConfigResponse> {
    console.log(
      "[Matrix Configuration] ValidateConfig called with:",
      request.config_json
    );

    try {
      const testConfig = JSON.parse(request.config_json);
      if (!validate(testConfig)) {
        const validationErrors =
          validate.errors?.map((error) => ({
            field_path:
              error.instancePath || error.schemaPath.replace("#/", ""),
            error_code: error.keyword?.toUpperCase() || "VALIDATION_ERROR",
            error_message: error.message || "Validation failed",
            provided_value: JSON.stringify(error.data || ""),
            expected_constraint: error.params
              ? JSON.stringify(error.params)
              : "See schema",
          })) || [];

        return {
          valid: false,
          validation_errors: validationErrors,
        };
      }

      console.log(
        "[Matrix Configuration] Testing connection to homeserver:",
        testConfig.homeserver
      );

      try {
        const testClient = createMatrixClient({
          baseUrl: testConfig.homeserver,
          accessToken: testConfig.accessToken,
          userId: testConfig.userId,
          deviceId: testConfig.deviceId || "call-assistant",
        });

        const whoamiResponse = await testClient.whoami();
        console.log(
          "[Matrix Configuration] Homeserver connection test successful:",
          whoamiResponse
        );

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
          "[Matrix Configuration] Homeserver connection test failed:",
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
}
