// +build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"nomad-mcp-builder/pkg/types"
)

// End-to-end integration test that runs a complete build pipeline
func TestE2EBuildPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}
	
	// Only run if E2E environment is available
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("E2E tests disabled. Set RUN_E2E_TESTS=true to enable")
	}
	
	cfg := getTestConfig(t)
	
	// Test with hello-world Docker image (simple test case)
	jobConfig := types.JobConfig{
		Owner:                   "e2e-test",
		RepoURL:                 "https://github.com/docker-library/hello-world.git",
		GitRef:                  "master",
		GitCredentialsPath:      "secret/test/git-creds",
		DockerfilePath:          "Dockerfile",
		ImageTags:               []string{"e2e-test"},
		RegistryURL:             "localhost:5000/hello-world",
		RegistryCredentialsPath: "secret/test/registry-creds",
		TestCommands:            []string{"/hello"}, // hello-world just runs /hello
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	// Submit job
	submitReq := types.SubmitJobRequest{
		JobConfig: jobConfig,
	}
	
	resp, err := makeHTTPRequest(cfg, "POST", "/mcp/submitJob", submitReq)
	if err != nil {
		t.Fatalf("Failed to submit E2E job: %v", err)
	}
	
	var submitResp types.SubmitJobResponse
	if err := json.Unmarshal(resp, &submitResp); err != nil {
		t.Fatalf("Failed to unmarshal submit response: %v", err)
	}
	
	jobID := submitResp.JobID
	t.Logf("Started E2E test with job ID: %s", jobID)
	
	// Poll for completion
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	var finalStatus types.JobStatus
	var finalError string
	
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("E2E test timeout after 10 minutes")
		case <-ticker.C:
			status, err := getJobStatus(cfg, jobID)
			if err != nil {
				t.Logf("Failed to get job status: %v", err)
				continue
			}
			
			t.Logf("Job %s status: %s", jobID, status.Status)
			
			// Log any errors
			if status.Error != "" && status.Error != finalError {
				t.Logf("Job error: %s", status.Error)
				finalError = status.Error
			}
			
			// Check if job is complete
			if status.Status == types.StatusSucceeded || status.Status == types.StatusFailed {
				finalStatus = status.Status
				goto completed
			}
		}
	}
	
completed:
	// Get final logs
	logs, err := getJobLogs(cfg, jobID)
	if err != nil {
		t.Logf("Failed to get final logs: %v", err)
	} else {
		t.Logf("Final logs - Build: %d lines, Test: %d lines, Publish: %d lines",
			len(logs.Build), len(logs.Test), len(logs.Publish))
		
		// Print some log samples for debugging
		if len(logs.Build) > 0 {
			t.Logf("Build log sample: %s", logs.Build[len(logs.Build)-1])
		}
		if len(logs.Test) > 0 {
			t.Logf("Test log sample: %s", logs.Test[len(logs.Test)-1])
		}
		if len(logs.Publish) > 0 {
			t.Logf("Publish log sample: %s", logs.Publish[len(logs.Publish)-1])
		}
	}
	
	// Verify final status
	if finalStatus != types.StatusSucceeded {
		t.Errorf("Expected job to succeed, but got status: %s (error: %s)", finalStatus, finalError)
	} else {
		t.Logf("E2E test completed successfully!")
	}
	
	// Cleanup - kill job if still running
	cleanupReq := types.CleanupRequest{
		JobID: jobID,
	}
	
	if _, err := makeHTTPRequest(cfg, "POST", "/mcp/cleanup", cleanupReq); err != nil {
		t.Logf("Cleanup warning: %v", err)
	}
}

func getJobStatus(cfg *config.Config, jobID string) (*types.GetStatusResponse, error) {
	statusReq := types.GetStatusRequest{
		JobID: jobID,
	}
	
	resp, err := makeHTTPRequest(cfg, "POST", "/mcp/getStatus", statusReq)
	if err != nil {
		return nil, err
	}
	
	var statusResp types.GetStatusResponse
	if err := json.Unmarshal(resp, &statusResp); err != nil {
		return nil, err
	}
	
	return &statusResp, nil
}

func getJobLogs(cfg *config.Config, jobID string) (*types.JobLogs, error) {
	logsReq := types.GetLogsRequest{
		JobID: jobID,
	}
	
	resp, err := makeHTTPRequest(cfg, "POST", "/mcp/getLogs", logsReq)
	if err != nil {
		return nil, err
	}
	
	var logsResp types.GetLogsResponse
	if err := json.Unmarshal(resp, &logsResp); err != nil {
		return nil, err
	}
	
	return &logsResp.Logs, nil
}

// Test job cancellation
func TestJobCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cancellation test in short mode")
	}
	
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("E2E tests disabled")
	}
	
	cfg := getTestConfig(t)
	
	// Submit a job that will run for a while
	jobConfig := types.JobConfig{
		Owner:                   "cancel-test",
		RepoURL:                 "https://github.com/docker-library/hello-world.git",
		GitRef:                  "master",
		GitCredentialsPath:      "secret/test/git-creds",
		DockerfilePath:          "Dockerfile",
		ImageTags:               []string{"cancel-test"},
		RegistryURL:             "localhost:5000/hello-world-cancel",
		RegistryCredentialsPath: "secret/test/registry-creds",
		TestCommands:            []string{"sleep 60"}, // Sleep to allow cancellation
	}
	
	submitReq := types.SubmitJobRequest{
		JobConfig: jobConfig,
	}
	
	resp, err := makeHTTPRequest(cfg, "POST", "/mcp/submitJob", submitReq)
	if err != nil {
		t.Fatalf("Failed to submit job for cancellation test: %v", err)
	}
	
	var submitResp types.SubmitJobResponse
	if err := json.Unmarshal(resp, &submitResp); err != nil {
		t.Fatalf("Failed to unmarshal submit response: %v", err)
	}
	
	jobID := submitResp.JobID
	t.Logf("Started cancellation test with job ID: %s", jobID)
	
	// Wait a moment for job to start
	time.Sleep(5 * time.Second)
	
	// Cancel the job
	killReq := types.KillJobRequest{
		JobID: jobID,
	}
	
	killResp, err := makeHTTPRequest(cfg, "POST", "/mcp/killJob", killReq)
	if err != nil {
		t.Fatalf("Failed to kill job: %v", err)
	}
	
	var killResponse types.KillJobResponse
	if err := json.Unmarshal(killResp, &killResponse); err != nil {
		t.Fatalf("Failed to unmarshal kill response: %v", err)
	}
	
	if !killResponse.Success {
		t.Errorf("Expected successful job kill, got: %s", killResponse.Message)
	} else {
		t.Logf("Job killed successfully: %s", killResponse.Message)
	}
	
	// Verify job is in failed state
	time.Sleep(2 * time.Second)
	
	status, err := getJobStatus(cfg, jobID)
	if err != nil {
		t.Fatalf("Failed to get job status after kill: %v", err)
	}
	
	if status.Status != types.StatusFailed {
		t.Errorf("Expected job status FAILED after kill, got: %s", status.Status)
	}
}