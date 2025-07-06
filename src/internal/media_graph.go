package internal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	entitiespb "github.com/shocklateboy92/call-assistant/src/api/proto/entities"
	modulepb "github.com/shocklateboy92/call-assistant/src/api/proto/module"
	pipelinepb "github.com/shocklateboy92/call-assistant/src/api/proto/pipeline"
)

// MediaGraphManager manages the media graph and entity discovery
type MediaGraphManager struct {
	registry *ModuleRegistry
	
	// Entity cache
	entitiesMu sync.RWMutex
	entities   map[string]*EntityInfo
	
	// Pipeline management
	pipelinesMu sync.RWMutex
	pipelines   map[string]*PipelineInfo
	
	// Discovery settings
	discoveryInterval time.Duration
	stopDiscovery     chan struct{}
}

// EntityInfo represents a discovered entity with its owning module
type EntityInfo struct {
	ModuleID string
	Entity   interface{} // MediaSource, MediaSink, Protocol, or Converter
	Type     string      // "media_source", "media_sink", "protocol", "converter"
}

// PipelineInfo represents an active pipeline
type PipelineInfo struct {
	Pipeline    *pipelinepb.Pipeline
	Connections map[string]*ConnectionInfo
}

// ConnectionInfo represents an active connection between entities
type ConnectionInfo struct {
	Connection  *entitiespb.Connection
	SourceModule string
	TargetModule string
}

// NewMediaGraphManager creates a new media graph manager
func NewMediaGraphManager(registry *ModuleRegistry) *MediaGraphManager {
	return &MediaGraphManager{
		registry:          registry,
		entities:          make(map[string]*EntityInfo),
		pipelines:         make(map[string]*PipelineInfo),
		discoveryInterval: 10 * time.Second,
		stopDiscovery:     make(chan struct{}),
	}
}

// StartDiscovery begins the entity discovery process
func (mgm *MediaGraphManager) StartDiscovery(ctx context.Context) {
	slog.Info("Starting entity discovery")
	
	ticker := time.NewTicker(mgm.discoveryInterval)
	defer ticker.Stop()
	
	// Run initial discovery
	mgm.discoverEntities()
	
	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping entity discovery due to context cancellation")
			return
		case <-mgm.stopDiscovery:
			slog.Info("Stopping entity discovery")
			return
		case <-ticker.C:
			mgm.discoverEntities()
		}
	}
}

// StopDiscovery stops the entity discovery process
func (mgm *MediaGraphManager) StopDiscovery() {
	close(mgm.stopDiscovery)
}

// discoverEntities queries all modules for their entities
func (mgm *MediaGraphManager) discoverEntities() {
	modules := mgm.registry.GetAllModules()
	
	for moduleID, module := range modules {
		if module.GRPCPort == 0 {
			continue // Module not running
		}
		
		entities, err := mgm.queryModuleEntities(moduleID, module.GRPCPort)
		if err != nil {
			slog.Error("Failed to query module entities", 
				"module_id", moduleID, 
				"error", err)
			continue
		}
		
		mgm.updateModuleEntities(moduleID, entities)
	}
}

// queryModuleEntities queries a specific module for its entities
func (mgm *MediaGraphManager) queryModuleEntities(moduleID string, grpcPort int) (*entitiespb.ListEntitiesResponse, error) {
	// Connect to module
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", grpcPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to module: %w", err)
	}
	defer conn.Close()
	
	// Create client and query entities
	client := modulepb.NewModuleServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	response, err := client.ListEntities(ctx, &entitiespb.ListEntitiesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list entities: %w", err)
	}
	
	if !response.Success {
		return nil, fmt.Errorf("module returned error: %s", response.ErrorMessage)
	}
	
	return response, nil
}

// updateModuleEntities updates the entity cache with entities from a module
func (mgm *MediaGraphManager) updateModuleEntities(moduleID string, entities *entitiespb.ListEntitiesResponse) {
	mgm.entitiesMu.Lock()
	defer mgm.entitiesMu.Unlock()
	
	// Remove old entities from this module
	for entityID, entityInfo := range mgm.entities {
		if entityInfo.ModuleID == moduleID {
			delete(mgm.entities, entityID)
		}
	}
	
	// Add new entities
	for _, source := range entities.MediaSources {
		mgm.entities[source.Id] = &EntityInfo{
			ModuleID: moduleID,
			Entity:   source,
			Type:     "media_source",
		}
	}
	
	for _, sink := range entities.MediaSinks {
		mgm.entities[sink.Id] = &EntityInfo{
			ModuleID: moduleID,
			Entity:   sink,
			Type:     "media_sink",
		}
	}
	
	for _, protocol := range entities.Protocols {
		mgm.entities[protocol.Id] = &EntityInfo{
			ModuleID: moduleID,
			Entity:   protocol,
			Type:     "protocol",
		}
	}
	
	for _, converter := range entities.Converters {
		mgm.entities[converter.Id] = &EntityInfo{
			ModuleID: moduleID,
			Entity:   converter,
			Type:     "converter",
		}
	}
	
	slog.Debug("Updated entities for module", 
		"module_id", moduleID,
		"sources", len(entities.MediaSources),
		"sinks", len(entities.MediaSinks),
		"protocols", len(entities.Protocols),
		"converters", len(entities.Converters))
}

// GetMediaGraph returns the current media graph
func (mgm *MediaGraphManager) GetMediaGraph(includePipelines, includeInactive bool) (*pipelinepb.MediaGraph, error) {
	mgm.entitiesMu.RLock()
	defer mgm.entitiesMu.RUnlock()
	
	var mediaSources []*entitiespb.MediaSource
	var mediaSinks []*entitiespb.MediaSink
	var protocols []*entitiespb.Protocol
	var converters []*entitiespb.Converter
	
	for _, entityInfo := range mgm.entities {
		switch entityInfo.Type {
		case "media_source":
			if source, ok := entityInfo.Entity.(*entitiespb.MediaSource); ok {
				if includeInactive || source.Status.State == entitiespb.EntityState_ENTITY_STATE_ACTIVE {
					mediaSources = append(mediaSources, source)
				}
			}
		case "media_sink":
			if sink, ok := entityInfo.Entity.(*entitiespb.MediaSink); ok {
				if includeInactive || sink.Status.State == entitiespb.EntityState_ENTITY_STATE_ACTIVE {
					mediaSinks = append(mediaSinks, sink)
				}
			}
		case "protocol":
			if protocol, ok := entityInfo.Entity.(*entitiespb.Protocol); ok {
				if includeInactive || protocol.Status.State == entitiespb.EntityState_ENTITY_STATE_ACTIVE {
					protocols = append(protocols, protocol)
				}
			}
		case "converter":
			if converter, ok := entityInfo.Entity.(*entitiespb.Converter); ok {
				if includeInactive || converter.Status.State == entitiespb.EntityState_ENTITY_STATE_ACTIVE {
					converters = append(converters, converter)
				}
			}
		}
	}
	
	graph := &pipelinepb.MediaGraph{
		MediaSources: mediaSources,
		MediaSinks:   mediaSinks,
		Protocols:    protocols,
		Converters:   converters,
		LastUpdated:  nil, // TODO: track last update time
	}
	
	if includePipelines {
		mgm.pipelinesMu.RLock()
		for _, pipelineInfo := range mgm.pipelines {
			graph.Pipelines = append(graph.Pipelines, pipelineInfo.Pipeline)
		}
		mgm.pipelinesMu.RUnlock()
	}
	
	return graph, nil
}

// ListEntities returns entities optionally filtered by type and state
func (mgm *MediaGraphManager) ListEntities(typeFilter string, stateFilter entitiespb.EntityState) (*entitiespb.ListEntitiesResponse, error) {
	mgm.entitiesMu.RLock()
	defer mgm.entitiesMu.RUnlock()
	
	var mediaSources []*entitiespb.MediaSource
	var mediaSinks []*entitiespb.MediaSink
	var protocols []*entitiespb.Protocol
	var converters []*entitiespb.Converter
	
	for _, entityInfo := range mgm.entities {
		// Apply type filter
		if typeFilter != "" && entityInfo.Type != typeFilter {
			continue
		}
		
		switch entityInfo.Type {
		case "media_source":
			if source, ok := entityInfo.Entity.(*entitiespb.MediaSource); ok {
				// Apply state filter
				if stateFilter != entitiespb.EntityState_ENTITY_STATE_UNSPECIFIED && 
				   source.Status.State != stateFilter {
					continue
				}
				mediaSources = append(mediaSources, source)
			}
		case "media_sink":
			if sink, ok := entityInfo.Entity.(*entitiespb.MediaSink); ok {
				if stateFilter != entitiespb.EntityState_ENTITY_STATE_UNSPECIFIED && 
				   sink.Status.State != stateFilter {
					continue
				}
				mediaSinks = append(mediaSinks, sink)
			}
		case "protocol":
			if protocol, ok := entityInfo.Entity.(*entitiespb.Protocol); ok {
				if stateFilter != entitiespb.EntityState_ENTITY_STATE_UNSPECIFIED && 
				   protocol.Status.State != stateFilter {
					continue
				}
				protocols = append(protocols, protocol)
			}
		case "converter":
			if converter, ok := entityInfo.Entity.(*entitiespb.Converter); ok {
				if stateFilter != entitiespb.EntityState_ENTITY_STATE_UNSPECIFIED && 
				   converter.Status.State != stateFilter {
					continue
				}
				converters = append(converters, converter)
			}
		}
	}
	
	return &entitiespb.ListEntitiesResponse{
		Success:      true,
		ErrorMessage: "",
		MediaSources: mediaSources,
		MediaSinks:   mediaSinks,
		Protocols:    protocols,
		Converters:   converters,
	}, nil
}

// GetEntityStatus returns the status of a specific entity
func (mgm *MediaGraphManager) GetEntityStatus(entityID string) (*entitiespb.GetEntityStatusResponse, error) {
	mgm.entitiesMu.RLock()
	defer mgm.entitiesMu.RUnlock()
	
	entityInfo, exists := mgm.entities[entityID]
	if !exists {
		return &entitiespb.GetEntityStatusResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Entity not found: %s", entityID),
			Status:       nil,
		}, nil
	}
	
	// Get status based on entity type
	var status *entitiespb.EntityStatus
	switch entityInfo.Type {
	case "media_source":
		if source, ok := entityInfo.Entity.(*entitiespb.MediaSource); ok {
			status = source.Status
		}
	case "media_sink":
		if sink, ok := entityInfo.Entity.(*entitiespb.MediaSink); ok {
			status = sink.Status
		}
	case "protocol":
		if protocol, ok := entityInfo.Entity.(*entitiespb.Protocol); ok {
			status = protocol.Status
		}
	case "converter":
		if converter, ok := entityInfo.Entity.(*entitiespb.Converter); ok {
			status = converter.Status
		}
	}
	
	if status == nil {
		return &entitiespb.GetEntityStatusResponse{
			Success:      false,
			ErrorMessage: "Could not retrieve entity status",
			Status:       nil,
		}, nil
	}
	
	return &entitiespb.GetEntityStatusResponse{
		Success:      true,
		ErrorMessage: "",
		Status:       status,
	}, nil
}

// GetEntityModuleID returns the module ID that owns an entity
func (mgm *MediaGraphManager) GetEntityModuleID(entityID string) (string, bool) {
	mgm.entitiesMu.RLock()
	defer mgm.entitiesMu.RUnlock()
	
	entityInfo, exists := mgm.entities[entityID]
	if !exists {
		return "", false
	}
	
	return entityInfo.ModuleID, true
}