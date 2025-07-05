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
)

// ModuleManager handles the lifecycle of modules
type ModuleManager struct {
	registry    *ModuleRegistry
	portManager *PortManager
	processes   map[string]*exec.Cmd
}

// PortManager manages gRPC port allocation
type PortManager struct {
	usedPorts map[int]bool
	nextPort  int
}

// NewPortManager creates a new port manager starting from the given port
func NewPortManager(startPort int) *PortManager {
	return &PortManager{
		usedPorts: make(map[int]bool),
		nextPort:  startPort,
	}
}

// AllocatePort allocates an available port
func (pm *PortManager) AllocatePort() (int, error) {
	for i := range 1000 { // Try up to 1000 ports
		port := pm.nextPort + i
		if !pm.usedPorts[port] && pm.isPortAvailable(port) {
			pm.usedPorts[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports found")
}

// ReleasePort releases a previously allocated port
func (pm *PortManager) ReleasePort(port int) {
	delete(pm.usedPorts, port)
}

// isPortAvailable checks if a port is available for binding
func (pm *PortManager) isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// NewModuleManager creates a new module manager
func NewModuleManager(registry *ModuleRegistry) *ModuleManager {
	return &ModuleManager{
		registry:    registry,
		portManager: NewPortManager(50051), // Start from port 50051
		processes:   make(map[string]*exec.Cmd),
	}
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
	if regModule.Status == ModuleStatusRunning {
		return fmt.Errorf("module %s is already running", moduleID)
	}

	// Update status to starting
	if err := mm.registry.UpdateModuleStatus(moduleID, ModuleStatusStarting, ""); err != nil {
		return fmt.Errorf("failed to update module status: %w", err)
	}

	// Allocate a port for the module
	port, err := mm.portManager.AllocatePort()
	if err != nil {
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusError, err.Error())
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
		mm.portManager.ReleasePort(port)
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusError, "empty command")
		return fmt.Errorf("empty command for module %s", moduleID)
	}

	// Create the command
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = regModule.Module.Path

	// Set environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("GRPC_PORT=%d", port))

	// Redirect output for logging
	cmd.Stdout = &moduleLogger{moduleID: moduleID, prefix: "OUT"}
	cmd.Stderr = &moduleLogger{moduleID: moduleID, prefix: "ERR"}

	// Start the process
	if err := cmd.Start(); err != nil {
		mm.portManager.ReleasePort(port)
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusError, err.Error())
		return fmt.Errorf("failed to start module process: %w", err)
	}

	// Store the process
	mm.processes[moduleID] = cmd

	// Update registry with process information
	if err := mm.registry.UpdateModuleProcess(moduleID, cmd.Process.Pid, port); err != nil {
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusError, err.Error())
		return fmt.Errorf("failed to update process info: %w", err)
	}

	// Start monitoring the process
	go mm.monitorProcess(moduleID, cmd, port)

	// Wait a bit to see if the process starts successfully
	time.Sleep(100 * time.Millisecond)

	// Check if process is still running
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		exitCode := cmd.ProcessState.ExitCode()
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusError, fmt.Sprintf("process exited with code %d", exitCode))
		return fmt.Errorf("module %s exited immediately with code %d", moduleID, exitCode)
	}

	// Update status to running
	if err := mm.registry.UpdateModuleStatus(moduleID, ModuleStatusRunning, ""); err != nil {
		return fmt.Errorf("failed to update module status: %w", err)
	}

	slog.Info("Module started successfully", "module_id", moduleID, "port", port, "pid", cmd.Process.Pid)
	return nil
}

// StopModule stops a running module
func (mm *ModuleManager) StopModule(moduleID string) error {
	regModule, exists := mm.registry.GetModule(moduleID)
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

	// Release the port
	mm.portManager.ReleasePort(regModule.GRPCPort)

	// Update status
	mm.registry.UpdateModuleStatus(moduleID, ModuleStatusStopped, "")

	// Remove from processes map
	delete(mm.processes, moduleID)

	slog.Info("Module stopped", "module_id", moduleID)
	return nil
}

// StopAllModules stops all running modules
func (mm *ModuleManager) StopAllModules() error {
	modules := mm.registry.GetModulesByStatus(ModuleStatusRunning)

	for _, module := range modules {
		if err := mm.StopModule(module.Module.ID); err != nil {
			slog.Warn("Failed to stop module", "module_id", module.Module.ID, "error", err)
		}
	}

	return nil
}

// monitorProcess monitors a module process and updates status when it exits
func (mm *ModuleManager) monitorProcess(moduleID string, cmd *exec.Cmd, port int) {
	// Wait for the process to exit
	err := cmd.Wait()

	// Release the port
	mm.portManager.ReleasePort(port)

	// Update status based on exit condition
	if err != nil {
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusError, err.Error())
		slog.Error("Module exited with error", "module_id", moduleID, "error", err)
	} else {
		mm.registry.UpdateModuleStatus(moduleID, ModuleStatusStopped, "")
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
	return len(mm.registry.GetModulesByStatus(ModuleStatusRunning))
}

// GetModuleStatus returns the status of a specific module
func (mm *ModuleManager) GetModuleStatus(moduleID string) (ModuleStatus, error) {
	regModule, exists := mm.registry.GetModule(moduleID)
	if !exists {
		return ModuleStatusUnknown, fmt.Errorf("module %s not found", moduleID)
	}
	return regModule.Status, nil
}
