package config

import (
	"fmt"
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
	"nomad-mcp-builder/pkg/types"
)

// LoadJobConfigFromYAML loads a job configuration from a YAML file
func LoadJobConfigFromYAML(filePath string) (*types.JobConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file %s: %w", filePath, err)
	}

	var config types.JobConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}

// LoadAndMergeJobConfigs loads and merges global and per-build YAML configurations
// Per-build settings override global settings (simple deep merge)
func LoadAndMergeJobConfigs(globalPath, perBuildPath string) (*types.JobConfig, error) {
	// Load global config if provided
	var globalConfig *types.JobConfig
	if globalPath != "" {
		var err error
		globalConfig, err = LoadJobConfigFromYAML(globalPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load global config: %w", err)
		}
	}

	// Load per-build config
	perBuildConfig, err := LoadJobConfigFromYAML(perBuildPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load per-build config: %w", err)
	}

	// If no global config, just return per-build config
	if globalConfig == nil {
		return perBuildConfig, nil
	}

	// Merge configs (per-build overrides global)
	merged := mergeJobConfigs(globalConfig, perBuildConfig)
	return merged, nil
}

// mergeJobConfigs performs a deep merge where perBuild values override global values
// Non-zero values in perBuild take precedence over global values
func mergeJobConfigs(global, perBuild *types.JobConfig) *types.JobConfig {
	result := &types.JobConfig{}

	// Use reflection to merge all fields
	mergeStructs(reflect.ValueOf(global).Elem(), reflect.ValueOf(perBuild).Elem(), reflect.ValueOf(result).Elem())

	return result
}

// mergeStructs performs a deep merge of two structs using reflection
// Non-zero values in override take precedence over base values
func mergeStructs(base, override, result reflect.Value) {
	for i := 0; i < base.NumField(); i++ {
		baseField := base.Field(i)
		overrideField := override.Field(i)
		resultField := result.Field(i)

		// Skip unexported fields
		if !resultField.CanSet() {
			continue
		}

		// Check if override field has a non-zero value
		if !isZeroValue(overrideField) {
			// Use override value
			resultField.Set(overrideField)
		} else {
			// Use base value
			resultField.Set(baseField)
		}
	}
}

// isZeroValue checks if a reflect.Value is the zero value for its type
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Struct:
		// For structs, check if all fields are zero
		for i := 0; i < v.NumField(); i++ {
			if !isZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// ParseYAMLString parses a YAML string into a JobConfig
func ParseYAMLString(yamlStr string) (*types.JobConfig, error) {
	var config types.JobConfig
	if err := yaml.Unmarshal([]byte(yamlStr), &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML string: %w", err)
	}
	return &config, nil
}
