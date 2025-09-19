package mcp

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ToolDefinition represents a tool definition loaded from YAML
type ToolDefinition struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	InputSchema InputSchemaDefinition  `yaml:"input_schema"`
}

// InputSchemaDefinition represents the input schema from YAML
type InputSchemaDefinition struct {
	Type       string                            `yaml:"type"`
	Required   []string                          `yaml:"required"`
	Properties map[string]PropertyDefinition    `yaml:"properties"`
}

// PropertyDefinition represents a property definition from YAML
type PropertyDefinition struct {
	Type        string              `yaml:"type"`
	Description string              `yaml:"description"`
	Default     interface{}         `yaml:"default"`
	Examples    []string            `yaml:"examples"`
	Enum        []string            `yaml:"enum"`
	Items       *PropertyDefinition `yaml:"items"`
	Properties  map[string]PropertyDefinition `yaml:"properties"`
}

// LoadToolsFromResources loads all tool definitions from YAML files
func LoadToolsFromResources() ([]Tool, error) {
	// Try multiple possible paths for the tools directory
	possiblePaths := []string{
		"resources/mcp/tools",                    // From project root
		"../resources/mcp/tools",                 // From subdirectory
		"../../resources/mcp/tools",              // From deeper subdirectory
		"../../../resources/mcp/tools",           // From test directories
	}

	var toolsDir string
	var found bool

	// Find the first existing directory
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			toolsDir = path
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("tools directory not found in any of the expected locations: %v", possiblePaths)
	}

	// Read the tools directory
	entries, err := ioutil.ReadDir(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tools directory: %w", err)
	}

	var tools []Tool
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		// Read the YAML file
		filePath := filepath.Join(toolsDir, entry.Name())
		yamlData, err := ioutil.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read tool file %s: %w", entry.Name(), err)
		}

		// Parse the YAML
		var toolDef ToolDefinition
		if err := yaml.Unmarshal(yamlData, &toolDef); err != nil {
			return nil, fmt.Errorf("failed to parse tool file %s: %w", entry.Name(), err)
		}

		// Convert to MCP Tool format
		tool := Tool{
			Name:        toolDef.Name,
			Description: toolDef.Description,
			InputSchema: convertInputSchema(toolDef.InputSchema),
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

// convertInputSchema converts the YAML input schema to the MCP format
func convertInputSchema(schema InputSchemaDefinition) ToolSchema {
	return ToolSchema{
		Type:       schema.Type,
		Required:   schema.Required,
		Properties: convertProperties(schema.Properties),
	}
}

// convertProperties converts YAML properties to MCP format
func convertProperties(props map[string]PropertyDefinition) map[string]interface{} {
	result := make(map[string]interface{})

	for key, prop := range props {
		propMap := map[string]interface{}{
			"type":        prop.Type,
			"description": prop.Description,
		}

		// Add optional fields if they exist
		if prop.Default != nil {
			propMap["default"] = prop.Default
		}
		if len(prop.Examples) > 0 {
			propMap["examples"] = prop.Examples
		}
		if len(prop.Enum) > 0 {
			propMap["enum"] = prop.Enum
		}
		if prop.Items != nil {
			propMap["items"] = convertPropertyToMap(*prop.Items)
		}
		if len(prop.Properties) > 0 {
			propMap["properties"] = convertProperties(prop.Properties)
		}

		result[key] = propMap
	}

	return result
}

// convertPropertyToMap converts a single PropertyDefinition to a map
func convertPropertyToMap(prop PropertyDefinition) map[string]interface{} {
	result := map[string]interface{}{
		"type": prop.Type,
	}

	if prop.Description != "" {
		result["description"] = prop.Description
	}
	if prop.Default != nil {
		result["default"] = prop.Default
	}
	if len(prop.Examples) > 0 {
		result["examples"] = prop.Examples
	}
	if len(prop.Enum) > 0 {
		result["enum"] = prop.Enum
	}
	if len(prop.Properties) > 0 {
		result["properties"] = convertProperties(prop.Properties)
	}

	return result
}