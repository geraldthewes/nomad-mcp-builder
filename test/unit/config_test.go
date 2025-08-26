package unit

import (
	"os"
	"testing"
	"time"

	"nomad-mcp-builder/internal/config"
)

func TestConfigLoad(t *testing.T) {
	// Test default configuration
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}
	
	if cfg.Build.BuildTimeout != 30*time.Minute {
		t.Errorf("Expected default build timeout 30m, got %v", cfg.Build.BuildTimeout)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Nomad: config.NomadConfig{
					Address: "http://localhost:4646",
				},
				Build: config.BuildConfig{
					BuildTimeout: 30 * time.Minute,
					TestTimeout:  15 * time.Minute,
				},
			},
			expectError: false,
		},
		{
			name: "invalid port",
			config: &config.Config{
				Server: config.ServerConfig{
					Port: 0,
				},
				Nomad: config.NomadConfig{
					Address: "http://localhost:4646",
				},
				Build: config.BuildConfig{
					BuildTimeout: 30 * time.Minute,
					TestTimeout:  15 * time.Minute,
				},
			},
			expectError: true,
		},
		{
			name: "missing nomad address",
			config: &config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Nomad: config.NomadConfig{
					Address: "",
				},
				Build: config.BuildConfig{
					BuildTimeout: 30 * time.Minute,
					TestTimeout:  15 * time.Minute,
				},
			},
			expectError: true,
		},
		{
			name: "invalid timeout",
			config: &config.Config{
				Server: config.ServerConfig{
					Port: 8080,
				},
				Nomad: config.NomadConfig{
					Address: "http://localhost:4646",
				},
				Build: config.BuildConfig{
					BuildTimeout: 0,
					TestTimeout:  15 * time.Minute,
				},
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

func TestConfigEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("NOMAD_ADDR", "http://test-nomad:4646")
	os.Setenv("BUILD_TIMEOUT", "45m")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("NOMAD_ADDR") 
		os.Unsetenv("BUILD_TIMEOUT")
	}()
	
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	if cfg.Server.Port != 9090 {
		t.Errorf("Expected port 9090 from env var, got %d", cfg.Server.Port)
	}
	
	if cfg.Nomad.Address != "http://test-nomad:4646" {
		t.Errorf("Expected nomad address from env var, got %s", cfg.Nomad.Address)
	}
	
	if cfg.Build.BuildTimeout != 45*time.Minute {
		t.Errorf("Expected build timeout 45m from env var, got %v", cfg.Build.BuildTimeout)
	}
}