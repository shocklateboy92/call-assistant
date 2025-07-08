import { TargetContact } from "call-assistant-protos/entities";
import { Room } from "matrix-js-sdk";
import { moduleId } from "./event-dispatch";

export class MatrixContact implements TargetContact {
  public get id(): string {
    return `${moduleId}/${this.room.roomId}`;
  }
  public get display_name(): string {
    return this.room.name || this.room.roomId;
  }
  public get protocol_entity_id(): string {
    return moduleId;
  }
  public get address(): string {
    return this.room.roomId;
  }

  constructor(private room: Room) {}
}
