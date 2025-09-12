package types

import "time"

// JobStatus represents the current status of a build job
type JobStatus string

const (
	StatusPending    JobStatus = "PENDING"
	StatusBuilding   JobStatus = "BUILDING"
	StatusTesting    JobStatus = "TESTING"
	StatusPublishing JobStatus = "PUBLISHING"
	StatusSucceeded  JobStatus = "SUCCEEDED"
	StatusFailed     JobStatus = "FAILED"
)

// JobConfig represents the configuration for a build job
type JobConfig struct {
	Owner                   string   `json:"owner"`
	RepoURL                 string   `json:"repo_url"`
	GitRef                  string   `json:"git_ref"`
	GitCredentialsPath      string   `json:"git_credentials_path"`
	DockerfilePath          string   `json:"dockerfile_path"`
	ImageName               string   `json:"image_name"`
	ImageTags               []string `json:"image_tags"`
	RegistryURL             string   `json:"registry_url"`
	RegistryCredentialsPath string   `json:"registry_credentials_path"`
	TestCommands            []string `json:"test_commands"`
	TestEntryPoint          bool     `json:"test_entry_point,omitempty"`
	ResourceLimits          *ResourceLimits `json:"resource_limits,omitempty"`
	BuildTimeout            *time.Duration  `json:"build_timeout,omitempty"`
	TestTimeout             *time.Duration  `json:"test_timeout,omitempty"`
}

// ResourceLimits defines resource constraints for build jobs
type ResourceLimits struct {
	CPU    string `json:"cpu"`    // e.g., "1000" (MHz)
	Memory string `json:"memory"` // e.g., "2048" (MB)
	Disk   string `json:"disk"`   // e.g., "10240" (MB)
}

// Job represents a build job with its current state
type Job struct {
	ID         string     `json:"id"`
	Config     JobConfig  `json:"config"`
	Status     JobStatus  `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	
	// Nomad job IDs for each phase
	BuildJobID   string   `json:"build_job_id,omitempty"`
	TestJobIDs   []string `json:"test_job_ids,omitempty"`  // Multiple test jobs
	PublishJobID string   `json:"publish_job_id,omitempty"`
	
	// Logs for each phase
	Logs JobLogs `json:"logs"`
	
	// Metrics
	Metrics JobMetrics `json:"metrics"`
	
	// Error information
	Error string `json:"error,omitempty"`
	FailedPhase string `json:"failed_phase,omitempty"`
}

// JobLogs contains logs for each phase of the build
type JobLogs struct {
	Build   []string `json:"build"`
	Test    []string `json:"test"`
	Publish []string `json:"publish"`
}

// JobMetrics contains performance metrics for the job
type JobMetrics struct {
	// Phase timings
	JobStart      *time.Time    `json:"job_start,omitempty"`
	BuildStart    *time.Time    `json:"build_start,omitempty"`
	BuildEnd      *time.Time    `json:"build_end,omitempty"`
	TestStart     *time.Time    `json:"test_start,omitempty"`
	TestEnd       *time.Time    `json:"test_end,omitempty"`
	PublishStart  *time.Time    `json:"publish_start,omitempty"`
	PublishEnd    *time.Time    `json:"publish_end,omitempty"`
	JobEnd        *time.Time    `json:"job_end,omitempty"`
	
	// Phase durations
	BuildDuration   time.Duration `json:"build_duration"`
	TestDuration    time.Duration `json:"test_duration"`
	PublishDuration time.Duration `json:"publish_duration"`
	TotalDuration   time.Duration `json:"total_duration"`
	ResourceUsage   ResourceUsage `json:"resource_usage"`
}

// ResourceUsage tracks actual resource consumption
type ResourceUsage struct {
	MaxCPU    float64 `json:"max_cpu"`    // CPU usage percentage
	MaxMemory float64 `json:"max_memory"` // Memory usage in MB
	DiskUsed  float64 `json:"disk_used"`  // Disk usage in MB
}

// JobHistory represents a historical job record
type JobHistory struct {
	ID        string        `json:"id"`
	Config    JobConfig     `json:"config"`
	Status    JobStatus     `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	Duration  time.Duration `json:"duration"`
	Metrics   JobMetrics    `json:"metrics"`
}