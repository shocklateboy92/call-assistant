package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// MatrixClient handles Matrix Synapse API interactions
type MatrixClient struct {
	baseURL string
	client  *http.Client
}

// NewMatrixClient creates a new Matrix client
func NewMatrixClient(baseURL string) *MatrixClient {
	return &MatrixClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// LoginRequest represents the Matrix login request payload
type LoginRequest struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// LoginResponse represents the Matrix login response
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

// RegisterRequest represents the Matrix registration request payload
type RegisterRequest struct {
	Username string       `json:"username"`
	Password string       `json:"password"`
	Auth     RegisterAuth `json:"auth"`
}

// RegisterAuth represents the auth section of registration request
type RegisterAuth struct {
	Type string `json:"type"`
}

// RegisterResponse represents the Matrix registration response
type RegisterResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

// ErrorResponse represents Matrix error response
type ErrorResponse struct {
	ErrCode string `json:"errcode"`
	Error   string `json:"error"`
}

// CheckHealth verifies that the Synapse server is responding
func (mc *MatrixClient) CheckHealth() error {
	slog.Info("Checking Synapse server health...")

	url := fmt.Sprintf("%s/_matrix/client/versions", mc.baseURL)
	resp, err := mc.client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to Synapse server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("synapse server returned status %d", resp.StatusCode)
	}

	slog.Info("Synapse server is running")
	return nil
}

// TryLogin attempts to login with the given credentials to check if user exists
func (mc *MatrixClient) TryLogin(username, password string) bool {
	url := fmt.Sprintf("%s/_matrix/client/r0/login", mc.baseURL)

	loginReq := LoginRequest{
		Type:     "m.login.password",
		User:     username,
		Password: password,
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		slog.Error("Failed to marshal login request", "error", err)
		return false
	}

	resp, err := mc.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("Failed to send login request", "error", err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed to read login response", "error", err)
		return false
	}

	if resp.StatusCode == http.StatusOK {
		var loginResp LoginResponse
		if err := json.Unmarshal(body, &loginResp); err == nil && loginResp.AccessToken != "" {
			return true
		}
	}

	return false
}

// CreateUser creates a new user in the Matrix server
func (mc *MatrixClient) CreateUser(username, password string) error {
	// First check if user already exists
	if mc.TryLogin(username, password) {
		slog.Info("User already exists", "username", username)
		return nil
	}

	slog.Info("Creating user", "username", username)

	url := fmt.Sprintf("%s/_matrix/client/r0/register", mc.baseURL)

	registerReq := RegisterRequest{
		Username: username,
		Password: password,
		Auth: RegisterAuth{
			Type: "m.login.dummy",
		},
	}

	jsonData, err := json.Marshal(registerReq)
	if err != nil {
		return fmt.Errorf("failed to marshal register request: %w", err)
	}

	resp, err := mc.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send register request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read register response: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		var registerResp RegisterResponse
		if err := json.Unmarshal(body, &registerResp); err == nil && registerResp.AccessToken != "" {
			slog.Info("Successfully created user", "username", username)
			return nil
		}
	}

	// Handle error response
	var errorResp ErrorResponse
	if err := json.Unmarshal(body, &errorResp); err == nil {
		if errorResp.ErrCode == "M_FORBIDDEN" {
			return fmt.Errorf("failed to create user %s: registration may be disabled. Check homeserver.yaml for enable_registration: true", username)
		}
		return fmt.Errorf("failed to create user %s: %s - %s", username, errorResp.ErrCode, errorResp.Error)
	}

	return fmt.Errorf("failed to create user %s: unexpected response (status %d): %s", username, resp.StatusCode, string(body))
}
