package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"nomad-mcp-builder/pkg/client"
	"nomad-mcp-builder/pkg/config"
	"nomad-mcp-builder/pkg/consul"
	"nomad-mcp-builder/pkg/types"
)

const (
	defaultServiceURL = "http://localhost:8080"
	usage = `jobforge - CLI client for JobForge Build Service

USAGE:
  jobforge [global-flags] <command> [command-args...]

DESCRIPTION:
  A command-line interface for the JobForge Build Service. Supports YAML
  configuration files and simplified image tagging (defaults to job-id).

COMMANDS:
  Job Management:
    submit-job [options] [config]     Submit a new build job
    get-status <job-id>               Get status of a job
    get-logs <job-id> [phase]         Get logs for a job (phase: build, test, publish)
    kill-job <job-id>                 Kill a running job
    cleanup <job-id>                  Clean up resources for a job
    get-history [limit] [offset]      Get job history (default: 10 recent jobs)

  Service Management:
    health                            Check service health status

GLOBAL FLAGS:
  -h, --help                Show this help message
  -u, --url <url>           Service URL (default: http://localhost:8080)
                            Can also be set via JOB_SERVICE_URL environment variable

SUBMIT-JOB OPTIONS:
  -global <file>            Global YAML configuration file (optional)
                            Typically: deploy/global.yaml

  -config <file>            Per-build YAML configuration file (required for YAML mode)
                            Per-build values override global values

  --image-tags <tags>       Image tags to use (comma-separated)
                            If not specified, defaults to job-id

  -w, --watch               Watch job progress in real-time using Consul KV
                            Displays status updates and exits when job completes
                            Requires Consul connection (default: localhost:8500)

  If neither -global nor -config is specified, reads YAML from stdin or argument.

YAML CONFIGURATION:
  Global config (deploy/global.yaml) - Shared settings:
    owner: myteam
    repo_url: https://github.com/myorg/myservice.git
    git_credentials_path: secret/nomad/jobs/git-credentials
    image_name: myservice
    registry_url: registry.example.com:5000/myapp

  Per-build config (build.yaml) - Build-specific overrides:
    git_ref: feature/new-feature
    test_entry_point: true

  See docs/JobSpec.md for complete configuration reference.

EXAMPLES:
  Submit Jobs:
    # With global + per-build YAML configs (recommended)
    jobforge submit-job -global deploy/global.yaml -config build.yaml

    # With single YAML config (must include all required fields)
    jobforge submit-job -config build.yaml

    # With custom image tags (defaults to job-id if not specified)
    jobforge submit-job -config build.yaml --image-tags "latest,stable"

    # Watch job progress in real-time (recommended for interactive use)
    jobforge submit-job -config build.yaml --watch

    # From stdin (YAML format)
    cat build.yaml | jobforge submit-job

    # From YAML string argument
    jobforge submit-job 'owner: test
repo_url: https://github.com/example/repo.git
...'

  Query Jobs:
    # Get job status
    jobforge get-status abc123

    # Get all logs
    jobforge get-logs abc123

    # Get phase-specific logs
    jobforge get-logs abc123 build
    jobforge get-logs abc123 test
    jobforge get-logs abc123 publish

    # Get job history (last 20 jobs)
    jobforge get-history 20

    # Get job history with pagination
    jobforge get-history 10 20  # limit=10, offset=20

  Manage Jobs:
    # Kill a running job
    jobforge kill-job abc123

    # Clean up job resources
    jobforge cleanup abc123

  Service Health:
    # Check service health
    jobforge health
    # Output:
    #   Service URL: http://10.0.1.16:21654
    #
    #   ✅ Overall Status: healthy
    #   Timestamp: 2025-10-08T12:34:56Z
    #
    #   Services:
    #     ✅ nomad: healthy
    #     ✅ consul: healthy

ENVIRONMENT VARIABLES:
  JOB_SERVICE_URL           Service URL (overrides default, can be overridden by -u flag)

FILES:
  deploy/global.yaml        Global configuration (optional)
  build.yaml                Per-build configuration

DOCUMENTATION:
  docs/JobSpec.md           Complete job configuration reference
  README.md                 Project overview and setup guide
  CLAUDE.md                 Development guidelines

For more information, visit: https://github.com/geraldthewes/nomad-mcp-builder
`
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}

	// Parse flags
	serviceURL := getServiceURL()
	var command string
	var commandArgs []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			fmt.Print(usage)
			return nil
		} else if arg == "-u" || arg == "--url" {
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires a value", arg)
			}
			serviceURL = args[i+1]
			i += 2
		} else if strings.HasPrefix(arg, "--url=") {
			serviceURL = strings.TrimPrefix(arg, "--url=")
			i++
		} else if !strings.HasPrefix(arg, "-") {
			command = arg
			commandArgs = args[i+1:]
			break
		} else {
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	if command == "" {
		return fmt.Errorf("no command specified")
	}

	// Create client
	c := client.NewClient(serviceURL)

	// Execute command
	switch command {
	case "submit-job":
		// Parse submit-job specific flags from commandArgs
		return handleSubmitJob(c, commandArgs)
	case "get-status":
		return handleGetStatus(c, commandArgs)
	case "get-logs":
		return handleGetLogs(c, commandArgs)
	case "kill-job":
		return handleKillJob(c, commandArgs)
	case "cleanup":
		return handleCleanup(c, commandArgs)
	case "get-history":
		return handleGetHistory(c, commandArgs)
	case "health":
		return handleHealth(c, commandArgs)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func getServiceURL() string {
	if url := os.Getenv("JOB_SERVICE_URL"); url != "" {
		return url
	}
	return defaultServiceURL
}

func handleSubmitJob(c *client.Client, args []string) error {
	// Parse submit-job specific flags
	var additionalImageTags []string
	var globalConfigPath string
	var perBuildConfigPath string
	var configData string
	var watch bool

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--watch" || arg == "-w" {
			watch = true
			i++
		} else if arg == "--image-tags" {
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires a value", arg)
			}
			tagStr := args[i+1]
			additionalImageTags = strings.Split(tagStr, ",")
			// Trim spaces from tags
			for j, tag := range additionalImageTags {
				additionalImageTags[j] = strings.TrimSpace(tag)
			}
			i += 2
		} else if strings.HasPrefix(arg, "--image-tags=") {
			tagStr := strings.TrimPrefix(arg, "--image-tags=")
			additionalImageTags = strings.Split(tagStr, ",")
			// Trim spaces from tags
			for j, tag := range additionalImageTags {
				additionalImageTags[j] = strings.TrimSpace(tag)
			}
			i++
		} else if arg == "-global" {
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires a value", arg)
			}
			globalConfigPath = args[i+1]
			i += 2
		} else if arg == "-config" {
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires a value", arg)
			}
			perBuildConfigPath = args[i+1]
			i += 2
		} else if !strings.HasPrefix(arg, "-") {
			// This is the config data (YAML string)
			configData = arg
			break
		} else {
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	var jobConfig *types.JobConfig
	var err error

	// Determine how to load configuration
	if perBuildConfigPath != "" {
		// Load from YAML files (with optional global config)
		jobConfig, err = config.LoadAndMergeJobConfigs(globalConfigPath, perBuildConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load YAML config: %w", err)
		}
	} else if configData != "" {
		// Parse from command line argument (YAML)
		jobConfig, err = parseConfigData(configData)
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	} else {
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		configData = string(data)

		if strings.TrimSpace(configData) == "" {
			return fmt.Errorf("no job configuration provided")
		}

		jobConfig, err = parseConfigData(configData)
		if err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// Set image tags from --image-tags flag if provided
	// If not provided, server will default to using job-id as the tag
	if len(additionalImageTags) > 0 {
		// Filter out empty tags
		var validTags []string
		for _, tag := range additionalImageTags {
			if tag != "" {
				validTags = append(validTags, tag)
			}
		}
		if len(validTags) > 0 {
			jobConfig.ImageTags = validTags
		}
	}

	// Submit job
	response, err := c.SubmitJob(jobConfig)
	if err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}

	// If --watch flag is set, watch the job progress instead of returning immediately
	if watch {
		return watchJobProgress(response.JobID, c.GetBaseURL())
	}

	// Output response
	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// parseConfigData parses config data as YAML
func parseConfigData(data string) (*types.JobConfig, error) {
	var jobConfig types.JobConfig

	// Parse as YAML
	if err := yaml.Unmarshal([]byte(data), &jobConfig); err != nil {
		return nil, fmt.Errorf("failed to parse as YAML: %w", err)
	}

	return &jobConfig, nil
}

func handleGetStatus(c *client.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("job ID required")
	}

	jobID := args[0]
	response, err := c.GetStatus(jobID)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	// Output response
	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func handleGetLogs(c *client.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("job ID required")
	}

	jobID := args[0]
	phase := ""
	if len(args) > 1 {
		phase = args[1]
	}

	response, err := c.GetLogs(jobID, phase)
	if err != nil {
		return fmt.Errorf("failed to get logs: %w", err)
	}

	// Output response
	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func handleKillJob(c *client.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("job ID required")
	}

	jobID := args[0]
	response, err := c.KillJob(jobID)
	if err != nil {
		return fmt.Errorf("failed to kill job: %w", err)
	}

	// Output response
	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func handleCleanup(c *client.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("job ID required")
	}

	jobID := args[0]
	response, err := c.Cleanup(jobID)
	if err != nil {
		return fmt.Errorf("failed to cleanup job: %w", err)
	}

	// Output response
	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func handleGetHistory(c *client.Client, args []string) error {
	limit := 10
	offset := 0

	if len(args) > 0 {
		var err error
		limit, err = strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid limit value: %s", args[0])
		}
	}
	if len(args) > 1 {
		var err error
		offset, err = strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid offset value: %s", args[1])
		}
	}

	response, err := c.GetHistory(limit, offset)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	// Output response
	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func handleHealth(c *client.Client, args []string) error {
	response, err := c.Health()
	if err != nil {
		return fmt.Errorf("failed to get health: %w", err)
	}

	// Display service URL
	fmt.Printf("Service URL: %s\n\n", c.GetBaseURL())

	// Display health status with color-coded output
	statusSymbol := "✅"
	if response.Status != "healthy" {
		statusSymbol = "❌"
	}

	fmt.Printf("%s Overall Status: %s\n", statusSymbol, response.Status)
	fmt.Printf("Timestamp: %s\n\n", response.Timestamp)

	fmt.Println("Services:")
	for service, status := range response.Services {
		serviceSymbol := "✅"
		if status != "healthy" {
			serviceSymbol = "❌"
		}
		fmt.Printf("  %s %s: %s\n", serviceSymbol, service, status)
	}

	return nil
}

// watchJobProgress watches a job's progress in real-time using Consul KV
func watchJobProgress(jobID string, serviceURL string) error {
	fmt.Printf("Watching job: %s\n", jobID)
	fmt.Printf("Service URL: %s\n\n", serviceURL)

	// Create Consul client
	consulClient, err := consul.NewClient("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to create Consul client: %v\n", err)
		fmt.Fprintf(os.Stderr, "Falling back to polling mode. Use 'jobforge get-status %s' to check status manually.\n", jobID)
		return fmt.Errorf("failed to create Consul client: %w", err)
	}

	// Create channels for updates and errors
	updates := make(chan consul.JobUpdate, 10)
	errors := make(chan error, 10)

	// Start watching in background
	go consulClient.WatchJob(jobID, updates, errors)

	var lastStatus types.JobStatus
	var lastPhase string

	// Process updates until job completes
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				// Channel closed, job finished
				return nil
			}

			// Only print if status or phase changed
			if update.Status != lastStatus || update.Phase != lastPhase {
				timestamp := update.Timestamp.Format("15:04:05")

				statusSymbol := getStatusSymbol(update.Status)
				fmt.Printf("[%s] %s Status: %s", timestamp, statusSymbol, update.Status)

				if update.Phase != "" {
					fmt.Printf(" | Phase: %s", update.Phase)
				}

				if update.Error != "" {
					fmt.Printf(" | Error: %s", update.Error)
				}

				fmt.Println()

				lastStatus = update.Status
				lastPhase = update.Phase
			}

			// Check if job reached terminal state
			if update.Status == types.StatusSucceeded {
				fmt.Printf("\n✅ Job completed successfully\n")
				return nil
			} else if update.Status == types.StatusFailed {
				fmt.Printf("\n❌ Job failed")
				if update.Error != "" {
					fmt.Printf(": %s", update.Error)
				}
				fmt.Println()
				return fmt.Errorf("job failed")
			}

		case err, ok := <-errors:
			if !ok {
				// Error channel closed
				continue
			}

			// Log non-fatal errors but continue watching
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}
}

// getStatusSymbol returns an emoji symbol for the given job status
func getStatusSymbol(status types.JobStatus) string {
	switch status {
	case types.StatusPending:
		return "⏳"
	case types.StatusBuilding:
		return "🔨"
	case types.StatusTesting:
		return "🧪"
	case types.StatusPublishing:
		return "📦"
	case types.StatusSucceeded:
		return "✅"
	case types.StatusFailed:
		return "❌"
	default:
		return "❓"
	}
}