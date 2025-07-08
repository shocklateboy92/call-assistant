import { HealthStatus } from "call-assistant-protos/common";
import {
  Protocol,
  EntityStatus,
  EntityCapabilities,
  EntityState,
  MediaType,
} from "call-assistant-protos/entities";
import {
  ClientEvent,
  KnownMembership,
  MatrixClient,
  MemoryStore,
  Room,
  RoomEvent,
  createClient as createMatrixClient,
} from "matrix-js-sdk";
import { MatrixModuleConfig } from "./configuration";
import { eventDispatch, moduleId } from "./event-dispatch";
import { MatrixContact } from "./matrix-contact";

export class MatrixProtocol implements Protocol {
  public readonly id: string;
  public readonly name: string;
  public readonly type: string = "matrix";
  public readonly status: EntityStatus;
  public readonly requires_audio: EntityCapabilities;
  public readonly requires_video: EntityCapabilities;
  public get contact_ids(): string[] {
    return Object.values(this.contacts).map((contact) => contact.id);
  }

  private matrixClient: MatrixClient;
  private contacts: Record<string, MatrixContact> = {};

  constructor(config: MatrixModuleConfig) {
    this.id = `matrix__${config.userId.replace(
      /[^a-zA-Z0-9]/g,
      "_"
    )}_${Date.now()}`;
    this.name = `Matrix (${config.userId})`;

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

    // Create new Matrix client
    this.matrixClient = createMatrixClient({
      baseUrl: config.homeserver,
      accessToken: config.accessToken,
      userId: config.userId,
      deviceId: config.deviceId || "call-assistant-module",
      // Otherwise it doesn't store anything at all,
      // making most sdk client methods useless.
      store: new MemoryStore(),
    });

    // Set up event handlers
    this.matrixClient.on(ClientEvent.Sync, (state: string) => {
      console.log(`Sync state: ${state}`);
    });

    this.onStart();
  }

  private async onStart(): Promise<void> {
    console.log(
      `Starting Matrix client for user: ${this.matrixClient.getUserId()}`
    );
    await this.dispatchEntityUpdate();

    // Initialize the Matrix client
    await this.matrixClient.startClient();
    console.log(
      `Matrix client started for user: ${this.matrixClient.getUserId()}`
    );

    // Emit initial protocol state
    await this.dispatchEntityUpdate();
    console.log(
      `Protocol state dispatched for user: ${this.matrixClient.getUserId()}`
    );

    // Watch for room changes
    this.matrixClient.on(
      RoomEvent.MyMembership,
      (room: Room, membership: string, prevMembership?: string) => {
        console.log(
          `Membership changed in room ${room.roomId}: '${prevMembership}' --> '${membership}'`
        );
        if (
          membership in
          [KnownMembership.Join, KnownMembership.Leave, KnownMembership.Ban]
        ) {
          this.OnRoomsChanged();
        }
      }
    );

    // Trigger initial room discovery
    await this.OnRoomsChanged();
  }

  private async OnRoomsChanged() {
    const { joined_rooms } = await this.matrixClient.getJoinedRooms();
    console.log(`Discovered joined rooms: ${joined_rooms.length}`);

    this.contacts = Object.fromEntries<MatrixContact>(
      joined_rooms
        .map(this.matrixClient.getRoom, this.matrixClient)
        .filter((room) => room !== null)
        .map((room) => {
          return [room.roomId, new MatrixContact(room)];
        })
    );

    // Now let the orchestrator know about the contacts
    await this.dispatchEntityUpdate();
  }

  private dispatchEntityUpdate() {
    return eventDispatch.sendEvent({
      $case: "entities_updated",
      entities_updated: {
        module_id: moduleId,
        reason: "new protocol created",
      },
    });
  }

  getMatrixClient(): MatrixClient {
    return this.matrixClient;
  }

  shutdown(): void {
    this.matrixClient.stopClient();
  }
}
