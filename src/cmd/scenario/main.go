package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	fmt.Println("🚀 Call Assistant Scenario")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// Step 1: Set up Matrix test users
	fmt.Println("Step 1: Setting up Matrix test users...")
	fmt.Println("───────────────────────────────────────────────────────────────")
	runMatrixSetup()

	// Step 2: Test orchestrator service
	fmt.Println("\nStep 2: Testing orchestrator service...")
	fmt.Println("───────────────────────────────────────────────────────────────")
	err := TestOrchestratorService()
	if err != nil {
		slog.Error("Orchestrator service test failed", "error", err)
		os.Exit(1)
	}

	fmt.Println("\n🎉 Scenario completed successfully!")
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

func runMatrixSetup() {
	// Create Matrix client
	client := NewMatrixClient(SynapseURL)

	// Check Synapse health
	if err := client.CheckHealth(); err != nil {
		slog.Error("Synapse server is not responding", "url", SynapseURL, "error", err)
		os.Exit(1)
	}

	// Create test users
	fmt.Println("Creating test users...")

	for _, username := range Users {
		if err := client.CreateUser(username, TestPassword); err != nil {
			slog.Error("Failed to create user", "username", username, "error", err)
			os.Exit(1)
		}
	}

	fmt.Println("✅ Matrix setup complete!")
}
