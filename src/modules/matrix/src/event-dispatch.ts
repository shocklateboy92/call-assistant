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

// This channel is connected once on module initialization.
// We're not doing reconnection logic here, because the orchestrator
// is expected to outlive its modules.
const channel = createChannel(process.env.EVENT_SERVICE_ADDRESS);
const eventClient = createClient(EventServiceDefinition, channel);

export type EventData = NonNullable<Event["event_data"]>;

export const eventDispatch = {
  sendEvent: async (eventData: EventData): Promise<void> => {
    const eventType = eventData.$case;
    console.log(`[Matrix Module] Sending event: ${eventType}`);

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
        console.log(`[Matrix Module] ✅ ${eventType} event sent successfully`);
      } else {
        console.error(
          `[Matrix Module] ❌ Failed to send ${eventType} event:`,
          response.error_message
        );
      }

      channel.close();
    } catch (error) {
      console.error(`[Matrix Module] Error sending ${eventType} event:`, error);
      // Swallow exceptions, because sending events is not critical
    }
  },
};
