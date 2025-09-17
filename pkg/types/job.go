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

// PhaseResourceLimits defines resource constraints for a single phase
type PhaseResourceLimits struct {
	CPU    string `json:"cpu"`    // e.g., "1000" (MHz)
	Memory string `json:"memory"` // e.g., "2048" (MB)
	Disk   string `json:"disk"`   // e.g., "10240" (MB)
}

// ResourceLimits defines resource constraints for build jobs per phase
type ResourceLimits struct {
	// Legacy fields for backward compatibility
	CPU    string `json:"cpu,omitempty"`    // e.g., "1000" (MHz) - applies to all phases if per-phase not specified
	Memory string `json:"memory,omitempty"` // e.g., "2048" (MB) - applies to all phases if per-phase not specified
	Disk   string `json:"disk,omitempty"`   // e.g., "10240" (MB) - applies to all phases if per-phase not specified

	// Per-phase resource limits
	Build   *PhaseResourceLimits `json:"build,omitempty"`
	Test    *PhaseResourceLimits `json:"test,omitempty"`
	Publish *PhaseResourceLimits `json:"publish,omitempty"`
}

// GetBuildLimits returns the resource limits for the build phase
func (rl *ResourceLimits) GetBuildLimits(defaults PhaseResourceLimits) PhaseResourceLimits {
	if rl == nil {
		return defaults
	}

	// If phase-specific limits are provided, use them
	if rl.Build != nil {
		result := PhaseResourceLimits{}
		if rl.Build.CPU != "" {
			result.CPU = rl.Build.CPU
		} else if rl.CPU != "" {
			result.CPU = rl.CPU // Fall back to legacy global
		} else {
			result.CPU = defaults.CPU
		}
		if rl.Build.Memory != "" {
			result.Memory = rl.Build.Memory
		} else if rl.Memory != "" {
			result.Memory = rl.Memory // Fall back to legacy global
		} else {
			result.Memory = defaults.Memory
		}
		if rl.Build.Disk != "" {
			result.Disk = rl.Build.Disk
		} else if rl.Disk != "" {
			result.Disk = rl.Disk // Fall back to legacy global
		} else {
			result.Disk = defaults.Disk
		}
		return result
	}

	// Fall back to legacy global limits if provided
	result := PhaseResourceLimits{}
	if rl.CPU != "" {
		result.CPU = rl.CPU
	} else {
		result.CPU = defaults.CPU
	}
	if rl.Memory != "" {
		result.Memory = rl.Memory
	} else {
		result.Memory = defaults.Memory
	}
	if rl.Disk != "" {
		result.Disk = rl.Disk
	} else {
		result.Disk = defaults.Disk
	}
	return result
}

// GetTestLimits returns the resource limits for the test phase
func (rl *ResourceLimits) GetTestLimits(defaults PhaseResourceLimits) PhaseResourceLimits {
	if rl == nil {
		return defaults
	}

	// If phase-specific limits are provided, use them
	if rl.Test != nil {
		result := PhaseResourceLimits{}
		if rl.Test.CPU != "" {
			result.CPU = rl.Test.CPU
		} else if rl.CPU != "" {
			result.CPU = rl.CPU // Fall back to legacy global
		} else {
			result.CPU = defaults.CPU
		}
		if rl.Test.Memory != "" {
			result.Memory = rl.Test.Memory
		} else if rl.Memory != "" {
			result.Memory = rl.Memory // Fall back to legacy global
		} else {
			result.Memory = defaults.Memory
		}
		if rl.Test.Disk != "" {
			result.Disk = rl.Test.Disk
		} else if rl.Disk != "" {
			result.Disk = rl.Disk // Fall back to legacy global
		} else {
			result.Disk = defaults.Disk
		}
		return result
	}

	// Fall back to legacy global limits if provided
	result := PhaseResourceLimits{}
	if rl.CPU != "" {
		result.CPU = rl.CPU
	} else {
		result.CPU = defaults.CPU
	}
	if rl.Memory != "" {
		result.Memory = rl.Memory
	} else {
		result.Memory = defaults.Memory
	}
	if rl.Disk != "" {
		result.Disk = rl.Disk
	} else {
		result.Disk = defaults.Disk
	}
	return result
}

// GetPublishLimits returns the resource limits for the publish phase
func (rl *ResourceLimits) GetPublishLimits(defaults PhaseResourceLimits) PhaseResourceLimits {
	if rl == nil {
		return defaults
	}

	// If phase-specific limits are provided, use them
	if rl.Publish != nil {
		result := PhaseResourceLimits{}
		if rl.Publish.CPU != "" {
			result.CPU = rl.Publish.CPU
		} else if rl.CPU != "" {
			result.CPU = rl.CPU // Fall back to legacy global
		} else {
			result.CPU = defaults.CPU
		}
		if rl.Publish.Memory != "" {
			result.Memory = rl.Publish.Memory
		} else if rl.Memory != "" {
			result.Memory = rl.Memory // Fall back to legacy global
		} else {
			result.Memory = defaults.Memory
		}
		if rl.Publish.Disk != "" {
			result.Disk = rl.Publish.Disk
		} else if rl.Disk != "" {
			result.Disk = rl.Disk // Fall back to legacy global
		} else {
			result.Disk = defaults.Disk
		}
		return result
	}

	// Fall back to legacy global limits if provided
	result := PhaseResourceLimits{}
	if rl.CPU != "" {
		result.CPU = rl.CPU
	} else {
		result.CPU = defaults.CPU
	}
	if rl.Memory != "" {
		result.Memory = rl.Memory
	} else {
		result.Memory = defaults.Memory
	}
	if rl.Disk != "" {
		result.Disk = rl.Disk
	} else {
		result.Disk = defaults.Disk
	}
	return result
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