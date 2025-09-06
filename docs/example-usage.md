# Enhanced Logging Usage Examples

The Nomad Build Service now supports enhanced logging with the following environment variables:

## Environment Variables

- `LOG_LEVEL`: Controls the overall log verbosity (trace, debug, info, warn, error, fatal, panic)
- `LOG_JOB_SPECS`: Enable/disable job specification logging (true/false, default: false)
- `LOG_HCL_FORMAT`: Output job specs in HCL format instead of JSON (true/false, default: false)

## Usage Examples

### Debug Job Placement Issues

To debug why jobs are not being placed by Nomad, set these environment variables:

```bash
export LOG_LEVEL=debug
export LOG_JOB_SPECS=true
export LOG_HCL_FORMAT=true
```

This will output the complete HCL job specification that would be submitted to Nomad, allowing you to:
1. Copy the HCL and test it manually with `nomad job run`
2. See the exact configuration being generated
3. Debug resource requirements, constraints, and other placement issues

### Production Logging (JSON format)

For production environments where logs are processed programmatically:

```bash
export LOG_LEVEL=info
export LOG_JOB_SPECS=true
export LOG_HCL_FORMAT=false
```

This outputs job specifications in JSON format for easier parsing by log aggregation tools.

### Normal Operation

For normal operation without debug output:

```bash
export LOG_LEVEL=info
export LOG_JOB_SPECS=false
```

## Sample Output

With `LOG_LEVEL=debug`, `LOG_JOB_SPECS=true`, and `LOG_HCL_FORMAT=true`, you would see logs like:

```
DEBU[2024-01-01T12:00:00Z] Generated Nomad job specification (HCL base64 encoded) job_spec_hcl_b64=am9iICJidWlsZC1hYmMxMjMiIHsKICBuYW1lICAgICAgPSAiYnVpbGQtYWJjMTIzIgogIHR5cGUgICAgICA9ICJiYXRjaCIKICBuYW1lc3BhY2UgPSAiZGVmYXVsdCIKICByZWdpb24gICAgPSAiZ2xvYmFsIgogIGRhdGFjZW50ZXJzID0gWyJjbHVzdGVyIl0KCiAgbWV0YSB7CiAgICBidWlsZC1zZXJ2aWNlLWpvYi1pZCA9ICJhYmMxMjMiCiAgICBwaGFzZSA9ICJidWlsZCIKICB9CgogIGdyb3VwICJidWlsZCIgewogICAgY291bnQgPSAxCgogICAgcmVzdGFydCB7CiAgICAgIGF0dGVtcHRzID0gMAogICAgfQoKICAgIGVwaGVtZXJhbF9kaXNrIHsKICAgICAgc2l6ZSA9IDEwMjQwCiAgICB9CgogICAgdGFzayAibWFpbiIgewogICAgICBkcml2ZXIgPSAiZG9ja2VyIgoKICAgICAgY29uZmlnIHsKICAgICAgICBpbWFnZSA9ICJxdWF5LmlvL2J1aWxkYWgvc3RhYmxlOmxhdGVzdCIKICAgICAgfQogICAgfQogIH0KfQo= nomad_job_id=build-abc123 phase=build
INFO[2024-01-01T12:00:00Z] To decode HCL: echo 'BASE64_STRING' | base64 -d > job.hcl nomad_job_id=build-abc123 phase=build
```

## Decoding the Job Specification

To extract the HCL job definition from the logs:

### For HCL format:
```bash
# Copy the base64 string from job_spec_hcl_b64 field and decode it
echo 'BASE64_STRING_FROM_LOG' | base64 -d > job.hcl

# Now you can validate and test it directly with Nomad
nomad job validate job.hcl
nomad job run job.hcl
```

The generated HCL will look like this:
```hcl
job "build-abc123" {
  name      = "build-abc123"
  type      = "batch" 
  namespace = "default"
  region    = "global"
  datacenters = ["cluster"]

  meta {
    build-service-job-id = "abc123"
    phase = "build"
  }

  group "build" {
    count = 1

    task "main" {
      driver = "docker"

      config {
        args = [
          "-c",
          <<EOF
#!/bin/bash
set -euo pipefail

# Clone repository  
git clone https://github.com/user/repo.git /tmp/repo
cd /tmp/repo
git checkout main

# Build image with Buildah
buildah bud --isolation=chroot --file Dockerfile --tag registry.cluster:5000/bdtemp/abc123:latest .

echo 'Build completed successfully'
EOF
        ]
        image = "quay.io/buildah/stable:latest"
        command = "/bin/bash"
      }

      env {
        BUILDAH_ISOLATION = "chroot"
        STORAGE_DRIVER = "overlay"
        STORAGE_OPTS = "overlay.mount_program=/usr/bin/fuse-overlayfs"
      }

      resources {
        cpu    = 1000
        memory = 2048
        disk   = 10240
      }

      vault {
        policies = ["nomad-build-service"]
        change_mode = "restart"
        role = "nomad-workloads"
      }

      template {
        destination = "/secrets/git-creds"
        change_mode = "restart"
        data = <<EOF
{{- with secret "secret/nomad/jobs/git-credentials" -}}
export GIT_USERNAME="{{ .Data.data.username }}"
export GIT_PASSWORD="{{ .Data.data.password }}"
{{- end -}}
EOF
      }
    }
  }
}
```

### For JSON format:
```bash
# Copy the base64 string from job_spec_json_b64 field and decode it  
echo 'BASE64_STRING' | base64 -d | jq > job.json
```

This approach eliminates all the escaping and newline issues, making it trivial to extract and test the exact job specification that's being submitted to Nomad.