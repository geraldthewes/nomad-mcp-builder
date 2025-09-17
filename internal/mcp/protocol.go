package mcp

import (
	"encoding/json"
)

// MCP Protocol structures following the MCP specification

// MCPRequest represents a standard MCP JSON-RPC request
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents a standard MCP JSON-RPC response
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents an MCP error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP Error codes
const (
	MCPErrorParseError     = -32700
	MCPErrorInvalidRequest = -32600
	MCPErrorMethodNotFound = -32601
	MCPErrorInvalidParams  = -32602
	MCPErrorInternalError  = -32603
)

// Tool represents an MCP tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema ToolSchema  `json:"inputSchema"`
}

// ToolSchema represents the input schema for a tool
type ToolSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// ToolListResult represents the result of tools/list
type ToolListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams represents parameters for tools/call
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolCallResult represents the result of tools/call
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// ContentItem represents a piece of content in tool results
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// GetTools returns the list of available MCP tools
func GetTools() []Tool {
	return []Tool{
		{
			Name:        "submitJob",
			Description: "Submit a new Docker build job",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"owner": map[string]interface{}{
						"type":        "string",
						"description": "Owner/organization of the repository",
					},
					"repo_url": map[string]interface{}{
						"type":        "string",
						"description": "Git repository URL",
					},
					"git_ref": map[string]interface{}{
						"type":        "string",
						"description": "Git reference (branch, tag, or commit)",
						"default":     "main",
					},
					"git_credentials_path": map[string]interface{}{
						"type":        "string",
						"description": "Vault path to git credentials",
						"default":     "secret/nomad/jobs/git-credentials",
					},
					"dockerfile_path": map[string]interface{}{
						"type":        "string",
						"description": "Path to Dockerfile in repository",
						"default":     "Dockerfile",
					},
					"image_name": map[string]interface{}{
						"type":        "string",
						"description": "Base name for the Docker image",
					},
					"image_tags": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "List of image tags to apply",
					},
					"registry_url": map[string]interface{}{
						"type":        "string",
						"description": "Docker registry URL for final images",
					},
					"registry_credentials_path": map[string]interface{}{
						"type":        "string",
						"description": "Vault path to registry credentials",
						"default":     "secret/nomad/jobs/registry-credentials",
					},
					"test_commands": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "List of test commands to run",
					},
					"test_entry_point": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to test the image entry point",
						"default":     false,
					},
					"resource_limits": map[string]interface{}{
						"type": "object",
						"description": "Resource limits for the build job. Supports both legacy global limits and per-phase limits.",
						"properties": map[string]interface{}{
							// Legacy global limits (for backward compatibility)
							"cpu": map[string]interface{}{
								"type":        "string",
								"description": "Global CPU limit in MHz (e.g., '1000') - applies to all phases if per-phase not specified",
							},
							"memory": map[string]interface{}{
								"type":        "string",
								"description": "Global memory limit in MB (e.g., '2048') - applies to all phases if per-phase not specified",
							},
							"disk": map[string]interface{}{
								"type":        "string",
								"description": "Global disk limit in MB (e.g., '10240') - applies to all phases if per-phase not specified",
							},
							// Per-phase resource limits
							"build": map[string]interface{}{
								"type": "object",
								"description": "Resource limits for the build phase",
								"properties": map[string]interface{}{
									"cpu": map[string]interface{}{
										"type":        "string",
										"description": "CPU limit in MHz (e.g., '2000')",
									},
									"memory": map[string]interface{}{
										"type":        "string",
										"description": "Memory limit in MB (e.g., '4096')",
									},
									"disk": map[string]interface{}{
										"type":        "string",
										"description": "Disk limit in MB (e.g., '20480')",
									},
								},
							},
							"test": map[string]interface{}{
								"type": "object",
								"description": "Resource limits for the test phase",
								"properties": map[string]interface{}{
									"cpu": map[string]interface{}{
										"type":        "string",
										"description": "CPU limit in MHz (e.g., '1000')",
									},
									"memory": map[string]interface{}{
										"type":        "string",
										"description": "Memory limit in MB (e.g., '2048')",
									},
									"disk": map[string]interface{}{
										"type":        "string",
										"description": "Disk limit in MB (e.g., '5120')",
									},
								},
							},
							"publish": map[string]interface{}{
								"type": "object",
								"description": "Resource limits for the publish phase",
								"properties": map[string]interface{}{
									"cpu": map[string]interface{}{
										"type":        "string",
										"description": "CPU limit in MHz (e.g., '500')",
									},
									"memory": map[string]interface{}{
										"type":        "string",
										"description": "Memory limit in MB (e.g., '1024')",
									},
									"disk": map[string]interface{}{
										"type":        "string",
										"description": "Disk limit in MB (e.g., '2048')",
									},
								},
							},
						},
					},
				},
				Required: []string{"owner", "repo_url", "image_name", "image_tags", "registry_url"},
			},
		},
		{
			Name:        "getStatus",
			Description: "Get the status of a build job",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the job to check",
					},
				},
				Required: []string{"job_id"},
			},
		},
		{
			Name:        "getLogs",
			Description: "Get logs from a build job",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the job to get logs from",
					},
					"phase": map[string]interface{}{
						"type":        "string",
						"description": "Phase to get logs for (build, test, publish)",
						"enum":        []string{"build", "test", "publish"},
					},
				},
				Required: []string{"job_id"},
			},
		},
		{
			Name:        "killJob",
			Description: "Terminate a running build job",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the job to terminate",
					},
				},
				Required: []string{"job_id"},
			},
		},
		{
			Name:        "cleanup",
			Description: "Cleanup resources for a build job",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the job to cleanup",
					},
				},
				Required: []string{"job_id"},
			},
		},
		{
			Name:        "getHistory",
			Description: "Get build job history",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"owner": map[string]interface{}{
						"type":        "string",
						"description": "Filter by owner/organization",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of jobs to return",
						"default":     10,
					},
				},
			},
		},
		{
			Name:        "purgeFailedJob",
			Description: "Purge a failed/dead job completely from Nomad (removes zombie jobs)",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"job_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the job to purge from Nomad",
					},
				},
				Required: []string{"job_id"},
			},
		},
	}
}

// Helper functions for creating MCP responses

func NewMCPResponse(id interface{}, result interface{}) MCPResponse {
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func NewMCPErrorResponse(id interface{}, code int, message string, data interface{}) MCPResponse {
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func NewMCPTextContent(text string) []ContentItem {
	return []ContentItem{
		{
			Type: "text",
			Text: text,
		},
	}
}

func NewMCPJSONContent(data interface{}) []ContentItem {
	jsonBytes, _ := json.MarshalIndent(data, "", "  ")
	return []ContentItem{
		{
			Type: "text",
			Text: string(jsonBytes),
		},
	}
}