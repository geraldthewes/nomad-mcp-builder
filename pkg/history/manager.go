package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"nomad-mcp-builder/pkg/types"
)

// Manager handles local build history management
type Manager struct {
	deployDir string
}

// NewManager creates a new history manager
// deployDir can be relative (resolved from CWD) or absolute
func NewManager(deployDir string) (*Manager, error) {
	if deployDir == "" {
		deployDir = "./deploy"
	}

	// Convert to absolute path if relative
	absPath, err := filepath.Abs(deployDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve deploy directory path: %w", err)
	}

	return &Manager{
		deployDir: absPath,
	}, nil
}

// GetDeployDir returns the absolute path to the deploy directory
func (m *Manager) GetDeployDir() string {
	return m.deployDir
}

// GetBuildDir returns the path to a specific build's directory
func (m *Manager) GetBuildDir(jobID string) string {
	return filepath.Join(m.deployDir, "builds", jobID)
}

// CreateBuildDirectory creates the directory structure for a build
func (m *Manager) CreateBuildDirectory(jobID string) error {
	buildDir := m.GetBuildDir(jobID)

	// Create builds directory with parents if needed
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return fmt.Errorf("failed to create build directory %s: %w", buildDir, err)
	}

	return nil
}

// InitialMetadata represents initial metadata written at job submission
type InitialMetadata struct {
	JobID       string              `yaml:"job_id"`
	SubmittedAt time.Time           `yaml:"submitted_at"`
	Status      types.JobStatus     `yaml:"status"`
	Branch      string              `yaml:"branch"`
	GitRef      string              `yaml:"git_ref"`
	JobConfig   types.JobConfig     `yaml:"job_config"`
}

// WriteInitialMetadata writes initial metadata at job submission time
func (m *Manager) WriteInitialMetadata(jobID string, config types.JobConfig) error {
	buildDir := m.GetBuildDir(jobID)

	metadata := InitialMetadata{
		JobID:       jobID,
		SubmittedAt: time.Now(),
		Status:      types.StatusPending,
		Branch:      extractBranch(config.GitRef),
		GitRef:      config.GitRef,
		JobConfig:   config,
	}

	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(buildDir, "metadata.yaml")
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// CompleteMetadata represents complete metadata written after job completion
type CompleteMetadata struct {
	JobID        string              `yaml:"job_id"`
	SubmittedAt  time.Time           `yaml:"submitted_at"`
	StartedAt    *time.Time          `yaml:"started_at,omitempty"`
	CompletedAt  *time.Time          `yaml:"completed_at,omitempty"`
	Status       types.JobStatus     `yaml:"status"`
	Branch       string              `yaml:"branch"`
	GitRef       string              `yaml:"git_ref"`
	Purpose      string              `yaml:"purpose,omitempty"`
	Error        string              `yaml:"error,omitempty"`
	FailedPhase  string              `yaml:"failed_phase,omitempty"`
	JobConfig    types.JobConfig     `yaml:"job_config"`
	Phases       map[string]PhaseInfo `yaml:"phases,omitempty"`
}

// PhaseInfo represents information about a completed phase
type PhaseInfo struct {
	Status      types.JobStatus `yaml:"status"`
	StartedAt   *time.Time      `yaml:"started_at,omitempty"`
	CompletedAt *time.Time      `yaml:"completed_at,omitempty"`
	Duration    string          `yaml:"duration,omitempty"`
}

// WriteCompleteMetadata writes complete metadata after job completion
func (m *Manager) WriteCompleteMetadata(jobID string, job *types.Job, submittedAt time.Time) error {
	buildDir := m.GetBuildDir(jobID)

	metadata := CompleteMetadata{
		JobID:       jobID,
		SubmittedAt: submittedAt,
		StartedAt:   job.StartedAt,
		CompletedAt: job.FinishedAt,
		Status:      job.Status,
		Branch:      extractBranch(job.Config.GitRef),
		GitRef:      job.Config.GitRef,
		Error:       job.Error,
		FailedPhase: job.FailedPhase,
		JobConfig:   job.Config,
		Phases:      make(map[string]PhaseInfo),
	}

	// Add phase information
	if job.Metrics.BuildStart != nil {
		metadata.Phases["build"] = PhaseInfo{
			Status:      types.StatusSucceeded,
			StartedAt:   job.Metrics.BuildStart,
			CompletedAt: job.Metrics.BuildEnd,
			Duration:    formatDuration(job.Metrics.BuildDuration),
		}
	}
	if job.Metrics.TestStart != nil {
		metadata.Phases["test"] = PhaseInfo{
			Status:      types.StatusSucceeded,
			StartedAt:   job.Metrics.TestStart,
			CompletedAt: job.Metrics.TestEnd,
			Duration:    formatDuration(job.Metrics.TestDuration),
		}
	}
	if job.Metrics.PublishStart != nil {
		metadata.Phases["publish"] = PhaseInfo{
			Status:      types.StatusSucceeded,
			StartedAt:   job.Metrics.PublishStart,
			CompletedAt: job.Metrics.PublishEnd,
			Duration:    formatDuration(job.Metrics.PublishDuration),
		}
	}

	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(buildDir, "metadata.yaml")
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// WriteStatusFile writes a human-readable status summary in Markdown
func (m *Manager) WriteStatusFile(jobID string, job *types.Job) error {
	buildDir := m.GetBuildDir(jobID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Build Status: %s\n\n", jobID))

	// Status section
	statusSymbol := "✅"
	if job.Status == types.StatusFailed {
		statusSymbol = "❌"
	} else if job.Status == types.StatusPending || job.Status == types.StatusBuilding ||
	           job.Status == types.StatusTesting || job.Status == types.StatusPublishing {
		statusSymbol = "⏳"
	}

	sb.WriteString(fmt.Sprintf("**Status**: %s %s\n", statusSymbol, job.Status))
	sb.WriteString(fmt.Sprintf("**Branch**: %s\n", extractBranch(job.Config.GitRef)))
	sb.WriteString(fmt.Sprintf("**Git Ref**: %s\n", job.Config.GitRef))

	if job.StartedAt != nil {
		sb.WriteString(fmt.Sprintf("**Started**: %s\n", job.StartedAt.Format(time.RFC3339)))
	}
	if job.FinishedAt != nil {
		sb.WriteString(fmt.Sprintf("**Completed**: %s\n", job.FinishedAt.Format(time.RFC3339)))
		if job.StartedAt != nil {
			duration := job.FinishedAt.Sub(*job.StartedAt)
			sb.WriteString(fmt.Sprintf("**Duration**: %s\n", formatDuration(duration)))
		}
	}

	// Phases section
	sb.WriteString("\n## Phases\n")
	if job.Metrics.BuildStart != nil {
		phaseStatus := getPhaseStatusSymbol(job.Status, job.FailedPhase, "build")
		sb.WriteString(fmt.Sprintf("- Build: %s (%s)\n", phaseStatus, formatDuration(job.Metrics.BuildDuration)))
	}
	if job.Metrics.TestStart != nil {
		phaseStatus := getPhaseStatusSymbol(job.Status, job.FailedPhase, "test")
		sb.WriteString(fmt.Sprintf("- Test: %s (%s)\n", phaseStatus, formatDuration(job.Metrics.TestDuration)))
	}
	if job.Metrics.PublishStart != nil {
		phaseStatus := getPhaseStatusSymbol(job.Status, job.FailedPhase, "publish")
		sb.WriteString(fmt.Sprintf("- Publish: %s (%s)\n", phaseStatus, formatDuration(job.Metrics.PublishDuration)))
	}

	// Image section
	if job.Config.ImageName != "" {
		sb.WriteString("\n## Image\n")
		sb.WriteString(fmt.Sprintf("- Registry: %s\n", job.Config.RegistryURL))
		sb.WriteString(fmt.Sprintf("- Image: %s\n", job.Config.ImageName))
		if len(job.Config.ImageTags) > 0 {
			sb.WriteString(fmt.Sprintf("- Tags: %s\n", strings.Join(job.Config.ImageTags, ", ")))
		}
	}

	// Error section
	if job.Error != "" {
		sb.WriteString("\n## Error\n")
		sb.WriteString(fmt.Sprintf("```\n%s\n```\n", job.Error))
	}

	statusPath := filepath.Join(buildDir, "status.md")
	if err := os.WriteFile(statusPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write status file: %w", err)
	}

	return nil
}

// WritePhaseLogs writes logs for a specific phase
func (m *Manager) WritePhaseLogs(jobID, phase string, logs []string) error {
	buildDir := m.GetBuildDir(jobID)

	logPath := filepath.Join(buildDir, fmt.Sprintf("%s.log", phase))
	content := strings.Join(logs, "\n")

	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s logs: %w", phase, err)
	}

	return nil
}

// HistoryEntry represents an entry in the history.md file
type HistoryEntry struct {
	JobID       string
	Branch      string
	GitRef      string
	Timestamp   time.Time
	Status      types.JobStatus
	Duration    time.Duration
	Purpose     string
	ImageTags   []string
	Error       string
}

// UpdateHistoryFile prepends a new entry to the history.md file
func (m *Manager) UpdateHistoryFile(entry HistoryEntry) error {
	historyPath := filepath.Join(m.deployDir, "history.md")

	// Format new entry
	var newEntry strings.Builder
	newEntry.WriteString(fmt.Sprintf("## %s (%s)\n", entry.JobID, entry.Timestamp.Format("2006-01-02 15:04:05 UTC")))
	newEntry.WriteString(fmt.Sprintf("**Branch**: %s | **Git Ref**: %s\n", entry.Branch, entry.GitRef))

	statusSymbol := "✅"
	if entry.Status == types.StatusFailed {
		statusSymbol = "❌"
	} else if entry.Status == types.StatusPending {
		statusSymbol = "⏳"
	}

	statusLine := fmt.Sprintf("**Status**: %s %s", statusSymbol, entry.Status)
	if entry.Duration > 0 {
		statusLine += fmt.Sprintf(" | **Duration**: %s", formatDuration(entry.Duration))
	}
	if entry.Purpose != "" {
		statusLine += fmt.Sprintf(" | **Purpose**: %s", entry.Purpose)
	}
	newEntry.WriteString(statusLine + "\n")

	if len(entry.ImageTags) > 0 {
		newEntry.WriteString(fmt.Sprintf("**Tags**: %s\n", strings.Join(entry.ImageTags, ", ")))
	}

	if entry.Error != "" {
		newEntry.WriteString(fmt.Sprintf("**Error**: %s\n", entry.Error))
	}

	newEntry.WriteString("\n")

	// Read existing history if it exists
	var existingContent []byte
	if _, err := os.Stat(historyPath); err == nil {
		existingContent, err = os.ReadFile(historyPath)
		if err != nil {
			return fmt.Errorf("failed to read existing history file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat history file: %w", err)
	}

	// Create new content with entry prepended
	var finalContent strings.Builder

	// Always add header
	finalContent.WriteString("# Build History\n\n")

	// Add new entry
	finalContent.WriteString(newEntry.String())

	// Add existing content (skip header if present)
	if len(existingContent) > 0 {
		content := string(existingContent)
		// Skip "# Build History" header if present
		if strings.HasPrefix(content, "# Build History\n\n") {
			content = strings.TrimPrefix(content, "# Build History\n\n")
		}
		finalContent.WriteString(content)
	}

	// Write updated history
	if err := os.WriteFile(historyPath, []byte(finalContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write history file: %w", err)
	}

	return nil
}

// Helper functions

// extractBranch extracts a readable branch name from a git ref
func extractBranch(gitRef string) string {
	if gitRef == "" {
		return "unknown"
	}

	// Handle common patterns
	// refs/heads/main -> main
	if strings.HasPrefix(gitRef, "refs/heads/") {
		return strings.TrimPrefix(gitRef, "refs/heads/")
	}
	// refs/tags/v1.0.0 -> v1.0.0
	if strings.HasPrefix(gitRef, "refs/tags/") {
		return strings.TrimPrefix(gitRef, "refs/tags/")
	}

	// Return as-is for branches, tags, or commit SHAs
	return gitRef
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// Round to seconds
	d = d.Round(time.Second)

	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// getPhaseStatusSymbol returns a status symbol for a phase
func getPhaseStatusSymbol(jobStatus types.JobStatus, failedPhase, phase string) string {
	if jobStatus == types.StatusFailed && failedPhase == phase {
		return "❌ FAILED"
	}
	return "✅ SUCCESS"
}
