package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModuleManifest represents the module.yaml configuration
type ModuleManifest struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Description  string   `yaml:"description"`
	Command      string   `yaml:"command"`
	DevCommand   string   `yaml:"dev_command"`
	Dependencies []string `yaml:"dependencies"`
}

// DiscoveredModule represents a module found during discovery
type DiscoveredModule struct {
	Manifest ModuleManifest
	Path     string
	ID       string
}

// ModuleDiscovery handles scanning and discovering modules
type ModuleDiscovery struct {
	modulesDir string
}

// NewModuleDiscovery creates a new module discovery instance
func NewModuleDiscovery(modulesDir string) *ModuleDiscovery {
	return &ModuleDiscovery{
		modulesDir: modulesDir,
	}
}

// DiscoverModules scans the modules directory and finds all module.yaml files
func (md *ModuleDiscovery) DiscoverModules() ([]DiscoveredModule, error) {
	var modules []DiscoveredModule

	// Check if modules directory exists
	if _, err := os.Stat(md.modulesDir); os.IsNotExist(err) {
		return modules, fmt.Errorf("modules directory does not exist: %s", md.modulesDir)
	}

	// Walk through the modules directory
	err := filepath.Walk(md.modulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Look for module.yaml files
		if info.Name() == "module.yaml" {
			module, err := md.parseModuleManifest(path)
			if err != nil {
				fmt.Printf("Warning: failed to parse module manifest at %s: %v\n", path, err)
				return nil // Continue walking, don't fail the entire discovery
			}

			modules = append(modules, module)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk modules directory: %w", err)
	}

	return modules, nil
}

// parseModuleManifest reads and parses a module.yaml file
func (md *ModuleDiscovery) parseModuleManifest(manifestPath string) (DiscoveredModule, error) {
	var module DiscoveredModule

	// Read the manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return module, fmt.Errorf("failed to read manifest file: %w", err)
	}

	// Parse YAML
	var manifest ModuleManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return module, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate required fields
	if manifest.Name == "" {
		return module, fmt.Errorf("module name is required")
	}
	if manifest.Command == "" {
		return module, fmt.Errorf("module command is required")
	}

	// Get the module directory path
	moduleDir := filepath.Dir(manifestPath)

	// Create the discovered module
	module = DiscoveredModule{
		Manifest: manifest,
		Path:     moduleDir,
		ID:       md.generateModuleID(manifest.Name, moduleDir),
	}

	return module, nil
}

// generateModuleID creates a unique ID for a module
func (md *ModuleDiscovery) generateModuleID(name, path string) string {
	// Use the last directory name as part of the ID for uniqueness
	dirName := filepath.Base(path)
	
	// Clean the name to make it a valid ID
	id := strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	
	// If the directory name is different from the cleaned name, append it
	if dirName != id {
		id = fmt.Sprintf("%s_%s", id, dirName)
	}
	
	return id
}

// GetModulesDirectory returns the modules directory path
func (md *ModuleDiscovery) GetModulesDirectory() string {
	return md.modulesDir
}