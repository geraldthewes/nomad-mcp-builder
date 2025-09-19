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
			Description: "Submit a new Docker build job. IMPORTANT: This service runs on multiple build servers in a distributed cluster. Each server has its own build cache (Buildah layer cache), so consecutive builds may run on different servers with different cache states. If you need consistent build caching, consider this when troubleshooting build performance variations.",
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
						"description": "Resource limits for the build job. Supports both legacy global limits and per-phase limits. Units: CPU in MHz, Memory/Disk in MB. Recommended minimums: Simple apps (1000 MHz, 2048 MB, 10240 MB), Complex builds (2000 MHz, 4096 MB, 20480 MB), Large ML/Data images (4000+ MHz, 8192+ MB, 40960+ MB).",
						"properties": map[string]interface{}{
							// Legacy global limits (for backward compatibility)
							"cpu": map[string]interface{}{
								"type":        "string",
								"description": "Global CPU limit in MHz (e.g., '2000' for 2 GHz). Applies to all phases if per-phase not specified. Minimum: 500, Recommended: 1000-4000 depending on complexity.",
								"examples":    []string{"1000", "2000", "4000"},
							},
							"memory": map[string]interface{}{
								"type":        "string",
								"description": "Global memory limit in MB (e.g., '4096' for 4 GB). Applies to all phases if per-phase not specified. Minimum: 1024, Recommended: 2048-8192 depending on image size.",
								"examples":    []string{"2048", "4096", "8192"},
							},
							"disk": map[string]interface{}{
								"type":        "string",
								"description": "Global disk limit in MB (e.g., '20480' for 20 GB). Applies to all phases if per-phase not specified. Minimum: 5120, Recommended: 10240-40960 depending on dependencies.",
								"examples":    []string{"10240", "20480", "40960"},
							},
							// Per-phase resource limits
							"build": map[string]interface{}{
								"type": "object",
								"description": "Resource limits for the build phase (compiling, dependency installation). Generally needs the most resources due to compilation and package downloads.",
								"properties": map[string]interface{}{
									"cpu": map[string]interface{}{
										"type":        "string",
										"description": "CPU limit in MHz. Recommended: 2000-4000 for complex builds, 1000 for simple apps.",
										"examples":    []string{"1000", "2000", "4000"},
									},
									"memory": map[string]interface{}{
										"type":        "string",
										"description": "Memory limit in MB. Recommended: 4096-8192 for complex builds, 2048 for simple apps.",
										"examples":    []string{"2048", "4096", "8192"},
									},
									"disk": map[string]interface{}{
										"type":        "string",
										"description": "Disk limit in MB. Recommended: 20480-40960 for builds with many dependencies, 10240 for simple apps.",
										"examples":    []string{"10240", "20480", "40960"},
									},
								},
							},
							"test": map[string]interface{}{
								"type": "object",
								"description": "Resource limits for the test phase (running tests in the built image). Usually needs moderate resources.",
								"properties": map[string]interface{}{
									"cpu": map[string]interface{}{
										"type":        "string",
										"description": "CPU limit in MHz. Recommended: 1000-2000 for most applications.",
										"examples":    []string{"1000", "1500", "2000"},
									},
									"memory": map[string]interface{}{
										"type":        "string",
										"description": "Memory limit in MB. Recommended: 2048-4096 depending on test requirements.",
										"examples":    []string{"2048", "3072", "4096"},
									},
									"disk": map[string]interface{}{
										"type":        "string",
										"description": "Disk limit in MB. Recommended: 5120-10240 for test artifacts and temporary files.",
										"examples":    []string{"5120", "8192", "10240"},
									},
								},
							},
							"publish": map[string]interface{}{
								"type": "object",
								"description": "Resource limits for the publish phase (pushing final images to registry). Generally needs minimal resources.",
								"properties": map[string]interface{}{
									"cpu": map[string]interface{}{
										"type":        "string",
										"description": "CPU limit in MHz. Recommended: 500-1000 (lightweight registry operations).",
										"examples":    []string{"500", "750", "1000"},
									},
									"memory": map[string]interface{}{
										"type":        "string",
										"description": "Memory limit in MB. Recommended: 1024-2048 (mainly for image manipulation).",
										"examples":    []string{"1024", "1536", "2048"},
									},
									"disk": map[string]interface{}{
										"type":        "string",
										"description": "Disk limit in MB. Recommended: 2048-5120 (temporary space for image layers).",
										"examples":    []string{"2048", "3072", "5120"},
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
			Description: "Get logs from a build job. NOTE: Logs are captured at job completion but may be unavailable if Nomad's log garbage collection is aggressive. In that case, logs will be null but you can infer build success/failure from job status.",
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