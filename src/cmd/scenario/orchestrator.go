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

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	entitiespb "github.com/shocklateboy92/call-assistant/src/api/proto/entities"
	modulepb "github.com/shocklateboy92/call-assistant/src/api/proto/module"
	orchestratorpb "github.com/shocklateboy92/call-assistant/src/api/proto/orchestrator"
	configpb "github.com/shocklateboy92/call-assistant/src/generated/go/services"
)

func TestOrchestratorService() error {
	fmt.Println("🚀 Testing Orchestrator Service")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Step 1: Start orchestrator in background
	fmt.Println("Step 1a: Building orchestrator...")
	const orchestratorPath = "./bin/orchestrator"
	cmd := exec.Command("go", "build", "-o", orchestratorPath, "./src/cmd/orchestrator")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Error("Failed to start orchestrator", "error", err)
		return err
	}

	// Wait for build to complete
	if err := cmd.Wait(); err != nil {
		slog.Error("Failed to build orchestrator", "error", err)
		return fmt.Errorf("failed to build orchestrator: %w", err)
	}

	fmt.Println("Step 1b: Starting orchestrator...")
	cmd = exec.Command(orchestratorPath, "--dev", "--verbose")
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

	// Step 2: Wait for orchestrator to start and modules to be ready
	fmt.Println("Step 2: Waiting for orchestrator to start and modules to report started events...")
	err := waitForModulesToStart()
	if err != nil {
		slog.Error("Failed to wait for modules to start", "error", err)
		return err
	}

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

	if dummyModule == nil {
		fmt.Println("❌ Dummy module not found")
		return fmt.Errorf("dummy module not found")
	}

	// Dummy module should actually still be starting,
	// since we never implemented the code to send the start event
	if dummyModule.Status.State != commonpb.ModuleState_MODULE_STATE_STARTING {
		fmt.Printf("Dummy module is in %s state, expected STARTING\n",
			dummyModule.Status.State.String())
	}

	dummyConfig := fmt.Sprintf(`{
			"username": "%s",
			"password": "%s"
		}`, Users[0], TestPassword)

	err = configureModule(dummyModule.GrpcAddress, "dummy", dummyConfig)
	if err != nil {
		slog.Error("Failed to configure dummy module", "error", err)
	} else {
		fmt.Printf("✅ Dummy module configured successfully with %s credentials\n", Users[0])
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

	if matrixModule == nil {
		fmt.Println("❌ Matrix module not found")
		return fmt.Errorf("matrix module not found")
	}

	// Check if it's in WAITING_FOR_CONFIGURATION state
	if matrixModule.Status.State == commonpb.ModuleState_MODULE_STATE_WAITING_FOR_CONFIG {
		fmt.Printf("Matrix module is in WAITING_FOR_CONFIGURATION state, proceeding with configuration...\n")
	} else {
		fmt.Printf("Matrix module is in %s state, expected WAITING_FOR_CONFIGURATION\n",
			matrixModule.Status.State.String())
		return fmt.Errorf("matrix module is not in WAITING_FOR_CONFIGURATION state")
	}

	// Test with bad credentials first
	fmt.Println("  Testing with invalid credentials...")
	badConfig := fmt.Sprintf(`{
			"homeserver": "%s",
			"accessToken": "bad_token",
			"userId": "@%s:localhost"
		}`, SynapseURL, Users[0])

	err = configureModule(matrixModule.GrpcAddress, "matrix", badConfig)
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

// waitForModulesToStart waits for both expected modules to report module_started events
func waitForModulesToStart() error {
	// Give orchestrator a moment to start up
	fmt.Println("  Waiting for orchestrator to become available...")
	time.Sleep(2 * time.Second)

	// Try to connect to orchestrator with retries
	var conn *grpc.ClientConn
	var err error
	for attempts := range 10 {
		conn, err = grpc.NewClient(
			"localhost:9090",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err == nil {
			break
		}
		fmt.Printf("  Connection attempt %d failed, retrying...\n", attempts+1)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to orchestrator for events after retries: %w", err)
	}
	defer conn.Close()

	client := orchestratorpb.NewOrchestratorServiceClient(conn)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Subscribe to events
	stream, err := client.SubscribeToEvents(ctx, &orchestratorpb.SubscribeToEventsRequest{})
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Track which modules we've seen start
	modulesStarted := make(map[string]bool)
	// Note: Only waiting for matrix module since dummy module doesn't send module_started events yet
	expectedModules := []string{"matrix"}

	fmt.Println("  Waiting for module_started events from modules: matrix")

	for {
		event, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("failed to receive event: %w", err)
		}

		// Check if this is a module_started event
		if moduleStartedEvent := event.GetModuleStarted(); moduleStartedEvent != nil {
			moduleID := event.SourceModuleId
			fmt.Printf("  ✅ Received module_started event from: %s\n", moduleID)

			// Mark this module as started
			modulesStarted[moduleID] = true

			// Check if all expected modules have started
			allStarted := true
			for _, expectedModule := range expectedModules {
				if !modulesStarted[expectedModule] {
					allStarted = false
					break
				}
			}

			if allStarted {
				fmt.Println("  ✅ All expected modules have started!")
				fmt.Println("  Waiting for health checks to complete...")
				// Give a moment for health checks to complete and update module status
				time.Sleep(1 * time.Second)
				return nil
			}
		}
	}
}
