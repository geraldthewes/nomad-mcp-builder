package unit

import (
	"reflect"
	"testing"

	"nomad-mcp-builder/pkg/types"
)

func TestResourceLimitsIntegration(t *testing.T) {
	t.Run("MCP resource limits parsing", func(t *testing.T) {
		// Simulate MCP arguments with resource limits like the original request
		mcpArgs := map[string]interface{}{
			"owner":           "gerald",
			"repo_url":        "https://github.com/geraldthewes/video-transcription-batch.git",
			"git_ref":         "main",
			"dockerfile_path": "docker/Dockerfile",
			"image_name":      "video-transcription-batch",
			"image_tags":      []interface{}{"latest", "v4.0.1"},
			"registry_url":    "registry.cluster:5000",
			"resource_limits": map[string]interface{}{
				"cpu":    "4000",
				"memory": "16384",
				"disk":   "40960",
			},
		}

		// Test the conversion logic that should exist in mcpSubmitJob
		jobConfig := convertMCPArgsToJobConfig(mcpArgs)

		// Verify resource limits were parsed correctly
		if jobConfig.ResourceLimits == nil {
			t.Fatal("ResourceLimits should not be nil")
		}

		expected := &types.ResourceLimits{
			CPU:    "4000",
			Memory: "16384",
			Disk:   "40960",
		}

		if !reflect.DeepEqual(jobConfig.ResourceLimits, expected) {
			t.Errorf("Resource limits mismatch.\nExpected: %+v\nGot: %+v", expected, jobConfig.ResourceLimits)
		}
	})

	t.Run("Per-phase resource limits parsing", func(t *testing.T) {
		mcpArgs := map[string]interface{}{
			"owner":           "test",
			"repo_url":        "https://github.com/test/repo.git",
			"git_ref":         "main",
			"dockerfile_path": "Dockerfile",
			"image_name":      "test-image",
			"image_tags":      []interface{}{"test"},
			"registry_url":    "registry.test",
			"resource_limits": map[string]interface{}{
				"build": map[string]interface{}{
					"cpu":    "4000",
					"memory": "16384",
					"disk":   "40960",
				},
				"test": map[string]interface{}{
					"cpu":    "2000",
					"memory": "8192",
					"disk":   "20480",
				},
				"publish": map[string]interface{}{
					"cpu":    "1000",
					"memory": "2048",
					"disk":   "10240",
				},
			},
		}

		jobConfig := convertMCPArgsToJobConfig(mcpArgs)

		if jobConfig.ResourceLimits == nil {
			t.Fatal("ResourceLimits should not be nil")
		}

		// Check build phase limits
		if jobConfig.ResourceLimits.Build == nil {
			t.Fatal("Build resource limits should not be nil")
		}
		if jobConfig.ResourceLimits.Build.CPU != "4000" {
			t.Errorf("Expected build CPU '4000', got '%s'", jobConfig.ResourceLimits.Build.CPU)
		}
		if jobConfig.ResourceLimits.Build.Memory != "16384" {
			t.Errorf("Expected build Memory '16384', got '%s'", jobConfig.ResourceLimits.Build.Memory)
		}

		// Check test phase limits
		if jobConfig.ResourceLimits.Test == nil {
			t.Fatal("Test resource limits should not be nil")
		}
		if jobConfig.ResourceLimits.Test.CPU != "2000" {
			t.Errorf("Expected test CPU '2000', got '%s'", jobConfig.ResourceLimits.Test.CPU)
		}

		// Check publish phase limits
		if jobConfig.ResourceLimits.Publish == nil {
			t.Fatal("Publish resource limits should not be nil")
		}
		if jobConfig.ResourceLimits.Publish.CPU != "1000" {
			t.Errorf("Expected publish CPU '1000', got '%s'", jobConfig.ResourceLimits.Publish.CPU)
		}
	})

	t.Run("Resource limits application to Nomad job", func(t *testing.T) {
		// Test that resource limits are properly applied to Nomad job specs
		jobConfig := &types.JobConfig{
			Owner:           "test",
			RepoURL:         "https://github.com/test/repo.git",
			GitRef:          "main",
			DockerfilePath:  "Dockerfile",
			ImageName:       "test-image",
			ImageTags:       []string{"test"},
			RegistryURL:     "registry.test",
			ResourceLimits: &types.ResourceLimits{
				CPU:    "4000",
				Memory: "16384",
				Disk:   "40960",
			},
		}

		// Create a mock job to test the resource limit application
		job := &types.Job{
			ID:     "test-job-123",
			Status: types.StatusPending,
			Config: *jobConfig,
		}

		// Test that GetBuildLimits works correctly with the provided values
		defaults := types.PhaseResourceLimits{
			CPU:    "1000",
			Memory: "2048",
			Disk:   "10240",
		}

		buildLimits := job.Config.ResourceLimits.GetBuildLimits(defaults)

		if buildLimits.CPU != "4000" {
			t.Errorf("Expected build CPU '4000', got '%s'", buildLimits.CPU)
		}
		if buildLimits.Memory != "16384" {
			t.Errorf("Expected build Memory '16384', got '%s'", buildLimits.Memory)
		}
		if buildLimits.Disk != "40960" {
			t.Errorf("Expected build Disk '40960', got '%s'", buildLimits.Disk)
		}
	})
}

