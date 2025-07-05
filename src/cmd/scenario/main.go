package main

import (
	"fmt"
	"log/slog"
	"os"
)

const (
	SynapseURL   = "http://synapse:8008"
	TestPassword = "TestPassword123"
)

func main() {
	fmt.Println("🚀 Setting up Synapse test users...")

	// Create Matrix client
	client := NewMatrixClient(SynapseURL)

	// Check Synapse health
	if err := client.CheckHealth(); err != nil {
		slog.Error("Synapse server is not responding", "url", SynapseURL, "error", err)
		os.Exit(1)
	}

	// Create test users
	fmt.Println("Creating test users...")

	users := []string{"alice", "bob"}

	for _, username := range users {
		if err := client.CreateUser(username, TestPassword); err != nil {
			slog.Error("Failed to create user", "username", username, "error", err)
			os.Exit(1)
		}
	}

	fmt.Println("🎉 Setup complete!")
}
