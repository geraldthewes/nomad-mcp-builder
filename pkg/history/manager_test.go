package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"nomad-mcp-builder/pkg/types"
)

// Helper function to create a temporary test directory
func createTempDir(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return tmpDir
}

// Helper function to clean up test directory
func cleanupTempDir(t *testing.T, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("failed to cleanup temp dir: %v", err)
	}
}

func TestNewManager(t *testing.T) {
	t.Run("with default deploy dir", func(t *testing.T) {
		mgr, err := NewManager("")
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		if mgr == nil {
			t.Fatal("NewManager returned nil manager")
		}
		// Should use default "./deploy" and convert to absolute
		if !filepath.IsAbs(mgr.GetDeployDir()) {
			t.Errorf("deploy dir should be absolute, got: %s", mgr.GetDeployDir())
		}
	})

	t.Run("with relative path", func(t *testing.T) {
		mgr, err := NewManager("./my-deploy")
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		// Should convert to absolute path
		if !filepath.IsAbs(mgr.GetDeployDir()) {
			t.Errorf("deploy dir should be absolute, got: %s", mgr.GetDeployDir())
		}
	})

	t.Run("with absolute path", func(t *testing.T) {
		tmpDir := createTempDir(t)
		defer cleanupTempDir(t, tmpDir)

		mgr, err := NewManager(tmpDir)
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		if mgr.GetDeployDir() != tmpDir {
			t.Errorf("expected deploy dir %s, got %s", tmpDir, mgr.GetDeployDir())
		}
	})
}

func TestCreateBuildDirectory(t *testing.T) {
	tmpDir := createTempDir(t)
	defer cleanupTempDir(t, tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	jobID := "test-job-123"

	err = mgr.CreateBuildDirectory(jobID)
	if err != nil {
		t.Fatalf("CreateBuildDirectory failed: %v", err)
	}

	// Verify directory was created
	buildDir := mgr.GetBuildDir(jobID)
	info, err := os.Stat(buildDir)
	if err != nil {
		t.Fatalf("build directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("build directory is not a directory")
	}

	// Verify path structure
	expectedPath := filepath.Join(tmpDir, "builds", jobID)
	if buildDir != expectedPath {
		t.Errorf("expected build dir %s, got %s", expectedPath, buildDir)
	}
}

func TestWriteInitialMetadata(t *testing.T) {
	tmpDir := createTempDir(t)
	defer cleanupTempDir(t, tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	jobID := "test-job-456"
	if err := mgr.CreateBuildDirectory(jobID); err != nil {
		t.Fatalf("CreateBuildDirectory failed: %v", err)
	}

	config := types.JobConfig{
		Owner:      "test-user",
		RepoURL:    "https://github.com/test/repo.git",
		GitRef:     "refs/heads/main",
		ImageName:  "test-image",
		ImageTags:  []string{"latest"},
	}

	err = mgr.WriteInitialMetadata(jobID, config)
	if err != nil {
		t.Fatalf("WriteInitialMetadata failed: %v", err)
	}

	// Read and verify metadata file
	metadataPath := filepath.Join(mgr.GetBuildDir(jobID), "metadata.yaml")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}

	var metadata InitialMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	// Verify fields
	if metadata.JobID != jobID {
		t.Errorf("expected job_id %s, got %s", jobID, metadata.JobID)
	}
	if metadata.Status != types.StatusPending {
		t.Errorf("expected status PENDING, got %s", metadata.Status)
	}
	if metadata.Branch != "main" {
		t.Errorf("expected branch 'main', got %s", metadata.Branch)
	}
	if metadata.GitRef != "refs/heads/main" {
		t.Errorf("expected git_ref 'refs/heads/main', got %s", metadata.GitRef)
	}
	if metadata.JobConfig.Owner != "test-user" {
		t.Errorf("expected owner 'test-user', got %s", metadata.JobConfig.Owner)
	}
}

func TestWriteStatusFile(t *testing.T) {
	tmpDir := createTempDir(t)
	defer cleanupTempDir(t, tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	jobID := "test-job-789"
	if err := mgr.CreateBuildDirectory(jobID); err != nil {
		t.Fatalf("CreateBuildDirectory failed: %v", err)
	}

	now := time.Now()
	later := now.Add(5 * time.Minute)

	job := &types.Job{
		ID:         jobID,
		Status:     types.StatusSucceeded,
		StartedAt:  &now,
		FinishedAt: &later,
		Config: types.JobConfig{
			GitRef:      "feature/test-branch",
			ImageName:   "test-image",
			ImageTags:   []string{"v1.0.0", "latest"},
			RegistryURL: "registry.example.com",
		},
		Metrics: types.JobMetrics{
			BuildDuration:   2 * time.Minute,
			TestDuration:    1 * time.Minute,
			PublishDuration: 30 * time.Second,
			BuildStart:      &now,
			TestStart:       &now,
			PublishStart:    &now,
		},
	}

	err = mgr.WriteStatusFile(jobID, job)
	if err != nil {
		t.Fatalf("WriteStatusFile failed: %v", err)
	}

	// Read and verify status file
	statusPath := filepath.Join(mgr.GetBuildDir(jobID), "status.md")
	content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("failed to read status file: %v", err)
	}

	statusStr := string(content)

	// Verify key content
	if !strings.Contains(statusStr, "# Build Status: "+jobID) {
		t.Errorf("status file missing title")
	}
	if !strings.Contains(statusStr, "✅ SUCCEEDED") {
		t.Errorf("status file missing status indicator")
	}
	if !strings.Contains(statusStr, "**Branch**: feature/test-branch") {
		t.Errorf("status file missing branch info")
	}
	if !strings.Contains(statusStr, "## Phases") {
		t.Errorf("status file missing phases section")
	}
	if !strings.Contains(statusStr, "## Image") {
		t.Errorf("status file missing image section")
	}
}

func TestWritePhaseLogs(t *testing.T) {
	tmpDir := createTempDir(t)
	defer cleanupTempDir(t, tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	jobID := "test-job-log"
	if err := mgr.CreateBuildDirectory(jobID); err != nil {
		t.Fatalf("CreateBuildDirectory failed: %v", err)
	}

	testLogs := []string{
		"[INFO] Building image...",
		"[INFO] Step 1/5: FROM golang:1.22",
		"[INFO] Step 2/5: COPY . /app",
		"[SUCCESS] Build completed",
	}

	err = mgr.WritePhaseLogs(jobID, "build", testLogs)
	if err != nil {
		t.Fatalf("WritePhaseLogs failed: %v", err)
	}

	// Read and verify log file
	logPath := filepath.Join(mgr.GetBuildDir(jobID), "build.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	logStr := string(content)
	expectedContent := strings.Join(testLogs, "\n")
	if logStr != expectedContent {
		t.Errorf("log content mismatch.\nExpected:\n%s\nGot:\n%s", expectedContent, logStr)
	}
}

func TestUpdateHistoryFile(t *testing.T) {
	tmpDir := createTempDir(t)
	defer cleanupTempDir(t, tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create first entry
	entry1 := HistoryEntry{
		JobID:     "job-001",
		Branch:    "main",
		GitRef:    "refs/heads/main",
		Timestamp: time.Now().Add(-2 * time.Hour),
		Status:    types.StatusSucceeded,
		Duration:  5 * time.Minute,
		ImageTags: []string{"v1.0.0"},
	}

	err = mgr.UpdateHistoryFile(entry1)
	if err != nil {
		t.Fatalf("UpdateHistoryFile failed for first entry: %v", err)
	}

	// Create second entry (should be prepended)
	entry2 := HistoryEntry{
		JobID:     "job-002",
		Branch:    "feature/new-feature",
		GitRef:    "feature/new-feature",
		Timestamp: time.Now().Add(-1 * time.Hour),
		Status:    types.StatusFailed,
		Duration:  3 * time.Minute,
		ImageTags: []string{"latest"},
		Error:     "Build failed: compilation error",
	}

	err = mgr.UpdateHistoryFile(entry2)
	if err != nil {
		t.Fatalf("UpdateHistoryFile failed for second entry: %v", err)
	}

	// Read and verify history file
	historyPath := filepath.Join(tmpDir, "history.md")
	content, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("failed to read history file: %v", err)
	}

	historyStr := string(content)

	// Verify structure
	if !strings.Contains(historyStr, "# Build History") {
		t.Errorf("history file missing header. Content:\n%s", historyStr)
	}

	// Verify entry 2 appears before entry 1 (prepended)
	idx1 := strings.Index(historyStr, "## job-001")
	idx2 := strings.Index(historyStr, "## job-002")

	if idx2 == -1 {
		t.Errorf("entry 2 not found in history")
	}
	if idx1 == -1 {
		t.Errorf("entry 1 not found in history")
	}
	if idx2 > idx1 {
		t.Errorf("entry 2 should appear before entry 1 (prepended), but idx2=%d > idx1=%d", idx2, idx1)
	}

	// Verify content
	if !strings.Contains(historyStr, "**Branch**: feature/new-feature") {
		t.Errorf("entry 2 missing branch info")
	}
	if !strings.Contains(historyStr, "❌ FAILED") {
		t.Errorf("entry 2 missing failed status")
	}
	if !strings.Contains(historyStr, "**Error**: Build failed: compilation error") {
		t.Errorf("entry 2 missing error info")
	}
}

func TestExtractBranch(t *testing.T) {
	tests := []struct {
		gitRef   string
		expected string
	}{
		{"refs/heads/main", "main"},
		{"refs/heads/feature/my-feature", "feature/my-feature"},
		{"refs/tags/v1.0.0", "v1.0.0"},
		{"main", "main"},
		{"abc123def456", "abc123def456"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.gitRef, func(t *testing.T) {
			result := extractBranch(tt.gitRef)
			if result != tt.expected {
				t.Errorf("extractBranch(%q) = %q, expected %q", tt.gitRef, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{1 * time.Minute, "1m0s"},
		{1*time.Minute + 30*time.Second, "1m30s"},
		{2*time.Hour + 15*time.Minute + 45*time.Second, "2h15m45s"},
		{90 * time.Second, "1m30s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, expected %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestWriteCompleteMetadata(t *testing.T) {
	tmpDir := createTempDir(t)
	defer cleanupTempDir(t, tmpDir)

	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	jobID := "test-job-complete"
	if err := mgr.CreateBuildDirectory(jobID); err != nil {
		t.Fatalf("CreateBuildDirectory failed: %v", err)
	}

	submittedAt := time.Now().Add(-10 * time.Minute)
	startedAt := submittedAt.Add(1 * time.Minute)
	completedAt := startedAt.Add(5 * time.Minute)

	buildEnd := startedAt.Add(2 * time.Minute)
	testStart := startedAt.Add(2 * time.Minute)
	testEnd := startedAt.Add(4 * time.Minute)
	publishStart := startedAt.Add(4 * time.Minute)

	job := &types.Job{
		ID:         jobID,
		Status:     types.StatusSucceeded,
		StartedAt:  &startedAt,
		FinishedAt: &completedAt,
		Config: types.JobConfig{
			Owner:      "test-user",
			RepoURL:    "https://github.com/test/repo.git",
			GitRef:     "refs/heads/develop",
			ImageName:  "test-image",
			ImageTags:  []string{"develop", "latest"},
		},
		Metrics: types.JobMetrics{
			BuildStart:      &startedAt,
			BuildEnd:        &buildEnd,
			BuildDuration:   2 * time.Minute,
			TestStart:       &testStart,
			TestEnd:         &testEnd,
			TestDuration:    2 * time.Minute,
			PublishStart:    &publishStart,
			PublishEnd:      &completedAt,
			PublishDuration: 1 * time.Minute,
		},
	}

	err = mgr.WriteCompleteMetadata(jobID, job, submittedAt)
	if err != nil {
		t.Fatalf("WriteCompleteMetadata failed: %v", err)
	}

	// Read and verify metadata file
	metadataPath := filepath.Join(mgr.GetBuildDir(jobID), "metadata.yaml")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}

	var metadata CompleteMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	// Verify fields
	if metadata.JobID != jobID {
		t.Errorf("expected job_id %s, got %s", jobID, metadata.JobID)
	}
	if metadata.Status != types.StatusSucceeded {
		t.Errorf("expected status SUCCEEDED, got %s", metadata.Status)
	}
	if metadata.Branch != "develop" {
		t.Errorf("expected branch 'develop', got %s", metadata.Branch)
	}

	// Verify phase information
	if metadata.Phases["build"].Status != types.StatusSucceeded {
		t.Errorf("expected build phase status SUCCEEDED, got %s", metadata.Phases["build"].Status)
	}
	if metadata.Phases["build"].Duration != "2m0s" {
		t.Errorf("expected build duration '2m0s', got %s", metadata.Phases["build"].Duration)
	}
}
