package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	orchestratorpb "github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator"
	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	configpb "github.com/shocklateboy92/call-assistant/src/generated/go/services"
	modulepb "github.com/shocklateboy92/call-assistant/src/api/proto/module"
	entitiespb "github.com/shocklateboy92/call-assistant/src/api/proto/entities"
	pipelinepb "github.com/shocklateboy92/call-assistant/src/api/proto/pipeline"
)

func TestOrchestratorService() error {
	fmt.Println("🚀 Testing Orchestrator Service")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Step 1: Start orchestrator in background
	fmt.Println("Step 1a: Building orchestrator...")
	cmd := exec.Command("go", "build", "--output", "./bin/", "./src/cmd/orchestrator")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Error("Failed to start orchestrator", "error", err)
		return err
	}

	cmd.Wait() // Wait for build to complete

	fmt.Println("Step 1b: Starting orchestrator...")
	cmd = exec.Command("./bin/orchestrator", "--dev", "--verbose")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Error("Failed to start orchestrator", "error", err)
		return err
	}

	// Ensure we kill the orchestrator when we're done
	defer func() {
		if cmd.Process != nil {
			fmt.Println("\n🛑 Terminating orchestrator...")
			err := cmd.Process.Signal(syscall.SIGTERM)
			if err != nil {
				slog.Error("Failed to terminate orchestrator", "error", err)
			}

			// Bound the time waiting for graceful shutdown
			cmd.WaitDelay = 5 * time.Second
			err = cmd.Wait()

			// If it doesn't exit cleanly, force kill it
			if err != nil && cmd.ProcessState != nil && !cmd.ProcessState.Exited() {
				fmt.Println("Orchestrator did not exit cleanly, force killing...")
				err = cmd.Process.Kill()
				if err != nil {
					slog.Error("Failed to kill orchestrator process", "error", err)
				}
			}

			// Log any error from waiting or killing the process
			if err != nil {
				slog.Error("Orchestrator process did not exit cleanly", "error", err)
			}
		}

		fmt.Println("Orchestrator terminated.")
	}()

	// Step 2: Wait for orchestrator to start
	fmt.Println("Step 2: Waiting for orchestrator to start...")
	time.Sleep(5 * time.Second)

	// Step 3: Connect to orchestrator gRPC service
	fmt.Println("Step 3: Connecting to orchestrator service...")
	conn, err := grpc.NewClient(
		"localhost:9090",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("Failed to connect to orchestrator", "error", err)
		return err
	}
	defer conn.Close()

	client := orchestratorpb.NewOrchestratorServiceClient(conn)

	// Step 4: List all modules
	fmt.Println("\nStep 4: Listing all modules...")
	fmt.Println("───────────────────────────────────────────────────────────────")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	listResp, err := client.ListModules(ctx, &emptypb.Empty{})
	if err != nil {
		slog.Error("Failed to list modules", "error", err)
		return err
	}

	fmt.Printf("Found %d modules:\n", len(listResp.Modules))
	
	// Validate we have exactly 2 modules
	if len(listResp.Modules) != 2 {
		return fmt.Errorf("expected 2 modules, but found %d", len(listResp.Modules))
	}
	
	// Validate we have the expected modules (dummy and matrix)
	moduleIds := make(map[string]bool)
	for _, module := range listResp.Modules {
		moduleIds[module.Id] = true
	}
	
	if !moduleIds["dummy"] {
		return fmt.Errorf("dummy module not found")
	}
	if !moduleIds["matrix"] {
		return fmt.Errorf("matrix module not found")
	}
	
	fmt.Println("✅ Expected modules found: dummy and matrix")
	
	for i, module := range listResp.Modules {
		fmt.Printf("  %d. %s (%s)\n", i+1, module.Name, module.Id)
		fmt.Printf("     Version: %s\n", module.Version)
		fmt.Printf("     Description: %s\n", module.Description)
		fmt.Printf("     State: %s\n", module.Status.State.String())
		fmt.Printf("     gRPC Address: %s\n", module.GrpcAddress)
		if module.Status.ErrorMessage != "" {
			fmt.Printf("     Error: %s\n", module.Status.ErrorMessage)
		}
		fmt.Println()
	}

	// Step 5: Get detailed info for each module
	fmt.Println("Step 5: Getting detailed module information...")
	fmt.Println("───────────────────────────────────────────────────────────────")

	for _, module := range listResp.Modules {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		infoResp, err := client.GetModuleInfo(ctx, &orchestratorpb.GetModuleInfoRequest{
			ModuleId: module.Id,
		})
		cancel()

		if err != nil {
			slog.Error("Failed to get module info", "module_id", module.Id, "error", err)
			continue
		}

		if !infoResp.Success {
			slog.Error(
				"GetModuleInfo failed",
				"module_id",
				module.Id,
				"error",
				infoResp.ErrorMessage,
			)
			continue
		}

		fmt.Printf("Module Details: %s\n", infoResp.ModuleInfo.Id)
		fmt.Printf("  Name: %s\n", infoResp.ModuleInfo.Name)
		fmt.Printf("  Version: %s\n", infoResp.ModuleInfo.Version)
		fmt.Printf("  Description: %s\n", infoResp.ModuleInfo.Description)
		fmt.Printf("  State: %s\n", infoResp.ModuleInfo.Status.State.String())
		fmt.Printf("  Health: %s\n", infoResp.ModuleInfo.Status.Health.String())
		fmt.Printf("  gRPC Address: %s\n", infoResp.ModuleInfo.GrpcAddress)
		fmt.Println()
	}

	// Step 6: Configure dummy module with test user credentials
	fmt.Printf("Step 6: Configuring dummy module with %s credentials...\n", Users[0])
	fmt.Println("───────────────────────────────────────────────────────────────")
	
	// Find the dummy module
	var dummyModule *commonpb.ModuleInfo
	for _, module := range listResp.Modules {
		if module.Id == "dummy" {
			dummyModule = module
			break
		}
	}
	
	if dummyModule != nil {
		dummyConfig := fmt.Sprintf(`{
			"username": "%s",
			"password": "%s"
		}`, Users[0], TestPassword)
		
		err := configureModule(dummyModule.GrpcAddress, "dummy", dummyConfig)
		if err != nil {
			slog.Error("Failed to configure dummy module", "error", err)
		} else {
			fmt.Printf("✅ Dummy module configured successfully with %s credentials\n", Users[0])
		}
	} else {
		fmt.Println("❌ Dummy module not found")
	}

	// Step 6.5: Test Matrix module configuration validation
	fmt.Println("\nStep 6.5: Testing Matrix module configuration...")
	fmt.Println("───────────────────────────────────────────────────────────────")
	
	// Find the matrix module
	var matrixModule *commonpb.ModuleInfo
	for _, module := range listResp.Modules {
		if module.Id == "matrix" {
			matrixModule = module
			break
		}
	}
	
	if matrixModule != nil {
		// Test with bad credentials first
		fmt.Println("  Testing with invalid credentials...")
		badConfig := fmt.Sprintf(`{
			"homeserver": "%s",
			"accessToken": "bad_token",
			"userId": "@%s:localhost"
		}`, SynapseURL, Users[0])
		
		err := configureModule(matrixModule.GrpcAddress, "matrix", badConfig)
		if err != nil {
			fmt.Printf("✅ Matrix module correctly rejected invalid credentials: %v\n", err)
		} else {
			fmt.Println("❌ Matrix module should have rejected invalid credentials")
		}
		
		// Now test with correct Alice credentials
		fmt.Printf("  Testing with valid %s credentials...\n", Users[0])
		aliceConfig := fmt.Sprintf(`{
			"homeserver": "%s",
			"accessToken": "%s",
			"userId": "@%s:localhost"
		}`, SynapseURL, GetAccessToken(Users[0]), Users[0])
		
		err = configureModule(matrixModule.GrpcAddress, "matrix", aliceConfig)
		if err != nil {
			slog.Error("Failed to configure matrix module with valid credentials", "error", err)
		} else {
			fmt.Printf("✅ Matrix module configured successfully with %s credentials\n", Users[0])
			
			// Test GetCallingProtocols after successful configuration
			fmt.Println("  Testing GetCallingProtocols...")
			err = testGetCallingProtocols(matrixModule.GrpcAddress)
			if err != nil {
				slog.Error("Failed to test GetCallingProtocols", "error", err)
			} else {
				fmt.Println("✅ GetCallingProtocols test passed")
			}
		}
	} else {
		fmt.Println("❌ Matrix module not found")
	}

	// Step 7: Subscribe to events for a few seconds
	fmt.Println("Step 7: Subscribing to events for 3 seconds...")
	fmt.Println("───────────────────────────────────────────────────────────────")

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eventStream, err := client.SubscribeToEvents(ctx, &orchestratorpb.SubscribeToEventsRequest{})
	if err != nil {
		slog.Error("Failed to subscribe to events", "error", err)
	} else {
		eventCount := 0
		for {
			event, err := eventStream.Recv()
			if err != nil {
				if ctx.Err() != nil {
					// Context timeout, expected
					break
				}
				slog.Error("Error receiving event", "error", err)
				break
			}

			eventCount++
			fmt.Printf("Received event %d: %s from %s\n", eventCount, event.Severity.String(), event.SourceModuleId)
		}

		if eventCount == 0 {
			fmt.Println("No events received during subscription period")
		}
	}

	// Step 8: Test entity discovery and media graph
	fmt.Println("\nStep 8: Testing entity discovery and media graph...")
	fmt.Println("───────────────────────────────────────────────────────────────")
	
	err = testEntityDiscovery(client)
	if err != nil {
		slog.Error("Entity discovery test failed", "error", err)
		return err
	}
	
	// Step 9: Test pipeline creation and management
	fmt.Println("\nStep 9: Testing pipeline creation and management...")
	fmt.Println("───────────────────────────────────────────────────────────────")
	
	err = testPipelineManagement(client)
	if err != nil {
		slog.Error("Pipeline management test failed", "error", err)
		return err
	}

	fmt.Println("\n🎉 Orchestrator service test completed successfully!")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	return nil
}

func configureModule(grpcAddress, moduleName, configJson string) error {
	// Connect to the module directly
	conn, err := grpc.NewClient(
		grpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to %s module: %w", moduleName, err)
	}
	defer conn.Close()

	client := configpb.NewConfigurableModuleServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First, get the current config schema
	fmt.Println("  Getting config schema...")
	schemaResp, err := client.GetConfigSchema(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to get config schema: %w", err)
	}
	
	if !schemaResp.Success {
		return fmt.Errorf("get config schema failed: %s", schemaResp.ErrorMessage)
	}
	
	fmt.Printf("  Schema version: %s\n", schemaResp.Schema.SchemaVersion)
	
	// First validate the config
	fmt.Println("  Validating configuration...")
	validateResp, err := client.ValidateConfig(ctx, &configpb.ValidateConfigRequest{
		ConfigJson: configJson,
	})
	if err != nil {
		return fmt.Errorf("failed to validate config: %w", err)
	}
	
	if !validateResp.Valid {
		var errorDetails []string
		for _, validationError := range validateResp.ValidationErrors {
			errorDetails = append(errorDetails, fmt.Sprintf("%s: %s", 
				validationError.FieldPath, validationError.ErrorMessage))
		}
		return fmt.Errorf("config validation failed: %v", errorDetails)
	}
	
	fmt.Println("  ✅ Configuration validation passed")
	
	// Apply the configuration
	fmt.Println("  Applying configuration...")
	applyResp, err := client.ApplyConfig(ctx, &configpb.ApplyConfigRequest{
		ConfigJson:    configJson,
		ConfigVersion: "1.0.0",
	})
	if err != nil {
		return fmt.Errorf("failed to apply config: %w", err)
	}
	
	if !applyResp.Success {
		return fmt.Errorf("apply config failed: %s", applyResp.ErrorMessage)
	}
	
	fmt.Printf("  Applied config version: %s\n", applyResp.AppliedConfigVersion)
	
	// Verify the configuration was applied
	fmt.Println("  Verifying configuration...")
	getCurrentResp, err := client.GetCurrentConfig(ctx, &emptypb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to get current config: %w", err)
	}
	
	if !getCurrentResp.Success {
		return fmt.Errorf("get current config failed: %s", getCurrentResp.ErrorMessage)
	}
	
	fmt.Printf("  Current config: %s\n", getCurrentResp.ConfigJson)
	
	return nil
}

func testGetCallingProtocols(grpcAddress string) error {
	// Connect to the module directly
	conn, err := grpc.NewClient(
		grpcAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to matrix module: %w", err)
	}
	defer conn.Close()

	client := modulepb.NewModuleServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// List all entities
	listResp, err := client.ListEntities(ctx, &entitiespb.ListEntitiesRequest{
		EntityTypeFilter: "protocol",
	})
	if err != nil {
		return fmt.Errorf("failed to list entities: %w", err)
	}
	
	if !listResp.Success {
		return fmt.Errorf("ListEntities failed: %s", listResp.ErrorMessage)
	}
	
	fmt.Printf("    Found %d protocol(s):\n", len(listResp.Protocols))
	for i, protocol := range listResp.Protocols {
		fmt.Printf("      %d. %s (%s)\n", i+1, protocol.Name, protocol.Id)
		fmt.Printf("         Type: %s\n", protocol.Type)
		fmt.Printf("         State: %s\n", protocol.Status.State.String())
		fmt.Printf("         Audio capabilities: %v\n", protocol.RequiresAudio.SupportedProtocols)
		fmt.Printf("         Video capabilities: %v\n", protocol.RequiresVideo.SupportedProtocols)
		if len(protocol.Config) > 0 {
			fmt.Printf("         Config:\n")
			for key, value := range protocol.Config {
				fmt.Printf("           %s: %s\n", key, value)
			}
		}
		fmt.Println()
	}
	
	return nil
}

func testEntityDiscovery(client orchestratorpb.OrchestratorServiceClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Test ListEntities
	fmt.Println("  Testing ListEntities...")
	entitiesResp, err := client.ListEntities(ctx, &entitiespb.ListEntitiesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list entities: %w", err)
	}
	
	if !entitiesResp.Success {
		return fmt.Errorf("ListEntities failed: %s", entitiesResp.ErrorMessage)
	}
	
	fmt.Printf("  Found entities:\n")
	fmt.Printf("    - Media Sources: %d\n", len(entitiesResp.MediaSources))
	fmt.Printf("    - Media Sinks: %d\n", len(entitiesResp.MediaSinks))
	fmt.Printf("    - Protocols: %d\n", len(entitiesResp.Protocols))
	fmt.Printf("    - Converters: %d\n", len(entitiesResp.Converters))
	
	// Expect at least 2 entities from dummy module (1 source, 1 sink)
	totalEntities := len(entitiesResp.MediaSources) + len(entitiesResp.MediaSinks) + 
					len(entitiesResp.Protocols) + len(entitiesResp.Converters)
	if totalEntities < 2 {
		return fmt.Errorf("expected at least 2 entities, but found %d", totalEntities)
	}
	
	// Test GetMediaGraph
	fmt.Println("  Testing GetMediaGraph...")
	graphResp, err := client.GetMediaGraph(ctx, &pipelinepb.GetMediaGraphRequest{
		IncludePipelines: true,
		IncludeInactiveEntities: true,
	})
	if err != nil {
		return fmt.Errorf("failed to get media graph: %w", err)
	}
	
	if !graphResp.Success {
		return fmt.Errorf("GetMediaGraph failed: %s", graphResp.ErrorMessage)
	}
	
	fmt.Printf("  Media graph contains:\n")
	fmt.Printf("    - Media Sources: %d\n", len(graphResp.Graph.MediaSources))
	fmt.Printf("    - Media Sinks: %d\n", len(graphResp.Graph.MediaSinks))
	fmt.Printf("    - Protocols: %d\n", len(graphResp.Graph.Protocols))
	fmt.Printf("    - Converters: %d\n", len(graphResp.Graph.Converters))
	fmt.Printf("    - Pipelines: %d\n", len(graphResp.Graph.Pipelines))
	
	fmt.Println("  ✅ Entity discovery test passed")
	return nil
}

func testPipelineManagement(client orchestratorpb.OrchestratorServiceClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// First, get entities to use in pipeline
	entitiesResp, err := client.ListEntities(ctx, &entitiespb.ListEntitiesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list entities: %w", err)
	}
	
	if len(entitiesResp.MediaSources) == 0 || len(entitiesResp.MediaSinks) == 0 {
		return fmt.Errorf("need at least 1 media source and 1 media sink for pipeline test")
	}
	
	sourceEntity := entitiesResp.MediaSources[0]
	sinkEntity := entitiesResp.MediaSinks[0]
	
	fmt.Printf("  Using source: %s (%s)\n", sourceEntity.Name, sourceEntity.Id)
	fmt.Printf("  Using sink: %s (%s)\n", sinkEntity.Name, sinkEntity.Id)
	
	// Test CalculatePath
	fmt.Println("  Testing CalculatePath...")
	pathResp, err := client.CalculatePath(ctx, &pipelinepb.CalculatePathRequest{
		PathRequest: &pipelinepb.PathRequest{
			SourceEntityId: sourceEntity.Id,
			TargetEntityId: sinkEntity.Id,
			MediaType:      entitiespb.MediaType_MEDIA_TYPE_VIDEO,
			DesiredQuality: &entitiespb.QualityProfile{
				Resolution: &entitiespb.Resolution{Width: 1920, Height: 1080},
				Bitrate:    2000000,
				Framerate:  30,
				VideoCodec: "test_pattern",
				AudioCodec: "",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to calculate path: %w", err)
	}
	
	if !pathResp.PathResponse.Success {
		return fmt.Errorf("CalculatePath failed: %s", pathResp.PathResponse.ErrorMessage)
	}
	
	fmt.Printf("  Path calculation succeeded: %v\n", pathResp.PathResponse.EntityPath)
	
	// Test CreatePipeline
	fmt.Println("  Testing CreatePipeline...")
	createResp, err := client.CreatePipeline(ctx, &pipelinepb.CreatePipelineRequest{
		Name:        "Test Pipeline",
		Description: "Automated test pipeline connecting dummy source to sink",
		EntityRefs: []*pipelinepb.EntityReference{
			{
				EntityId:      sourceEntity.Id,
				SessionId:     "test_session_1",
				SessionConfig: map[string]string{"test": "true"},
			},
			{
				EntityId:      sinkEntity.Id,
				SessionId:     "test_session_2",
				SessionConfig: map[string]string{"test": "true"},
			},
		},
		Connections: pathResp.PathResponse.Connections,
	})
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	
	if !createResp.Success {
		return fmt.Errorf("CreatePipeline failed: %s", createResp.ErrorMessage)
	}
	
	pipelineID := createResp.Pipeline.Id
	fmt.Printf("  Created pipeline: %s\n", pipelineID)
	
	// Test ListPipelines
	fmt.Println("  Testing ListPipelines...")
	listResp, err := client.ListPipelines(ctx, &pipelinepb.ListPipelinesRequest{})
	if err != nil {
		return fmt.Errorf("failed to list pipelines: %w", err)
	}
	
	if !listResp.Success {
		return fmt.Errorf("ListPipelines failed: %s", listResp.ErrorMessage)
	}
	
	fmt.Printf("  Found %d pipeline(s)\n", len(listResp.Pipelines))
	
	// Test StartPipeline
	fmt.Println("  Testing StartPipeline...")
	startResp, err := client.StartPipeline(ctx, &pipelinepb.StartPipelineRequest{
		PipelineId: pipelineID,
		RuntimeConfig: map[string]string{
			"test_mode": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}
	
	if !startResp.Success {
		return fmt.Errorf("StartPipeline failed: %s", startResp.ErrorMessage)
	}
	
	fmt.Printf("  Pipeline started successfully, state: %s\n", startResp.Status.State.String())
	
	// Let the pipeline run for a few seconds
	fmt.Println("  Letting pipeline run for 5 seconds...")
	time.Sleep(5 * time.Second)
	
	// Test GetPipelineStatus
	fmt.Println("  Testing GetPipelineStatus...")
	statusResp, err := client.GetPipelineStatus(ctx, &pipelinepb.GetPipelineStatusRequest{
		PipelineId: pipelineID,
	})
	if err != nil {
		return fmt.Errorf("failed to get pipeline status: %w", err)
	}
	
	if !statusResp.Success {
		return fmt.Errorf("GetPipelineStatus failed: %s", statusResp.ErrorMessage)
	}
	
	fmt.Printf("  Pipeline status: %s\n", statusResp.Status.State.String())
	if statusResp.Status.ErrorMessage != "" {
		fmt.Printf("  Pipeline error: %s\n", statusResp.Status.ErrorMessage)
	}
	
	// Test StopPipeline
	fmt.Println("  Testing StopPipeline...")
	stopResp, err := client.StopPipeline(ctx, &pipelinepb.StopPipelineRequest{
		PipelineId: pipelineID,
		Force:      false,
	})
	if err != nil {
		return fmt.Errorf("failed to stop pipeline: %w", err)
	}
	
	if !stopResp.Success {
		return fmt.Errorf("StopPipeline failed: %s", stopResp.ErrorMessage)
	}
	
	fmt.Printf("  Pipeline stopped, state: %s\n", stopResp.Status.State.String())
	
	// Test DeletePipeline
	fmt.Println("  Testing DeletePipeline...")
	deleteResp, err := client.DeletePipeline(ctx, &pipelinepb.DeletePipelineRequest{
		PipelineId: pipelineID,
		Force:      false,
	})
	if err != nil {
		return fmt.Errorf("failed to delete pipeline: %w", err)
	}
	
	if !deleteResp.Success {
		return fmt.Errorf("DeletePipeline failed: %s", deleteResp.ErrorMessage)
	}
	
	fmt.Println("  Pipeline deleted successfully")
	fmt.Println("  ✅ Pipeline management test passed")
	return nil
}
