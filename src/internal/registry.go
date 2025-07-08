package internal

import (
	"fmt"
	"maps"
	"sort"
	"sync"

	commonpb "github.com/shocklateboy92/call-assistant/src/api/proto/common"
)

// RegisteredModule represents a module that has been registered with the orchestrator
type RegisteredModule struct {
	Module     DiscoveredModule
	Status     commonpb.ModuleState
	GRPCPort   int
	ProcessID  int
	ErrorMsg   string
	StartOrder int
}

// ModuleRegistry manages the registry of discovered and running modules
type ModuleRegistry struct {
	modules map[string]*RegisteredModule
	mutex   sync.RWMutex
}

// NewModuleRegistry creates a new module registry
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]*RegisteredModule),
	}
}

// RegisterModule adds a discovered module to the registry
func (mr *ModuleRegistry) RegisterModule(module DiscoveredModule) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	// Check if module already exists
	if _, exists := mr.modules[module.ID]; exists {
		return fmt.Errorf("module with ID %s already registered", module.ID)
	}

	// Register the module
	mr.modules[module.ID] = &RegisteredModule{
		Module:     module,
		Status:     commonpb.ModuleState_MODULE_STATE_UNSPECIFIED,
		GRPCPort:   0, // Will be assigned later
		ProcessID:  0, // Will be assigned when started
		ErrorMsg:   "",
		StartOrder: 0, // Will be calculated based on dependencies
	}

	return nil
}

// GetModule retrieves a module by ID
func (mr *ModuleRegistry) GetModule(id string) (*RegisteredModule, bool) {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	module, exists := mr.modules[id]
	return module, exists
}

// GetAllModules returns all registered modules
func (mr *ModuleRegistry) GetAllModules() map[string]*RegisteredModule {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	// Create a copy to avoid concurrent access issues
	result := make(map[string]*RegisteredModule)
	maps.Copy(result, mr.modules)

	return result
}

// GetModulesByStatus returns modules filtered by status
func (mr *ModuleRegistry) GetModulesByStatus(status commonpb.ModuleState) []*RegisteredModule {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	var result []*RegisteredModule
	for _, module := range mr.modules {
		if module.Status == status {
			result = append(result, module)
		}
	}

	return result
}

// UpdateModuleStatus updates the status of a module
func (mr *ModuleRegistry) UpdateModuleStatus(
	id string,
	status commonpb.ModuleState,
	errorMsg string,
) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	module, exists := mr.modules[id]
	if !exists {
		return fmt.Errorf("module with ID %s not found", id)
	}

	module.Status = status
	module.ErrorMsg = errorMsg

	return nil
}

// UpdateModuleProcess updates the process information for a module
func (mr *ModuleRegistry) UpdateModuleProcess(id string, processID int, grpcPort int) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	module, exists := mr.modules[id]
	if !exists {
		return fmt.Errorf("module with ID %s not found", id)
	}

	module.ProcessID = processID
	module.GRPCPort = grpcPort

	return nil
}

// CalculateStartOrder calculates the startup order based on dependencies
func (mr *ModuleRegistry) CalculateStartOrder() error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	// Build dependency graph
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	// Initialize graph and in-degree count
	for id := range mr.modules {
		graph[id] = []string{}
		inDegree[id] = 0
	}

	// Build the dependency graph
	for id, module := range mr.modules {
		for _, dep := range module.Module.Manifest.Dependencies {
			// Find the dependency module ID
			depID := mr.findModuleByName(dep)
			if depID == "" {
				return fmt.Errorf("dependency %s not found for module %s", dep, id)
			}

			graph[depID] = append(graph[depID], id)
			inDegree[id]++
		}
	}

	// Topological sort using Kahn's algorithm
	queue := []string{}
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	order := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		mr.modules[current].StartOrder = order
		order++

		// Process dependencies
		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Check for circular dependencies
	if order != len(mr.modules) {
		return fmt.Errorf("circular dependency detected in modules")
	}

	return nil
}

// GetModulesInStartOrder returns modules sorted by start order
func (mr *ModuleRegistry) GetModulesInStartOrder() []*RegisteredModule {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	var modules []*RegisteredModule
	for _, module := range mr.modules {
		modules = append(modules, module)
	}

	// Sort by start order
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].StartOrder < modules[j].StartOrder
	})

	return modules
}

// findModuleByName finds a module ID by its name
func (mr *ModuleRegistry) findModuleByName(name string) string {
	for id, regModule := range mr.modules {
		if regModule.Module.Manifest.Name == name {
			return id
		}
	}
	return ""
}

// GetModuleCount returns the total number of registered modules
func (mr *ModuleRegistry) GetModuleCount() int {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	return len(mr.modules)
}

// Clear removes all modules from the registry
func (mr *ModuleRegistry) Clear() {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	mr.modules = make(map[string]*RegisteredModule)
}
