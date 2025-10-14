package config

import (
	"os"
	"path/filepath"
	"testing"

	"nomad-mcp-builder/pkg/types"
)

func TestLoadJobConfigFromYAML(t *testing.T) {
	// Create temporary YAML file
	tempDir := t.TempDir()
	yamlFile := filepath.Join(tempDir, "test.yaml")

	yamlContent := `owner: test-owner
repo_url: https://github.com/test/repo.git
git_ref: main
dockerfile_path: Dockerfile
image_name: test-image
image_tags:
  - v1.0.0
  - latest
registry_url: registry.example.com/test
`

	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test YAML file: %v", err)
	}

	config, err := LoadJobConfigFromYAML(yamlFile)
	if err != nil {
		t.Fatalf("LoadJobConfigFromYAML failed: %v", err)
	}

	// Verify parsed values
	if config.Owner != "test-owner" {
		t.Errorf("Expected owner 'test-owner', got '%s'", config.Owner)
	}
	if config.RepoURL != "https://github.com/test/repo.git" {
		t.Errorf("Expected repo_url 'https://github.com/test/repo.git', got '%s'", config.RepoURL)
	}
	if config.GitRef != "main" {
		t.Errorf("Expected git_ref 'main', got '%s'", config.GitRef)
	}
	if config.ImageName != "test-image" {
		t.Errorf("Expected image_name 'test-image', got '%s'", config.ImageName)
	}
	if len(config.ImageTags) != 2 {
		t.Errorf("Expected 2 image tags, got %d", len(config.ImageTags))
	}
}

func TestLoadAndMergeJobConfigs(t *testing.T) {
	tempDir := t.TempDir()

	// Create global config
	globalFile := filepath.Join(tempDir, "global.yaml")
	globalContent := `owner: global-owner
repo_url: https://github.com/global/repo.git
git_ref: main
dockerfile_path: Dockerfile
image_name: global-image
image_tags:
  - latest
registry_url: registry.example.com/global
registry_credentials_path: secret/global/creds
`
	if err := os.WriteFile(globalFile, []byte(globalContent), 0644); err != nil {
		t.Fatalf("Failed to write global YAML: %v", err)
	}

	// Create per-build config (overrides some values)
	perBuildFile := filepath.Join(tempDir, "build.yaml")
	perBuildContent := `git_ref: feature/test-branch
image_tags:
  - v2.0.0
  - beta
`
	if err := os.WriteFile(perBuildFile, []byte(perBuildContent), 0644); err != nil {
		t.Fatalf("Failed to write per-build YAML: %v", err)
	}

	// Load and merge
	merged, err := LoadAndMergeJobConfigs(globalFile, perBuildFile)
	if err != nil {
		t.Fatalf("LoadAndMergeJobConfigs failed: %v", err)
	}

	// Verify merged values
	// Values from global that weren't overridden
	if merged.Owner != "global-owner" {
		t.Errorf("Expected owner 'global-owner' from global, got '%s'", merged.Owner)
	}
	if merged.RepoURL != "https://github.com/global/repo.git" {
		t.Errorf("Expected repo_url from global, got '%s'", merged.RepoURL)
	}
	if merged.ImageName != "global-image" {
		t.Errorf("Expected image_name from global, got '%s'", merged.ImageName)
	}

	// Values overridden by per-build
	if merged.GitRef != "feature/test-branch" {
		t.Errorf("Expected git_ref 'feature/test-branch' from per-build, got '%s'", merged.GitRef)
	}
	if len(merged.ImageTags) != 2 {
		t.Errorf("Expected 2 image tags from per-build, got %d", len(merged.ImageTags))
	} else {
		if merged.ImageTags[0] != "v2.0.0" {
			t.Errorf("Expected first tag 'v2.0.0', got '%s'", merged.ImageTags[0])
		}
		if merged.ImageTags[1] != "beta" {
			t.Errorf("Expected second tag 'beta', got '%s'", merged.ImageTags[1])
		}
	}
}

func TestLoadAndMergeJobConfigsNilGlobal(t *testing.T) {
	tempDir := t.TempDir()

	// Create only per-build config
	perBuildFile := filepath.Join(tempDir, "build.yaml")
	perBuildContent := `owner: test-owner
repo_url: https://github.com/test/repo.git
git_ref: main
dockerfile_path: Dockerfile
image_name: test-image
image_tags:
  - v1.0.0
registry_url: registry.example.com/test
`
	if err := os.WriteFile(perBuildFile, []byte(perBuildContent), 0644); err != nil {
		t.Fatalf("Failed to write per-build YAML: %v", err)
	}

	// Load without global config
	config, err := LoadAndMergeJobConfigs("", perBuildFile)
	if err != nil {
		t.Fatalf("LoadAndMergeJobConfigs failed: %v", err)
	}

	// Should just return per-build config
	if config.Owner != "test-owner" {
		t.Errorf("Expected owner 'test-owner', got '%s'", config.Owner)
	}
	if config.ImageName != "test-image" {
		t.Errorf("Expected image_name 'test-image', got '%s'", config.ImageName)
	}
}

func TestParseYAMLString(t *testing.T) {
	yamlStr := `owner: test-owner
repo_url: https://github.com/test/repo.git
git_ref: main
dockerfile_path: Dockerfile
image_name: test-image
image_tags:
  - v1.0.0
registry_url: registry.example.com/test
`

	config, err := ParseYAMLString(yamlStr)
	if err != nil {
		t.Fatalf("ParseYAMLString failed: %v", err)
	}

	if config.Owner != "test-owner" {
		t.Errorf("Expected owner 'test-owner', got '%s'", config.Owner)
	}
	if config.ImageName != "test-image" {
		t.Errorf("Expected image_name 'test-image', got '%s'", config.ImageName)
	}
}

func TestMergeJobConfigsComplexTypes(t *testing.T) {
	global := &types.JobConfig{
		Owner:          "global-owner",
		RepoURL:        "https://github.com/global/repo.git",
		GitRef:         "main",
		DockerfilePath: "Dockerfile",
		ImageName:      "global-image",
		ImageTags:      []string{"latest"},
		RegistryURL:    "registry.example.com/global",
		Test: &types.TestConfig{
			Commands: []string{"test1", "test2"},
		},
	}

	perBuild := &types.JobConfig{
		GitRef:    "feature/branch",
		ImageTags: []string{"v2.0.0", "beta"},
		Test: &types.TestConfig{
			Commands: []string{"test3"},
		},
	}

	merged := mergeJobConfigs(global, perBuild)

	// Global values that weren't overridden
	if merged.Owner != "global-owner" {
		t.Errorf("Expected owner from global, got '%s'", merged.Owner)
	}
	if merged.ImageName != "global-image" {
		t.Errorf("Expected image_name from global, got '%s'", merged.ImageName)
	}

	// Overridden values
	if merged.GitRef != "feature/branch" {
		t.Errorf("Expected git_ref from per-build, got '%s'", merged.GitRef)
	}

	// For slices, per-build completely replaces global
	if len(merged.ImageTags) != 2 {
		t.Errorf("Expected 2 image tags, got %d", len(merged.ImageTags))
	}
	if merged.Test == nil || len(merged.Test.Commands) != 1 {
		if merged.Test == nil {
			t.Errorf("Expected Test config to be set")
		} else {
			t.Errorf("Expected 1 test command, got %d", len(merged.Test.Commands))
		}
	}
}
