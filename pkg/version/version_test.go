package version

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionString(t *testing.T) {
	v := &Version{Major: 1, Minor: 2, Patch: 3}

	if v.String() != "1.2.3" {
		t.Errorf("Expected version string '1.2.3', got '%s'", v.String())
	}

	if v.Tag() != "v1.2.3" {
		t.Errorf("Expected version tag 'v1.2.3', got '%s'", v.Tag())
	}
}

func TestBranchTag(t *testing.T) {
	v := &Version{Major: 1, Minor: 2, Patch: 3}

	tests := []struct {
		branch   string
		expected string
	}{
		{"main", "main-v1.2.3"},
		{"feature/new-ui", "feature-new-ui-v1.2.3"},
		{"bugfix/fix_parser", "bugfix-fix-parser-v1.2.3"},
		{"release-1.0", "release-1.0-v1.2.3"},
	}

	for _, tt := range tests {
		result := v.BranchTag(tt.branch)
		if result != tt.expected {
			t.Errorf("BranchTag(%s): expected '%s', got '%s'", tt.branch, tt.expected, result)
		}
	}
}

func TestLoadSaveVersion(t *testing.T) {
	tempDir := t.TempDir()

	// Test loading non-existent version (should return 0.0.0)
	v, err := LoadVersion(tempDir)
	if err != nil {
		t.Fatalf("LoadVersion failed: %v", err)
	}
	if v.Major != 0 || v.Minor != 0 || v.Patch != 0 {
		t.Errorf("Expected 0.0.0, got %s", v.String())
	}

	// Test saving version
	v = &Version{Major: 1, Minor: 2, Patch: 3}
	if err := SaveVersion(tempDir, v); err != nil {
		t.Fatalf("SaveVersion failed: %v", err)
	}

	// Verify file was created
	versionPath := filepath.Join(tempDir, "version.yaml")
	if _, err := os.Stat(versionPath); os.IsNotExist(err) {
		t.Fatalf("Version file was not created")
	}

	// Test loading saved version
	loaded, err := LoadVersion(tempDir)
	if err != nil {
		t.Fatalf("LoadVersion failed after save: %v", err)
	}

	if loaded.Major != 1 || loaded.Minor != 2 || loaded.Patch != 3 {
		t.Errorf("Expected 1.2.3, got %s", loaded.String())
	}
}

func TestIncrementPatch(t *testing.T) {
	tempDir := t.TempDir()

	// Start with 0.0.0
	v := &Version{Major: 0, Minor: 1, Patch: 5}
	if err := SaveVersion(tempDir, v); err != nil {
		t.Fatalf("SaveVersion failed: %v", err)
	}

	// Increment patch
	incremented, err := IncrementPatch(tempDir)
	if err != nil {
		t.Fatalf("IncrementPatch failed: %v", err)
	}

	if incremented.Patch != 6 {
		t.Errorf("Expected patch 6, got %d", incremented.Patch)
	}

	// Verify it was saved
	loaded, err := LoadVersion(tempDir)
	if err != nil {
		t.Fatalf("LoadVersion failed: %v", err)
	}

	if loaded.String() != "0.1.6" {
		t.Errorf("Expected 0.1.6, got %s", loaded.String())
	}
}

func TestSetMajor(t *testing.T) {
	tempDir := t.TempDir()

	// Start with 1.2.3
	v := &Version{Major: 1, Minor: 2, Patch: 3}
	if err := SaveVersion(tempDir, v); err != nil {
		t.Fatalf("SaveVersion failed: %v", err)
	}

	// Set major to 2
	newVersion, err := SetMajor(tempDir, 2)
	if err != nil {
		t.Fatalf("SetMajor failed: %v", err)
	}

	if newVersion.String() != "2.0.0" {
		t.Errorf("Expected 2.0.0, got %s", newVersion.String())
	}

	// Verify negative major fails
	_, err = SetMajor(tempDir, -1)
	if err == nil {
		t.Error("Expected error for negative major version")
	}
}

func TestSetMinor(t *testing.T) {
	tempDir := t.TempDir()

	// Start with 1.2.3
	v := &Version{Major: 1, Minor: 2, Patch: 3}
	if err := SaveVersion(tempDir, v); err != nil {
		t.Fatalf("SaveVersion failed: %v", err)
	}

	// Set minor to 5
	newVersion, err := SetMinor(tempDir, 5)
	if err != nil {
		t.Fatalf("SetMinor failed: %v", err)
	}

	if newVersion.String() != "1.5.0" {
		t.Errorf("Expected 1.5.0, got %s", newVersion.String())
	}

	// Verify negative minor fails
	_, err = SetMinor(tempDir, -1)
	if err == nil {
		t.Error("Expected error for negative minor version")
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main", "main"},
		{"feature/new-feature", "feature-new-feature"},
		{"bugfix/fix_bug", "bugfix-fix-bug"},
		{"hotfix/urgent!fix", "hotfix-urgent-fix"},
		{"release-1.0", "release-1.0"},
		{"test@branch#123", "test-branch-123"},
		{"--leading-trailing--", "leading-trailing"},
	}

	for _, tt := range tests {
		result := sanitizeBranchName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeBranchName(%s): expected '%s', got '%s'", tt.input, tt.expected, result)
		}
	}
}
