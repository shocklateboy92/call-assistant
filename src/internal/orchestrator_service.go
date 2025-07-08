package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	eventspb "github.com/shocklateboy92/call-assistant/src/api/proto/events"
	modulepb "github.com/shocklateboy92/call-assistant/src/api/proto/module"
	orchestratorpb "github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator"
)

// OrchestratorService implements the gRPC OrchestratorService
type OrchestratorService struct {
	orchestratorpb.UnimplementedOrchestratorServiceServer
	registry     *ModuleRegistry
	manager      *ModuleManager
	eventService *EventService
}

// NewOrchestratorService creates a new orchestrator service
func NewOrchestratorService(registry *ModuleRegistry, manager *ModuleManager) *OrchestratorService {
	eventService := NewEventService()
	
	orchestratorService := &OrchestratorService{
		registry:     registry,
		manager:      manager,
		eventService: eventService,
	}
	
	// Register callback for module_started events
	eventService.AddEventCallback(orchestratorService.handleEventCallback)
	
	return orchestratorService
}

// StartGRPCServer starts the gRPC server for the orchestrator service
func (s *OrchestratorService) StartGRPCServer(ctx context.Context, port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	grpcServer := grpc.NewServer()
	orchestratorpb.RegisterOrchestratorServiceServer(grpcServer, s)
	eventspb.RegisterEventServiceServer(grpcServer, s.eventService)

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

	s.eventService.AddEventStream(streamID, eventChan)

	// Cleanup on exit
	defer func() {
		s.eventService.RemoveEventStream(streamID)
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

	s.eventService.AddMetricsStream(streamID, metricsChan)

	// Cleanup on exit
	defer func() {
		s.eventService.RemoveMetricsStream(streamID)
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

// handleEventCallback handles all events and dispatches to specific handlers
func (s *OrchestratorService) handleEventCallback(ctx context.Context, event *eventspb.Event) {
	// Handle module_started event
	if moduleStartedEvent := event.GetModuleStarted(); moduleStartedEvent != nil {
		s.handleModuleStartedEvent(ctx, event.SourceModuleId, moduleStartedEvent)
	}
}

// handleModuleStartedEvent handles module_started events and performs initial health check
func (s *OrchestratorService) handleModuleStartedEvent(
	_ context.Context,
	moduleID string,
	event *eventspb.ModuleStartedEvent,
) {
	slog.Info("Module started event received",
		"module_id", moduleID,
		"version", event.ModuleVersion,
		"startup_duration_ms", event.StartupDurationMs,
		"grpc_port", event.GrpcPort,
	)

	// Update module registry with the confirmed gRPC port
	// Use the existing UpdateModuleProcess method to update the gRPC port
	regModule, exists := s.registry.GetModule(moduleID)
	if !exists {
		slog.Error("Module not found for gRPC port update", "module_id", moduleID)
		return
	}

	if err := s.registry.UpdateModuleProcess(moduleID, regModule.ProcessID, int(event.GrpcPort)); err != nil {
		slog.Error("Failed to update module gRPC port", "module_id", moduleID, "error", err)
		return
	}

	// Perform initial health check - will be waited on by event service before broadcast
	go s.performHealthCheck(context.Background(), moduleID)
}

// performHealthCheck performs the first health check after module startup
func (s *OrchestratorService) performHealthCheck(ctx context.Context, moduleID string) {
	// Create a timeout context for the health check based on the passed context
	healthCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	regModule, exists := s.registry.GetModule(moduleID)
	if !exists {
		slog.Error("Module not found for health check", "module_id", moduleID)
		return
	}

	if regModule.GRPCPort == 0 {
		slog.Error("Module has no gRPC port for health check", "module_id", moduleID)
		return
	}

	// Connect to module and perform health check
	address := fmt.Sprintf("localhost:%d", regModule.GRPCPort)
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("Failed to connect to module for health check",
			"module_id", moduleID,
			"address", address,
			"error", err,
		)
		s.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, err.Error())
		return
	}
	defer conn.Close()

	// Import module service client
	moduleClient := modulepb.NewModuleServiceClient(conn)

	// Perform health check
	healthReq := &modulepb.HealthCheckRequest{}
	healthResp, err := moduleClient.HealthCheck(healthCtx, healthReq)
	if err != nil {
		slog.Error("Health check failed",
			"module_id", moduleID,
			"error", err,
		)
		s.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, err.Error())
		return
	}

	// Update module status based on health check
	if healthResp.Status != nil {
		slog.Info("Health check successful",
			"module_id", moduleID,
			"state", healthResp.Status.State,
			"health", healthResp.Status.Health,
		)
		s.registry.UpdateModuleStatus(moduleID, healthResp.Status.State, "")
	} else {
		slog.Warn("Health check returned no status", "module_id", moduleID)
	}
}
