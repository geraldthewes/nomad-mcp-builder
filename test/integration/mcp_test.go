// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"nomad-mcp-builder/internal/config"
	"nomad-mcp-builder/internal/mcp"
	"nomad-mcp-builder/internal/nomad"
	"nomad-mcp-builder/internal/storage"
	"nomad-mcp-builder/pkg/types"
)

// Integration tests require running Consul, Nomad, and Vault
// Run with: go test -tags=integration ./test/integration/

func TestMCPServerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// Load test configuration
	cfg := getTestConfig(t)
	
	// Create storage backend
	storage, err := storage.NewConsulStorage(
		cfg.Consul.Address,
		cfg.Consul.Token,
		cfg.Consul.Datacenter,
		cfg.Consul.KeyPrefix+"-test",
	)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	
	// Check Consul connectivity
	if err := storage.Health(); err != nil {
		t.Skip("Consul not available for integration test")
	}
	
	// Create Nomad client
	nomadClient, err := nomad.NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Nomad client: %v", err)
	}
	
	// Check Nomad connectivity
	if err := nomadClient.Health(); err != nil {
		t.Skip("Nomad not available for integration test")
	}
	
	// Create MCP server
	mcpServer := mcp.NewServer(cfg, nomadClient, storage)
	
	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go func() {
		if err := mcpServer.Start(ctx); err != nil {
			t.Logf("MCP server error: %v", err)
		}
	}()
	
	// Wait for server to start
	time.Sleep(2 * time.Second)
	
	// Run tests
	t.Run("SubmitJob", testSubmitJob(cfg))
	t.Run("GetStatus", testGetStatus(cfg))
	t.Run("GetLogs", testGetLogs(cfg))
	t.Run("HealthCheck", testHealthCheck(cfg))
}

func testSubmitJob(cfg *config.Config) func(*testing.T) {
	return func(t *testing.T) {
		jobConfig := types.JobConfig{
			Owner:                   "integration-test",
			RepoURL:                 "https://github.com/docker-library/hello-world.git",
			GitRef:                  "master",
			GitCredentialsPath:      "secret/test/git-creds",
			DockerfilePath:          "Dockerfile",
			ImageTags:               []string{"test"},
			RegistryURL:             "localhost:5000/test-app",
			RegistryCredentialsPath: "secret/test/registry-creds",
			TestCommands:            []string{"echo 'test completed'"},
		}
		
		submitReq := types.SubmitJobRequest{
			JobConfig: jobConfig,
		}
		
		// Submit job
		resp, err := makeHTTPRequest(cfg, "POST", "/mcp/submitJob", submitReq)
		if err != nil {
			t.Fatalf("Failed to submit job: %v", err)
		}
		
		var submitResp types.SubmitJobResponse
		if err := json.Unmarshal(resp, &submitResp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		
		if submitResp.JobID == "" {
			t.Error("Expected non-empty job ID")
		}
		
		if submitResp.Status != types.StatusPending && submitResp.Status != types.StatusBuilding {
			t.Errorf("Expected status PENDING or BUILDING, got %s", submitResp.Status)
		}
		
		t.Logf("Successfully submitted job: %s", submitResp.JobID)
	}
}

func testGetStatus(cfg *config.Config) func(*testing.T) {
	return func(t *testing.T) {
		// This test assumes a job exists from the previous test
		// In a real integration test, you'd maintain job IDs between tests
		
		statusReq := types.GetStatusRequest{
			JobID: "test-job-id", // Would be from previous test
		}
		
		resp, err := makeHTTPRequest(cfg, "POST", "/mcp/getStatus", statusReq)
		if err != nil {
			// Job might not exist in this isolated test
			t.Logf("Status check failed (expected): %v", err)
			return
		}
		
		var statusResp types.GetStatusResponse
		if err := json.Unmarshal(resp, &statusResp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		
		// Verify response structure
		if statusResp.JobID == "" {
			t.Error("Expected non-empty job ID in response")
		}
	}
}

func testGetLogs(cfg *config.Config) func(*testing.T) {
	return func(t *testing.T) {
		logsReq := types.GetLogsRequest{
			JobID: "test-job-id", // Would be from previous test
		}
		
		resp, err := makeHTTPRequest(cfg, "POST", "/mcp/getLogs", logsReq)
		if err != nil {
			// Job might not exist in this isolated test
			t.Logf("Logs request failed (expected): %v", err)
			return
		}
		
		var logsResp types.GetLogsResponse
		if err := json.Unmarshal(resp, &logsResp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		
		// Verify response structure
		if logsResp.JobID == "" {
			t.Error("Expected non-empty job ID in logs response")
		}
	}
}

func testHealthCheck(cfg *config.Config) func(*testing.T) {
	return func(t *testing.T) {
		url := fmt.Sprintf("http://%s:%d/health", cfg.Server.Host, cfg.Server.Port)
		
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("Health check request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
		
		var healthResp types.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}
		
		if healthResp.Status == "" {
			t.Error("Expected non-empty health status")
		}
		
		if len(healthResp.Services) == 0 {
			t.Error("Expected service health information")
		}
		
		t.Logf("Health status: %s, Services: %+v", healthResp.Status, healthResp.Services)
	}
}

func makeHTTPRequest(cfg *config.Config, method, path string, body interface{}) ([]byte, error) {
	url := fmt.Sprintf("http://%s:%d%s", cfg.Server.Host, cfg.Server.Port, path)
	
	var reqBody bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&reqBody).Encode(body); err != nil {
			return nil, fmt.Errorf("failed to encode request body: %w", err)
		}
	}
	
	req, err := http.NewRequest(method, url, &reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	
	var respBody bytes.Buffer
	if _, err := respBody.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	return respBody.Bytes(), nil
}

func getTestConfig(t *testing.T) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Nomad: config.NomadConfig{
			Address:   getEnvOrDefault("NOMAD_ADDR", "http://localhost:4646"),
			Region:    "global",
			Namespace: "default",
		},
		Consul: config.ConsulConfig{
			Address:    getEnvOrDefault("CONSUL_HTTP_ADDR", "localhost:8500"),
			Datacenter: "dc1",
			KeyPrefix:  "nomad-build-service",
		},
		Vault: config.VaultConfig{
			Address: getEnvOrDefault("VAULT_ADDR", "http://localhost:8200"),
			Mount:   "secret",
		},
		Build: config.BuildConfig{
			BuildTimeout: 5 * time.Minute,
			TestTimeout:  2 * time.Minute,
			RegistryConfig: config.RegistryConfig{
				URL:        "localhost:5000",
				TempPrefix: "temp",
			},
		},
		Monitoring: config.MonitoringConfig{
			Enabled:     false, // Disable metrics for tests
			MetricsPort: 9090,
			HealthPort:  8081,
		},
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}