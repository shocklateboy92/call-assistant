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

	// Step 6: Subscribe to events for a few seconds
	fmt.Println("Step 6: Subscribing to events for 3 seconds...")
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
