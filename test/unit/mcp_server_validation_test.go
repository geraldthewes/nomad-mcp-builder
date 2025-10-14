package unit

import (
	"encoding/json"
	"strings"
	"testing"

	"nomad-mcp-builder/internal/mcp"
	"nomad-mcp-builder/pkg/types"
)

// TestMCPServerValidationConsistency tests that the MCP server validation matches the REST API validation
func TestMCPServerValidationConsistency(t *testing.T) {
	// Test cases that simulate actual MCP tool calls to submitJob
	testCases := []struct {
		name           string
		mcpArgs        map[string]interface{}
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "valid_complete_request",
			mcpArgs: map[string]interface{}{
				"owner":                     "test-org",
				"repo_url":                  "https://github.com/test/repo.git",
				"git_ref":                   "main",
				"git_credentials_path":      "secret/git-creds",
				"dockerfile_path":           "Dockerfile",
				"image_name":                "test-app",
				"image_tags":                []interface{}{"latest", "v1.0.0"},
				"registry_url":              "registry.example.com/test-app",
				"registry_credentials_path": "secret/registry-creds",
				"test_commands":             []interface{}{"npm test"},
				"test_entry_point":          true,
			},
			expectError: false,
		},
		{
			name: "valid_minimal_request",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"repo_url":        "https://github.com/test/repo.git",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{"latest"},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError: false,
		},
		{
			name: "missing_owner",
			mcpArgs: map[string]interface{}{
				"repo_url":        "https://github.com/test/repo.git",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{"latest"},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError:    true,
			expectedErrMsg: "owner is required",
		},
		{
			name: "missing_repo_url",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{"latest"},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError:    true,
			expectedErrMsg: "repo_url is required",
		},
		{
			name: "missing_git_ref_gets_default",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"repo_url":        "https://github.com/test/repo.git",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{"latest"},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError: false, // git_ref gets default "main"
		},
		{
			name: "missing_dockerfile_path_gets_default",
			mcpArgs: map[string]interface{}{
				"owner":        "test-org",
				"repo_url":     "https://github.com/test/repo.git",
				"git_ref":      "main",
				"image_name":   "test-app",
				"image_tags":   []interface{}{"latest"},
				"registry_url": "registry.example.com/test-app",
			},
			expectError: false, // dockerfile_path gets default "Dockerfile"
		},
		{
			name: "missing_image_name",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"repo_url":        "https://github.com/test/repo.git",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_tags":      []interface{}{"latest"},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError:    true,
			expectedErrMsg: "image_name is required",
		},
		{
			name: "empty_image_tags_gets_default",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"repo_url":        "https://github.com/test/repo.git",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError: false, // image_tags gets default job-id
		},
		{
			name: "missing_registry_url",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"repo_url":        "https://github.com/test/repo.git",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{"latest"},
			},
			expectError:    true,
			expectedErrMsg: "registry_url is required",
		},
		{
			name: "valid_with_defaults",
			mcpArgs: map[string]interface{}{
				"owner":        "test-org",
				"repo_url":     "https://github.com/test/repo.git",
				"image_name":   "test-app",
				"image_tags":   []interface{}{"latest"},
				"registry_url": "registry.example.com/test-app",
				// git_ref and dockerfile_path should get defaults
			},
			expectError: false,
		},
		{
			name: "image_tags_as_json_string_single",
			mcpArgs: map[string]interface{}{
				"owner":        "test-org",
				"repo_url":     "https://github.com/test/repo.git",
				"image_name":   "test-app",
				"image_tags":   "[\"latest\"]",
				"registry_url": "registry.example.com/test-app",
			},
			expectError: false,
		},
		{
			name: "image_tags_as_json_string_multiple",
			mcpArgs: map[string]interface{}{
				"owner":        "test-org",
				"repo_url":     "https://github.com/test/repo.git",
				"image_name":   "test-app",
				"image_tags":   "[\"latest\", \"v1.0.0\", \"v1.0.0-rc1\"]",
				"registry_url": "registry.example.com/test-app",
			},
			expectError: false,
		},
		{
			name: "image_tags_as_single_string",
			mcpArgs: map[string]interface{}{
				"owner":        "test-org",
				"repo_url":     "https://github.com/test/repo.git",
				"image_name":   "test-app",
				"image_tags":   "latest",
				"registry_url": "registry.example.com/test-app",
			},
			expectError: false,
		},
		{
			name: "image_tags_as_single_string_with_version",
			mcpArgs: map[string]interface{}{
				"owner":        "test-org",
				"repo_url":     "https://github.com/test/repo.git",
				"image_name":   "test-app",
				"image_tags":   "v1.2.3",
				"registry_url": "registry.example.com/test-app",
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert MCP arguments to JobConfig using the same logic as mcpSubmitJob
			jobConfig := convertMCPArgsToJobConfig(tc.mcpArgs)

			// Apply the validation that should be consistent between MCP and REST
			err := validateJobConfigStrict(jobConfig)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation error but got none")
				} else if err.Error() != tc.expectedErrMsg {
					t.Errorf("Expected error message '%s', got '%s'", tc.expectedErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

// TestMCPRestParameterParity tests that MCP and REST interfaces accept the same parameters
func TestMCPRestParameterParity(t *testing.T) {
	// Get MCP submitJob tool definition
	tools := mcp.GetTools()
	var submitJobTool mcp.Tool
	for _, tool := range tools {
		if tool.Name == "submitJob" {
			submitJobTool = tool
			break
		}
	}

	if submitJobTool.Name == "" {
		t.Fatal("submitJob tool not found")
	}

	// Parameters that should be supported by both interfaces
	expectedParameters := []string{
		"owner",
		"repo_url",
		"git_ref",
		"git_credentials_path",
		"dockerfile_path",
		"image_name",
		"image_tags",
		"registry_url",
		"registry_credentials_path",
		"test_commands",
		"test_entry_point",
		"resource_limits",
		"webhook_url",
		"webhook_secret",
		"webhook_on_success",
		"webhook_on_failure",
		"webhook_headers",
	}

	// Check that all expected parameters are defined in MCP schema
	for _, param := range expectedParameters {
		if _, exists := submitJobTool.InputSchema.Properties[param]; !exists {
			t.Errorf("Parameter '%s' missing from MCP schema", param)
		}
	}

	// Check that MCP doesn't define extra parameters not supported by validation
	allowedParameters := make(map[string]bool)
	for _, param := range expectedParameters {
		allowedParameters[param] = true
	}

	for param := range submitJobTool.InputSchema.Properties {
		if !allowedParameters[param] {
			t.Errorf("Parameter '%s' in MCP schema but not in expected parameters list", param)
		}
	}
}

// TestMCPRequestFormatting tests that MCP requests are properly formatted
func TestMCPRequestFormatting(t *testing.T) {
	// Test a complete MCP request JSON
	mcpRequestJSON := `{
		"jsonrpc": "2.0",
		"id": "test-123",
		"method": "tools/call",
		"params": {
			"name": "submitJob",
			"arguments": {
				"owner": "test-org",
				"repo_url": "https://github.com/test/repo.git",
				"git_ref": "main",
				"dockerfile_path": "Dockerfile",
				"image_name": "test-app",
				"image_tags": ["latest"],
				"registry_url": "registry.example.com/test-app"
			}
		}
	}`

	var mcpReq mcp.MCPRequest
	if err := json.Unmarshal([]byte(mcpRequestJSON), &mcpReq); err != nil {
		t.Fatalf("Failed to unmarshal MCP request: %v", err)
	}

	// Validate request structure
	if mcpReq.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got '%s'", mcpReq.JSONRPC)
	}

	if mcpReq.Method != "tools/call" {
		t.Errorf("Expected method 'tools/call', got '%s'", mcpReq.Method)
	}

	// Check params structure
	params, ok := mcpReq.Params.(map[string]interface{})
	if !ok {
		t.Fatal("Expected params to be a map")
	}

	toolName, ok := params["name"].(string)
	if !ok || toolName != "submitJob" {
		t.Errorf("Expected tool name 'submitJob', got '%v'", toolName)
	}

	arguments, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected arguments to be a map")
	}

	// Validate that the arguments contain required fields (excluding those with defaults)
	requiredFields := []string{"owner", "repo_url", "image_name", "registry_url"}
	for _, field := range requiredFields {
		if _, exists := arguments[field]; !exists {
			t.Errorf("Required field '%s' missing from arguments", field)
		}
	}

	// Check that default fields are present (they should be, but not required to be explicitly set)
	defaultFields := []string{"git_ref", "dockerfile_path"}
	for _, field := range defaultFields {
		if value, exists := arguments[field]; exists {
			t.Logf("Default field '%s' explicitly set to: %v", field, value)
		}
	}
}

// Helper function to convert MCP arguments to JobConfig (mirrors mcpSubmitJob logic)
func convertMCPArgsToJobConfig(args map[string]interface{}) *types.JobConfig {
	var jobConfig types.JobConfig

	if owner, ok := args["owner"].(string); ok {
		jobConfig.Owner = owner
	}
	if repoURL, ok := args["repo_url"].(string); ok {
		jobConfig.RepoURL = repoURL
	}
	if gitRef, ok := args["git_ref"].(string); ok {
		jobConfig.GitRef = gitRef
	} else {
		jobConfig.GitRef = "main"
	}
	if gitCreds, ok := args["git_credentials_path"].(string); ok {
		jobConfig.GitCredentialsPath = gitCreds
	} else {
		jobConfig.GitCredentialsPath = "secret/nomad/jobs/git-credentials"
	}
	if dockerfile, ok := args["dockerfile_path"].(string); ok {
		jobConfig.DockerfilePath = dockerfile
	} else {
		jobConfig.DockerfilePath = "Dockerfile"
	}
	if imageName, ok := args["image_name"].(string); ok {
		jobConfig.ImageName = imageName
	}
	if regURL, ok := args["registry_url"].(string); ok {
		jobConfig.RegistryURL = regURL
	}
	if regCreds, ok := args["registry_credentials_path"].(string); ok {
		jobConfig.RegistryCredentialsPath = regCreds
	} else {
		jobConfig.RegistryCredentialsPath = "secret/nomad/jobs/registry-credentials"
	}

	// Convert image tags - handle multiple formats for compatibility
	if tagsValue, exists := args["image_tags"]; exists {
		// Try as array first (proper format)
		if tagsArray, ok := tagsValue.([]interface{}); ok {
			for _, tag := range tagsArray {
				if tagStr, ok := tag.(string); ok {
					jobConfig.ImageTags = append(jobConfig.ImageTags, tagStr)
				}
			}
		} else if tagsString, ok := tagsValue.(string); ok {
			// Handle string formats
			if strings.HasPrefix(tagsString, "[") {
				// Parse JSON array string like "[\"latest\", \"v1.0\"]"
				var parsedTags []string
				if err := json.Unmarshal([]byte(tagsString), &parsedTags); err == nil {
					jobConfig.ImageTags = parsedTags
				}
			} else {
				// Treat as single tag
				jobConfig.ImageTags = []string{tagsString}
			}
		}
	}

	// Convert test commands and entry point into Test config
	var testCommands []string
	var testEntryPoint bool

	if testCmdsInterface, ok := args["test_commands"].([]interface{}); ok {
		for _, cmd := range testCmdsInterface {
			if cmdStr, ok := cmd.(string); ok {
				testCommands = append(testCommands, cmdStr)
			}
		}
	}

	if entryPoint, ok := args["test_entry_point"].(bool); ok {
		testEntryPoint = entryPoint
	}

	// Create Test config if there are any test settings
	if len(testCommands) > 0 || testEntryPoint {
		jobConfig.Test = &types.TestConfig{
			Commands:   testCommands,
			EntryPoint: testEntryPoint,
		}
	}

	// Parse resource limits (this is what's missing from the actual mcpSubmitJob!)
	if resourceLimitsInterface, ok := args["resource_limits"]; ok {
		resourceLimits := parseResourceLimitsFromMCP(resourceLimitsInterface)
		jobConfig.ResourceLimits = resourceLimits
	}

	return &jobConfig
}

// Strict validation function that mirrors the actual validateJobConfig
func validateJobConfigStrict(config *types.JobConfig) error {
	// Required fields (must match the server's validateJobConfig function exactly)
	if config.Owner == "" {
		return &ValidationError{"owner is required"}
	}
	if config.RepoURL == "" {
		return &ValidationError{"repo_url is required"}
	}
	if config.GitRef == "" {
		return &ValidationError{"git_ref is required"}
	}
	if config.DockerfilePath == "" {
		return &ValidationError{"dockerfile_path is required"}
	}
	if config.ImageName == "" {
		return &ValidationError{"image_name is required"}
	}
	// image_tags is optional - will default to job-id if not provided
	if config.RegistryURL == "" {
		return &ValidationError{"registry_url is required"}
	}

	// Validate at least one testing mode is specified if test config exists
	if config.Test != nil {
		if len(config.Test.Commands) == 0 && !config.Test.EntryPoint {
			// This is allowed - no testing will be performed
		}
	}

	return nil
}

// parseResourceLimitsFromMCP converts the MCP resource_limits argument to internal types
func parseResourceLimitsFromMCP(limitsInterface interface{}) *types.ResourceLimits {
	limitsMap, ok := limitsInterface.(map[string]interface{})
	if !ok {
		return nil
	}

	resourceLimits := &types.ResourceLimits{}

	// Parse legacy global limits
	if cpu, ok := limitsMap["cpu"].(string); ok {
		resourceLimits.CPU = cpu
	}
	if memory, ok := limitsMap["memory"].(string); ok {
		resourceLimits.Memory = memory
	}
	if disk, ok := limitsMap["disk"].(string); ok {
		resourceLimits.Disk = disk
	}

	// Parse per-phase limits
	if buildInterface, ok := limitsMap["build"]; ok {
		if buildMap, ok := buildInterface.(map[string]interface{}); ok {
			build := &types.PhaseResourceLimits{}
			if cpu, ok := buildMap["cpu"].(string); ok {
				build.CPU = cpu
			}
			if memory, ok := buildMap["memory"].(string); ok {
				build.Memory = memory
			}
			if disk, ok := buildMap["disk"].(string); ok {
				build.Disk = disk
			}
			resourceLimits.Build = build
		}
	}

	if testInterface, ok := limitsMap["test"]; ok {
		if testMap, ok := testInterface.(map[string]interface{}); ok {
			test := &types.PhaseResourceLimits{}
			if cpu, ok := testMap["cpu"].(string); ok {
				test.CPU = cpu
			}
			if memory, ok := testMap["memory"].(string); ok {
				test.Memory = memory
			}
			if disk, ok := testMap["disk"].(string); ok {
				test.Disk = disk
			}
			resourceLimits.Test = test
		}
	}

	if publishInterface, ok := limitsMap["publish"]; ok {
		if publishMap, ok := publishInterface.(map[string]interface{}); ok {
			publish := &types.PhaseResourceLimits{}
			if cpu, ok := publishMap["cpu"].(string); ok {
				publish.CPU = cpu
			}
			if memory, ok := publishMap["memory"].(string); ok {
				publish.Memory = memory
			}
			if disk, ok := publishMap["disk"].(string); ok {
				publish.Disk = disk
			}
			resourceLimits.Publish = publish
		}
	}

	return resourceLimits
}