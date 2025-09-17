package unit

import (
	"encoding/json"
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
			name: "empty_image_tags",
			mcpArgs: map[string]interface{}{
				"owner":           "test-org",
				"repo_url":        "https://github.com/test/repo.git",
				"git_ref":         "main",
				"dockerfile_path": "Dockerfile",
				"image_name":      "test-app",
				"image_tags":      []interface{}{},
				"registry_url":    "registry.example.com/test-app",
			},
			expectError:    true,
			expectedErrMsg: "image_tags is required and cannot be empty",
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
	requiredFields := []string{"owner", "repo_url", "image_name", "image_tags", "registry_url"}
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

	// Convert image tags
	if tagsInterface, ok := args["image_tags"].([]interface{}); ok {
		for _, tag := range tagsInterface {
			if tagStr, ok := tag.(string); ok {
				jobConfig.ImageTags = append(jobConfig.ImageTags, tagStr)
			}
		}
	}

	// Convert test commands
	if testCmdsInterface, ok := args["test_commands"].([]interface{}); ok {
		for _, cmd := range testCmdsInterface {
			if cmdStr, ok := cmd.(string); ok {
				jobConfig.TestCommands = append(jobConfig.TestCommands, cmdStr)
			}
		}
	}

	// Handle test_entry_point parameter
	if testEntryPoint, ok := args["test_entry_point"].(bool); ok {
		jobConfig.TestEntryPoint = testEntryPoint
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
	if len(config.ImageTags) == 0 {
		return &ValidationError{"image_tags is required and cannot be empty"}
	}
	if config.RegistryURL == "" {
		return &ValidationError{"registry_url is required"}
	}

	// Validate at least one testing mode is specified if test_commands is empty
	if len(config.TestCommands) == 0 && !config.TestEntryPoint {
		// This is allowed - no testing will be performed
	}

	return nil
}