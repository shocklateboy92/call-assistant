package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
)

// ModuleManager handles the lifecycle of modules
type ModuleManager struct {
	registry  *ModuleRegistry
	processes map[string]*exec.Cmd
}

// NewModuleManager creates a new module manager
func NewModuleManager(registry *ModuleRegistry) *ModuleManager {
	return &ModuleManager{
		registry:  registry,
		processes: make(map[string]*exec.Cmd),
	}
}

// allocatePort allocates an available port by letting the OS choose
func allocatePort() (int, error) {
	// Listen on port 0 to let the OS choose an available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to allocate port: %w", err)
	}
	defer listener.Close()

	// Extract the port number from the address
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// StartAllModules starts all registered modules in dependency order
func (mm *ModuleManager) StartAllModules(ctx context.Context, isDev bool) error {
	modules := mm.registry.GetModulesInStartOrder()

	slog.Info("Starting modules in dependency order", "count", len(modules))

	for _, module := range modules {
		if err := mm.StartModule(ctx, module.Module.ID, isDev); err != nil {
			return fmt.Errorf("failed to start module %s: %w", module.Module.ID, err)
		}

		// Add a small delay between module starts
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	return nil
}

// StartModule starts a single module
func (mm *ModuleManager) StartModule(ctx context.Context, moduleID string, isDev bool) error {
	regModule, exists := mm.registry.GetModule(moduleID)
	if !exists {
		return fmt.Errorf("module %s not found in registry", moduleID)
	}

	// Check if module is already running
	if regModule.Status == commonpb.ModuleState_MODULE_STATE_READY {
		return fmt.Errorf("module %s is already running", moduleID)
	}

	// Update status to starting
	if err := mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_STARTING, ""); err != nil {
		return fmt.Errorf("failed to update module status: %w", err)
	}

	// Allocate a port for the module
	port, err := allocatePort()
	if err != nil {
		mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, err.Error())
		return fmt.Errorf("failed to allocate port: %w", err)
	}

	// Determine which command to use
	command := regModule.Module.Manifest.Command
	if isDev && regModule.Module.Manifest.DevCommand != "" {
		command = regModule.Module.Manifest.DevCommand
	}

	// Parse the command
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, "empty command")
		return fmt.Errorf("empty command for module %s", moduleID)
	}

	// Create the command
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = regModule.Module.Path

	// Set environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("GRPC_PORT=%d", port))
	cmd.Env = append(cmd.Env, fmt.Sprintf("EVENT_SERVICE_ADDRESS=localhost:%d", port))

	// Redirect output for logging
	cmd.Stdout = &moduleLogger{moduleID: moduleID, prefix: "OUT"}
	cmd.Stderr = &moduleLogger{moduleID: moduleID, prefix: "ERR"}

	// Start the process
	if err := cmd.Start(); err != nil {
		mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, err.Error())
		return fmt.Errorf("failed to start module process: %w", err)
	}

	// Store the process
	mm.processes[moduleID] = cmd

	// Update registry with process information
	if err := mm.registry.UpdateModuleProcess(moduleID, cmd.Process.Pid, port); err != nil {
		mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, err.Error())
		return fmt.Errorf("failed to update process info: %w", err)
	}

	// Start monitoring the process
	go mm.monitorProcess(moduleID, cmd, port)

	// We don't explicitly wait for the process to be ready here.
	// When the module starts, it will send an event to the event service

	slog.Info(
		"Module started successfully",
		"module_id",
		moduleID,
		"port",
		port,
		"pid",
		cmd.Process.Pid,
	)
	return nil
}

// StopModule stops a running module
func (mm *ModuleManager) StopModule(moduleID string) error {
	_, exists := mm.registry.GetModule(moduleID)
	if !exists {
		return fmt.Errorf("module %s not found in registry", moduleID)
	}

	// Get the process
	cmd, exists := mm.processes[moduleID]
	if !exists {
		return fmt.Errorf("no process found for module %s", moduleID)
	}

	// Send termination signal
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for graceful shutdown or force kill after timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(10 * time.Second):
		// Force kill if not stopped within timeout
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		<-done // Wait for the process to actually exit
	case <-done:
		// Process exited gracefully
	}

	// Update status
	mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_STOPPING, "")

	// Remove from processes map
	delete(mm.processes, moduleID)

	slog.Info("Module stopped", "module_id", moduleID)
	return nil
}

// StopAllModules stops all running modules
func (mm *ModuleManager) StopAllModules() error {
	modules := mm.registry.GetModulesByStatus(commonpb.ModuleState_MODULE_STATE_READY)

	for _, module := range modules {
		if err := mm.StopModule(module.Module.ID); err != nil {
			slog.Warn("Failed to stop module", "module_id", module.Module.ID, "error", err)
		}
	}

	return nil
}

// monitorProcess monitors a module process and updates status when it exits
func (mm *ModuleManager) monitorProcess(moduleID string, cmd *exec.Cmd, _ int) {
	// Wait for the process to exit
	err := cmd.Wait()

	// Update status based on exit condition
	if err != nil {
		mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_ERROR, err.Error())
		slog.Error("Module exited with error", "module_id", moduleID, "error", err)
	} else {
		mm.registry.UpdateModuleStatus(moduleID, commonpb.ModuleState_MODULE_STATE_STOPPING, "")
		slog.Info("Module exited normally", "module_id", moduleID)
	}

	// Remove from processes map
	delete(mm.processes, moduleID)
}

// moduleLogger implements io.Writer for module logging
type moduleLogger struct {
	moduleID string
	prefix   string
}

func (ml *moduleLogger) Write(p []byte) (n int, err error) {
	message := strings.TrimSpace(string(p))
	if message != "" {
		slog.Info("Module log", "module_id", ml.moduleID, "stream", ml.prefix, "message", message)
	}
	return len(p), nil
}

// GetRunningModules returns the count of running modules
func (mm *ModuleManager) GetRunningModules() int {
	return len(mm.registry.GetModulesByStatus(commonpb.ModuleState_MODULE_STATE_READY))
}

// GetModuleStatus returns the status of a specific module
func (mm *ModuleManager) GetModuleStatus(moduleID string) (commonpb.ModuleState, error) {
	regModule, exists := mm.registry.GetModule(moduleID)
	if !exists {
		return commonpb.ModuleState_MODULE_STATE_UNSPECIFIED, fmt.Errorf("module %s not found", moduleID)
	}
	return regModule.Status, nil
}
