# MCP Resources

This directory contains externalized resource files for the Nomad Build Service MCP server.

## Structure

```
resources/
├── mcp/
│   └── tools/          # MCP tool definitions
│       ├── submitJob.yaml
│       ├── getStatus.yaml
│       ├── getLogs.yaml
│       ├── killJob.yaml
│       ├── cleanup.yaml
│       ├── getHistory.yaml
│       └── purgeFailedJob.yaml
└── README.md           # This file
```

## Tool Definition Format

Each tool is defined in a separate YAML file with the following structure:

```yaml
name: "toolName"
description: |
  Tool description supporting multi-line text.
  Can include detailed explanations and usage notes.

input_schema:
  type: "object"
  required: ["param1", "param2"]
  properties:
    param1:
      type: "string"
      description: "Parameter description"
      default: "default_value"

    param2:
      type: "array"
      items:
        type: "string"
      description: "Array parameter description"
```

## Supported Property Types

- `string`: Text values
- `integer`: Numeric values
- `boolean`: True/false values
- `array`: Lists of items
- `object`: Complex nested objects

## Property Attributes

- `type`: Data type (required)
- `description`: Human-readable description
- `default`: Default value if not provided
- `examples`: List of example values
- `enum`: List of allowed values
- `items`: For array types, defines the item schema
- `properties`: For object types, defines nested properties

## Benefits of External Resources

1. **Maintainability**: Tool documentation can be updated without code changes
2. **Collaboration**: Non-developers can update documentation
3. **Validation**: YAML structure enforces consistent documentation format
4. **Localization**: Easy to add multi-language support in the future
5. **Version Control**: Documentation changes are tracked separately from code logic

## Loading Mechanism

The MCP server loads these resources at runtime using `internal/mcp/resource_loader.go`. If resource files are missing or invalid, the service will log warnings but continue to operate (returning an empty tool list as fallback).