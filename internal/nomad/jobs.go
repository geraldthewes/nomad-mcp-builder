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
	
	// Build timeout
	timeout := int64(nc.config.Build.BuildTimeout.Seconds())
	if job.Config.BuildTimeout != nil {
		timeout = int64(job.Config.BuildTimeout.Seconds())
	}
	
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
		"#!/bin/bash",
		"set -euo pipefail",
		"",
		"# Clone repository",
		fmt.Sprintf("git clone %s /tmp/repo", job.Config.RepoURL),
		"cd /tmp/repo",
		fmt.Sprintf("git checkout %s", job.Config.GitRef),
		"",
		"# Build image with Buildah",
		fmt.Sprintf("buildah bud --isolation=chroot --file %s --tag %s .", 
			job.Config.DockerfilePath, tempImageName),
		"",
		"# Push temporary image to registry",
		fmt.Sprintf("buildah push %s docker://%s", tempImageName, tempImageName),
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
				Name:  stringPtr("build"),
				Count: intPtr(1),
				RestartPolicy: &nomadapi.RestartPolicy{
					Attempts: intPtr(0), // No restart for build jobs
				},
				Tasks: []*nomadapi.Task{
					{
						Name:   "main",
						Driver: "docker",
						Config: map[string]interface{}{
							"image": "quay.io/buildah/stable:latest",
							"command": "/bin/bash",
							"args": []string{"-c", strings.Join(buildCommands, "\n")},
							"privileged": false,
							"devices": []map[string]interface{}{
								{
									"host_path":      "/dev/fuse",
									"container_path": "/dev/fuse",
								},
							},
							"mount": []map[string]interface{}{
								{
									"type":   "bind",
									"source": nc.config.Build.BuildCachePath,
									"target": "/var/lib/containers",
									"bind_options": map[string]interface{}{
										"propagation": "rprivate",
									},
								},
							},
						},
						Env: map[string]string{
							"BUILDAH_ISOLATION": "chroot",
							"STORAGE_DRIVER":    "overlay",
							"STORAGE_OPTS":      "overlay.mount_program=/usr/bin/fuse-overlayfs",
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: durationPtr("30s"),
						Vault: &nomadapi.Vault{
							Policies:   []string{"nomad-build-service"},
							ChangeMode: stringPtr("restart"),
							Role:       "nomad-workloads", // Use the correct JWT role
						},
						Templates: []*nomadapi.Template{
							// Git credentials template
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
							// Registry credentials template
							{
								DestPath:   stringPtr("/secrets/registry-creds"),
								ChangeMode: stringPtr("restart"),
								EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export REGISTRY_USERNAME="{{ .Data.data.username }}"
export REGISTRY_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
`, job.Config.RegistryCredentialsPath)),
							},
						},
					},
				},
				EphemeralDisk: &nomadapi.EphemeralDisk{
					SizeMB: intPtr(disk),
				},
			},
		},
		// Set job timeout
		Stop:        boolPtr(true),
	}
	
	// Add kill timeout at job level
	if timeout > 0 {
		jobSpec.TaskGroups[0].Tasks[0].KillTimeout = durationPtr(fmt.Sprintf("%ds", timeout))
	}
	
	return jobSpec, nil
}

// createTestJobSpec creates a Nomad job specification for the test phase
func (nc *Client) createTestJobSpec(job *types.Job) (*nomadapi.Job, error) {
	jobID := fmt.Sprintf("test-%s", job.ID)
	
	// Test timeout
	timeout := int64(nc.config.Build.TestTimeout.Seconds())
	if job.Config.TestTimeout != nil {
		timeout = int64(job.Config.TestTimeout.Seconds())
	}
	
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
				Name:  stringPtr("test"),
				Count: intPtr(1),
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
							"devices": []map[string]interface{}{
								{
									"host_path":      "/dev/fuse",
									"container_path": "/dev/fuse",
								},
							},
						},
						Env: map[string]string{
							"BUILDAH_ISOLATION": "chroot",
							"STORAGE_DRIVER":    "overlay",
							"STORAGE_OPTS":      "overlay.mount_program=/usr/bin/fuse-overlayfs",
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: durationPtr(fmt.Sprintf("%ds", timeout)),
						Vault: &nomadapi.Vault{
							Policies:   []string{"nomad-build-service"},
							ChangeMode: stringPtr("restart"),
							Role:       "nomad-workloads", // Use the correct JWT role
						},
						Templates: []*nomadapi.Template{
							// Registry credentials for pulling temp image
							{
								DestPath:   stringPtr("/secrets/registry-creds"),
								ChangeMode: stringPtr("restart"),
								EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export REGISTRY_USERNAME="{{ .Data.data.username }}"
export REGISTRY_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
`, job.Config.RegistryCredentialsPath)),
							},
						},
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
				Name:  stringPtr("publish"),
				Count: intPtr(1),
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
							"devices": []map[string]interface{}{
								{
									"host_path":      "/dev/fuse",
									"container_path": "/dev/fuse",
								},
							},
						},
						Env: map[string]string{
							"BUILDAH_ISOLATION": "chroot",
							"STORAGE_DRIVER":    "overlay",
							"STORAGE_OPTS":      "overlay.mount_program=/usr/bin/fuse-overlayfs",
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: durationPtr("300s"), // 5 minutes for registry operations
						Vault: &nomadapi.Vault{
							Policies:   []string{"nomad-build-service"},
							ChangeMode: stringPtr("restart"),
							Role:       "nomad-workloads", // Use the correct JWT role
						},
						Templates: []*nomadapi.Template{
							// Registry credentials
							{
								DestPath:   stringPtr("/secrets/registry-creds"),
								ChangeMode: stringPtr("restart"),
								EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export REGISTRY_USERNAME="{{ .Data.data.username }}"
export REGISTRY_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
`, job.Config.RegistryCredentialsPath)),
							},
						},
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
						KillTimeout: durationPtr("60s"),
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