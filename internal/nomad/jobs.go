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
		"#!/bin/bash",
		"set -euo pipefail",
		"",
		"# Clone repository", 
		fmt.Sprintf("git clone %s /tmp/repo", job.Config.RepoURL),
		"cd /tmp/repo",
		fmt.Sprintf("git checkout %s", job.Config.GitRef),
		"",
		"# Ensure cache directories exist and have proper permissions",
		"mkdir -p /var/lib/containers /var/lib/shared /var/lib/containers/tmp",
		"",
		"# Configure Buildah to use fuse-overlayfs to prevent layer conflicts",
		"export STORAGE_DRIVER=overlay",
		"export STORAGE_OPTS='overlay.mount_program=/usr/bin/fuse-overlayfs,overlay.mountopt=nodev'",
		"",
		"# Build image with Buildah using layer caching and fuse-overlayfs",
		fmt.Sprintf("buildah bud --isolation=chroot --layers --file %s --tag %s .", 
			job.Config.DockerfilePath, tempImageName),
		"",
		"# Authenticate with registry if credentials are provided and non-empty",
		"if [ -f /secrets/registry-creds ]; then",
		"  source /secrets/registry-creds",
		"  if [ -n \"$REGISTRY_USERNAME\" ] && [ -n \"$REGISTRY_PASSWORD\" ]; then",
		"    echo 'Logging in to registry with credentials...'",
		fmt.Sprintf("    buildah login --username \"$REGISTRY_USERNAME\" --password \"$REGISTRY_PASSWORD\" %s", registryHost(tempImageName)),
		"  else",
		"    echo 'No registry credentials provided, attempting anonymous push...'",
		"  fi",
		"else",
		"  echo 'No registry credentials file, attempting anonymous push...'",
		"fi",
		"",
		"# Push temporary image to registry using fuse-overlayfs",
		fmt.Sprintf("STORAGE_DRIVER=overlay STORAGE_OPTS='overlay.mount_program=/usr/bin/fuse-overlayfs,overlay.mountopt=nodev' buildah push %s docker://%s", tempImageName, tempImageName),
		"",
		"# Verify image is fully available in registry before proceeding",
		"echo 'Verifying image availability in registry...'",
		fmt.Sprintf("for i in {1..30}; do"),
		fmt.Sprintf("  if STORAGE_DRIVER=overlay STORAGE_OPTS='overlay.mount_program=/usr/bin/fuse-overlayfs,overlay.mountopt=nodev' buildah pull docker://%s >/dev/null 2>&1; then", tempImageName),
		"    echo 'Image verified available in registry'",
		"    break",
		"  else",
		"    echo \"Attempt $i: Image not yet available, waiting 2 seconds...\"",
		"    sleep 2",
		"  fi",
		"  if [ $i -eq 30 ]; then",
		"    echo 'ERROR: Image verification timeout after 60 seconds'",
		"    exit 1",
		"  fi",
		"done",
		"",
		"# Force sync file system to ensure all writes are committed",
		"sync",
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
							"image": "quay.io/buildah/stable:latest",
							"command": "/bin/bash",
							"args": []string{"-c", strings.Join(buildCommands, "\n")},
							"privileged": false, // Use proper security configuration instead of privileged
							"devices": []map[string]interface{}{
								{
									"host_path":      "/dev/fuse",
									"container_path": "/dev/fuse",
								},
							},
							"security_opt": []string{
								"seccomp=unconfined", // Required for Buildah syscalls
								"apparmor=unconfined",
							},
							"volumes": []string{
								"/opt/nomad/data/buildah-cache:/var/lib/containers:rw",
								"/opt/nomad/data/buildah-shared:/var/lib/shared:ro", // Additional image stores
								"/etc/docker/certs.d:/etc/containers/certs.d:ro",   // Registry certificates
							},
						},
						Env: map[string]string{
							"BUILDAH_ISOLATION": "chroot",
							"STORAGE_DRIVER":    "overlay",
							"STORAGE_OPTS":      "overlay.mount_program=/usr/bin/fuse-overlayfs",
							"TMPDIR":            "/var/lib/containers/tmp", // Use persistent tmp for caching
						},
						Resources: &nomadapi.Resources{
							CPU:      intPtr(cpu),
							MemoryMB: intPtr(memory),
							DiskMB:   intPtr(disk),
						},
						KillTimeout: &nc.config.Build.KillTimeout,
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
	
	// Add Vault integration and templates only if needed
	templates := buildTemplates(job)
	if len(templates) > 0 {
		mainTask := jobSpec.TaskGroups[0].Tasks[0]
		mainTask.Vault = &nomadapi.Vault{
			Policies:   []string{"nomad-build-service"},
			ChangeMode: stringPtr("restart"),
			Role:       "nomad-workloads",
		}
		mainTask.Templates = templates
	}
	
	return jobSpec, nil
}

// createTestJobSpecs creates Nomad job specifications for the test phase using Docker driver directly
func (nc *Client) createTestJobSpecs(job *types.Job, buildNodeID string) ([]*nomadapi.Job, error) {
	var testJobs []*nomadapi.Job
	
	// Resource limits for test jobs
	cpu := 500    // Less than build phase since tests are simpler
	memory := 1024
	disk := 2048  // Tests need minimal disk
	
	if job.Config.ResourceLimits != nil {
		if job.Config.ResourceLimits.CPU != "" {
			fmt.Sscanf(job.Config.ResourceLimits.CPU, "%d", &cpu)
		}
		if job.Config.ResourceLimits.Memory != "" {
			fmt.Sscanf(job.Config.ResourceLimits.Memory, "%d", &memory)
		}
	}
	
	// Create temporary image name - this is the image built in the build phase
	tempImageName := fmt.Sprintf("%s/%s/%s:latest", 
		nc.config.Build.RegistryConfig.URL, 
		nc.config.Build.RegistryConfig.TempPrefix, 
		job.ID)
	
	// Mode 1: Create separate test jobs for each custom test command
	if len(job.Config.TestCommands) > 0 {
		for i, testCmd := range job.Config.TestCommands {
			jobID := fmt.Sprintf("test-cmd-%s-%d", job.ID, i)
			
			testJobSpec := &nomadapi.Job{
				ID:          &jobID,
				Name:        &jobID,
				Type:        stringPtr("batch"),
				Namespace:   stringPtr(nc.config.Nomad.Namespace),
				Region:      stringPtr(nc.config.Nomad.Region),
				Datacenters: nc.config.Nomad.Datacenters,
				Meta: map[string]string{
					"build-service-job-id": job.ID,
					"phase":                "test",
					"test-type":            "command",
					"test-index":           fmt.Sprintf("%d", i),
					"test-command":         testCmd,
				},
				TaskGroups: []*nomadapi.TaskGroup{
					{
						Name:        stringPtr("test"),
						Count:       intPtr(1),
						Constraints: func() []*nomadapi.Constraint {
							if buildNodeID != "" {
								// Prevent test jobs from running on the same node as build job to avoid Docker layer conflicts
								return []*nomadapi.Constraint{
									{
										LTarget: "${node.unique.id}",
										RTarget: buildNodeID,
										Operand: "!=",
									},
								}
							}
							return []*nomadapi.Constraint{} // No constraints if buildNodeID is empty
						}(),
						RestartPolicy: &nomadapi.RestartPolicy{
							Attempts: intPtr(0), // No retries for test failures
						},
						Tasks: []*nomadapi.Task{
							{
								Name:   "main",
								Driver: "docker",
								Config: map[string]interface{}{
									"image":   tempImageName,  // Use the built image directly
									"command": "sh",
									"args":    []string{"-c", testCmd},
									"force_pull": true, // Force Docker to always pull fresh image to avoid layer conflicts
									// Add registry certificates for private registries
									"volumes": []string{
										"/etc/docker/certs.d:/etc/docker/certs.d:ro",
									},
								},
								Resources: &nomadapi.Resources{
									CPU:      intPtr(cpu),
									MemoryMB: intPtr(memory),
									DiskMB:   intPtr(disk),
								},
								KillTimeout: &nc.config.Build.KillTimeout,
							},
						},
						EphemeralDisk: &nomadapi.EphemeralDisk{
							SizeMB: intPtr(disk),
						},
					},
				},
			}
			
			// Add registry credentials if needed for private registries
			if job.Config.RegistryCredentialsPath != "" {
				templates := testTemplates(job)
				if len(templates) > 0 {
					mainTask := testJobSpec.TaskGroups[0].Tasks[0]
					mainTask.Vault = &nomadapi.Vault{
						Policies:   []string{"nomad-build-service"},
						ChangeMode: stringPtr("restart"),
						Role:       "nomad-workloads",
					}
					mainTask.Templates = templates
				}
			}
			
			testJobs = append(testJobs, testJobSpec)
		}
	}
	
	// Mode 2: Create a test job that runs the image's entry point/CMD
	if job.Config.TestEntryPoint {
		jobID := fmt.Sprintf("test-entry-%s", job.ID)
		
		testJobSpec := &nomadapi.Job{
			ID:          &jobID,
			Name:        &jobID,
			Type:        stringPtr("batch"),
			Namespace:   stringPtr(nc.config.Nomad.Namespace),
			Region:      stringPtr(nc.config.Nomad.Region),
			Datacenters: nc.config.Nomad.Datacenters,
			Meta: map[string]string{
				"build-service-job-id": job.ID,
				"phase":                "test",
				"test-type":            "entrypoint",
			},
			TaskGroups: []*nomadapi.TaskGroup{
				{
					Name:        stringPtr("test"),
					Count:       intPtr(1),
					Constraints: func() []*nomadapi.Constraint {
						if buildNodeID != "" {
							// Prevent test jobs from running on the same node as build job to avoid Docker layer conflicts
							return []*nomadapi.Constraint{
								{
									LTarget: "${node.unique.id}",
									RTarget: buildNodeID,
									Operand: "!=",
								},
							}
						}
						return []*nomadapi.Constraint{} // No constraints if buildNodeID is empty
					}(),
					RestartPolicy: &nomadapi.RestartPolicy{
						Attempts: intPtr(0), // No retries for test failures
					},
					Tasks: []*nomadapi.Task{
						{
							Name:   "main",
							Driver: "docker",
							Config: map[string]interface{}{
								"image": tempImageName, // Use the built image directly - no command/args means use ENTRYPOINT/CMD
								"force_pull": true, // Force Docker to always pull fresh image to avoid layer conflicts
								// Add registry certificates for private registries
								"volumes": []string{
									"/etc/docker/certs.d:/etc/docker/certs.d:ro",
								},
							},
							Resources: &nomadapi.Resources{
								CPU:      intPtr(cpu),
								MemoryMB: intPtr(memory),
								DiskMB:   intPtr(disk),
							},
							KillTimeout: &nc.config.Build.KillTimeout,
						},
					},
					EphemeralDisk: &nomadapi.EphemeralDisk{
						SizeMB: intPtr(disk),
					},
				},
			},
		}
		
		// Add registry credentials if needed for private registries
		if job.Config.RegistryCredentialsPath != "" {
			templates := testTemplates(job)
			if len(templates) > 0 {
				mainTask := testJobSpec.TaskGroups[0].Tasks[0]
				mainTask.Vault = &nomadapi.Vault{
					Policies:   []string{"nomad-build-service"},
					ChangeMode: stringPtr("restart"),
					Role:       "nomad-workloads",
				}
				mainTask.Templates = templates
			}
		}
		
		testJobs = append(testJobs, testJobSpec)
	}
	
	// If no test configuration, return empty array (caller will skip test phase)
	if len(testJobs) == 0 {
		return []*nomadapi.Job{}, nil
	}
	
	return testJobs, nil
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
		finalImageName := fmt.Sprintf("%s/%s:%s", job.Config.RegistryURL, job.Config.ImageName, tag)
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
					Attempts: intPtr(0), // No restart for publish jobs
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
								"/etc/docker/certs.d:/etc/containers/certs.d:ro",   // Registry certificates
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
					},
				},
				EphemeralDisk: &nomadapi.EphemeralDisk{
					SizeMB: intPtr(disk),
				},
			},
		},
	}
	
	// Add Vault integration and templates only if needed
	templates := publishTemplates(job)
	if len(templates) > 0 {
		mainTask := jobSpec.TaskGroups[0].Tasks[0]
		mainTask.Vault = &nomadapi.Vault{
			Policies:   []string{"nomad-build-service"},
			ChangeMode: stringPtr("restart"),
			Role:       "nomad-workloads",
		}
		mainTask.Templates = templates
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
	var templates []*nomadapi.Template
	
	// Debug logging
	fmt.Printf("DEBUG: buildTemplates - GitCredentialsPath: '%s', RegistryCredentialsPath: '%s'\n", 
		job.Config.GitCredentialsPath, job.Config.RegistryCredentialsPath)
	
	// Only add git credentials template if path is provided and not empty
	if job.Config.GitCredentialsPath != "" {
		gitTemplate := &nomadapi.Template{
			DestPath:   stringPtr("/secrets/git-creds"),
			ChangeMode: stringPtr("restart"),
			EmbeddedTmpl: stringPtr(fmt.Sprintf(`
{{- with secret "%s" -}}
export GIT_USERNAME="{{ .Data.data.username }}"
export GIT_PASSWORD="{{ .Data.data.password }}"
export GIT_SSH_KEY="{{ .Data.data.ssh_key }}"
{{- end -}}
`, job.Config.GitCredentialsPath)),
		}
		templates = append(templates, gitTemplate)
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

// registryHost extracts the registry host from an image name
// e.g., "registry.cluster:5000/bdtemp/image:tag" -> "registry.cluster:5000"
func registryHost(imageName string) string {
	parts := strings.Split(imageName, "/")
	if len(parts) > 1 && strings.Contains(parts[0], ":") {
		return parts[0]
	}
	return "docker.io" // default to Docker Hub if no registry specified
}