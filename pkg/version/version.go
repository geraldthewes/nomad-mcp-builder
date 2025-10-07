package version

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Version represents semantic version information
type Version struct {
	Major int `yaml:"major"`
	Minor int `yaml:"minor"`
	Patch int `yaml:"patch"`
}

// VersionFile represents the structure of deploy/version.yaml
type VersionFile struct {
	Version Version `yaml:"version"`
}

// String returns the version in standard format (X.Y.Z)
func (v *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Tag returns the version with 'v' prefix (vX.Y.Z)
func (v *Version) Tag() string {
	return fmt.Sprintf("v%s", v.String())
}

// BranchTag returns branch-aware version tag (branchname-vX.Y.Z)
func (v *Version) BranchTag(branch string) string {
	// Sanitize branch name for use in tags
	sanitized := sanitizeBranchName(branch)
	return fmt.Sprintf("%s-v%s", sanitized, v.String())
}

// LoadVersion loads version from deploy/version.yaml
func LoadVersion(deployDir string) (*Version, error) {
	versionPath := filepath.Join(deployDir, "version.yaml")

	data, err := os.ReadFile(versionPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, start with 0.0.0
			return &Version{Major: 0, Minor: 0, Patch: 0}, nil
		}
		return nil, fmt.Errorf("failed to read version file: %w", err)
	}

	var vf VersionFile
	if err := yaml.Unmarshal(data, &vf); err != nil {
		return nil, fmt.Errorf("failed to parse version YAML: %w", err)
	}

	return &vf.Version, nil
}

// SaveVersion saves version to deploy/version.yaml
func SaveVersion(deployDir string, v *Version) error {
	versionPath := filepath.Join(deployDir, "version.yaml")

	// Ensure deploy directory exists
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		return fmt.Errorf("failed to create deploy directory: %w", err)
	}

	vf := VersionFile{Version: *v}
	data, err := yaml.Marshal(&vf)
	if err != nil {
		return fmt.Errorf("failed to marshal version YAML: %w", err)
	}

	if err := os.WriteFile(versionPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write version file: %w", err)
	}

	return nil
}

// IncrementPatch increments the patch version and saves it
func IncrementPatch(deployDir string) (*Version, error) {
	v, err := LoadVersion(deployDir)
	if err != nil {
		return nil, err
	}

	v.Patch++

	if err := SaveVersion(deployDir, v); err != nil {
		return nil, err
	}

	return v, nil
}

// SetMajor sets the major version (resets minor and patch to 0)
func SetMajor(deployDir string, major int) (*Version, error) {
	if major < 0 {
		return nil, fmt.Errorf("major version must be non-negative")
	}

	v := &Version{Major: major, Minor: 0, Patch: 0}
	if err := SaveVersion(deployDir, v); err != nil {
		return nil, err
	}

	return v, nil
}

// SetMinor sets the minor version (resets patch to 0)
func SetMinor(deployDir string, minor int) (*Version, error) {
	if minor < 0 {
		return nil, fmt.Errorf("minor version must be non-negative")
	}

	v, err := LoadVersion(deployDir)
	if err != nil {
		return nil, err
	}

	v.Minor = minor
	v.Patch = 0

	if err := SaveVersion(deployDir, v); err != nil {
		return nil, err
	}

	return v, nil
}

// GetCurrentBranch gets the current git branch name
func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git branch: %w", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("empty branch name")
	}

	return branch, nil
}

// sanitizeBranchName converts branch name to tag-safe format
// Replaces slashes and other special characters with dashes
func sanitizeBranchName(branch string) string {
	// Replace common branch separators with dashes
	sanitized := strings.ReplaceAll(branch, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")

	// Remove any other special characters
	sanitized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			return r
		}
		return '-'
	}, sanitized)

	// Remove leading/trailing dashes
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}
