/**
 * This module provides a simple event dispatch mechanism for the Matrix module.
 * It sends events to the orchestrator's event service using gRPC.
 *
 * The events are sent as `ReportEventRequest` messages, which contain an `Event`
 * object with details about the event being reported.
 */

import {
  EventServiceDefinition,
  ReportEventRequest,
  Event,
  EventSeverity,
} from "call-assistant-protos/events";
import { createChannel, createClient } from "nice-grpc";

if (!process.env.EVENT_SERVICE_ADDRESS) {
  throw new Error("EVENT_SERVICE_ADDRESS environment variable is not set");
}

if (!process.env.MODULE_ID) {
  throw new Error("MODULE_ID environment variable is not set");
}
export const moduleId = process.env.MODULE_ID;

// This channel is connected once on module initialization.
// We're not doing reconnection logic here, because the orchestrator
// is expected to outlive its modules.
const channel = createChannel(process.env.EVENT_SERVICE_ADDRESS);
const eventClient = createClient(EventServiceDefinition, channel);

export type EventData = NonNullable<Event["event_data"]>;

export const eventDispatch = {
  sendEvent: async (eventData: EventData): Promise<boolean> => {
    const eventType = eventData.$case;
    console.log(`Sending event: ${eventType}`);

    const request: ReportEventRequest = {
      event: {
        id: `matrix_module/${eventType}/${Date.now()}`,
        severity: EventSeverity.EVENT_SEVERITY_INFO,
        source_module_id: "matrix",
        timestamp: new Date(),
        event_data: eventData,
      },
    };

    try {
      const response = await eventClient.reportEvent(request);

      if (response.success) {
        console.log(`✅ ${eventType} event sent successfully`);
        return true;
      } else {
        console.error(
          `❌ Failed to send ${eventType} event:`,
          response.error_message
        );
      }

      channel.close();
    } catch (error) {
      console.error(`Error sending ${eventType} event:`, error);
      // Swallow exceptions, because sending events is not critical
    }

    return false;
  },
};
