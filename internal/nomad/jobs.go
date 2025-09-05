package nomad

import (
	"fmt"
	"strings"
	"time"

	nomadapi "github.com/hashicorp/nomad/api"
	
	"nomad-mcp-builder/pkg/types"
)

// createBuildJobSpec creates a Nomad job specification for the build phase
func (nc *Client) createBuildJobSpec(job *types.Job) (*nomadapi.Job, error) {
	jobID := fmt.Sprintf("build-%s", job.ID)
	
	// Resource limits
	cpu := 1000 // Default 1000 MHz
	memory := 2048 // Default 2048 MB
	disk := 10240 // Default 10 GB
	
	if job.Config.ResourceLimits != nil {
		if job.Config.ResourceLimits.CPU != "" {
			fmt.Sscanf(job.Config.ResourceLimits.CPU, "%d", &cpu)
		}
		if job.Config.ResourceLimits.Memory != "" {
			fmt.Sscanf(job.Config.ResourceLimits.Memory, "%d", &memory)
		}
		if job.Config.ResourceLimits.Disk != "" {
			fmt.Sscanf(job.Config.ResourceLimits.Disk, "%d", &disk)
		}
	}
	
	// Create temporary image name
	tempImageName := fmt.Sprintf("%s/%s/%s:latest", 
		nc.config.Build.RegistryConfig.URL, 
		nc.config.Build.RegistryConfig.TempPrefix, 
		job.ID)
	
	// Build the task commands
	buildCommands := []string{
		"#!/bin/sh",
		"set -eu",  // Alpine sh doesn't support pipefail
		"",
		"# Start Docker daemon in background with reduced verbosity",
		"dockerd-entrypoint.sh --tls=false --host=tcp://0.0.0.0:2375 --host=unix:///var/run/docker.sock > /dev/null 2>&1 &",
		"sleep 15",  // Give more time for daemon to start
		"",
		"# Install git if not available",
		"if ! command -v git >/dev/null 2>&1; then",  // Alpine sh syntax
		"    apk add --no-cache git",
		"fi",
		"",
		"# Clean up any existing repo directory",
		"rm -rf /tmp/repo",
		"",
		"# Clone repository",
		fmt.Sprintf("git clone %s /tmp/repo", job.Config.RepoURL),
		"cd /tmp/repo",
		fmt.Sprintf("git checkout %s", job.Config.GitRef),
		"",
		"# Wait for Docker daemon to be ready",
		"until docker info >/dev/null 2>&1; do",
		"    echo 'Waiting for Docker daemon...'",
		"    sleep 2",
		"done",
		"echo 'Docker daemon ready'",
		"",
		"# Build image with Docker",
		fmt.Sprintf("docker build -f %s -t %s .", 
			job.Config.DockerfilePath, tempImageName),
		"",
		"# Push temporary image to registry", 
		fmt.Sprintf("docker push %s", tempImageName),
		"",
		"echo 'Build completed successfully'",
	}
	
	jobSpec := &nomadapi.Job{
		ID:          &jobID,
		Name:        &jobID,
		Type:        stringPtr("batch"),
		Namespace:   stringPtr(nc.config.Nomad.Namespace),
		Region:      stringPtr(nc.config.Nomad.Region),
		Datacenters: nc.config.Nomad.Datacenters,
		Meta: map[string]string{
			"build-service-job-id": job.ID,
			"phase":                "build",
		},
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:        stringPtr("build"),
				Count:       intPtr(1),
				Constraints: []*nomadapi.Constraint{}, // Override automatic constraints
				RestartPolicy: &nomadapi.RestartPolicy{
					Attempts: intPtr(0), // No restart for build jobs
				},
				Tasks: []*nomadapi.Task{
					{
						Name:   "main",
						Driver: "docker",
						Config: map[string]interface{}{
							"image": "docker:24-dind",  // Use Docker-in-Docker instead of Buildah
							"command": "/bin/sh",       // Alpine uses /bin/sh not /bin/bash
							"args": []string{"-c", strings.Join(buildCommands, "\n")},
							"privileged": true,  // Enable privileged mode for cgroup access
							// Add capabilities required for Buildah user namespaces
							"cap_add": []string{
								"SYS_ADMIN",     // Required for mount operations
								"SETUID",        // Required for user namespace operations
								"SETGID",        // Required for user namespace operations
								"SYS_CHROOT",    // Required for chroot operations
							},
							// Enable user namespace mapping
							"userns_mode": "host",
							// Add security options for user namespace
							"security_opt": []string{
								"seccomp=unconfined",
								"apparmor=unconfined",
							},
							// Mount cgroup filesystem for container management
							"volumes": []string{
								"/sys/fs/cgroup:/sys/fs/cgroup:rw",
								"/tmp:/tmp:rw",  // Ensure tmp is writable
							},
							// Temporarily removed mount to test scheduling
						},
						Env: map[string]string{
							"DOCKER_TLS_CERTDIR":   "",       // Disable TLS for simplicity
							"DOCKER_HOST":          "unix:///var/run/docker.sock", // Docker socket
							"DOCKER_DRIVER":        "overlay2", // Use overlay2 storage driver
							"TINI_SUBREAPER":       "1",      // Fix tini zombie reaping
							"DOCKER_BUILDKIT":      "1",      // Enable BuildKit for better builds
							// Docker-in-Docker configuration
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: &nc.config.Build.KillTimeout,
						Vault: &nomadapi.Vault{
							Policies:   []string{"nomad-build-service"},
							ChangeMode: stringPtr("restart"),
							Role:       "nomad-workloads", // Use the correct JWT role
						},
						Templates: buildTemplates(job),
					},
				},
				EphemeralDisk: &nomadapi.EphemeralDisk{
					SizeMB: intPtr(disk),
				},
			},
		},
		// Allow job to run (don't auto-stop)
		Stop:        boolPtr(false),
	}
	
	// KillTimeout is now set per task using configurable value
	
	return jobSpec, nil
}

// createTestJobSpec creates a Nomad job specification for the test phase
func (nc *Client) createTestJobSpec(job *types.Job) (*nomadapi.Job, error) {
	jobID := fmt.Sprintf("test-%s", job.ID)
	
	// Resource limits (similar to build phase)
	cpu := 1000
	memory := 2048
	disk := 5120 // Tests typically need less disk
	
	if job.Config.ResourceLimits != nil {
		if job.Config.ResourceLimits.CPU != "" {
			fmt.Sscanf(job.Config.ResourceLimits.CPU, "%d", &cpu)
		}
		if job.Config.ResourceLimits.Memory != "" {
			fmt.Sscanf(job.Config.ResourceLimits.Memory, "%d", &memory)
		}
	}
	
	// Create temporary image name
	tempImageName := fmt.Sprintf("%s/%s/%s:latest", 
		nc.config.Build.RegistryConfig.URL, 
		nc.config.Build.RegistryConfig.TempPrefix, 
		job.ID)
	
	// Build test commands
	testCommands := []string{
		"#!/bin/bash",
		"set -euo pipefail",
		"",
		"# Pull the built image",
		fmt.Sprintf("buildah pull docker://%s", tempImageName),
		"",
		"# Run each test command",
	}
	
	for i, testCmd := range job.Config.TestCommands {
		testCommands = append(testCommands, 
			fmt.Sprintf("echo 'Running test %d: %s'", i+1, testCmd),
			fmt.Sprintf("buildah run %s -- %s", tempImageName, testCmd),
			"echo 'Test completed successfully'",
			"",
		)
	}
	
	testCommands = append(testCommands, "echo 'All tests completed successfully'")
	
	jobSpec := &nomadapi.Job{
		ID:          &jobID,
		Name:        &jobID,
		Type:        stringPtr("batch"),
		Namespace:   stringPtr(nc.config.Nomad.Namespace),
		Region:      stringPtr(nc.config.Nomad.Region),
		Datacenters: nc.config.Nomad.Datacenters,
		Meta: map[string]string{
			"build-service-job-id": job.ID,
			"phase":                "test",
		},
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:        stringPtr("test"),
				Count:       intPtr(1),
				Constraints: []*nomadapi.Constraint{}, // Override automatic constraints
				RestartPolicy: &nomadapi.RestartPolicy{
					Attempts: intPtr(0),
				},
				Networks: []*nomadapi.NetworkResource{
					{
						Mode: "bridge", // Enable networking for tests
					},
				},
				Tasks: []*nomadapi.Task{
					{
						Name:   "main",
						Driver: "docker",
						Config: map[string]interface{}{
							"image":   "quay.io/buildah/stable:latest",
							"command": "/bin/bash",
							"args":    []string{"-c", strings.Join(testCommands, "\n")},
							// Add capabilities required for Buildah user namespaces
							"cap_add": []string{
								"SYS_ADMIN",     // Required for mount operations
								"SETUID",        // Required for user namespace operations
								"SETGID",        // Required for user namespace operations
								"SYS_CHROOT",    // Required for chroot operations
							},
							// Enable user namespace mapping
							"userns_mode": "host",
							// Add security options for user namespace
							"security_opt": []string{
								"seccomp=unconfined",
								"apparmor=unconfined",
							},
							// Mount cgroup filesystem for container management
							"volumes": []string{
								"/sys/fs/cgroup:/sys/fs/cgroup:rw",
								"/tmp:/tmp:rw",  // Ensure tmp is writable
							},
							// Removed /dev/fuse device to avoid version constraints
						},
						Env: map[string]string{
							"BUILDAH_ISOLATION": "chroot",  // Use chroot isolation to avoid cgroup issues
							"STORAGE_DRIVER":    "vfs",     // Use VFS storage driver for better compatibility
							"BUILDAH_FORMAT":    "oci",     // Ensure OCI format
							"BUILDAH_LAYERS":    "false",   // Disable layer caching to simplify
							"_BUILDAH_STARTED_IN_USERNS": "1", // Tell buildah we're in a user namespace
							"BUILDAH_ROOTLESS":  "1",       // Enable rootless mode
							// Remove cgroup manager to avoid delegation issues
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: &nc.config.Build.KillTimeout,
						Vault: &nomadapi.Vault{
							Policies:   []string{"nomad-build-service"},
							ChangeMode: stringPtr("restart"),
							Role:       "nomad-workloads", // Use the correct JWT role
						},
						Templates: testTemplates(job),
					},
				},
				EphemeralDisk: &nomadapi.EphemeralDisk{
					SizeMB: intPtr(disk),
				},
			},
		},
	}
	
	return jobSpec, nil
}

// createPublishJobSpec creates a Nomad job specification for the publish phase
func (nc *Client) createPublishJobSpec(job *types.Job) (*nomadapi.Job, error) {
	jobID := fmt.Sprintf("publish-%s", job.ID)
	
	// Resource limits (minimal for publish phase)
	cpu := 500
	memory := 1024
	disk := 2048
	
	// Create temporary and final image names
	tempImageName := fmt.Sprintf("%s/%s/%s:latest", 
		nc.config.Build.RegistryConfig.URL, 
		nc.config.Build.RegistryConfig.TempPrefix, 
		job.ID)
	
	// Build publish commands for each tag
	publishCommands := []string{
		"#!/bin/bash",
		"set -euo pipefail",
		"",
		"# Pull the tested image",
		fmt.Sprintf("buildah pull docker://%s", tempImageName),
		"",
		"# Tag and push to final destinations",
	}
	
	for _, tag := range job.Config.ImageTags {
		finalImageName := fmt.Sprintf("%s:%s", job.Config.RegistryURL, tag)
		publishCommands = append(publishCommands,
			fmt.Sprintf("buildah tag %s %s", tempImageName, finalImageName),
			fmt.Sprintf("buildah push %s docker://%s", finalImageName, finalImageName),
			fmt.Sprintf("echo 'Published image: %s'", finalImageName),
			"",
		)
	}
	
	publishCommands = append(publishCommands, "echo 'All images published successfully'")
	
	jobSpec := &nomadapi.Job{
		ID:          &jobID,
		Name:        &jobID,
		Type:        stringPtr("batch"),
		Namespace:   stringPtr(nc.config.Nomad.Namespace),
		Region:      stringPtr(nc.config.Nomad.Region),
		Datacenters: nc.config.Nomad.Datacenters,
		Meta: map[string]string{
			"build-service-job-id": job.ID,
			"phase":                "publish",
		},
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:        stringPtr("publish"),
				Count:       intPtr(1),
				Constraints: []*nomadapi.Constraint{}, // Override automatic constraints
				RestartPolicy: &nomadapi.RestartPolicy{
					Attempts: intPtr(1), // Allow one retry for publish
				},
				Tasks: []*nomadapi.Task{
					{
						Name:   "main",
						Driver: "docker",
						Config: map[string]interface{}{
							"image":   "quay.io/buildah/stable:latest",
							"command": "/bin/bash",
							"args":    []string{"-c", strings.Join(publishCommands, "\n")},
							// Add capabilities required for Buildah user namespaces
							"cap_add": []string{
								"SYS_ADMIN",     // Required for mount operations
								"SETUID",        // Required for user namespace operations
								"SETGID",        // Required for user namespace operations
								"SYS_CHROOT",    // Required for chroot operations
							},
							// Enable user namespace mapping
							"userns_mode": "host",
							// Add security options for user namespace
							"security_opt": []string{
								"seccomp=unconfined",
								"apparmor=unconfined",
							},
							// Mount cgroup filesystem for container management
							"volumes": []string{
								"/sys/fs/cgroup:/sys/fs/cgroup:rw",
								"/tmp:/tmp:rw",  // Ensure tmp is writable
							},
							// Removed /dev/fuse device to avoid version constraints
						},
						Env: map[string]string{
							"BUILDAH_ISOLATION": "chroot",  // Use chroot isolation to avoid cgroup issues
							"STORAGE_DRIVER":    "vfs",     // Use VFS storage driver for better compatibility
							"BUILDAH_FORMAT":    "oci",     // Ensure OCI format
							"BUILDAH_LAYERS":    "false",   // Disable layer caching to simplify
							"_BUILDAH_STARTED_IN_USERNS": "1", // Tell buildah we're in a user namespace
							"BUILDAH_ROOTLESS":  "1",       // Enable rootless mode
							// Remove cgroup manager to avoid delegation issues
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: &nc.config.Build.KillTimeout, // Configurable timeout
						Vault: &nomadapi.Vault{
							Policies:   []string{"nomad-build-service"},
							ChangeMode: stringPtr("restart"),
							Role:       "nomad-workloads", // Use the correct JWT role
						},
						Templates: publishTemplates(job),
					},
				},
				EphemeralDisk: &nomadapi.EphemeralDisk{
					SizeMB: intPtr(disk),
				},
			},
		},
	}
	
	return jobSpec, nil
}

// createCleanupJobSpec creates a Nomad job specification for cleanup operations
func (nc *Client) createCleanupJobSpec(job *types.Job) *nomadapi.Job {
	jobID := fmt.Sprintf("cleanup-%s", job.ID)
	
	// Create temporary image name for cleanup
	tempImageName := fmt.Sprintf("%s/%s/%s:latest", 
		nc.config.Build.RegistryConfig.URL, 
		nc.config.Build.RegistryConfig.TempPrefix, 
		job.ID)
	
	cleanupCommands := []string{
		"#!/bin/bash",
		"set -euo pipefail",
		"",
		"# Remove temporary image from registry",
		"# Note: This is a simplified example - actual registry cleanup depends on registry type",
		fmt.Sprintf("echo 'Cleaning up temporary image: %s'", tempImageName),
		"# Add actual cleanup commands here based on your registry",
		"",
		"echo 'Cleanup completed'",
	}
	
	jobSpec := &nomadapi.Job{
		ID:          &jobID,
		Name:        &jobID,
		Type:        stringPtr("batch"),
		Namespace:   stringPtr(nc.config.Nomad.Namespace),
		Region:      stringPtr(nc.config.Nomad.Region),
		Datacenters: nc.config.Nomad.Datacenters,
		Meta: map[string]string{
			"build-service-job-id": job.ID,
			"phase":                "cleanup",
		},
		TaskGroups: []*nomadapi.TaskGroup{
			{
				Name:  stringPtr("cleanup"),
				Count: intPtr(1),
				RestartPolicy: &nomadapi.RestartPolicy{
					Attempts: intPtr(0),
				},
				Tasks: []*nomadapi.Task{
					{
						Name:   "main",
						Driver: "docker",
						Config: map[string]interface{}{
							"image":   "quay.io/buildah/stable:latest",
							"command": "/bin/bash",
							"args":    []string{"-c", strings.Join(cleanupCommands, "\n")},
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(100),
							MemoryMB: intPtr(256),
							DiskMB:   intPtr(512),
						},
						KillTimeout: &nc.config.Build.KillTimeout,
					},
				},
				EphemeralDisk: &nomadapi.EphemeralDisk{
					SizeMB: intPtr(512),
				},
			},
		},
	}
	
	return jobSpec
}

// Helper functions for creating Nomad API types
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func durationPtr(d string) *time.Duration {
	duration, _ := time.ParseDuration(d)
	return &duration
}

// buildTemplates creates Vault templates for a job, conditionally including registry credentials
func buildTemplates(job *types.Job) []*nomadapi.Template {
	templates := []*nomadapi.Template{
		// Git credentials template (always included)
		{
			DestPath:   stringPtr("/secrets/git-creds"),
			ChangeMode: stringPtr("restart"),
			EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export GIT_USERNAME="{{ .Data.data.username }}"
export GIT_PASSWORD="{{ .Data.data.password }}"
export GIT_SSH_KEY="{{ .Data.data.ssh_key }}"
{{- end -}}
`, job.Config.GitCredentialsPath)),
		},
	}
	
	// Only add registry credentials template if path is provided and not empty
	if job.Config.RegistryCredentialsPath != "" {
		registryTemplate := &nomadapi.Template{
			DestPath:   stringPtr("/secrets/registry-creds"),
			ChangeMode: stringPtr("restart"),
			EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export REGISTRY_USERNAME="{{ .Data.data.username }}"
export REGISTRY_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
`, job.Config.RegistryCredentialsPath)),
		}
		templates = append(templates, registryTemplate)
	}
	
	return templates
}

// testTemplates creates Vault templates for the test phase (only registry credentials if needed)
func testTemplates(job *types.Job) []*nomadapi.Template {
	var templates []*nomadapi.Template
	
	// Only add registry credentials template if path is provided and not empty
	if job.Config.RegistryCredentialsPath != "" {
		registryTemplate := &nomadapi.Template{
			DestPath:   stringPtr("/secrets/registry-creds"),
			ChangeMode: stringPtr("restart"),
			EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export REGISTRY_USERNAME="{{ .Data.data.username }}"
export REGISTRY_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
`, job.Config.RegistryCredentialsPath)),
		}
		templates = append(templates, registryTemplate)
	}
	
	return templates
}

// publishTemplates creates Vault templates for the publish phase (only registry credentials if needed)
func publishTemplates(job *types.Job) []*nomadapi.Template {
	var templates []*nomadapi.Template
	
	// Only add registry credentials template if path is provided and not empty
	if job.Config.RegistryCredentialsPath != "" {
		registryTemplate := &nomadapi.Template{
			DestPath:   stringPtr("/secrets/registry-creds"),
			ChangeMode: stringPtr("restart"),
			EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export REGISTRY_USERNAME="{{ .Data.data.username }}"
export REGISTRY_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
`, job.Config.RegistryCredentialsPath)),
		}
		templates = append(templates, registryTemplate)
	}
	
	return templates
}