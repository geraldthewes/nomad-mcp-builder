package unit

import (
	"testing"
	"time"

	"nomad-mcp-builder/pkg/types"
)

func TestJobStatus(t *testing.T) {
	tests := []struct {
		status   types.JobStatus
		expected string
	}{
		{types.StatusPending, "PENDING"},
		{types.StatusBuilding, "BUILDING"},
		{types.StatusTesting, "TESTING"},
		{types.StatusPublishing, "PUBLISHING"},
		{types.StatusSucceeded, "SUCCEEDED"},
		{types.StatusFailed, "FAILED"},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

func TestJobConfigValidation(t *testing.T) {
	validConfig := types.JobConfig{
		Owner:                   "test-user",
		RepoURL:                 "https://github.com/test/repo.git",
		GitRef:                  "main",
		GitCredentialsPath:      "secret/git-creds",
		DockerfilePath:          "Dockerfile",
		ImageTags:               []string{"latest", "v1.0.0"},
		RegistryURL:             "docker.io/test/app",
		RegistryCredentialsPath: "secret/registry-creds",
		Test: &types.TestConfig{
			Commands: []string{"make test", "make integration-test"},
		},
	}
	
	// Test that valid config doesn't cause issues in creation
	job := &types.Job{
		ID:        "test-job-id",
		Config:    validConfig,
		Status:    types.StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	if job.ID != "test-job-id" {
		t.Errorf("Expected job ID 'test-job-id', got %s", job.ID)
	}
	
	if job.Status != types.StatusPending {
		t.Errorf("Expected status PENDING, got %s", job.Status)
	}
	
	if job.Config.Owner != "test-user" {
		t.Errorf("Expected owner 'test-user', got %s", job.Config.Owner)
	}
}

func TestJobMetricsCalculation(t *testing.T) {
	job := &types.Job{
		ID:     "test-job",
		Status: types.StatusSucceeded,
		Metrics: types.JobMetrics{
			BuildDuration:   5 * time.Minute,
			TestDuration:    2 * time.Minute,
			PublishDuration: 1 * time.Minute,
		},
	}
	
	// Calculate total duration
	totalDuration := job.Metrics.BuildDuration + job.Metrics.TestDuration + job.Metrics.PublishDuration
	expectedTotal := 8 * time.Minute
	
	if totalDuration != expectedTotal {
		t.Errorf("Expected total duration %v, got %v", expectedTotal, totalDuration)
	}
}

func TestMCPRequestResponseTypes(t *testing.T) {
	// Test SubmitJobRequest
	submitReq := types.SubmitJobRequest{
		JobConfig: types.JobConfig{
			Owner:   "test",
			RepoURL: "https://github.com/test/repo.git",
		},
	}
	
	if submitReq.JobConfig.Owner != "test" {
		t.Errorf("Expected owner 'test', got %s", submitReq.JobConfig.Owner)
	}
	
	// Test SubmitJobResponse
	submitResp := types.SubmitJobResponse{
		JobID:  "job-123",
		Status: types.StatusPending,
	}
	
	if submitResp.JobID != "job-123" {
		t.Errorf("Expected job ID 'job-123', got %s", submitResp.JobID)
	}
	
	// Test GetStatusResponse
	statusResp := types.GetStatusResponse{
		JobID:  "job-123",
		Status: types.StatusBuilding,
		Metrics: types.JobMetrics{
			BuildDuration: 1 * time.Minute,
		},
	}
	
	if statusResp.Status != types.StatusBuilding {
		t.Errorf("Expected status BUILDING, got %s", statusResp.Status)
	}
}

func TestResourceLimits(t *testing.T) {
	t.Run("Legacy global resource limits", func(t *testing.T) {
		limits := &types.ResourceLimits{
			CPU:    "2000",
			Memory: "4096",
			Disk:   "20480",
		}

		if limits.CPU != "2000" {
			t.Errorf("Expected CPU limit '2000', got %s", limits.CPU)
		}

		if limits.Memory != "4096" {
			t.Errorf("Expected Memory limit '4096', got %s", limits.Memory)
		}

		// Test that legacy limits apply to all phases
		defaults := types.PhaseResourceLimits{CPU: "500", Memory: "1024", Disk: "2048"}

		buildLimits := limits.GetBuildLimits(defaults)
		if buildLimits.CPU != "2000" {
			t.Errorf("Expected build CPU '2000', got %s", buildLimits.CPU)
		}
		if buildLimits.Memory != "4096" {
			t.Errorf("Expected build Memory '4096', got %s", buildLimits.Memory)
		}

		testLimits := limits.GetTestLimits(defaults)
		if testLimits.CPU != "2000" {
			t.Errorf("Expected test CPU '2000', got %s", testLimits.CPU)
		}

		publishLimits := limits.GetPublishLimits(defaults)
		if publishLimits.CPU != "2000" {
			t.Errorf("Expected publish CPU '2000', got %s", publishLimits.CPU)
		}
	})

	t.Run("Per-phase resource limits", func(t *testing.T) {
		limits := &types.ResourceLimits{
			Build: &types.PhaseResourceLimits{
				CPU:    "3000",
				Memory: "8192",
				Disk:   "40960",
			},
			Test: &types.PhaseResourceLimits{
				CPU:    "1500",
				Memory: "2048",
				Disk:   "5120",
			},
			Publish: &types.PhaseResourceLimits{
				CPU:    "800",
				Memory: "1024",
				Disk:   "2048",
			},
		}

		defaults := types.PhaseResourceLimits{CPU: "500", Memory: "1024", Disk: "2048"}

		buildLimits := limits.GetBuildLimits(defaults)
		if buildLimits.CPU != "3000" {
			t.Errorf("Expected build CPU '3000', got %s", buildLimits.CPU)
		}
		if buildLimits.Memory != "8192" {
			t.Errorf("Expected build Memory '8192', got %s", buildLimits.Memory)
		}

		testLimits := limits.GetTestLimits(defaults)
		if testLimits.CPU != "1500" {
			t.Errorf("Expected test CPU '1500', got %s", testLimits.CPU)
		}
		if testLimits.Memory != "2048" {
			t.Errorf("Expected test Memory '2048', got %s", testLimits.Memory)
		}

		publishLimits := limits.GetPublishLimits(defaults)
		if publishLimits.CPU != "800" {
			t.Errorf("Expected publish CPU '800', got %s", publishLimits.CPU)
		}
		if publishLimits.Memory != "1024" {
			t.Errorf("Expected publish Memory '1024', got %s", publishLimits.Memory)
		}
	})

	t.Run("Mixed legacy and per-phase limits", func(t *testing.T) {
		limits := &types.ResourceLimits{
			CPU:    "2000", // Legacy global
			Memory: "4096", // Legacy global
			Build: &types.PhaseResourceLimits{
				CPU: "4000", // Override just CPU for build
				// Memory and Disk will fall back to legacy values
			},
		}

		defaults := types.PhaseResourceLimits{CPU: "500", Memory: "1024", Disk: "2048"}

		buildLimits := limits.GetBuildLimits(defaults)
		if buildLimits.CPU != "4000" { // Per-phase override
			t.Errorf("Expected build CPU '4000', got %s", buildLimits.CPU)
		}
		if buildLimits.Memory != "4096" { // Legacy fallback
			t.Errorf("Expected build Memory '4096', got %s", buildLimits.Memory)
		}

		testLimits := limits.GetTestLimits(defaults)
		if testLimits.CPU != "2000" { // Legacy global
			t.Errorf("Expected test CPU '2000', got %s", testLimits.CPU)
		}
		if testLimits.Memory != "4096" { // Legacy global
			t.Errorf("Expected test Memory '4096', got %s", testLimits.Memory)
		}
	})

	t.Run("Nil resource limits", func(t *testing.T) {
		var limits *types.ResourceLimits = nil
		defaults := types.PhaseResourceLimits{CPU: "500", Memory: "1024", Disk: "2048"}

		buildLimits := limits.GetBuildLimits(defaults)
		if buildLimits.CPU != "500" {
			t.Errorf("Expected default CPU '500', got %s", buildLimits.CPU)
		}
		if buildLimits.Memory != "1024" {
			t.Errorf("Expected default Memory '1024', got %s", buildLimits.Memory)
		}
	})
}

func TestJobLogs(t *testing.T) {
	logs := types.JobLogs{
		Build:   []string{"Building image...", "Build completed"},
		Test:    []string{"Running tests...", "Tests passed"},
		Publish: []string{"Pushing to registry...", "Push completed"},
	}
	
	if len(logs.Build) != 2 {
		t.Errorf("Expected 2 build log entries, got %d", len(logs.Build))
	}
	
	if logs.Build[0] != "Building image..." {
		t.Errorf("Expected first build log entry, got %s", logs.Build[0])
	}
	
	totalLogEntries := len(logs.Build) + len(logs.Test) + len(logs.Publish)
	if totalLogEntries != 6 {
		t.Errorf("Expected 6 total log entries, got %d", totalLogEntries)
	}
}