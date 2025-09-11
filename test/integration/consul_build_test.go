package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nomad-mcp-builder/pkg/types"
)

// TestResult represents the result of a build and test
type TestResult struct {
	JobID        string            `json:"job_id"`
	BuildSuccess bool              `json:"build_success"`
	TestSuccess  bool              `json:"test_success"`
	BuildLogs    []string          `json:"build_logs"`
	TestLogs     []string          `json:"test_logs"`
	Error        string            `json:"error,omitempty"`
	Timestamp    map[string]string `json:"timestamp"`
	Duration     map[string]string `json:"duration"`
}

// TestBuildWorkflow tests the complete build workflow with service discovery
func TestBuildWorkflow(t *testing.T) {
	// Create results directory
	resultsDir := "test_results"
	err := os.MkdirAll(resultsDir, 0755)
	require.NoError(t, err, "Failed to create results directory")

	startTime := time.Now()
	result := TestResult{
		Timestamp: make(map[string]string),
		Duration:  make(map[string]string),
	}
	result.Timestamp["start"] = startTime.UTC().Format(time.RFC3339)

	// Step 1: Discover service URL via Consul
	t.Log("Discovering nomad-build-service via Consul...")
	serviceURL, err := discoverServiceURL()
	if err != nil {
		result.Error = fmt.Sprintf("Service discovery failed: %v", err)
		saveTestResult(t, resultsDir, result)
		t.Fatalf("Failed to discover service: %v", err)
	}
	t.Logf("Discovered service at: %s", serviceURL)

	// Step 2: Submit build job
	t.Log("Submitting build job...")
	jobConfig := types.JobConfig{
		Owner:           "test",
		RepoURL:         "https://github.com/geraldthewes/docker-build-hello-world.git",
		GitRef:          "main",
		DockerfilePath:  "Dockerfile",
		ImageTags:       []string{"hello-world-test"},
		RegistryURL:     "registry.cluster:5000/helloworld",
		TestCommands:    []string{}, // Empty to use entry point
		TestEntryPoint:  true,
	}

	jobID, err := submitJob(serviceURL, jobConfig)
	if err != nil {
		result.Error = fmt.Sprintf("Job submission failed: %v", err)
		saveTestResult(t, resultsDir, result)
		t.Fatalf("Failed to submit job: %v", err)
	}
	result.JobID = jobID
	t.Logf("Job submitted with ID: %s", jobID)

	// Step 3: Monitor job until completion
	t.Log("Monitoring job progress...")
	finalStatus, err := monitorJobUntilComplete(serviceURL, jobID, 10*time.Minute)
	if err != nil {
		result.Error = fmt.Sprintf("Job monitoring failed: %v", err)
		saveTestResult(t, resultsDir, result)
		t.Fatalf("Failed to monitor job: %v", err)
	}
	t.Logf("Job completed with status: %s", finalStatus)

	// Step 4: Retrieve logs and metrics (always try to get them, especially on failure)
	t.Log("Retrieving job logs...")
	logs, err := getJobLogs(serviceURL, jobID)
	if err != nil {
		result.Error = fmt.Sprintf("Log retrieval failed: %v", err)
		t.Logf("Warning: Failed to retrieve logs via API: %v", err)
		// Continue with test even if logs can't be retrieved via API
	} else {
		// Store logs in result
		result.BuildLogs = logs.Build
		result.TestLogs = logs.Test
	}
	
	// Get job status for metrics
	t.Log("Retrieving job metrics...")
	status, err := getJobStatus(serviceURL, jobID)
	var metrics *types.JobMetrics
	if err != nil {
		t.Logf("Warning: Failed to retrieve metrics via API: %v", err)
	} else {
		metrics = &status.Metrics
	}
		
	// If job failed, print the logs to help with debugging
	if finalStatus == types.StatusFailed {
		t.Log("=== JOB FAILED - DISPLAYING AVAILABLE LOGS ===")
		
		if len(result.BuildLogs) > 0 {
			t.Log("=== BUILD LOGS ===")
			for _, line := range result.BuildLogs {
				t.Logf("BUILD: %s", line)
			}
		} else {
			t.Log("No build logs available from service API")
		}
		
		if len(result.TestLogs) > 0 {
			t.Log("=== TEST LOGS ===")
			for _, line := range result.TestLogs {
				t.Logf("TEST: %s", line)
			}
		} else {
			t.Log("No test logs available from service API")
		}
		t.Log("=== END FAILURE LOGS ===")
	}

	// Step 5: Determine success/failure and calculate durations
	endTime := time.Now()
	result.BuildSuccess = len(result.BuildLogs) > 0 && finalStatus != types.StatusFailed
	result.TestSuccess = len(result.TestLogs) > 0 && finalStatus == types.StatusSucceeded
	result.Timestamp["job_end"] = endTime.UTC().Format(time.RFC3339)
	result.Duration["total"] = time.Since(startTime).String()
	
	// Get detailed timing from job metrics if available
	if metrics != nil {
		if metrics.JobStart != nil {
			result.Timestamp["job_start"] = metrics.JobStart.Format(time.RFC3339)
		}
		if metrics.BuildStart != nil {
			result.Timestamp["build_start"] = metrics.BuildStart.Format(time.RFC3339)
		}
		if metrics.BuildEnd != nil {
			result.Timestamp["build_end"] = metrics.BuildEnd.Format(time.RFC3339)
		}
		if metrics.TestStart != nil {
			result.Timestamp["test_start"] = metrics.TestStart.Format(time.RFC3339)
		}
		if metrics.TestEnd != nil {
			result.Timestamp["test_end"] = metrics.TestEnd.Format(time.RFC3339)
		}
		if metrics.PublishStart != nil {
			result.Timestamp["publish_start"] = metrics.PublishStart.Format(time.RFC3339)
		}
		if metrics.PublishEnd != nil {
			result.Timestamp["publish_end"] = metrics.PublishEnd.Format(time.RFC3339)
		}
		if metrics.JobEnd != nil {
			result.Timestamp["job_end"] = metrics.JobEnd.Format(time.RFC3339)
		}
		
		if metrics.BuildDuration > 0 {
			result.Duration["build"] = metrics.BuildDuration.String()
		}
		if metrics.TestDuration > 0 {
			result.Duration["test"] = metrics.TestDuration.String()
		}
		if metrics.PublishDuration > 0 {
			result.Duration["publish"] = metrics.PublishDuration.String()
		}
		if metrics.TotalDuration > 0 {
			result.Duration["total"] = metrics.TotalDuration.String()
		}
	}

	// Step 6: Save detailed logs to files
	if len(result.BuildLogs) > 0 {
		buildLogFile := filepath.Join(resultsDir, fmt.Sprintf("build_logs_%s.txt", jobID))
		err = saveLogsToFile(buildLogFile, result.BuildLogs)
		require.NoError(t, err, "Failed to save build logs")
		t.Logf("Build logs saved to: %s", buildLogFile)
	}

	if len(result.TestLogs) > 0 {
		testLogFile := filepath.Join(resultsDir, fmt.Sprintf("test_logs_%s.txt", jobID))
		err = saveLogsToFile(testLogFile, result.TestLogs)
		require.NoError(t, err, "Failed to save test logs")
		t.Logf("Test logs saved to: %s", testLogFile)
	}

	// Step 7: Save test result summary
	saveTestResult(t, resultsDir, result)

	// Step 8: Assert test results
	assert.True(t, result.BuildSuccess, "Build should succeed")
	assert.True(t, result.TestSuccess, "Test should succeed")
	assert.Equal(t, types.StatusSucceeded, finalStatus, "Job should complete successfully")
	assert.NotEmpty(t, result.BuildLogs, "Build logs should not be empty")

	t.Logf("Test completed successfully in %s", result.Duration)
}

// discoverServiceURL discovers the nomad-build-service URL via Consul
func discoverServiceURL() (string, error) {
	// Create Consul client
	config := consulapi.DefaultConfig()
	if addr := os.Getenv("CONSUL_HTTP_ADDR"); addr != "" {
		config.Address = addr
	} else {
		config.Address = "10.0.1.12:8500" // Default to cluster address
	}

	client, err := consulapi.NewClient(config)
	if err != nil {
		return "", fmt.Errorf("failed to create Consul client: %w", err)
	}

	// Query for nomad-build-service
	services, _, err := client.Catalog().Service("nomad-build-service", "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to query Consul catalog: %w", err)
	}

	if len(services) == 0 {
		return "", fmt.Errorf("nomad-build-service not found in Consul")
	}

	// Use the first healthy service
	service := services[0]
	serviceURL := fmt.Sprintf("http://%s:%d", service.ServiceAddress, service.ServicePort)
	return serviceURL, nil
}

// submitJob submits a build job to the service
func submitJob(serviceURL string, jobConfig types.JobConfig) (string, error) {
	submitReq := types.SubmitJobRequest{
		JobConfig: jobConfig,
	}

	reqBody, err := json.Marshal(submitReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(serviceURL+"/mcp/submitJob", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to submit job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("job submission failed with status %d: %s", resp.StatusCode, body)
	}

	var submitResp types.SubmitJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return submitResp.JobID, nil
}

// monitorJobUntilComplete polls job status until completion or timeout
func monitorJobUntilComplete(serviceURL, jobID string, timeout time.Duration) (types.JobStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return types.StatusFailed, fmt.Errorf("job monitoring timed out after %v", timeout)
		case <-ticker.C:
			url := fmt.Sprintf("%s/mcp/job/%s/status", serviceURL, jobID)
			resp, err := http.Get(url)
			if err != nil {
				continue // Retry on error
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue // Retry on error
			}

			var statusResp types.GetStatusResponse
			err = json.NewDecoder(resp.Body).Decode(&statusResp)
			resp.Body.Close()
			if err != nil {
				continue // Retry on error
			}

			// Check for terminal states
			switch statusResp.Status {
			case types.StatusSucceeded, types.StatusFailed:
				return statusResp.Status, nil
			default:
				// Continue monitoring
			}
		}
	}
}

// getJobLogs retrieves logs for a job using RESTful endpoint
func getJobLogs(serviceURL, jobID string) (types.JobLogs, error) {
	url := fmt.Sprintf("%s/mcp/job/%s/logs", serviceURL, jobID)
	resp, err := http.Get(url)
	if err != nil {
		return types.JobLogs{}, fmt.Errorf("failed to get logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return types.JobLogs{}, fmt.Errorf("logs retrieval failed with status %d: %s", resp.StatusCode, body)
	}

	var logsResp types.GetLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&logsResp); err != nil {
		return types.JobLogs{}, fmt.Errorf("failed to decode logs response: %w", err)
	}

	return logsResp.Logs, nil
}

// getJobStatus retrieves status and metrics for a job using RESTful endpoint
func getJobStatus(serviceURL, jobID string) (types.GetStatusResponse, error) {
	url := fmt.Sprintf("%s/mcp/job/%s/status", serviceURL, jobID)
	resp, err := http.Get(url)
	if err != nil {
		return types.GetStatusResponse{}, fmt.Errorf("failed to get status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return types.GetStatusResponse{}, fmt.Errorf("status retrieval failed with status %d: %s", resp.StatusCode, body)
	}

	var statusResp types.GetStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return types.GetStatusResponse{}, fmt.Errorf("failed to decode status response: %w", err)
	}

	return statusResp, nil
}

// saveLogsToFile saves log lines to a file
func saveLogsToFile(filename string, logs []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer file.Close()

	for _, line := range logs {
		if _, err := fmt.Fprintln(file, line); err != nil {
			return fmt.Errorf("failed to write log line: %w", err)
		}
	}

	return nil
}

// saveTestResult saves the test result summary as JSON
func saveTestResult(t *testing.T, resultsDir string, result TestResult) {
	resultFile := filepath.Join(resultsDir, fmt.Sprintf("test_result_%s.json", result.JobID))
	if result.JobID == "" {
		resultFile = filepath.Join(resultsDir, fmt.Sprintf("test_result_%d.json", time.Now().Unix()))
	}

	file, err := os.Create(resultFile)
	if err != nil {
		t.Logf("Failed to create result file: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		t.Logf("Failed to encode result: %v", err)
		return
	}

	t.Logf("Test result saved to: %s", resultFile)

	// Also log summary to console
	status := "FAILED"
	if result.BuildSuccess && result.TestSuccess {
		status = "PASSED"
	}

	t.Logf("=== TEST SUMMARY ===")
	t.Logf("Status: %s", status)
	t.Logf("Job ID: %s", result.JobID)
	t.Logf("Build Success: %t", result.BuildSuccess)
	t.Logf("Test Success: %t", result.TestSuccess)
	t.Logf("Duration: %s", result.Duration)
	if result.Error != "" {
		t.Logf("Error: %s", result.Error)
	}
}