package nomad

import (
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
	
	return &Client{
		client: client,
		config: cfg,
		logger: logrus.New(),
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
	
	evalID, _, err := nc.client.Jobs().Register(buildJobSpec, nil)
	if err != nil {
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
			// Build completed, start test phase
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
			job.Status = types.StatusFailed
			job.FailedPhase = "build"
			job.Error = "Build phase failed"
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
			// Tests completed, start publish phase
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
			job.Status = types.StatusFailed
			job.FailedPhase = "test"
			job.Error = "Test phase failed"
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
			job.Status = types.StatusSucceeded
			now := time.Now()
			job.FinishedAt = &now
		case "failed":
			job.Status = types.StatusFailed
			job.FailedPhase = "publish"
			job.Error = "Publish phase failed"
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
	
	// Map Nomad status to our simplified status
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

func (nc *Client) startTestPhase(job *types.Job) error {
	if len(job.Config.TestCommands) == 0 {
		// No tests to run, skip to publish phase
		return nc.startPublishPhase(job)
	}
	
	testJobSpec, err := nc.createTestJobSpec(job)
	if err != nil {
		return fmt.Errorf("failed to create test job spec: %w", err)
	}
	
	evalID, _, err := nc.client.Jobs().Register(testJobSpec, nil)
	if err != nil {
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
	
	evalID, _, err := nc.client.Jobs().Register(publishJobSpec, nil)
	if err != nil {
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
	
	evalID, _, err := nc.client.Jobs().Register(cleanupJobSpec, nil)
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