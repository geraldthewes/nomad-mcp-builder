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
	usage = `nomad-build - CLI client for nomad build service

Usage:
  nomad-build [flags] <command> [args...]

Commands:
  submit-job [options] [config]     Submit a new build job
  get-status <job-id>               Get status of a job
  get-logs <job-id> [phase]         Get logs for a job (optional phase: build, test, publish)
  kill-job <job-id>                 Kill a running job
  cleanup <job-id>                  Clean up resources for a job
  get-history [limit] [offset]      Get job history (optional limit, optional offset)
  version-info                      Show current version
  version-major <major>             Set major version (resets minor and patch to 0)
  version-minor <minor>             Set minor version (resets patch to 0)

Global Flags:
  -h, --help                Show this help message
  -u, --url <url>           Service URL (default: http://localhost:8080)
                            Can also be set via NOMAD_BUILD_URL environment variable

Submit-Job Options:
  -global <file>            Global YAML configuration file (optional)
  -config <file>            Per-build YAML configuration file (required if using YAML)
  --image-tags <tags>       Additional image tags to append (comma-separated)

  If neither -global nor -config is specified, reads JSON or YAML from stdin or argument

Examples:
  # Submit job using YAML files (global + per-build)
  nomad-build submit-job -global deploy/global.yaml -config build.yaml

  # Submit job using single YAML file
  nomad-build submit-job -config build.yaml

  # Submit job from JSON argument (legacy mode)
  nomad-build submit-job '{"owner":"test","repo_url":"https://github.com/example/repo.git",...}'

  # Submit job from stdin (JSON or YAML)
  cat build.yaml | nomad-build submit-job

  # Submit with additional image tags
  nomad-build submit-job -config build.yaml --image-tags "v4.0.16,latest"

  # Get job status
  nomad-build get-status abc123

  # Get build logs
  nomad-build get-logs abc123 build

  # Kill job
  nomad-build kill-job abc123

  # Clean up job
  nomad-build cleanup abc123

  # Get history
  nomad-build get-history
  nomad-build get-history 10 0

  # Version management
  nomad-build version-info
  nomad-build version-major 1
  nomad-build version-minor 2
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
			// This is the config data (JSON or YAML string)
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
		// Parse from command line argument (JSON or YAML)
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

// parseConfigData attempts to parse config data as JSON first, then YAML
func parseConfigData(data string) (*types.JobConfig, error) {
	var jobConfig types.JobConfig

	// Try JSON first
	if err := json.Unmarshal([]byte(data), &jobConfig); err == nil {
		return &jobConfig, nil
	}

	// Try YAML
	if err := yaml.Unmarshal([]byte(data), &jobConfig); err != nil {
		return nil, fmt.Errorf("failed to parse as JSON or YAML: %w", err)
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