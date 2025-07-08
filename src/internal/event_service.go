package internal

import (
	"context"
	"log/slog"
	"sync"
	"time"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	eventspb "github.com/shocklateboy92/call-assistant/src/api/proto/events"
)

// EventCallback is a function type for event callbacks
type EventCallback func(ctx context.Context, event *eventspb.Event)

// EventService handles event reporting and broadcasting
type EventService struct {
	eventspb.UnimplementedEventServiceServer

	// Event streaming
	eventStreamsMu sync.RWMutex
	eventStreams   map[string]chan *eventspb.Event

	metricsStreamsMu sync.RWMutex
	metricsStreams   map[string]chan *commonpb.Metrics

	// Event callbacks
	callbacksMu sync.RWMutex
	callbacks   []EventCallback
}

// NewEventService creates a new event service
func NewEventService() *EventService {
	return &EventService{
		eventStreams:   make(map[string]chan *eventspb.Event),
		metricsStreams: make(map[string]chan *commonpb.Metrics),
		callbacks:      make([]EventCallback, 0),
	}
}

// AddEventCallback adds a callback function that will be called for all events
func (es *EventService) AddEventCallback(callback EventCallback) {
	es.callbacksMu.Lock()
	defer es.callbacksMu.Unlock()
	es.callbacks = append(es.callbacks, callback)
}

// ReportEvent handles event reports from modules
func (es *EventService) ReportEvent(
	ctx context.Context,
	req *eventspb.ReportEventRequest,
) (*eventspb.ReportEventResponse, error) {
	if req.Event == nil {
		return &eventspb.ReportEventResponse{
			Success:      false,
			ErrorMessage: "Event is required",
		}, nil
	}

	// Log the event
	slog.Info("Received event from module",
		"module_id", req.Event.SourceModuleId,
		"event_id", req.Event.Id,
		"severity", req.Event.Severity,
	)

	// Execute callbacks first (before broadcasting)
	es.executeCallbacks(ctx, req.Event)

	// Broadcast the event to all subscribed streams
	es.BroadcastEvent(req.Event)

	return &eventspb.ReportEventResponse{
		Success: true,
	}, nil
}

// ReportMetrics handles metrics reports from modules
func (es *EventService) ReportMetrics(
	ctx context.Context,
	req *eventspb.ReportMetricsRequest,
) (*eventspb.ReportMetricsResponse, error) {
	if req.Metrics == nil {
		return &eventspb.ReportMetricsResponse{
			Success:      false,
			ErrorMessage: "Metrics are required",
		}, nil
	}

	// Log the metrics
	slog.Debug("Received metrics from module",
		"module_id", req.Metrics.ModuleId,
		"metric_count", len(req.Metrics.Metrics),
	)

	// Broadcast the metrics to all subscribed streams
	es.BroadcastMetrics(req.Metrics)

	return &eventspb.ReportMetricsResponse{
		Success: true,
	}, nil
}

// executeCallbacks runs all registered callbacks for an event with controlled concurrency
func (es *EventService) executeCallbacks(ctx context.Context, event *eventspb.Event) {
	es.callbacksMu.RLock()
	callbacks := make([]EventCallback, len(es.callbacks))
	copy(callbacks, es.callbacks)
	es.callbacksMu.RUnlock()

	if len(callbacks) == 0 {
		return
	}

	// Use a wait group to ensure all callbacks complete before broadcasting
	var wg sync.WaitGroup

	for _, callback := range callbacks {
		wg.Add(1)
		// Execute each callback in its own goroutine to allow parallel execution
		go func(cb EventCallback) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("Event callback panicked", "error", r, "event_id", event.Id)
				}
			}()
			cb(ctx, event)
		}(callback)
	}

	// Wait for all callbacks to complete before returning, with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All callbacks completed successfully
	case <-time.After(5 * time.Second):
		// Timeout - log warning but continue to avoid indefinite blocking
		slog.Warn("Event callbacks timed out after 5s, proceeding with broadcast",
			"event_id", event.Id,
			"source_module_id", event.SourceModuleId)
	}
}

// BroadcastEvent sends an event to all subscribed event streams
func (es *EventService) BroadcastEvent(event *eventspb.Event) {
	es.eventStreamsMu.RLock()
	defer es.eventStreamsMu.RUnlock()

	for streamID, eventChan := range es.eventStreams {
		select {
		case eventChan <- event:
			// Event sent successfully
		default:
			// Channel is full, skip this stream
			slog.Warn("Event stream buffer full, dropping event", "stream_id", streamID)
		}
	}
}

// BroadcastMetrics sends metrics to all subscribed metrics streams
func (es *EventService) BroadcastMetrics(metrics *commonpb.Metrics) {
	es.metricsStreamsMu.RLock()
	defer es.metricsStreamsMu.RUnlock()

	for streamID, metricsChan := range es.metricsStreams {
		select {
		case metricsChan <- metrics:
			// Metrics sent successfully
		default:
			// Channel is full, skip this stream
			slog.Warn("Metrics stream buffer full, dropping metrics", "stream_id", streamID)
		}
	}
}

// AddEventStream adds a new event stream
func (es *EventService) AddEventStream(streamID string, eventChan chan *eventspb.Event) {
	es.eventStreamsMu.Lock()
	defer es.eventStreamsMu.Unlock()
	es.eventStreams[streamID] = eventChan
}

// RemoveEventStream removes an event stream
func (es *EventService) RemoveEventStream(streamID string) {
	es.eventStreamsMu.Lock()
	defer es.eventStreamsMu.Unlock()
	delete(es.eventStreams, streamID)
}

// AddMetricsStream adds a new metrics stream
func (es *EventService) AddMetricsStream(streamID string, metricsChan chan *commonpb.Metrics) {
	es.metricsStreamsMu.Lock()
	defer es.metricsStreamsMu.Unlock()
	es.metricsStreams[streamID] = metricsChan
}

// RemoveMetricsStream removes a metrics stream
func (es *EventService) RemoveMetricsStream(streamID string) {
	es.metricsStreamsMu.Lock()
	defer es.metricsStreamsMu.Unlock()
	delete(es.metricsStreams, streamID)
}
