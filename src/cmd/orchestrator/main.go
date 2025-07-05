package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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

	// Run the orchestrator
	if err := runOrchestrator(ctx, *modulesDir, *devMode, *verbose); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\nShutdown signal received. Gracefully stopping modules...")
	cancel()

	// Give some time for cleanup
	time.Sleep(2 * time.Second)
	fmt.Println("Orchestrator shutdown complete.")
}

func runOrchestrator(ctx context.Context, modulesDir string, devMode bool, verbose bool) error {
	// Convert to absolute path
	absModulesDir, err := filepath.Abs(modulesDir)
	if err != nil {
		return fmt.Errorf("failed to resolve modules directory path: %w", err)
	}

	if verbose {
		fmt.Printf("Resolved modules directory: %s\n", absModulesDir)
	}

	// Phase 1: Module Discovery
	fmt.Println("\n🔍 Phase 1: Module Discovery")
	fmt.Println("───────────────────────────────────────────────────────────────")

	discovery := internal.NewModuleDiscovery(absModulesDir)
	modules, err := discovery.DiscoverModules()
	if err != nil {
		return fmt.Errorf("module discovery failed: %w", err)
	}

	fmt.Printf("✓ Discovered %d modules:\n", len(modules))
	for _, module := range modules {
		fmt.Printf("  • %s (v%s) - %s\n", module.Manifest.Name, module.Manifest.Version, module.Manifest.Description)
		if verbose {
			fmt.Printf("    Path: %s\n", module.Path)
			fmt.Printf("    Command: %s\n", module.Manifest.Command)
			if module.Manifest.DevCommand != "" {
				fmt.Printf("    Dev Command: %s\n", module.Manifest.DevCommand)
			}
			if len(module.Manifest.Dependencies) > 0 {
				fmt.Printf("    Dependencies: %v\n", module.Manifest.Dependencies)
			}
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
		fmt.Printf("✓ Registered module: %s\n", module.ID)
	}

	// Phase 3: Dependency Resolution
	fmt.Println("\n🔗 Phase 3: Dependency Resolution")
	fmt.Println("───────────────────────────────────────────────────────────────")

	if err := registry.CalculateStartOrder(); err != nil {
		return fmt.Errorf("failed to calculate start order: %w", err)
	}

	orderedModules := registry.GetModulesInStartOrder()
	fmt.Printf("✓ Calculated startup order:\n")
	for i, module := range orderedModules {
		fmt.Printf("  %d. %s\n", i+1, module.Module.ID)
	}

	// Phase 4: Module Lifecycle Management
	fmt.Println("\n🚀 Phase 4: Module Startup")
	fmt.Println("───────────────────────────────────────────────────────────────")

	manager := internal.NewModuleManager(registry)

	// Start all modules
	if err := manager.StartAllModules(ctx, devMode); err != nil {
		return fmt.Errorf("failed to start modules: %w", err)
	}

	fmt.Printf("✓ All modules started successfully\n")

	// Phase 5: Status Monitoring
	fmt.Println("\n📊 Phase 5: Status Monitoring")
	fmt.Println("───────────────────────────────────────────────────────────────")

	// Start status monitoring routine
	go monitorModuleStatus(ctx, registry, verbose)

	// Wait for context cancellation
	<-ctx.Done()

	// Cleanup
	fmt.Println("\n🛑 Shutting down modules...")
	fmt.Println("───────────────────────────────────────────────────────────────")

	if err := manager.StopAllModules(); err != nil {
		fmt.Printf("Warning: Error during module shutdown: %v\n", err)
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
	
	fmt.Printf("\n📊 Module Status Report (%s)\n", time.Now().Format("15:04:05"))
	fmt.Println("───────────────────────────────────────────────────────────────")
	
	statusCounts := make(map[internal.ModuleStatus]int)
	
	for _, module := range allModules {
		statusCounts[module.Status]++
		
		statusIcon := "?"
		switch module.Status {
		case internal.ModuleStatusRunning:
			statusIcon = "✓"
		case internal.ModuleStatusError:
			statusIcon = "✗"
		case internal.ModuleStatusStarting:
			statusIcon = "⏳"
		case internal.ModuleStatusStopped:
			statusIcon = "⏸"
		}
		
		fmt.Printf("  %s %s [%s]", statusIcon, module.Module.ID, module.Status)
		
		if module.GRPCPort > 0 {
			fmt.Printf(" (Port: %d)", module.GRPCPort)
		}
		
		if module.ProcessID > 0 {
			fmt.Printf(" (PID: %d)", module.ProcessID)
		}
		
		if module.ErrorMsg != "" {
			fmt.Printf(" - Error: %s", module.ErrorMsg)
		}
		
		fmt.Println()
	}
	
	fmt.Println("───────────────────────────────────────────────────────────────")
	fmt.Printf("Total: %d modules", len(allModules))
	for status, count := range statusCounts {
		if count > 0 {
			fmt.Printf(" | %s: %d", status, count)
		}
	}
	fmt.Println()
}