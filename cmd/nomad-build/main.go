package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"nomad-mcp-builder/pkg/client"
	"nomad-mcp-builder/pkg/types"
)

const (
	defaultServiceURL = "http://localhost:8080"
	usage = `nomad-build - CLI client for nomad build service

Usage:
  nomad-build [flags] <command> [args...]

Commands:
  submit-job [--image-tags tag1,tag2,...] <json>     Submit a new build job (JSON from arg or stdin)
  get-status <job-id>   Get status of a job
  get-logs <job-id> [phase]  Get logs for a job (optional phase: build, test, publish)
  kill-job <job-id>     Kill a running job
  cleanup <job-id>      Clean up resources for a job
  get-history [limit] [offset]  Get job history (optional limit, optional offset)

Flags:
  -h, --help           Show this help message
  -u, --url <url>      Service URL (default: http://localhost:8080)
                       Can also be set via NOMAD_BUILD_URL environment variable
  --image-tags <tags>  Additional image tags to append (comma-separated, submit-job only)

Examples:
  # Submit job from command line argument
  nomad-build submit-job '{"owner":"test","repo_url":"https://github.com/example/repo.git","git_ref":"main","dockerfile_path":"Dockerfile","image_name":"test","image_tags":["v1.0"],"registry_url":"registry.example.com/test"}'

  # Submit job with additional image tags
  nomad-build submit-job --image-tags "v4.0.16,latest" '{"owner":"test","repo_url":"https://github.com/example/repo.git","git_ref":"main","dockerfile_path":"Dockerfile","image_name":"test","image_tags":["v1.0"],"registry_url":"registry.example.com/test"}'

  # Submit job from stdin
  echo '{"owner":"test",...}' | nomad-build submit-job

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
	var jobJSON string

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
		} else if !strings.HasPrefix(arg, "-") {
			// This is the JSON argument
			jobJSON = arg
			break
		} else {
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	// If no JSON from command line arguments, read from stdin
	if jobJSON == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		jobJSON = string(data)
	}

	if strings.TrimSpace(jobJSON) == "" {
		return fmt.Errorf("no job configuration provided")
	}

	// Parse job config
	var jobConfig types.JobConfig
	if err := json.Unmarshal([]byte(jobJSON), &jobConfig); err != nil {
		return fmt.Errorf("failed to parse job JSON: %w", err)
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
	response, err := c.SubmitJob(&jobConfig)
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