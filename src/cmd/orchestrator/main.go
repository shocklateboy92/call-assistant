package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
	"github.com/shocklateboy92/call-assistant/src/internal"
)

func main() {
	// Parse command line flags
	var (
		modulesDir = flag.String("modules-dir", "src/modules", "Directory containing modules")
		devMode    = flag.Bool("dev", false, "Run in development mode (use dev_command)")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	// Print startup banner
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  Call Assistant Orchestrator")
	fmt.Println("  Module Discovery and Management System")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("  Modules Directory: %s\n", *modulesDir)
	fmt.Printf("  Development Mode: %v\n", *devMode)
	fmt.Printf("  Verbose Logging: %v\n", *verbose)
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start signal handler in a goroutine
	go func() {
		<-sigChan
		fmt.Println("\nShutdown signal received. Gracefully stopping modules...")
		cancel()
	}()

	// Run the orchestrator (this will block until ctx is cancelled)
	if err := runOrchestrator(ctx, *modulesDir, *devMode, *verbose); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Orchestrator shutdown complete.")
}

func runOrchestrator(ctx context.Context, modulesDir string, devMode bool, verbose bool) error {
	// Convert to absolute path
	absModulesDir, err := filepath.Abs(modulesDir)
	if err != nil {
		return fmt.Errorf("failed to resolve modules directory path: %w", err)
	}

	if verbose {
		slog.Info("Resolved modules directory", "path", absModulesDir)
	}

	// Phase 1: Module Discovery
	fmt.Println("\n🔍 Phase 1: Module Discovery")
	fmt.Println("───────────────────────────────────────────────────────────────")

	discovery := internal.NewModuleDiscovery(absModulesDir)
	modules, err := discovery.DiscoverModules()
	if err != nil {
		return fmt.Errorf("module discovery failed: %w", err)
	}

	slog.Info("Module discovery completed", "count", len(modules))
	for _, module := range modules {
		slog.Info(
			"Discovered module",
			"name",
			module.Manifest.Name,
			"version",
			module.Manifest.Version,
			"description",
			module.Manifest.Description,
		)
		if verbose {
			slog.Debug(
				"Module details",
				"path",
				module.Path,
				"command",
				module.Manifest.Command,
				"dev_command",
				module.Manifest.DevCommand,
				"dependencies",
				module.Manifest.Dependencies,
			)
		}
	}

	// Phase 2: Module Registration
	fmt.Println("\n📋 Phase 2: Module Registration")
	fmt.Println("───────────────────────────────────────────────────────────────")

	registry := internal.NewModuleRegistry()
	for _, module := range modules {
		if err := registry.RegisterModule(module); err != nil {
			return fmt.Errorf("failed to register module %s: %w", module.ID, err)
		}
		slog.Info("Registered module", "id", module.ID)
	}

	// Phase 3: Dependency Resolution
	fmt.Println("\n🔗 Phase 3: Dependency Resolution")
	fmt.Println("───────────────────────────────────────────────────────────────")

	if err := registry.CalculateStartOrder(); err != nil {
		return fmt.Errorf("failed to calculate start order: %w", err)
	}

	orderedModules := registry.GetModulesInStartOrder()
	slog.Info("Calculated startup order")
	for i, module := range orderedModules {
		slog.Info("Module startup order", "position", i+1, "module_id", module.Module.ID)
	}

	// Phase 4: Module Lifecycle Management
	fmt.Println("\n🚀 Phase 4: Module Startup")
	fmt.Println("───────────────────────────────────────────────────────────────")

	const orchestratorPort = 9090
	manager := internal.NewModuleManager(registry, orchestratorPort)

	// Start all modules
	if err := manager.StartAllModules(ctx, devMode); err != nil {
		return fmt.Errorf("failed to start modules: %w", err)
	}

	slog.Info("All modules started successfully")

	// Phase 5: Start gRPC Server
	fmt.Println("\n🌐 Phase 5: Starting gRPC Server")
	fmt.Println("───────────────────────────────────────────────────────────────")

	orchestratorService := internal.NewOrchestratorService(registry, manager)

	// Start gRPC server in in main thread, it will listen in goroutine
	if err := orchestratorService.StartGRPCServer(ctx, orchestratorPort); err != nil {
		slog.Error("gRPC server error", "error", err)
	}

	// Phase 6: Status Monitoring
	fmt.Println("\n📊 Phase 6: Status Monitoring")
	fmt.Println("───────────────────────────────────────────────────────────────")

	// Start status monitoring routine
	go monitorModuleStatus(ctx, registry, verbose)

	// Wait for context cancellation
	<-ctx.Done()

	// Cleanup
	fmt.Println("\n🛑 Shutting down modules...")
	fmt.Println("───────────────────────────────────────────────────────────────")

	if err := manager.StopAllModules(); err != nil {
		slog.Warn("Error during module shutdown", "error", err)
	}

	return nil
}

func monitorModuleStatus(ctx context.Context, registry *internal.ModuleRegistry, verbose bool) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if verbose {
				printModuleStatus(registry)
			}
		}
	}
}

func printModuleStatus(registry *internal.ModuleRegistry) {
	allModules := registry.GetAllModules()

	slog.Info("Module Status Report")

	statusCounts := make(map[commonpb.ModuleState]int)

	for _, module := range allModules {
		statusCounts[module.Status]++

		logAttrs := []any{
			"module_id", module.Module.ID,
			"status", module.Status,
		}

		if module.GRPCPort > 0 {
			logAttrs = append(logAttrs, "port", module.GRPCPort)
		}

		if module.ProcessID > 0 {
			logAttrs = append(logAttrs, "pid", module.ProcessID)
		}

		if module.ErrorMsg != "" {
			logAttrs = append(logAttrs, "error", module.ErrorMsg)
		}

		switch module.Status {
		case commonpb.ModuleState_MODULE_STATE_ERROR:
			slog.Error("Module status", logAttrs...)
		case commonpb.ModuleState_MODULE_STATE_READY:
			slog.Info("Module status", logAttrs...)
		default:
			slog.Debug("Module status", logAttrs...)
		}
	}

	logAttrs := []any{"total", len(allModules)}
	for status, count := range statusCounts {
		if count > 0 {
			logAttrs = append(logAttrs, string(rune(status)), count)
		}
	}
	slog.Info("Module status summary", logAttrs...)
}
