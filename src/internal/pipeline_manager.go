package internal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	entitiespb "github.com/shocklateboy92/call-assistant/src/api/proto/entities"
	modulepb "github.com/shocklateboy92/call-assistant/src/api/proto/module"
	pipelinepb "github.com/shocklateboy92/call-assistant/src/api/proto/pipeline"
)

// PipelineManager manages pipeline creation, lifecycle, and coordination
type PipelineManager struct {
	registry    *ModuleRegistry
	mediaGraph  *MediaGraphManager
	
	// Pipeline state
	pipelinesMu sync.RWMutex
	pipelines   map[string]*PipelineState
	nextID      int
}

// PipelineState represents the state of an active pipeline
type PipelineState struct {
	Pipeline    *pipelinepb.Pipeline
	Connections map[string]*ConnectionState
	StartedAt   *time.Time
}

// ConnectionState represents the state of a connection within a pipeline
type ConnectionState struct {
	Connection     *entitiespb.Connection
	SourceModuleID string
	TargetModuleID string
	CreatedAt      time.Time
	StartedAt      *time.Time
}

// NewPipelineManager creates a new pipeline manager
func NewPipelineManager(registry *ModuleRegistry, mediaGraph *MediaGraphManager) *PipelineManager {
	return &PipelineManager{
		registry:   registry,
		mediaGraph: mediaGraph,
		pipelines:  make(map[string]*PipelineState),
		nextID:     1,
	}
}

// CreatePipeline creates a new pipeline with the specified entity references and connections
func (pm *PipelineManager) CreatePipeline(req *pipelinepb.CreatePipelineRequest) (*pipelinepb.CreatePipelineResponse, error) {
	pm.pipelinesMu.Lock()
	defer pm.pipelinesMu.Unlock()
	
	// Generate pipeline ID
	pipelineID := fmt.Sprintf("pipeline_%d", pm.nextID)
	pm.nextID++
	
	// Validate entity references exist
	for _, entityRef := range req.EntityRefs {
		if _, exists := pm.mediaGraph.GetEntityModuleID(entityRef.EntityId); !exists {
			return &pipelinepb.CreatePipelineResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Entity not found: %s", entityRef.EntityId),
				Pipeline:     nil,
			}, nil
		}
	}
	
	// Create pipeline
	pipeline := &pipelinepb.Pipeline{
		Id:          pipelineID,
		Name:        req.Name,
		Description: req.Description,
		EntityRefs:  req.EntityRefs,
		Connections: req.Connections,
		Status: &pipelinepb.PipelineStatus{
			State:         pipelinepb.PipelineState_PIPELINE_STATE_CREATED,
			Health:        0, // TODO: use proper health enum
			ErrorMessage:  "",
			Metrics:       make(map[string]*commonpb.MetricValue),
			BandwidthUsage: 0,
			LatencyMs:     0,
			LastUpdated:   timestamppb.Now(),
		},
		CreatedAt: timestamppb.Now(),
	}
	
	// Create pipeline state
	pipelineState := &PipelineState{
		Pipeline:    pipeline,
		Connections: make(map[string]*ConnectionState),
	}
	
	// Initialize connection states
	for _, conn := range req.Connections {
		sourceModuleID, _ := pm.mediaGraph.GetEntityModuleID(conn.SourceEntityId)
		targetModuleID, _ := pm.mediaGraph.GetEntityModuleID(conn.TargetEntityId)
		
		pipelineState.Connections[conn.Id] = &ConnectionState{
			Connection:     conn,
			SourceModuleID: sourceModuleID,
			TargetModuleID: targetModuleID,
			CreatedAt:      time.Now(),
		}
	}
	
	pm.pipelines[pipelineID] = pipelineState
	
	slog.Info("Created pipeline", 
		"pipeline_id", pipelineID,
		"name", req.Name,
		"entities", len(req.EntityRefs),
		"connections", len(req.Connections))
	
	return &pipelinepb.CreatePipelineResponse{
		Success:  true,
		Pipeline: pipeline,
	}, nil
}

// StartPipeline starts all connections in a pipeline
func (pm *PipelineManager) StartPipeline(req *pipelinepb.StartPipelineRequest) (*pipelinepb.StartPipelineResponse, error) {
	pm.pipelinesMu.Lock()
	defer pm.pipelinesMu.Unlock()
	
	pipelineState, exists := pm.pipelines[req.PipelineId]
	if !exists {
		return &pipelinepb.StartPipelineResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Pipeline not found: %s", req.PipelineId),
			Status:       nil,
		}, nil
	}
	
	// Update pipeline state
	pipelineState.Pipeline.Status.State = pipelinepb.PipelineState_PIPELINE_STATE_INITIALIZING
	startTime := time.Now()
	pipelineState.StartedAt = &startTime
	pipelineState.Pipeline.StartedAt = timestamppb.New(startTime)
	
	// Start all connections
	var errors []string
	for _, connState := range pipelineState.Connections {
		err := pm.startConnection(connState, req.RuntimeConfig)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Connection %s: %v", connState.Connection.Id, err))
			continue
		}
		connState.StartedAt = &startTime
	}
	
	// Update pipeline status based on connection results
	if len(errors) > 0 {
		pipelineState.Pipeline.Status.State = pipelinepb.PipelineState_PIPELINE_STATE_ERROR
		pipelineState.Pipeline.Status.ErrorMessage = fmt.Sprintf("Failed to start connections: %v", errors)
		
		return &pipelinepb.StartPipelineResponse{
			Success:      false,
			ErrorMessage: pipelineState.Pipeline.Status.ErrorMessage,
			Status:       pipelineState.Pipeline.Status,
		}, nil
	}
	
	pipelineState.Pipeline.Status.State = pipelinepb.PipelineState_PIPELINE_STATE_ACTIVE
	pipelineState.Pipeline.Status.LastUpdated = timestamppb.Now()
	
	slog.Info("Started pipeline", "pipeline_id", req.PipelineId)
	
	return &pipelinepb.StartPipelineResponse{
		Success: true,
		Status:  pipelineState.Pipeline.Status,
	}, nil
}

// startConnection starts a specific connection by calling the owning module
func (pm *PipelineManager) startConnection(connState *ConnectionState, runtimeConfig map[string]string) error {
	conn := connState.Connection
	
	// Determine which module should handle this connection
	// For now, use the source module (could be more sophisticated)
	moduleID := connState.SourceModuleID
	_ = connState.TargetModuleID // Mark as used
	
	module, exists := pm.registry.GetModule(moduleID)
	if !exists {
		return fmt.Errorf("module not found: %s", moduleID)
	}
	
	if module.GRPCPort == 0 {
		return fmt.Errorf("module not running: %s", moduleID)
	}
	
	// Connect to module
	grpcConn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", module.GRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to module: %w", err)
	}
	defer grpcConn.Close()
	
	// Create pipeline service client
	client := modulepb.NewPipelineServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Connect entities
	connectResp, err := client.ConnectEntities(ctx, &modulepb.ConnectEntitiesRequest{
		SourceEntityId: conn.SourceEntityId,
		TargetEntityId: conn.TargetEntityId,
		MediaType:      conn.MediaType,
		Quality:        conn.Quality,
		ConnectionConfig: runtimeConfig,
	})
	if err != nil {
		return fmt.Errorf("failed to connect entities: %w", err)
	}
	
	if !connectResp.Success {
		return fmt.Errorf("module failed to connect entities: %s", connectResp.ErrorMessage)
	}
	
	// Update connection with response from module
	if connectResp.Connection != nil {
		connState.Connection = connectResp.Connection
	}
	
	// Start flow
	flowResp, err := client.StartFlow(ctx, &modulepb.StartFlowRequest{
		ConnectionId:  conn.Id,
		RuntimeConfig: runtimeConfig,
	})
	if err != nil {
		return fmt.Errorf("failed to start flow: %w", err)
	}
	
	if !flowResp.Success {
		return fmt.Errorf("module failed to start flow: %s", flowResp.ErrorMessage)
	}
	
	return nil
}

// StopPipeline stops all connections in a pipeline
func (pm *PipelineManager) StopPipeline(req *pipelinepb.StopPipelineRequest) (*pipelinepb.StopPipelineResponse, error) {
	pm.pipelinesMu.Lock()
	defer pm.pipelinesMu.Unlock()
	
	pipelineState, exists := pm.pipelines[req.PipelineId]
	if !exists {
		return &pipelinepb.StopPipelineResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Pipeline not found: %s", req.PipelineId),
			Status:       nil,
		}, nil
	}
	
	// Stop all connections
	var errors []string
	for _, connState := range pipelineState.Connections {
		err := pm.stopConnection(connState, req.Force)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Connection %s: %v", connState.Connection.Id, err))
		}
	}
	
	// Update pipeline status
	pipelineState.Pipeline.Status.State = pipelinepb.PipelineState_PIPELINE_STATE_STOPPED
	pipelineState.Pipeline.Status.LastUpdated = timestamppb.Now()
	pipelineState.Pipeline.StoppedAt = timestamppb.Now()
	
	if len(errors) > 0 {
		pipelineState.Pipeline.Status.ErrorMessage = fmt.Sprintf("Errors stopping connections: %v", errors)
	}
	
	slog.Info("Stopped pipeline", "pipeline_id", req.PipelineId)
	
	return &pipelinepb.StopPipelineResponse{
		Success: len(errors) == 0,
		Status:  pipelineState.Pipeline.Status,
	}, nil
}

// stopConnection stops a specific connection
func (pm *PipelineManager) stopConnection(connState *ConnectionState, force bool) error {
	moduleID := connState.SourceModuleID
	
	module, exists := pm.registry.GetModule(moduleID)
	if !exists {
		return fmt.Errorf("module not found: %s", moduleID)
	}
	
	if module.GRPCPort == 0 {
		return nil // Module not running, consider connection stopped
	}
	
	// Connect to module
	grpcConn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", module.GRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to module: %w", err)
	}
	defer grpcConn.Close()
	
	client := modulepb.NewPipelineServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Stop flow
	_, err = client.StopFlow(ctx, &modulepb.StopFlowRequest{
		ConnectionId: connState.Connection.Id,
		Force:        force,
	})
	if err != nil {
		return fmt.Errorf("failed to stop flow: %w", err)
	}
	
	// Disconnect entities
	_, err = client.DisconnectEntities(ctx, &modulepb.DisconnectEntitiesRequest{
		ConnectionId: connState.Connection.Id,
		Force:        force,
	})
	if err != nil {
		return fmt.Errorf("failed to disconnect entities: %w", err)
	}
	
	return nil
}

// DeletePipeline removes a pipeline
func (pm *PipelineManager) DeletePipeline(req *pipelinepb.DeletePipelineRequest) (*pipelinepb.DeletePipelineResponse, error) {
	pm.pipelinesMu.Lock()
	defer pm.pipelinesMu.Unlock()
	
	pipelineState, exists := pm.pipelines[req.PipelineId]
	if !exists {
		return &pipelinepb.DeletePipelineResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Pipeline not found: %s", req.PipelineId),
		}, nil
	}
	
	// Stop pipeline if it's running
	if pipelineState.Pipeline.Status.State == pipelinepb.PipelineState_PIPELINE_STATE_ACTIVE {
		_, err := pm.StopPipeline(&pipelinepb.StopPipelineRequest{
			PipelineId: req.PipelineId,
			Force:      req.Force,
		})
		if err != nil {
			return &pipelinepb.DeletePipelineResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Failed to stop pipeline before deletion: %v", err),
			}, nil
		}
	}
	
	// Remove pipeline
	delete(pm.pipelines, req.PipelineId)
	
	slog.Info("Deleted pipeline", "pipeline_id", req.PipelineId)
	
	return &pipelinepb.DeletePipelineResponse{
		Success: true,
	}, nil
}

// GetPipelineStatus returns the status of a specific pipeline
func (pm *PipelineManager) GetPipelineStatus(req *pipelinepb.GetPipelineStatusRequest) (*pipelinepb.GetPipelineStatusResponse, error) {
	pm.pipelinesMu.RLock()
	defer pm.pipelinesMu.RUnlock()
	
	pipelineState, exists := pm.pipelines[req.PipelineId]
	if !exists {
		return &pipelinepb.GetPipelineStatusResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Pipeline not found: %s", req.PipelineId),
			Status:       nil,
		}, nil
	}
	
	return &pipelinepb.GetPipelineStatusResponse{
		Success: true,
		Status:  pipelineState.Pipeline.Status,
	}, nil
}

// ListPipelines returns all pipelines, optionally filtered
func (pm *PipelineManager) ListPipelines(req *pipelinepb.ListPipelinesRequest) (*pipelinepb.ListPipelinesResponse, error) {
	pm.pipelinesMu.RLock()
	defer pm.pipelinesMu.RUnlock()
	
	var pipelines []*pipelinepb.Pipeline
	for _, pipelineState := range pm.pipelines {
		// Apply state filter
		if req.StateFilter != pipelinepb.PipelineState_PIPELINE_STATE_UNSPECIFIED &&
		   pipelineState.Pipeline.Status.State != req.StateFilter {
			continue
		}
		
		// Apply entity filter
		if req.EntityFilter != "" {
			entityFound := false
			for _, entityRef := range pipelineState.Pipeline.EntityRefs {
				if entityRef.EntityId == req.EntityFilter {
					entityFound = true
					break
				}
			}
			if !entityFound {
				continue
			}
		}
		
		pipelines = append(pipelines, pipelineState.Pipeline)
	}
	
	return &pipelinepb.ListPipelinesResponse{
		Success:   true,
		Pipelines: pipelines,
	}, nil
}

// CalculatePath calculates an optimal path between source and target entities
func (pm *PipelineManager) CalculatePath(req *pipelinepb.CalculatePathRequest) (*pipelinepb.CalculatePathResponse, error) {
	pathReq := req.PathRequest
	
	// For now, implement simple direct connection
	// TODO: Implement sophisticated path calculation with converters
	
	// Verify source and target exist
	_, sourceExists := pm.mediaGraph.GetEntityModuleID(pathReq.SourceEntityId)
	_, targetExists := pm.mediaGraph.GetEntityModuleID(pathReq.TargetEntityId)
	
	if !sourceExists {
		return &pipelinepb.CalculatePathResponse{
			PathResponse: &pipelinepb.PathResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Source entity not found: %s", pathReq.SourceEntityId),
			},
		}, nil
	}
	
	if !targetExists {
		return &pipelinepb.CalculatePathResponse{
			PathResponse: &pipelinepb.PathResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("Target entity not found: %s", pathReq.TargetEntityId),
			},
		}, nil
	}
	
	// Create direct connection
	connectionID := fmt.Sprintf("conn_%d", time.Now().UnixNano())
	connection := &entitiespb.Connection{
		Id:               connectionID,
		SourceEntityId:   pathReq.SourceEntityId,
		TargetEntityId:   pathReq.TargetEntityId,
		MediaType:        pathReq.MediaType,
		TransportProtocol: "synthetic", // TODO: determine appropriate protocol
		Quality:          pathReq.DesiredQuality,
		Status:           entitiespb.ConnectionStatus_CONNECTION_STATUS_INITIALIZING,
		Metrics:          make(map[string]*commonpb.MetricValue),
	}
	
	return &pipelinepb.CalculatePathResponse{
		PathResponse: &pipelinepb.PathResponse{
			Success:           true,
			ErrorMessage:      "",
			EntityPath:        []string{pathReq.SourceEntityId, pathReq.TargetEntityId},
			Connections:       []*entitiespb.Connection{connection},
			EstimatedLatencyMs: 10, // TODO: calculate based on actual path
			EstimatedBandwidth: int64(pathReq.DesiredQuality.GetBitrate()),
		},
	}, nil
}