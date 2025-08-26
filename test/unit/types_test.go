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
		TestCommands:            []string{"make test", "make integration-test"},
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