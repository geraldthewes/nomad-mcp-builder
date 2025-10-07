# Job Specification

This document describes the complete job configuration schema for the Nomad Build Service.

## Overview

Job configurations can be provided in either **JSON** or **YAML** format. The CLI tool supports YAML with a two-file approach (global + per-build), while the MCP and JSON-RPC APIs accept either format.

## Required Fields

These fields **must** be provided in every job configuration:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `owner` | string | Job owner/team identifier | `"myteam"` |
| `repo_url` | string | Git repository URL (HTTP/HTTPS) | `"https://github.com/org/repo.git"` |
| `image_name` | string | Base name for Docker image | `"myservice"` |
| `image_tags` | array[string] | Image tags to apply | `["v1.0.0", "latest"]` |
| `registry_url` | string | Target registry URL | `"registry.example.com:5000/myapp"` |

## Optional Fields with Defaults

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `git_ref` | string | `"main"` | Git branch, tag, or commit SHA |
| `dockerfile_path` | string | `"Dockerfile"` | Path to Dockerfile in repo |
| `git_credentials_path` | string | - | Vault path for Git credentials |
| `registry_credentials_path` | string | - | Vault path for registry credentials |

## Optional Fields

### Testing Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `test_commands` | array[string] | `[]` | Commands to run in test phase |
| `test_entry_point` | boolean | `false` | Test container's ENTRYPOINT |

**Note**: If both `test_commands` and `test_entry_point` are empty/false, the test phase is **skipped**.

**Examples**:

```yaml
# Run specific test commands
test_commands:
  - /app/run-tests.sh
  - /app/validate.sh

# Or test the entrypoint
test_entry_point: true
```

### Resource Limits

Resource limits can be specified globally or per-phase. Per-phase limits override global limits.

#### Global Resource Limits (Legacy)

```yaml
resource_limits:
  cpu: "2000"      # MHz
  memory: "4096"   # MB
  disk: "20480"    # MB
```

Applies to all phases (build, test, publish) unless overridden.

#### Per-Phase Resource Limits (Recommended)

```yaml
resource_limits:
  build:
    cpu: "2000"
    memory: "4096"
    disk: "20480"
  test:
    cpu: "1000"
    memory: "2048"
    disk: "10240"
  publish:
    cpu: "1000"
    memory: "2048"
    disk: "10240"
```

**Default values** (if not specified):
- CPU: `1000` MHz
- Memory: `2048` MB
- Disk: `10240` MB

### Timeouts

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `build_timeout` | duration | `30m` | Maximum build phase duration |
| `test_timeout` | duration | `15m` | Maximum test phase duration |

**Duration format**: Use Go duration strings like `"30m"`, `"1h30m"`, `"45s"`.

### Build Cache

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `clear_cache` | boolean | `false` | Clear Buildah layer cache before build |

### Webhooks

Configure HTTP webhooks for build notifications:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `webhook_url` | string | - | URL to POST notifications |
| `webhook_secret` | string | - | HMAC secret for signature verification |
| `webhook_on_success` | boolean | `true` | Send webhook on success |
| `webhook_on_failure` | boolean | `true` | Send webhook on failure |
| `webhook_headers` | map[string]string | - | Custom HTTP headers |

## Complete Examples

### Minimal Configuration (JSON)

```json
{
  "owner": "myteam",
  "repo_url": "https://github.com/myorg/myservice.git",
  "image_name": "myservice",
  "image_tags": ["v1.0.0"],
  "registry_url": "registry.example.com:5000/myapp"
}
```

### Minimal Configuration (YAML)

```yaml
owner: myteam
repo_url: https://github.com/myorg/myservice.git
image_name: myservice
image_tags:
  - v1.0.0
registry_url: registry.example.com:5000/myapp
```

### Complete Configuration with All Options (YAML)

```yaml
# Required fields
owner: myteam
repo_url: https://github.com/myorg/myservice.git
image_name: myservice
image_tags:
  - v1.2.3
  - latest
  - stable
registry_url: registry.example.com:5000/myapp

# Git configuration
git_ref: feature/new-feature
git_credentials_path: secret/nomad/jobs/git-credentials

# Build configuration
dockerfile_path: docker/Dockerfile
registry_credentials_path: secret/nomad/jobs/registry-credentials

# Testing
test_commands:
  - /app/run-tests.sh
  - /app/validate-config.sh

# Resource limits (per-phase)
resource_limits:
  build:
    cpu: "2000"
    memory: "4096"
    disk: "20480"
  test:
    cpu: "1000"
    memory: "2048"
    disk: "10240"
  publish:
    cpu: "1000"
    memory: "2048"
    disk: "10240"

# Timeouts
build_timeout: 45m
test_timeout: 20m

# Cache management
clear_cache: false

# Webhooks
webhook_url: https://my-webhook.example.com/notify
webhook_secret: my-secret-key
webhook_on_success: true
webhook_on_failure: true
webhook_headers:
  X-Custom-Header: my-value
  X-Team: myteam
```

### Global + Per-Build Configuration (CLI)

**deploy/global.yaml** (shared settings):

```yaml
owner: myteam
repo_url: https://github.com/myorg/myservice.git
git_credentials_path: secret/nomad/jobs/git-credentials
dockerfile_path: Dockerfile
image_name: myservice
registry_url: registry.example.com:5000/myapp
registry_credentials_path: secret/nomad/jobs/registry-credentials

# Default resource limits
resource_limits:
  build:
    cpu: "2000"
    memory: "4096"
    disk: "20480"
  test:
    cpu: "1000"
    memory: "2048"
    disk: "10240"
  publish:
    cpu: "1000"
    memory: "2048"
    disk: "10240"
```

**build.yaml** (per-build overrides):

```yaml
git_ref: feature/auth-fix
image_tags:
  - test
  - dev
test_entry_point: true
webhook_url: https://hooks.slack.com/services/T00/B00/XXX
```

**Usage**:
```bash
nomad-build submit-job -global deploy/global.yaml -config build.yaml
```

**Result**: The CLI merges these configs, with `build.yaml` values overriding `global.yaml`, and automatically adds a branch-aware version tag.

## Vault Credentials Format

### Git Credentials

Store at the path specified in `git_credentials_path`:

```bash
vault kv put secret/nomad/jobs/git-credentials \
  username="git-user" \
  password="ghp_token123..." \
  ssh_key="$(cat ~/.ssh/id_rsa)"  # Optional
```

### Registry Credentials

Store at the path specified in `registry_credentials_path`:

```bash
vault kv put secret/nomad/jobs/registry-credentials \
  username="registry-user" \
  password="registry-password"
```

**Note**: Registry credentials are only needed for **private registries**. Public registries (like Docker Hub for public images) don't require credentials.

## Field Validation Rules

### String Fields
- **owner**: Must be non-empty
- **repo_url**: Must be non-empty, should be valid Git URL
- **image_name**: Must be non-empty
- **registry_url**: Must be non-empty

### Array Fields
- **image_tags**: Must have at least one tag
- **test_commands**: Can be empty (skips test phase)

### Path Fields
- **dockerfile_path**: Defaults to `"Dockerfile"` if empty
- **git_ref**: Defaults to `"main"` if empty

### Resource Limits
- **cpu**: Positive integer as string (MHz)
- **memory**: Positive integer as string (MB)
- **disk**: Positive integer as string (MB)

### Timeouts
- **build_timeout**: Valid Go duration string (e.g., `"30m"`, `"1h"`)
- **test_timeout**: Valid Go duration string

## CLI Version Management

When using the CLI tool, additional behavior applies:

### Automatic Version Tags

The CLI automatically:
1. **Increments** patch version in `deploy/version.yaml`
2. **Detects** current Git branch
3. **Generates** branch-aware tag (e.g., `feature-auth-v0.1.5`)
4. **Appends** the tag to `image_tags`

**Example**:
```bash
# deploy/version.yaml shows: version: { major: 0, minor: 1, patch: 4 }
# Current branch: feature-auth
# Your build.yaml specifies: image_tags: ["test"]

nomad-build submit-job -config build.yaml

# Actual tags sent to server:
# - "test" (from build.yaml)
# - "feature-auth-v0.1.5" (auto-generated)
# Version file updated to: version: { major: 0, minor: 1, patch: 5 }
```

### Additional Tags via CLI

You can add more tags using `--image-tags`:

```bash
nomad-build submit-job -config build.yaml --image-tags "latest,stable"

# Final tags:
# - Tags from build.yaml
# - Auto-generated version tag
# - "latest", "stable" (from --image-tags)
```

## API Submission Formats

### MCP Protocol (JSON-RPC)

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "submitJob",
    "arguments": {
      "job_config": {
        "owner": "myteam",
        "repo_url": "https://github.com/myorg/myservice.git",
        "git_ref": "main",
        "dockerfile_path": "Dockerfile",
        "image_name": "myservice",
        "image_tags": ["v1.0.0"],
        "registry_url": "registry.example.com:5000/myapp"
      }
    }
  }
}
```

### Direct JSON-RPC API

```bash
curl -X POST http://localhost:8080/json/submitJob \
  -H "Content-Type: application/json" \
  -d '{
    "job_config": {
      "owner": "myteam",
      "repo_url": "https://github.com/myorg/myservice.git",
      "git_ref": "main",
      "image_name": "myservice",
      "image_tags": ["v1.0.0"],
      "registry_url": "registry.example.com:5000/myapp"
    }
  }'
```

### CLI Tool (YAML)

```bash
# Create build.yaml with the configuration
nomad-build submit-job -config build.yaml
```

## Build Phases

Understanding the three-phase workflow:

### 1. Build Phase
- Clones Git repository
- Builds Docker image using Buildah
- Pushes to temporary registry location: `{registry_url}/temp/{job_id}:build`
- Uses resource limits from `resource_limits.build` or global `resource_limits`
- Timeout: `build_timeout` (default: 30m)

### 2. Test Phase
- **Skipped if**: No `test_commands` and `test_entry_point` is `false`
- Runs Docker container with built image
- Executes test commands OR tests entrypoint
- Multiple test jobs run in parallel if multiple test commands specified
- Uses resource limits from `resource_limits.test` or global `resource_limits`
- Timeout: `test_timeout` (default: 15m)

### 3. Publish Phase
- Pulls image from temporary location
- Re-tags with all specified `image_tags`
- Pushes final images to `{registry_url}/{image_name}:{tag}`
- Cleans up temporary image
- Uses resource limits from `resource_limits.publish` or global `resource_limits`

## Common Patterns

### Public Repository, Public Registry
```yaml
owner: myteam
repo_url: https://github.com/myorg/public-repo.git
git_ref: main
dockerfile_path: Dockerfile
image_name: myapp
image_tags: [v1.0.0]
registry_url: docker.io/myorg
```

### Private Repository, Private Registry
```yaml
owner: myteam
repo_url: https://github.com/myorg/private-repo.git
git_credentials_path: secret/nomad/jobs/git-credentials
git_ref: main
dockerfile_path: Dockerfile
image_name: myapp
image_tags: [v1.0.0]
registry_url: registry.example.com:5000/myapp
registry_credentials_path: secret/nomad/jobs/registry-credentials
```

### With Tests and Custom Resources
```yaml
owner: myteam
repo_url: https://github.com/myorg/myservice.git
git_ref: main
dockerfile_path: Dockerfile
image_name: myservice
image_tags: [v1.0.0]
registry_url: registry.example.com:5000/myapp

test_commands:
  - npm test
  - npm run lint

resource_limits:
  build:
    cpu: "4000"
    memory: "8192"
    disk: "40960"
  test:
    cpu: "2000"
    memory: "4096"
    disk: "20480"
```

### With Webhooks for CI/CD Integration
```yaml
owner: myteam
repo_url: https://github.com/myorg/myservice.git
git_ref: main
dockerfile_path: Dockerfile
image_name: myservice
image_tags: [v1.0.0]
registry_url: registry.example.com:5000/myapp

webhook_url: https://ci.example.com/webhook/build-complete
webhook_secret: super-secret-key
webhook_on_success: true
webhook_on_failure: true
webhook_headers:
  X-CI-Project: myservice
  X-CI-Pipeline: production
```
