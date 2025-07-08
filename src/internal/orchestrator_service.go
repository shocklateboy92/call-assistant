package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	eventspb "github.com/shocklateboy92/call-assistant/src/api/proto/events"
	orchestratorpb "github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator"
)

// OrchestratorService implements the gRPC OrchestratorService and EventService
type OrchestratorService struct {
	orchestratorpb.UnimplementedOrchestratorServiceServer
	eventspb.UnimplementedEventServiceServer
	registry *ModuleRegistry
	manager  *ModuleManager

	// Event streaming
	eventStreamsMu sync.RWMutex
	eventStreams   map[string]chan *eventspb.Event

	metricsStreamsMu sync.RWMutex
	metricsStreams   map[string]chan *commonpb.Metrics
}

// NewOrchestratorService creates a new orchestrator service
func NewOrchestratorService(registry *ModuleRegistry, manager *ModuleManager) *OrchestratorService {
	return &OrchestratorService{
		registry:       registry,
		manager:        manager,
		eventStreams:   make(map[string]chan *eventspb.Event),
		metricsStreams: make(map[string]chan *commonpb.Metrics),
	}
}

// StartGRPCServer starts the gRPC server for the orchestrator service
func (s *OrchestratorService) StartGRPCServer(ctx context.Context, port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	grpcServer := grpc.NewServer()
	orchestratorpb.RegisterOrchestratorServiceServer(grpcServer, s)
	eventspb.RegisterEventServiceServer(grpcServer, s)

	slog.Info("Starting orchestrator gRPC server", "port", port)

	// Start server in goroutine
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			slog.Error("gRPC server error", "error", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	slog.Info("Shutting down orchestrator gRPC server")
	grpcServer.GracefulStop()

	return nil
}

// ListModules returns all registered modules
func (s *OrchestratorService) ListModules(
	ctx context.Context,
	req *emptypb.Empty,
) (*orchestratorpb.ListModulesResponse, error) {
	modules := s.registry.GetAllModules()

	var moduleInfos []*commonpb.ModuleInfo
	for _, module := range modules {
		moduleInfo := &commonpb.ModuleInfo{
			Id:          module.Module.ID,
			Name:        module.Module.Manifest.Name,
			Version:     module.Module.Manifest.Version,
			Description: module.Module.Manifest.Description,
			Status: &commonpb.ModuleStatus{
				State:        module.Status,
				Health:       commonpb.HealthStatus_HEALTH_STATUS_HEALTHY, // TODO: implement health tracking
				ErrorMessage: module.ErrorMsg,
			},
		}

		// Add gRPC address if module is running
		if module.GRPCPort > 0 {
			moduleInfo.GrpcAddress = fmt.Sprintf("localhost:%d", module.GRPCPort)
		}

		moduleInfos = append(moduleInfos, moduleInfo)
	}

	return &orchestratorpb.ListModulesResponse{
		Modules: moduleInfos,
	}, nil
}

// GetModuleInfo returns information about a specific module
func (s *OrchestratorService) GetModuleInfo(
	ctx context.Context,
	req *orchestratorpb.GetModuleInfoRequest,
) (*orchestratorpb.GetModuleInfoResponse, error) {
	module, exists := s.registry.GetModule(req.ModuleId)
	if !exists {
		return &orchestratorpb.GetModuleInfoResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Module '%s' not found", req.ModuleId),
		}, nil
	}

	moduleInfo := &commonpb.ModuleInfo{
		Id:          module.Module.ID,
		Name:        module.Module.Manifest.Name,
		Version:     module.Module.Manifest.Version,
		Description: module.Module.Manifest.Description,
		Status: &commonpb.ModuleStatus{
			State:        module.Status,
			Health:       commonpb.HealthStatus_HEALTH_STATUS_HEALTHY, // TODO: implement health tracking
			ErrorMessage: module.ErrorMsg,
		},
	}

	// Add gRPC address if module is running
	if module.GRPCPort > 0 {
		moduleInfo.GrpcAddress = fmt.Sprintf("localhost:%d", module.GRPCPort)
	}

	return &orchestratorpb.GetModuleInfoResponse{
		Success:    true,
		ModuleInfo: moduleInfo,
	}, nil
}

// RestartModule restarts a specific module
func (s *OrchestratorService) RestartModule(
	ctx context.Context,
	req *orchestratorpb.RestartModuleRequest,
) (*orchestratorpb.RestartModuleResponse, error) {
	// TODO: Implement module restart functionality
	return &orchestratorpb.RestartModuleResponse{
		Success:      false,
		ErrorMessage: "Module restart not yet implemented",
	}, nil
}

// ShutdownModule shuts down a specific module
func (s *OrchestratorService) ShutdownModule(
	ctx context.Context,
	req *orchestratorpb.ShutdownModuleRequest,
) (*orchestratorpb.ShutdownModuleResponse, error) {
	// TODO: Implement module shutdown functionality
	return &orchestratorpb.ShutdownModuleResponse{
		Success:      false,
		ErrorMessage: "Module shutdown not yet implemented",
	}, nil
}

// SubscribeToEvents subscribes to module events
func (s *OrchestratorService) SubscribeToEvents(
	req *orchestratorpb.SubscribeToEventsRequest,
	stream orchestratorpb.OrchestratorService_SubscribeToEventsServer,
) error {
	// Create a channel for this subscription
	eventChan := make(chan *eventspb.Event, 100)
	streamID := fmt.Sprintf("events_%p", stream)

	s.eventStreamsMu.Lock()
	s.eventStreams[streamID] = eventChan
	s.eventStreamsMu.Unlock()

	// Cleanup on exit
	defer func() {
		s.eventStreamsMu.Lock()
		delete(s.eventStreams, streamID)
		s.eventStreamsMu.Unlock()
		close(eventChan)
	}()

	slog.Info("Client subscribed to events", "stream_id", streamID)

	// Stream events until context is done
	for {
		select {
		case <-stream.Context().Done():
			slog.Info("Event stream closed", "stream_id", streamID)
			return stream.Context().Err()
		case event := <-eventChan:
			if err := stream.Send(event); err != nil {
				slog.Error("Failed to send event", "error", err, "stream_id", streamID)
				return err
			}
		}
	}
}

// SubscribeToMetrics subscribes to module metrics
func (s *OrchestratorService) SubscribeToMetrics(
	req *orchestratorpb.SubscribeToMetricsRequest,
	stream orchestratorpb.OrchestratorService_SubscribeToMetricsServer,
) error {
	// Create a channel for this subscription
	metricsChan := make(chan *commonpb.Metrics, 100)
	streamID := fmt.Sprintf("metrics_%p", stream)

	s.metricsStreamsMu.Lock()
	s.metricsStreams[streamID] = metricsChan
	s.metricsStreamsMu.Unlock()

	// Cleanup on exit
	defer func() {
		s.metricsStreamsMu.Lock()
		delete(s.metricsStreams, streamID)
		s.metricsStreamsMu.Unlock()
		close(metricsChan)
	}()

	slog.Info("Client subscribed to metrics", "stream_id", streamID)

	// Stream metrics until context is done
	for {
		select {
		case <-stream.Context().Done():
			slog.Info("Metrics stream closed", "stream_id", streamID)
			return stream.Context().Err()
		case metrics := <-metricsChan:
			if err := stream.Send(metrics); err != nil {
				slog.Error("Failed to send metrics", "error", err, "stream_id", streamID)
				return err
			}
		}
	}
}

// BroadcastEvent sends an event to all subscribed event streams
func (s *OrchestratorService) BroadcastEvent(event *eventspb.Event) {
	s.eventStreamsMu.RLock()
	defer s.eventStreamsMu.RUnlock()

	for streamID, eventChan := range s.eventStreams {
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
func (s *OrchestratorService) BroadcastMetrics(metrics *commonpb.Metrics) {
	s.metricsStreamsMu.RLock()
	defer s.metricsStreamsMu.RUnlock()

	for streamID, metricsChan := range s.metricsStreams {
		select {
		case metricsChan <- metrics:
			// Metrics sent successfully
		default:
			// Channel is full, skip this stream
			slog.Warn("Metrics stream buffer full, dropping metrics", "stream_id", streamID)
		}
	}
}

// EventService implementation - methods for modules to report events

// ReportEvent handles event reports from modules
func (s *OrchestratorService) ReportEvent(
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

	// Broadcast the event to all subscribed streams
	s.BroadcastEvent(req.Event)

	return &eventspb.ReportEventResponse{
		Success: true,
	}, nil
}

// ReportMetrics handles metrics reports from modules
func (s *OrchestratorService) ReportMetrics(
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
	s.BroadcastMetrics(req.Metrics)

	return &eventspb.ReportMetricsResponse{
		Success: true,
	}, nil
}
