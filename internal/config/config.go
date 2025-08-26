package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/sirupsen/logrus"
)

// Config holds the application configuration
type Config struct {
	// Server configuration
	Server ServerConfig `json:"server"`
	
	// Nomad configuration
	Nomad NomadConfig `json:"nomad"`
	
	// Consul configuration
	Consul ConsulConfig `json:"consul"`
	
	// Vault configuration
	Vault VaultConfig `json:"vault"`
	
	// Build configuration
	Build BuildConfig `json:"build"`
	
	// Monitoring configuration
	Monitoring MonitoringConfig `json:"monitoring"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	TLS  bool   `json:"tls"`
}

type NomadConfig struct {
	Address   string `json:"address"`
	Token     string `json:"token"`
	Region    string `json:"region"`
	Namespace string `json:"namespace"`
}

type ConsulConfig struct {
	Address    string `json:"address"`
	Token      string `json:"token"`
	Datacenter string `json:"datacenter"`
	KeyPrefix  string `json:"key_prefix"`
}

type VaultConfig struct {
	Address   string `json:"address"`
	Token     string `json:"token"`
	Mount     string `json:"mount"`
	RoleID    string `json:"role_id"`
	SecretID  string `json:"secret_id"`
}

type BuildConfig struct {
	DefaultResourceLimits ResourceLimits    `json:"default_resource_limits"`
	BuildTimeout          time.Duration     `json:"build_timeout"`
	TestTimeout           time.Duration     `json:"test_timeout"`
	RegistryConfig        RegistryConfig    `json:"registry_config"`
	BuildCachePath        string            `json:"build_cache_path"`
}

type ResourceLimits struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
}

type RegistryConfig struct {
	URL         string `json:"url"`
	TempPrefix  string `json:"temp_prefix"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

type MonitoringConfig struct {
	Enabled     bool   `json:"enabled"`
	MetricsPort int    `json:"metrics_port"`
	HealthPort  int    `json:"health_port"`
}

// Load loads configuration from environment variables and Consul
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8080),
			TLS:  getEnvBool("SERVER_TLS", false),
		},
		Nomad: NomadConfig{
			Address:   getEnv("NOMAD_ADDR", "http://localhost:4646"),
			Token:     getEnv("NOMAD_TOKEN", ""),
			Region:    getEnv("NOMAD_REGION", "global"),
			Namespace: getEnv("NOMAD_NAMESPACE", "default"),
		},
		Consul: ConsulConfig{
			Address:    getEnv("CONSUL_HTTP_ADDR", "localhost:8500"),
			Token:      getEnv("CONSUL_HTTP_TOKEN", ""),
			Datacenter: getEnv("CONSUL_DATACENTER", "dc1"),
			KeyPrefix:  getEnv("CONSUL_KEY_PREFIX", "nomad-build-service"),
		},
		Vault: VaultConfig{
			Address:  getEnv("VAULT_ADDR", "http://localhost:8200"),
			Token:    getEnv("VAULT_TOKEN", ""),
			Mount:    getEnv("VAULT_MOUNT", "secret"),
			RoleID:   getEnv("VAULT_ROLE_ID", ""),
			SecretID: getEnv("VAULT_SECRET_ID", ""),
		},
		Build: BuildConfig{
			DefaultResourceLimits: ResourceLimits{
				CPU:    getEnv("DEFAULT_CPU_LIMIT", "1000"),
				Memory: getEnv("DEFAULT_MEMORY_LIMIT", "2048"),
				Disk:   getEnv("DEFAULT_DISK_LIMIT", "10240"),
			},
			BuildTimeout: getEnvDuration("BUILD_TIMEOUT", 30*time.Minute),
			TestTimeout:  getEnvDuration("TEST_TIMEOUT", 15*time.Minute),
			RegistryConfig: RegistryConfig{
				URL:        getEnv("REGISTRY_URL", ""),
				TempPrefix: getEnv("REGISTRY_TEMP_PREFIX", "temp"),
				Username:   getEnv("REGISTRY_USERNAME", ""),
				Password:   getEnv("REGISTRY_PASSWORD", ""),
			},
			BuildCachePath: getEnv("BUILD_CACHE_PATH", "/opt/nomad/data/buildah-cache"),
		},
		Monitoring: MonitoringConfig{
			Enabled:     getEnvBool("MONITORING_ENABLED", true),
			MetricsPort: getEnvInt("METRICS_PORT", 9090),
			HealthPort:  getEnvInt("HEALTH_PORT", 8081),
		},
	}
	
	// Load additional configuration from Consul if available
	if err := loadFromConsul(config); err != nil {
		logrus.WithError(err).Warn("Failed to load configuration from Consul, using defaults")
	}
	
	return config, nil
}

// loadFromConsul loads additional configuration from Consul KV store
func loadFromConsul(config *Config) error {
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = config.Consul.Address
	consulConfig.Token = config.Consul.Token
	consulConfig.Datacenter = config.Consul.Datacenter
	
	client, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return fmt.Errorf("failed to create Consul client: %w", err)
	}
	
	kv := client.KV()
	keyPrefix := config.Consul.KeyPrefix + "/config/"
	
	// Load build timeout from Consul
	if pair, _, err := kv.Get(keyPrefix+"build_timeout", nil); err == nil && pair != nil {
		if timeout, err := time.ParseDuration(string(pair.Value)); err == nil {
			config.Build.BuildTimeout = timeout
		}
	}
	
	// Load test timeout from Consul
	if pair, _, err := kv.Get(keyPrefix+"test_timeout", nil); err == nil && pair != nil {
		if timeout, err := time.ParseDuration(string(pair.Value)); err == nil {
			config.Build.TestTimeout = timeout
		}
	}
	
	// Load resource limits from Consul
	if pair, _, err := kv.Get(keyPrefix+"default_resource_limits/cpu", nil); err == nil && pair != nil {
		config.Build.DefaultResourceLimits.CPU = string(pair.Value)
	}
	if pair, _, err := kv.Get(keyPrefix+"default_resource_limits/memory", nil); err == nil && pair != nil {
		config.Build.DefaultResourceLimits.Memory = string(pair.Value)
	}
	if pair, _, err := kv.Get(keyPrefix+"default_resource_limits/disk", nil); err == nil && pair != nil {
		config.Build.DefaultResourceLimits.Disk = string(pair.Value)
	}
	
	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	
	if c.Nomad.Address == "" {
		return fmt.Errorf("nomad address is required")
	}
	
	if c.Build.BuildTimeout <= 0 {
		return fmt.Errorf("build timeout must be positive")
	}
	
	if c.Build.TestTimeout <= 0 {
		return fmt.Errorf("test timeout must be positive")
	}
	
	return nil
}

// Helper functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}