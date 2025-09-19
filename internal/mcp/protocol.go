package mcp

import (
	"encoding/json"
	"fmt"
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

// GetTools returns the list of available MCP tools loaded from resource files
func GetTools() []Tool {
	tools, err := LoadToolsFromResources()
	if err != nil {
		// Log the error but don't crash the service
		// This ensures service continues to work even if resource files are missing
		fmt.Printf("Warning: Failed to load MCP tools from resources: %v\n", err)
		return []Tool{}
	}
	return tools
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