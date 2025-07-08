import { HealthStatus } from "call-assistant-protos/common";
import {
  Protocol,
  EntityStatus,
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
  SyncState,
  createClient as createMatrixClient,
} from "matrix-js-sdk";
import { MatrixModuleConfig } from "./configuration";
import { eventDispatch, moduleId } from "./event-dispatch";
import { MatrixContact } from "./matrix-contact";
import { ISyncStateData } from "matrix-js-sdk/lib/sync";

export class MatrixProtocol implements Protocol {
  public readonly id: string;
  public readonly name: string;
  public readonly type: string = "matrix";

  public get status() {
    return this._status;
  }
  public get contacts(): MatrixContact[] {
    return Object.values(this._contacts);
  }

  public readonly requires_audio = {
    media_type: MediaType.MEDIA_TYPE_AUDIO,
    supported_protocols: ["webrtc"],
    supported_codecs: ["opus", "aac"],
    properties: {},
  };
  public readonly requires_video = {
    media_type: MediaType.MEDIA_TYPE_VIDEO,
    supported_protocols: ["webrtc"],
    supported_codecs: ["h264", "vp8", "vp9"],
    properties: {},
  };

  private matrixClient: MatrixClient;
  private _contacts: Record<string, MatrixContact> = {};
  private _status: EntityStatus = {
    state: EntityState.ENTITY_STATE_CREATED,
    health: HealthStatus.HEALTH_STATUS_HEALTHY,
    error_message: "",
    active_connections: [],
    metrics: {},
    created_at: new Date(),
    last_updated: new Date(),
  };

  constructor(config: MatrixModuleConfig) {
    this.id = `${moduleId}/${config.userId}`;
    this.name = `Matrix (${config.userId})`;

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

    this.onStart();
  }

  public shutdown(): void {
    this.matrixClient.stopClient();
  }

  private async onStart(): Promise<void> {
    console.log(
      `Starting Matrix client for user: ${this.matrixClient.getUserId()}`
    );
    // Emit initial protocol state, so the orchestrator knows this protocol exists
    await this.dispatchEntityUpdate("protocol created");

    const initialSyncPromise = this.addListenerAndWaitForFirstSync();

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

    // Initialize the Matrix client
    await this.matrixClient.startClient();
    console.log(
      `Matrix client started for user: ${this.matrixClient.getUserId()}`
    );

    // This will also take care of broadcasting the state update
    await initialSyncPromise;

    // Trigger initial room discovery
    // Note: this will not trigger if the initial sync fails
    await this.OnRoomsChanged();
  }

  private async OnRoomsChanged() {
    const { joined_rooms } = await this.matrixClient.getJoinedRooms();
    console.log(`Discovered joined rooms: ${joined_rooms.length}`);

    this._contacts = Object.fromEntries<MatrixContact>(
      joined_rooms
        .map(this.matrixClient.getRoom, this.matrixClient)
        .filter((room) => room !== null)
        .map((room) => {
          return [room.roomId, new MatrixContact(room)];
        })
    );

    // Now let the orchestrator know about the contacts
    await this.dispatchEntityUpdate("rooms changed");
  }

  private dispatchEntityUpdate(reason: string) {
    return eventDispatch.sendEvent(
      {
        $case: "entities_updated",
        entities_updated: {
          module_id: moduleId,
          reason,
        },
      },
      reason
    );
  }

  private updateStatus(state: EntityState, errorMessage?: string) {
    this._status = {
      state,
      health: errorMessage
        ? HealthStatus.HEALTH_STATUS_UNHEALTHY
        : HealthStatus.HEALTH_STATUS_HEALTHY,
      error_message: errorMessage || "",
      active_connections: [],
      metrics: {},
      created_at: this._status.created_at,
      last_updated: new Date(),
    };

    return this.dispatchEntityUpdate("status updated to " + EntityState[state]);
  }

  private addListenerAndWaitForFirstSync(): Promise<void> {
    return new Promise((resolve, reject) => {
      // If already prepared, resolve immediately
      if (this.matrixClient.getSyncState() === SyncState.Prepared) {
        resolve();
      }

      this.matrixClient.on(
        ClientEvent.Sync,
        async (
          state: SyncState,
          _prevState: SyncState | null,
          data?: ISyncStateData
        ) => {
          // Keep the listener on so we get this log message
          console.log(`Sync state changed: ${state}`);
          if (state === SyncState.Prepared) {
            await this.updateStatus(EntityState.ENTITY_STATE_ACTIVE);

            resolve();
          } else if (state === SyncState.Error) {
            const error = data?.error;
            console.error(`Sync error: ${error}`);

            await this.updateStatus(
              EntityState.ENTITY_STATE_ERROR,
              `Matrix sync error: ${error}`
            );

            // Reject when this is the first sync
            reject(new Error(`Matrix sync error: ${error}`));
          }
        }
      );
    });
  }
}
