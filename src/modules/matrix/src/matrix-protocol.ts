import { HealthStatus } from "call-assistant-protos/common";
import { Protocol, EntityStatus, EntityCapabilities, EntityState, MediaType } from "call-assistant-protos/entities";
import { MatrixClient } from "matrix-js-sdk";
import { MatrixModuleConfig } from "./configuration";

export class MatrixProtocol implements Protocol {
  public readonly id: string;
  public readonly name: string;
  public readonly type: string = "matrix";
  public readonly config: { [key: string]: string; };
  public readonly status: EntityStatus;
  public readonly requires_audio: EntityCapabilities;
  public readonly requires_video: EntityCapabilities;

  constructor(private matrixClient: MatrixClient, moduleConfig: MatrixModuleConfig) {
    this.id = `matrix__${moduleConfig.userId.replace(/[^a-zA-Z0-9]/g, "_")}_${Date.now()}`;
    this.name = `Matrix Protocol (${moduleConfig.userId})`;
    this.config = {
      homeserver: moduleConfig.homeserver,
      user_id: moduleConfig.userId,
      device_id: moduleConfig.deviceId || "call-assistant-module",
    };
    this.status = {
      state: EntityState.ENTITY_STATE_ACTIVE,
      health: HealthStatus.HEALTH_STATUS_HEALTHY,
      error_message: "",
      active_connections: [],
      metrics: {},
      created_at: new Date(),
      last_updated: new Date(),
    };
    this.requires_audio = {
      media_type: MediaType.MEDIA_TYPE_AUDIO,
      supported_protocols: ["webrtc"],
      supported_codecs: ["opus", "aac"],
      properties: {},
    };
    this.requires_video = {
      media_type: MediaType.MEDIA_TYPE_VIDEO,
      supported_protocols: ["webrtc"],
      supported_codecs: ["h264", "vp8", "vp9"],
      properties: {},
    };
  }

  getMatrixClient(): MatrixClient {
    return this.matrixClient;
  }

  shutdown(): void {
    this.matrixClient.stopClient();
  }
}
