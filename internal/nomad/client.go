package nomad

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	nomadapi "github.com/hashicorp/nomad/api"
	"github.com/sirupsen/logrus"
	
	"nomad-mcp-builder/internal/config"
	"nomad-mcp-builder/pkg/types"
)

// Client wraps the Nomad API client with build service specific functionality
type Client struct {
	client *nomadapi.Client
	config *config.Config
	logger *logrus.Logger
}

// NewClient creates a new Nomad client
func NewClient(cfg *config.Config) (*Client, error) {
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = cfg.Nomad.Address
	nomadConfig.SecretID = cfg.Nomad.Token
	nomadConfig.Region = cfg.Nomad.Region
	nomadConfig.Namespace = cfg.Nomad.Namespace
	
	client, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}
	
	logger := logrus.New()
	
	// Set log level from configuration
	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		logger.WithField("log_level", cfg.Logging.Level).Warn("Invalid log level in nomad client, using info")
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	
	return &Client{
		client: client,
		config: cfg,
		logger: logger,
	}, nil
}

// CreateJob creates a new build job and starts the build phase
func (nc *Client) CreateJob(jobConfig *types.JobConfig) (*types.Job, error) {
	jobID := uuid.New().String()
	now := time.Now()
	
	job := &types.Job{
		ID:        jobID,
		Config:    *jobConfig,
		Status:    types.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
		Logs:      types.JobLogs{},
		Metrics:   types.JobMetrics{},
	}
	
	// Create and submit the build job to Nomad
	buildJobSpec, err := nc.createBuildJobSpec(job)
	if err != nil {
		return nil, fmt.Errorf("failed to create build job spec: %w", err)
	}
	
	// Log the job specification for debugging
	nc.logJobSpec(buildJobSpec, "build")
	
	// Use proper WriteOptions and RegisterOpts to match CLI behavior
	registerOpts := &nomadapi.RegisterOptions{
		PolicyOverride: false,
		PreserveCounts: false,
	}
	writeOpts := &nomadapi.WriteOptions{
		Region:    nc.config.Nomad.Region,
		Namespace: nc.config.Nomad.Namespace,
	}
	
	evalID, _, err := nc.client.Jobs().RegisterOpts(buildJobSpec, registerOpts, writeOpts)
	if err != nil {
		// Check for specific Vault template errors and provide better feedback
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "Template failed") && strings.Contains(errorMsg, "vault.read: invalid format") {
			return nil, fmt.Errorf("vault template error: empty or invalid secret path provided. Please check that GitCredentialsPath and RegistryCredentialsPath are valid Vault paths or leave them empty if not needed. Original error: %w", err)
		}
		if strings.Contains(errorMsg, "Template failed") && strings.Contains(errorMsg, "vault.read") {
			return nil, fmt.Errorf("vault template error: failed to read secret from Vault. Please verify the secret path exists and the service has proper permissions. Original error: %w", err)
		}
		return nil, fmt.Errorf("failed to submit build job to Nomad: %w", err)
	}
	
	job.BuildJobID = *buildJobSpec.ID
	job.Status = types.StatusBuilding
	job.StartedAt = &now
	
	nc.logger.WithFields(logrus.Fields{
		"job_id":      jobID,
		"nomad_job":   *buildJobSpec.ID,
		"eval_id":     evalID,
	}).Info("Build job submitted to Nomad")
	
	return job, nil
}

// UpdateJobStatus updates the job status by querying Nomad
func (nc *Client) UpdateJobStatus(job *types.Job) (*types.Job, error) {
	// Check build job status
	if job.BuildJobID != "" {
		buildStatus, err := nc.getJobStatus(job.BuildJobID)
		if err != nil {
			return job, fmt.Errorf("failed to get build job status: %w", err)
		}
		
		switch buildStatus {
		case "running":
			job.Status = types.StatusBuilding
		case "complete":
			// Build completed, capture logs before they disappear
			if err := nc.capturePhaseLogs(job, "build"); err != nil {
				nc.logger.WithError(err).Warn("Failed to capture build logs")
			}
			
			// Start test phase
			if job.Status == types.StatusBuilding {
				if err := nc.startTestPhase(job); err != nil {
					job.Status = types.StatusFailed
					job.Error = fmt.Sprintf("Failed to start test phase: %v", err)
					now := time.Now()
					job.FinishedAt = &now
				} else {
					job.Status = types.StatusTesting
				}
			}
		case "failed":
			// Capture logs from failed build before they disappear
			if err := nc.capturePhaseLogs(job, "build"); err != nil {
				nc.logger.WithError(err).Warn("Failed to capture failed build logs")
			}
			
			job.Status = types.StatusFailed
			job.FailedPhase = "build"
			// Get detailed error information
			errorMsg, err := nc.getJobErrorDetails(job.BuildJobID)
			if err != nil {
				job.Error = "Build phase failed - unable to get error details"
			} else {
				job.Error = fmt.Sprintf("Build phase failed: %s", errorMsg)
			}
			now := time.Now()
			job.FinishedAt = &now
		}
	}
	
	// Check test job status
	if job.TestJobID != "" {
		testStatus, err := nc.getJobStatus(job.TestJobID)
		if err != nil {
			return job, fmt.Errorf("failed to get test job status: %w", err)
		}
		
		switch testStatus {
		case "running":
			job.Status = types.StatusTesting
		case "complete":
			// Tests completed, capture logs before they disappear
			if err := nc.capturePhaseLogs(job, "test"); err != nil {
				nc.logger.WithError(err).Warn("Failed to capture test logs")
			}
			
			// Start publish phase
			if job.Status == types.StatusTesting {
				if err := nc.startPublishPhase(job); err != nil {
					job.Status = types.StatusFailed
					job.Error = fmt.Sprintf("Failed to start publish phase: %v", err)
					now := time.Now()
					job.FinishedAt = &now
				} else {
					job.Status = types.StatusPublishing
				}
			}
		case "failed":
			// Capture logs from failed test before they disappear
			if err := nc.capturePhaseLogs(job, "test"); err != nil {
				nc.logger.WithError(err).Warn("Failed to capture failed test logs")
			}
			
			job.Status = types.StatusFailed
			job.FailedPhase = "test"
			// Get detailed error information
			errorMsg, err := nc.getJobErrorDetails(job.TestJobID)
			if err != nil {
				job.Error = "Test phase failed - unable to get error details"
			} else {
				job.Error = fmt.Sprintf("Test phase failed: %s", errorMsg)
			}
			now := time.Now()
			job.FinishedAt = &now
		}
	}
	
	// Check publish job status
	if job.PublishJobID != "" {
		publishStatus, err := nc.getJobStatus(job.PublishJobID)
		if err != nil {
			return job, fmt.Errorf("failed to get publish job status: %w", err)
		}
		
		switch publishStatus {
		case "running":
			job.Status = types.StatusPublishing
		case "complete":
			// Publish completed, capture logs before they disappear
			if err := nc.capturePhaseLogs(job, "publish"); err != nil {
				nc.logger.WithError(err).Warn("Failed to capture publish logs")
			}
			
			job.Status = types.StatusSucceeded
			now := time.Now()
			job.FinishedAt = &now
		case "failed":
			// Capture logs from failed publish before they disappear
			if err := nc.capturePhaseLogs(job, "publish"); err != nil {
				nc.logger.WithError(err).Warn("Failed to capture failed publish logs")
			}
			
			job.Status = types.StatusFailed
			job.FailedPhase = "publish"
			// Get detailed error information
			errorMsg, err := nc.getJobErrorDetails(job.PublishJobID)
			if err != nil {
				job.Error = "Publish phase failed - unable to get error details"
			} else {
				job.Error = fmt.Sprintf("Publish phase failed: %s", errorMsg)
			}
			now := time.Now()
			job.FinishedAt = &now
		}
	}
	
	job.UpdatedAt = time.Now()
	return job, nil
}

// GetJobLogs retrieves logs from Nomad for all phases
func (nc *Client) GetJobLogs(job *types.Job) (types.JobLogs, error) {
	logs := types.JobLogs{
		Build:   []string{},
		Test:    []string{},
		Publish: []string{},
	}
	
	// Get build logs
	if job.BuildJobID != "" {
		buildLogs, err := nc.getJobLogs(job.BuildJobID)
		if err != nil {
			nc.logger.WithError(err).Warn("Failed to get build logs")
		} else {
			logs.Build = buildLogs
		}
	}
	
	// Get test logs
	if job.TestJobID != "" {
		testLogs, err := nc.getJobLogs(job.TestJobID)
		if err != nil {
			nc.logger.WithError(err).Warn("Failed to get test logs")
		} else {
			logs.Test = testLogs
		}
	}
	
	// Get publish logs
	if job.PublishJobID != "" {
		publishLogs, err := nc.getJobLogs(job.PublishJobID)
		if err != nil {
			nc.logger.WithError(err).Warn("Failed to get publish logs")
		} else {
			logs.Publish = publishLogs
		}
	}
	
	return logs, nil
}

// KillJob terminates a running job
func (nc *Client) KillJob(job *types.Job) error {
	var errors []string
	
	// Kill build job if running
	if job.BuildJobID != "" {
		if _, _, err := nc.client.Jobs().Deregister(job.BuildJobID, true, nil); err != nil {
			errors = append(errors, fmt.Sprintf("build: %v", err))
		}
	}
	
	// Kill test job if running
	if job.TestJobID != "" {
		if _, _, err := nc.client.Jobs().Deregister(job.TestJobID, true, nil); err != nil {
			errors = append(errors, fmt.Sprintf("test: %v", err))
		}
	}
	
	// Kill publish job if running
	if job.PublishJobID != "" {
		if _, _, err := nc.client.Jobs().Deregister(job.PublishJobID, true, nil); err != nil {
			errors = append(errors, fmt.Sprintf("publish: %v", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("failed to kill some jobs: %s", strings.Join(errors, ", "))
	}
	
	return nil
}

// CleanupJob cleans up resources for a job
func (nc *Client) CleanupJob(job *types.Job) error {
	// First kill the job if it's still running
	if err := nc.KillJob(job); err != nil {
		nc.logger.WithError(err).Warn("Failed to kill job during cleanup")
	}
	
	// Clean up any temporary registry images
	if err := nc.cleanupTempImages(job); err != nil {
		nc.logger.WithError(err).Warn("Failed to cleanup temporary images")
	}
	
	return nil
}

// CleanupFailedJobs removes dead/failed jobs from Nomad after capturing their details
func (nc *Client) CleanupFailedJobs(job *types.Job) error {
	var errors []string
	
	// Clean up build job if it's dead/failed
	if job.BuildJobID != "" {
		if status, err := nc.getJobStatus(job.BuildJobID); err == nil && (status == "failed" || status == "dead") {
			// Purge (force remove) the dead job from Nomad
			if _, _, err := nc.client.Jobs().Deregister(job.BuildJobID, true, &nomadapi.WriteOptions{}); err != nil {
				errors = append(errors, fmt.Sprintf("build job cleanup: %v", err))
			} else {
				nc.logger.WithField("job_id", job.BuildJobID).Info("Cleaned up failed build job from Nomad")
			}
		}
	}
	
	// Clean up test job if it's dead/failed
	if job.TestJobID != "" {
		if status, err := nc.getJobStatus(job.TestJobID); err == nil && (status == "failed" || status == "dead") {
			if _, _, err := nc.client.Jobs().Deregister(job.TestJobID, true, &nomadapi.WriteOptions{}); err != nil {
				errors = append(errors, fmt.Sprintf("test job cleanup: %v", err))
			} else {
				nc.logger.WithField("job_id", job.TestJobID).Info("Cleaned up failed test job from Nomad")
			}
		}
	}
	
	// Clean up publish job if it's dead/failed  
	if job.PublishJobID != "" {
		if status, err := nc.getJobStatus(job.PublishJobID); err == nil && (status == "failed" || status == "dead") {
			if _, _, err := nc.client.Jobs().Deregister(job.PublishJobID, true, &nomadapi.WriteOptions{}); err != nil {
				errors = append(errors, fmt.Sprintf("publish job cleanup: %v", err))
			} else {
				nc.logger.WithField("job_id", job.PublishJobID).Info("Cleaned up failed publish job from Nomad")
			}
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("failed to cleanup some jobs: %s", strings.Join(errors, ", "))
	}
	
	return nil
}

// Health checks the health of the Nomad connection
func (nc *Client) Health() error {
	_, err := nc.client.Status().Leader()
	if err != nil {
		return fmt.Errorf("nomad health check failed: %w", err)
	}
	return nil
}

// Private helper methods

func (nc *Client) getJobStatus(nomadJobID string) (string, error) {
	job, _, err := nc.client.Jobs().Info(nomadJobID, nil)
	if err != nil {
		return "", err
	}
	
	// Check allocations for more detailed status
	allocs, _, err := nc.client.Jobs().Allocations(nomadJobID, false, nil)
	if err != nil {
		// If we can't get allocations but job exists, something is wrong
		nc.logger.WithError(err).WithField("job_id", nomadJobID).Warn("Failed to get job allocations")
	}
	
	// Handle case where job is dead with no allocations (scheduling failure)
	if *job.Status == "dead" && (allocs == nil || len(allocs) == 0) {
		return "failed", nil
	}
	
	if err == nil && len(allocs) > 0 {
		// Check if any allocation failed
		for _, alloc := range allocs {
			if alloc.ClientStatus == "failed" {
				return "failed", nil
			}
			// Check task states for more granular failure detection
			if alloc.TaskStates != nil {
				for _, taskState := range alloc.TaskStates {
					if taskState.State == "dead" && taskState.Failed {
						return "failed", nil
					}
				}
			}
		}
		
		// If we have allocations, check their status
		hasRunning := false
		allComplete := true
		for _, alloc := range allocs {
			switch alloc.ClientStatus {
			case "running":
				hasRunning = true
				allComplete = false
			case "pending":
				allComplete = false
			case "complete":
				// Keep checking others
			default:
				allComplete = false
			}
		}
		
		if hasRunning {
			return "running", nil
		}
		if allComplete {
			return "complete", nil
		}
	}
	
	// Fall back to basic job status
	switch *job.Status {
	case "pending":
		return "pending", nil
	case "running":
		return "running", nil
	case "complete":
		return "complete", nil
	case "failed", "cancelled":
		return "failed", nil
	default:
		return "unknown", nil
	}
}

func (nc *Client) getJobLogs(nomadJobID string) ([]string, error) {
	// Get allocations for the job
	allocs, _, err := nc.client.Jobs().Allocations(nomadJobID, false, nil)
	if err != nil {
		return nil, err
	}
	
	var logs []string
	for _, alloc := range allocs {
		// Get full allocation details
		allocDetail, _, err := nc.client.Allocations().Info(alloc.ID, nil)
		if err != nil {
			continue
		}
		
		// Try to get logs using ReadAt (this is a simplified approach)
		logReader, err := nc.client.AllocFS().ReadAt(allocDetail, "/alloc/logs/main.stdout.0", 0, 0, nil)
		if err != nil {
			continue
		}
		defer logReader.Close()
		
		// Read the log data
		buffer := make([]byte, 4096)
		n, err := logReader.Read(buffer)
		if err == nil && n > 0 {
			logs = append(logs, string(buffer[:n]))
		}
	}
	
	return logs, nil
}

func (nc *Client) getJobErrorDetails(nomadJobID string) (string, error) {
	// Check for scheduling failures
	allocs, _, err := nc.client.Jobs().Allocations(nomadJobID, false, nil)
	if err != nil {
		return "Failed to get job allocations", nil
	}
	
	// If no allocations, job failed to schedule
	if len(allocs) == 0 {
		// Get job evaluations to understand why it failed to schedule
		evals, _, err := nc.client.Jobs().Evaluations(nomadJobID, nil)
		if err == nil && len(evals) > 0 {
			for _, eval := range evals {
				if eval.Status == "blocked" || eval.Status == "failed" {
					return fmt.Sprintf("Job failed to schedule: %s", eval.StatusDescription), nil
				}
			}
		}
		return "Job failed to schedule - no allocations placed", nil
	}
	
	// Check allocation failures
	var errorMessages []string
	for _, alloc := range allocs {
		if alloc.ClientStatus == "failed" {
			// Get task states for detailed error info
			if alloc.TaskStates != nil {
				for taskName, taskState := range alloc.TaskStates {
					if taskState.Failed {
						// Get the most recent failed event
						if len(taskState.Events) > 0 {
							lastEvent := taskState.Events[len(taskState.Events)-1]
							errorMessages = append(errorMessages, 
								fmt.Sprintf("Task '%s': %s - %s", taskName, lastEvent.Type, lastEvent.DisplayMessage))
						} else {
							errorMessages = append(errorMessages, 
								fmt.Sprintf("Task '%s' failed", taskName))
						}
					}
				}
			}
		}
	}
	
	if len(errorMessages) > 0 {
		return fmt.Sprintf("%s", errorMessages[0]), nil // Return first error for brevity
	}
	
	return "Unknown failure", nil
}

func (nc *Client) startTestPhase(job *types.Job) error {
	if len(job.Config.TestCommands) == 0 && !job.Config.TestEntryPoint {
		// No tests configured, skip to publish phase
		nc.logger.WithField("job_id", job.ID).Info("No tests configured, skipping test phase")
		return nc.startPublishPhase(job)
	}
	
	testJobSpec, err := nc.createTestJobSpec(job)
	if err != nil {
		return fmt.Errorf("failed to create test job spec: %w", err)
	}
	
	// Log the job specification for debugging
	nc.logJobSpec(testJobSpec, "test")
	
	// Use proper WriteOptions and RegisterOpts to match CLI behavior
	registerOpts := &nomadapi.RegisterOptions{
		PolicyOverride: false,
		PreserveCounts: false,
	}
	writeOpts := &nomadapi.WriteOptions{
		Region:    nc.config.Nomad.Region,
		Namespace: nc.config.Nomad.Namespace,
	}
	
	evalID, _, err := nc.client.Jobs().RegisterOpts(testJobSpec, registerOpts, writeOpts)
	if err != nil {
		// Check for specific Vault template errors and provide better feedback
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "Template failed") && strings.Contains(errorMsg, "vault.read: invalid format") {
			return fmt.Errorf("vault template error in test job: empty or invalid secret path provided. Please check that RegistryCredentialsPath is a valid Vault path or leave it empty if not needed. Original error: %w", err)
		}
		if strings.Contains(errorMsg, "Template failed") && strings.Contains(errorMsg, "vault.read") {
			return fmt.Errorf("vault template error in test job: failed to read secret from Vault. Please verify the secret path exists and the service has proper permissions. Original error: %w", err)
		}
		return fmt.Errorf("failed to submit test job to Nomad: %w", err)
	}
	
	job.TestJobID = *testJobSpec.ID
	
	nc.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"nomad_job":   *testJobSpec.ID,
		"eval_id":     evalID,
	}).Info("Test job submitted to Nomad")
	
	return nil
}

func (nc *Client) startPublishPhase(job *types.Job) error {
	publishJobSpec, err := nc.createPublishJobSpec(job)
	if err != nil {
		return fmt.Errorf("failed to create publish job spec: %w", err)
	}
	
	// Log the job specification for debugging
	nc.logJobSpec(publishJobSpec, "publish")
	
	// Use proper WriteOptions and RegisterOpts to match CLI behavior
	registerOpts := &nomadapi.RegisterOptions{
		PolicyOverride: false,
		PreserveCounts: false,
	}
	writeOpts := &nomadapi.WriteOptions{
		Region:    nc.config.Nomad.Region,
		Namespace: nc.config.Nomad.Namespace,
	}
	
	evalID, _, err := nc.client.Jobs().RegisterOpts(publishJobSpec, registerOpts, writeOpts)
	if err != nil {
		// Check for specific Vault template errors and provide better feedback
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "Template failed") && strings.Contains(errorMsg, "vault.read: invalid format") {
			return fmt.Errorf("vault template error in publish job: empty or invalid secret path provided. Please check that RegistryCredentialsPath is a valid Vault path or leave it empty if not needed. Original error: %w", err)
		}
		if strings.Contains(errorMsg, "Template failed") && strings.Contains(errorMsg, "vault.read") {
			return fmt.Errorf("vault template error in publish job: failed to read secret from Vault. Please verify the secret path exists and the service has proper permissions. Original error: %w", err)
		}
		return fmt.Errorf("failed to submit publish job to Nomad: %w", err)
	}
	
	job.PublishJobID = *publishJobSpec.ID
	
	nc.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"nomad_job":   *publishJobSpec.ID,
		"eval_id":     evalID,
	}).Info("Publish job submitted to Nomad")
	
	return nil
}

func (nc *Client) cleanupTempImages(job *types.Job) error {
	// Create a cleanup job to remove temporary images from registry
	cleanupJobSpec := nc.createCleanupJobSpec(job)
	
	// Log the job specification for debugging
	nc.logJobSpec(cleanupJobSpec, "cleanup")
	
	// Use proper WriteOptions and RegisterOpts to match CLI behavior
	registerOpts := &nomadapi.RegisterOptions{
		PolicyOverride: false,
		PreserveCounts: false,
	}
	writeOpts := &nomadapi.WriteOptions{
		Region:    nc.config.Nomad.Region,
		Namespace: nc.config.Nomad.Namespace,
	}
	
	evalID, _, err := nc.client.Jobs().RegisterOpts(cleanupJobSpec, registerOpts, writeOpts)
	if err != nil {
		return fmt.Errorf("failed to submit cleanup job to Nomad: %w", err)
	}
	
	nc.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"nomad_job":   *cleanupJobSpec.ID,
		"eval_id":     evalID,
	}).Info("Cleanup job submitted to Nomad")
	
	return nil
}

// logJobSpec logs the job specification based on configuration settings
func (nc *Client) logJobSpec(jobSpec *nomadapi.Job, phase string) {
	if !nc.config.Logging.LogJobSpecs {
		return
	}

	logEntry := nc.logger.WithFields(logrus.Fields{
		"nomad_job_id": *jobSpec.ID,
		"phase":        phase,
	})

	if nc.config.Logging.LogHCLFormat {
		// Convert job spec to HCL-style format for easier debugging
		hcl, err := nc.JobSpecToHCL(jobSpec)
		if err != nil {
			logEntry.WithError(err).Warn("Failed to convert job spec to HCL format")
		} else {
			// Encode as base64 to avoid escaping hell
			encoded := base64.StdEncoding.EncodeToString([]byte(hcl))
			logEntry.WithField("job_spec_hcl_b64", encoded).Debug("Generated Nomad job specification (HCL base64 encoded)")
			
			// Also log decode instructions
			logEntry.Info("To decode HCL: echo 'BASE64_STRING' | base64 -d > job.hcl")
		}
	} else {
		// Log as JSON for programmatic consumption
		jsonSpec, err := json.MarshalIndent(jobSpec, "", "  ")
		if err != nil {
			logEntry.WithError(err).Warn("Failed to marshal job spec to JSON")
		} else {
			// Encode JSON as base64 too for consistency
			encoded := base64.StdEncoding.EncodeToString(jsonSpec)
			logEntry.WithField("job_spec_json_b64", encoded).Debug("Generated Nomad job specification (JSON base64 encoded)")
			
			// Also log decode instructions  
			logEntry.Info("To decode JSON: echo 'BASE64_STRING' | base64 -d | jq")
		}
	}
}

// JobSpecToHCL converts a job specification to HCL-like format for debugging
func (nc *Client) JobSpecToHCL(jobSpec *nomadapi.Job) (string, error) {
	var hcl strings.Builder
	
	hcl.WriteString(fmt.Sprintf("job \"%s\" {\n", *jobSpec.ID))
	hcl.WriteString(fmt.Sprintf("  name      = \"%s\"\n", *jobSpec.Name))
	hcl.WriteString(fmt.Sprintf("  type      = \"%s\"\n", *jobSpec.Type))
	hcl.WriteString(fmt.Sprintf("  namespace = \"%s\"\n", *jobSpec.Namespace))
	hcl.WriteString(fmt.Sprintf("  region    = \"%s\"\n", *jobSpec.Region))
	
	if len(jobSpec.Datacenters) > 0 {
		hcl.WriteString("  datacenters = [")
		for i, dc := range jobSpec.Datacenters {
			if i > 0 {
				hcl.WriteString(", ")
			}
			hcl.WriteString(fmt.Sprintf("\"%s\"", dc))
		}
		hcl.WriteString("]\n")
	}
	
	// Add meta fields
	if len(jobSpec.Meta) > 0 {
		hcl.WriteString("\n  meta {\n")
		for key, value := range jobSpec.Meta {
			hcl.WriteString(fmt.Sprintf("    %s = \"%s\"\n", key, value))
		}
		hcl.WriteString("  }\n")
	}
	
	// Add task groups
	for _, tg := range jobSpec.TaskGroups {
		hcl.WriteString(fmt.Sprintf("\n  group \"%s\" {\n", *tg.Name))
		hcl.WriteString(fmt.Sprintf("    count = %d\n", *tg.Count))
		
		// Add restart policy
		if tg.RestartPolicy != nil {
			hcl.WriteString("\n    restart {\n")
			hcl.WriteString(fmt.Sprintf("      attempts = %d\n", *tg.RestartPolicy.Attempts))
			hcl.WriteString("    }\n")
		}
		
		// Add network configuration
		if len(tg.Networks) > 0 {
			for _, network := range tg.Networks {
				hcl.WriteString("\n    network {\n")
				if network.Mode != "" {
					hcl.WriteString(fmt.Sprintf("      mode = \"%s\"\n", network.Mode))
				}
				hcl.WriteString("    }\n")
			}
		}
		
		// Add ephemeral disk
		if tg.EphemeralDisk != nil {
			hcl.WriteString("\n    ephemeral_disk {\n")
			hcl.WriteString(fmt.Sprintf("      size = %d\n", *tg.EphemeralDisk.SizeMB))
			hcl.WriteString("    }\n")
		}
		
		// Add tasks
		for _, task := range tg.Tasks {
			hcl.WriteString(fmt.Sprintf("\n    task \"%s\" {\n", task.Name))
			hcl.WriteString(fmt.Sprintf("      driver = \"%s\"\n", task.Driver))
			
			// Add task config
			if len(task.Config) > 0 {
				hcl.WriteString("\n      config {\n")
				for key, value := range task.Config {
					switch v := value.(type) {
					case string:
						// Handle multi-line strings properly with heredocs
						if strings.Contains(v, "\n") {
							hcl.WriteString(fmt.Sprintf("        %s = <<EOF\n%s\nEOF\n", key, v))
						} else {
							// Escape quotes in single-line strings
							escaped := strings.ReplaceAll(v, "\"", "\\\"")
							hcl.WriteString(fmt.Sprintf("        %s = \"%s\"\n", key, escaped))
						}
					case []string:
						hcl.WriteString(fmt.Sprintf("        %s = [\n", key))
						for i, item := range v {
							// Handle multi-line items in arrays with proper escaping
							if strings.Contains(item, "\n") {
								hcl.WriteString("          <<EOF\n")
								hcl.WriteString(item)
								hcl.WriteString("\nEOF")
								if i < len(v)-1 {
									hcl.WriteString(",")
								}
								hcl.WriteString("\n")
							} else {
								// Escape quotes for single-line items
								escaped := strings.ReplaceAll(item, "\"", "\\\"")
								hcl.WriteString(fmt.Sprintf("          \"%s\"", escaped))
								if i < len(v)-1 {
									hcl.WriteString(",")
								}
								hcl.WriteString("\n")
							}
						}
						hcl.WriteString("        ]\n")
					case bool:
						hcl.WriteString(fmt.Sprintf("        %s = %t\n", key, v))
					default:
						// Convert to JSON for complex types
						jsonVal, _ := json.Marshal(v)
						hcl.WriteString(fmt.Sprintf("        %s = %s\n", key, string(jsonVal)))
					}
				}
				hcl.WriteString("      }\n")
			}
			
			// Add environment variables
			if len(task.Env) > 0 {
				hcl.WriteString("\n      env {\n")
				for key, value := range task.Env {
					hcl.WriteString(fmt.Sprintf("        %s = \"%s\"\n", key, value))
				}
				hcl.WriteString("      }\n")
			}
			
			// Add resources
			if task.Resources != nil {
				hcl.WriteString("\n      resources {\n")
				if task.Resources.CPU != nil {
					hcl.WriteString(fmt.Sprintf("        cpu    = %d\n", *task.Resources.CPU))
				}
				if task.Resources.MemoryMB != nil {
					hcl.WriteString(fmt.Sprintf("        memory = %d\n", *task.Resources.MemoryMB))
				}
				if task.Resources.DiskMB != nil {
					hcl.WriteString(fmt.Sprintf("        disk   = %d\n", *task.Resources.DiskMB))
				}
				hcl.WriteString("      }\n")
			}
			
			// Add Vault configuration
			if task.Vault != nil {
				hcl.WriteString("\n      vault {\n")
				if len(task.Vault.Policies) > 0 {
					hcl.WriteString("        policies = [")
					for i, policy := range task.Vault.Policies {
						if i > 0 {
							hcl.WriteString(", ")
						}
						hcl.WriteString(fmt.Sprintf("\"%s\"", policy))
					}
					hcl.WriteString("]\n")
				}
				if task.Vault.ChangeMode != nil {
					hcl.WriteString(fmt.Sprintf("        change_mode = \"%s\"\n", *task.Vault.ChangeMode))
				}
				if task.Vault.Role != "" {
					hcl.WriteString(fmt.Sprintf("        role = \"%s\"\n", task.Vault.Role))
				}
				hcl.WriteString("      }\n")
			}
			
			// Add templates
			if len(task.Templates) > 0 {
				for _, template := range task.Templates {
					hcl.WriteString("\n      template {\n")
					if template.DestPath != nil {
						hcl.WriteString(fmt.Sprintf("        destination = \"%s\"\n", *template.DestPath))
					}
					if template.ChangeMode != nil {
						hcl.WriteString(fmt.Sprintf("        change_mode = \"%s\"\n", *template.ChangeMode))
					}
					if template.EmbeddedTmpl != nil {
						// Use heredoc for template data to handle multi-line content
						hcl.WriteString("        data = <<EOF\n")
						hcl.WriteString(*template.EmbeddedTmpl)
						hcl.WriteString("\nEOF\n")
					}
					hcl.WriteString("      }\n")
				}
			}
			
			hcl.WriteString("    }\n")
		}
		
		hcl.WriteString("  }\n")
	}
	
	hcl.WriteString("}\n")
	
	return hcl.String(), nil
}

// capturePhaseLogs captures logs from a completed phase and stores them persistently
func (nc *Client) capturePhaseLogs(job *types.Job, phase string) error {
	var jobID string
	
	switch phase {
	case "build":
		jobID = job.BuildJobID
	case "test":
		jobID = job.TestJobID
	case "publish":
		jobID = job.PublishJobID
	default:
		return fmt.Errorf("unknown phase: %s", phase)
	}
	
	if jobID == "" {
		return nil // No job to capture logs from
	}
	
	// Get logs from the completed job
	logs, err := nc.getJobLogs(jobID)
	if err != nil {
		return fmt.Errorf("failed to get logs for %s phase: %w", phase, err)
	}
	
	// Store logs in the job structure based on phase
	switch phase {
	case "build":
		job.Logs.Build = logs
	case "test":
		job.Logs.Test = logs
	case "publish":
		job.Logs.Publish = logs
	}
	
	nc.logger.WithFields(logrus.Fields{
		"job_id": job.ID,
		"phase":  phase,
		"log_lines": len(logs),
	}).Info("Captured phase logs")
	
	return nil
}