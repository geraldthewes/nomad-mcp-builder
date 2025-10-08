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
	"nomad-mcp-builder/pkg/types"
	"nomad-mcp-builder/pkg/version"
)

const (
	defaultServiceURL = "http://localhost:8080"
	defaultDeployDir  = "deploy"
	usage = `nomad-build - CLI client for Nomad Build Service

USAGE:
  nomad-build [global-flags] <command> [command-args...]

DESCRIPTION:
  A command-line interface for the Nomad Build Service. Supports YAML
  configuration files, automatic semantic versioning, and branch-aware
  image tagging.

COMMANDS:
  Job Management:
    submit-job [options] [config]     Submit a new build job
    get-status <job-id>               Get status of a job
    get-logs <job-id> [phase]         Get logs for a job (phase: build, test, publish)
    kill-job <job-id>                 Kill a running job
    cleanup <job-id>                  Clean up resources for a job
    get-history [limit] [offset]      Get job history (default: 10 recent jobs)

  Version Management:
    version-info                      Show current version and branch information
    version-major <major>             Set major version (resets minor and patch to 0)
    version-minor <minor>             Set minor version (resets patch to 0)

GLOBAL FLAGS:
  -h, --help                Show this help message
  -u, --url <url>           Service URL (default: http://localhost:8080)
                            Can also be set via NOMAD_BUILD_URL environment variable

SUBMIT-JOB OPTIONS:
  -global <file>            Global YAML configuration file (optional)
                            Typically: deploy/global.yaml

  -config <file>            Per-build YAML configuration file (required for YAML mode)
                            Per-build values override global values

  --image-tags <tags>       Additional image tags to append (comma-separated)
                            Added to auto-generated version tag

  If neither -global nor -config is specified, reads YAML from stdin or argument.

VERSION MANAGEMENT:
  The CLI automatically manages semantic versioning in deploy/version.yaml:

  • Each 'submit-job' auto-increments the patch version
  • Generates branch-aware tags: <branch>-v<MAJOR>.<MINOR>.<PATCH>
  • Example: On branch 'feature-auth' with version 0.1.5
             → tag: feature-auth-v0.1.5

  Version file format (deploy/version.yaml):
    version:
      major: 0
      minor: 1
      patch: 5

YAML CONFIGURATION:
  Global config (deploy/global.yaml) - Shared settings:
    owner: myteam
    repo_url: https://github.com/myorg/myservice.git
    git_credentials_path: secret/nomad/jobs/git-credentials
    image_name: myservice
    registry_url: registry.example.com:5000/myapp

  Per-build config (build.yaml) - Build-specific overrides:
    git_ref: feature/new-feature
    image_tags:
      - test
      - dev
    test_entry_point: true

  See docs/JobSpec.md for complete configuration reference.

EXAMPLES:
  Submit Jobs:
    # With global + per-build YAML configs (recommended)
    nomad-build submit-job -global deploy/global.yaml -config build.yaml

    # With single YAML config (must include all required fields)
    nomad-build submit-job -config build.yaml

    # With additional tags beyond auto-generated version tag
    nomad-build submit-job -config build.yaml --image-tags "latest,stable"

    # From stdin (YAML format)
    cat build.yaml | nomad-build submit-job

    # From YAML string argument
    nomad-build submit-job 'owner: test
repo_url: https://github.com/example/repo.git
...'

  Query Jobs:
    # Get job status
    nomad-build get-status abc123

    # Get all logs
    nomad-build get-logs abc123

    # Get phase-specific logs
    nomad-build get-logs abc123 build
    nomad-build get-logs abc123 test
    nomad-build get-logs abc123 publish

    # Get job history (last 20 jobs)
    nomad-build get-history 20

    # Get job history with pagination
    nomad-build get-history 10 20  # limit=10, offset=20

  Manage Jobs:
    # Kill a running job
    nomad-build kill-job abc123

    # Clean up job resources
    nomad-build cleanup abc123

  Version Management:
    # Show current version and branch
    nomad-build version-info
    # Output:
    #   Version: 0.1.5
    #   Tag: v0.1.5
    #   Branch: feature-new-feature
    #   Branch Tag: feature-new-feature-v0.1.5

    # Bump major version (creates v1.0.0)
    nomad-build version-major 1

    # Bump minor version (e.g., v0.2.0)
    nomad-build version-minor 2

    # Note: Patch version auto-increments on each submit-job

ENVIRONMENT VARIABLES:
  NOMAD_BUILD_URL           Service URL (overrides default, can be overridden by -u flag)

FILES:
  deploy/version.yaml       Current version tracking
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
	case "version-info":
		return handleVersionInfo(commandArgs)
	case "version-major":
		return handleVersionMajor(commandArgs)
	case "version-minor":
		return handleVersionMinor(commandArgs)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func getServiceURL() string {
	if url := os.Getenv("NOMAD_BUILD_URL"); url != "" {
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

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--image-tags" {
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

	// Auto-increment patch version
	v, err := version.IncrementPatch(defaultDeployDir)
	if err != nil {
		return fmt.Errorf("failed to increment version: %w", err)
	}

	// Get current branch for branch-aware tagging
	branch, err := version.GetCurrentBranch()
	if err != nil {
		// If git fails, use simple version tag
		fmt.Fprintf(os.Stderr, "Warning: Could not detect git branch: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using version tag: %s\n", v.Tag())
		jobConfig.ImageTags = append(jobConfig.ImageTags, v.Tag())
	} else {
		branchTag := v.BranchTag(branch)
		fmt.Printf("Version: %s\n", v.String())
		fmt.Printf("Branch: %s\n", branch)
		fmt.Printf("Image tag: %s\n", branchTag)
		jobConfig.ImageTags = append(jobConfig.ImageTags, branchTag)
	}

	// Merge additional image tags if provided
	if len(additionalImageTags) > 0 {
		// Filter out empty tags
		var validTags []string
		for _, tag := range additionalImageTags {
			if tag != "" {
				validTags = append(validTags, tag)
			}
		}
		if len(validTags) > 0 {
			jobConfig.ImageTags = append(jobConfig.ImageTags, validTags...)
		}
	}

	// Submit job
	response, err := c.SubmitJob(jobConfig)
	if err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
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

func handleVersionInfo(args []string) error {
	v, err := version.LoadVersion(defaultDeployDir)
	if err != nil {
		return fmt.Errorf("failed to load version: %w", err)
	}

	branch, err := version.GetCurrentBranch()
	if err != nil {
		// If git fails, show version without branch info
		fmt.Printf("Version: %s\n", v.String())
		fmt.Printf("Tag: %s\n", v.Tag())
		return nil
	}

	branchTag := v.BranchTag(branch)
	fmt.Printf("Version: %s\n", v.String())
	fmt.Printf("Tag: %s\n", v.Tag())
	fmt.Printf("Branch: %s\n", branch)
	fmt.Printf("Branch Tag: %s\n", branchTag)

	return nil
}

func handleVersionMajor(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("major version number required")
	}

	major, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid major version: %s", args[0])
	}

	v, err := version.SetMajor(defaultDeployDir, major)
	if err != nil {
		return fmt.Errorf("failed to set major version: %w", err)
	}

	fmt.Printf("Version updated to: %s\n", v.String())
	return nil
}

func handleVersionMinor(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("minor version number required")
	}

	minor, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid minor version: %s", args[0])
	}

	v, err := version.SetMinor(defaultDeployDir, minor)
	if err != nil {
		return fmt.Errorf("failed to set minor version: %w", err)
	}

	fmt.Printf("Version updated to: %s\n", v.String())
	return nil
}