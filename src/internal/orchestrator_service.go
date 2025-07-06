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
	entitiespb "github.com/shocklateboy92/call-assistant/src/api/proto/entities"
	eventspb "github.com/shocklateboy92/call-assistant/src/api/proto/events"
	orchestratorpb "github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator"
	pipelinepb "github.com/shocklateboy92/call-assistant/src/api/proto/pipeline"
)

// OrchestratorService implements the gRPC OrchestratorService and EventService
type OrchestratorService struct {
	orchestratorpb.UnimplementedOrchestratorServiceServer
	eventspb.UnimplementedEventServiceServer
	registry        *ModuleRegistry
	manager         *ModuleManager
	mediaGraph      *MediaGraphManager
	pipelineManager *PipelineManager

	// Event streaming
	eventStreamsMu sync.RWMutex
	eventStreams   map[string]chan *eventspb.Event

	statusStreamsMu sync.RWMutex
	statusStreams   map[string]chan *commonpb.StatusUpdate

	metricsStreamsMu sync.RWMutex
	metricsStreams   map[string]chan *commonpb.Metrics
}

// NewOrchestratorService creates a new orchestrator service
func NewOrchestratorService(registry *ModuleRegistry, manager *ModuleManager, mediaGraph *MediaGraphManager, pipelineManager *PipelineManager) *OrchestratorService {
	return &OrchestratorService{
		registry:        registry,
		manager:         manager,
		mediaGraph:      mediaGraph,
		pipelineManager: pipelineManager,
		eventStreams:    make(map[string]chan *eventspb.Event),
		statusStreams:   make(map[string]chan *commonpb.StatusUpdate),
		metricsStreams:  make(map[string]chan *commonpb.Metrics),
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

// SubscribeToStatusUpdates subscribes to module status updates
func (s *OrchestratorService) SubscribeToStatusUpdates(
	req *orchestratorpb.SubscribeToStatusRequest,
	stream orchestratorpb.OrchestratorService_SubscribeToStatusUpdatesServer,
) error {
	// Create a channel for this subscription
	statusChan := make(chan *commonpb.StatusUpdate, 100)
	streamID := fmt.Sprintf("status_%p", stream)

	s.statusStreamsMu.Lock()
	s.statusStreams[streamID] = statusChan
	s.statusStreamsMu.Unlock()

	// Cleanup on exit
	defer func() {
		s.statusStreamsMu.Lock()
		delete(s.statusStreams, streamID)
		s.statusStreamsMu.Unlock()
		close(statusChan)
	}()

	slog.Info("Client subscribed to status updates", "stream_id", streamID)

	// Stream status updates until context is done
	for {
		select {
		case <-stream.Context().Done():
			slog.Info("Status stream closed", "stream_id", streamID)
			return stream.Context().Err()
		case status := <-statusChan:
			if err := stream.Send(status); err != nil {
				slog.Error("Failed to send status update", "error", err, "stream_id", streamID)
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

// BroadcastStatusUpdate sends a status update to all subscribed status streams
func (s *OrchestratorService) BroadcastStatusUpdate(status *commonpb.StatusUpdate) {
	s.statusStreamsMu.RLock()
	defer s.statusStreamsMu.RUnlock()

	for streamID, statusChan := range s.statusStreams {
		select {
		case statusChan <- status:
			// Status sent successfully
		default:
			// Channel is full, skip this stream
			slog.Warn("Status stream buffer full, dropping update", "stream_id", streamID)
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

// ReportStatus handles status update reports from modules
func (s *OrchestratorService) ReportStatus(
	ctx context.Context,
	req *eventspb.ReportStatusRequest,
) (*eventspb.ReportStatusResponse, error) {
	if req.StatusUpdate == nil {
		return &eventspb.ReportStatusResponse{
			Success:      false,
			ErrorMessage: "Status update is required",
		}, nil
	}

	// Log the status update
	slog.Info("Received status update from module",
		"module_id", req.StatusUpdate.ModuleId,
		"state", req.StatusUpdate.ModuleStatus.State,
		"health", req.StatusUpdate.ModuleStatus.Health,
	)

	// Update the registry with the new status
	if module, exists := s.registry.GetModule(req.StatusUpdate.ModuleId); exists {
		module.Status = req.StatusUpdate.ModuleStatus.State
		module.ErrorMsg = req.StatusUpdate.ModuleStatus.ErrorMessage
	}

	// Broadcast the status update to all subscribed streams
	s.BroadcastStatusUpdate(req.StatusUpdate)

	return &eventspb.ReportStatusResponse{
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

// Entity management methods

// ListEntities returns all entities from all modules
func (s *OrchestratorService) ListEntities(
	ctx context.Context,
	req *entitiespb.ListEntitiesRequest,
) (*entitiespb.ListEntitiesResponse, error) {
	return s.mediaGraph.ListEntities(req.EntityTypeFilter, req.StateFilter)
}

// GetEntityStatus returns the status of a specific entity
func (s *OrchestratorService) GetEntityStatus(
	ctx context.Context,
	req *entitiespb.GetEntityStatusRequest,
) (*entitiespb.GetEntityStatusResponse, error) {
	return s.mediaGraph.GetEntityStatus(req.EntityId)
}

// GetMediaGraph returns the current media graph
func (s *OrchestratorService) GetMediaGraph(
	ctx context.Context,
	req *pipelinepb.GetMediaGraphRequest,
) (*pipelinepb.GetMediaGraphResponse, error) {
	graph, err := s.mediaGraph.GetMediaGraph(req.IncludePipelines, req.IncludeInactiveEntities)
	if err != nil {
		return &pipelinepb.GetMediaGraphResponse{
			Success:      false,
			ErrorMessage: err.Error(),
			Graph:        nil,
		}, nil
	}

	return &pipelinepb.GetMediaGraphResponse{
		Success: true,
		Graph:   graph,
	}, nil
}

// Pipeline management methods

// CreatePipeline creates a new pipeline
func (s *OrchestratorService) CreatePipeline(
	ctx context.Context,
	req *pipelinepb.CreatePipelineRequest,
) (*pipelinepb.CreatePipelineResponse, error) {
	return s.pipelineManager.CreatePipeline(req)
}

// StartPipeline starts a pipeline
func (s *OrchestratorService) StartPipeline(
	ctx context.Context,
	req *pipelinepb.StartPipelineRequest,
) (*pipelinepb.StartPipelineResponse, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panic in StartPipeline", "panic", r, "pipeline_id", req.PipelineId)
		}
	}()
	
	slog.Info("OrchestratorService.StartPipeline called", "pipeline_id", req.PipelineId)
	
	result, err := s.pipelineManager.StartPipeline(req)
	if err != nil {
		slog.Error("PipelineManager.StartPipeline returned error", "error", err, "pipeline_id", req.PipelineId)
	} else if result != nil && !result.Success {
		slog.Error("PipelineManager.StartPipeline failed", "error", result.ErrorMessage, "pipeline_id", req.PipelineId)
	} else {
		slog.Info("PipelineManager.StartPipeline succeeded", "pipeline_id", req.PipelineId)
	}
	
	return result, err
}

// StopPipeline stops a pipeline
func (s *OrchestratorService) StopPipeline(
	ctx context.Context,
	req *pipelinepb.StopPipelineRequest,
) (*pipelinepb.StopPipelineResponse, error) {
	return s.pipelineManager.StopPipeline(req)
}

// DeletePipeline deletes a pipeline
func (s *OrchestratorService) DeletePipeline(
	ctx context.Context,
	req *pipelinepb.DeletePipelineRequest,
) (*pipelinepb.DeletePipelineResponse, error) {
	return s.pipelineManager.DeletePipeline(req)
}

// GetPipelineStatus returns the status of a specific pipeline
func (s *OrchestratorService) GetPipelineStatus(
	ctx context.Context,
	req *pipelinepb.GetPipelineStatusRequest,
) (*pipelinepb.GetPipelineStatusResponse, error) {
	return s.pipelineManager.GetPipelineStatus(req)
}

// ListPipelines returns all pipelines
func (s *OrchestratorService) ListPipelines(
	ctx context.Context,
	req *pipelinepb.ListPipelinesRequest,
) (*pipelinepb.ListPipelinesResponse, error) {
	return s.pipelineManager.ListPipelines(req)
}

// AdjustQuality adjusts the quality of a pipeline connection
func (s *OrchestratorService) AdjustQuality(
	ctx context.Context,
	req *pipelinepb.AdjustQualityRequest,
) (*pipelinepb.AdjustQualityResponse, error) {
	// TODO: Implement quality adjustment
	return &pipelinepb.AdjustQualityResponse{
		Success:      false,
		ErrorMessage: "Quality adjustment not yet implemented",
	}, nil
}

// CalculatePath calculates an optimal path between entities
func (s *OrchestratorService) CalculatePath(
	ctx context.Context,
	req *pipelinepb.CalculatePathRequest,
) (*pipelinepb.CalculatePathResponse, error) {
	return s.pipelineManager.CalculatePath(req)
}
