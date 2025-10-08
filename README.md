# Nomad Build Service

A lightweight build automation system consisting of:
- **MCP Server**: Stateless Go server providing JSON-RPC over HTTP interface
- **CLI Tool**: Command-line client with YAML configuration support and version management

The system enables users and coding agents to submit Docker image build jobs remotely using Nomad as the backend infrastructure.

## Features

- **MCP Protocol Support**: JSON-RPC over HTTP for agent communication
- **CLI Tool**: User-friendly command-line interface with YAML configuration
- **Semantic Versioning**: Automatic patch incrementing with branch-aware tagging
- **Three-Phase Build Pipeline**: Build → Test → Publish workflow orchestration
- **Rootless Buildah Integration**: Secure, daemonless container building
- **Private Registry Workflow**: Intermediate image handling via private registries
- **Consul/Vault Integration**: Configuration and secret management
- **Prometheus Metrics**: Comprehensive monitoring and observability
- **Build History**: Optional job tracking for debugging

## Architecture

```

┌─────────────┐
│   Agent     │
│             │
└─────────────┘
      │
      ▼

┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   CLI       │───▶│   Build     │───▶│   Nomad     │
│             │    │   Server    │    │  Cluster    │
└─────────────┘    └─────────────┘    └─────────────┘
                           │                   │
                           ▼                   ▼
                   ┌─────────────┐    ┌─────────────┐
                   │   Consul    │    │   Buildah   │
                   │     KV      │    │    Jobs     │
                   └─────────────┘    └─────────────┘
                           │
                           ▼
                   ┌─────────────┐
                   │    Vault    │
                   │  Secrets    │
                   └─────────────┘
```

## API Endpoints

The service provides two ways to interact with it:

### 1. CLI Tool (Recommended)

The `jobforge` CLI tool provides the easiest way to interact with the build service:

```bash
# Submit a build job with YAML configuration
jobforge submit-job -config build.yaml

# Submit with global config + per-build override
jobforge submit-job -global deploy/global.yaml -config build.yaml

# Add additional image tags
jobforge submit-job -config build.yaml --image-tags "v1.0.0,latest"

# Query job status and logs
jobforge get-status <job-id>
jobforge get-logs <job-id> [phase]

# Job management
jobforge kill-job <job-id>
jobforge cleanup <job-id>
jobforge get-history [limit] [offset]

# Version management
jobforge version-info           # Show current version and branch
jobforge version-major <ver>    # Set major version (resets minor/patch to 0)
jobforge version-minor <ver>    # Set minor version (resets patch to 0)
```

**Key Features:**
- **YAML Configuration**: Support for global config + per-build overrides
- **Automatic Versioning**: Auto-increments patch version on each build
- **Branch-Aware Tags**: Generates tags like `feature-auth-v0.1.5`
- **Simple Interface**: No need to manually construct JSON-RPC requests

### 2. MCP Protocol Endpoint (Agent/Tool Integration)

The **Model Context Protocol (MCP)** endpoint is used for agent communication:

- **Endpoint:** `/mcp`
- **Transport:** JSON-RPC 2.0 over HTTP
- **Use with:** MCP Inspector, MCP clients, coding agents
- **Connection:** `http://localhost:8080/mcp`

**Supported MCP tools:** `submitJob`, `getStatus`, `getLogs`, `killJob`, `cleanup`, `getHistory`

### 3. Direct JSON-RPC API (Testing/Debugging)

Direct HTTP/JSON endpoints for testing and non-MCP integrations:
- `POST /json/submitJob` - Submit build jobs
- `POST /json/getStatus` - Get job status
- `POST /json/getLogs` - Get job logs
- `GET /json/job/{id}/status` - RESTful status endpoint
- `GET /json/job/{id}/logs` - RESTful logs endpoint
- `POST /json/killJob` - Terminate jobs
- `POST /json/cleanup` - Cleanup resources
- `POST /json/getHistory` - Get job history
- `GET /json/streamLogs?job_id=<id>` - WebSocket log streaming

**Important:** The `/json/*` endpoints are **NOT** part of the MCP protocol - they are custom HTTP endpoints for direct integration and testing.

### 3. Health & Monitoring

- `GET /health` - Service health check (on SERVER_PORT, default 8080)
- `GET /ready` - Readiness probe (on SERVER_PORT, default 8080)
- `GET /metrics` - Prometheus metrics (on METRICS_PORT, default 9090)

### Connection Examples

**MCP Inspector (Recommended):**
```
URL: http://localhost:8080/mcp
Transport: Simple JSON-RPC
```

**Advanced MCP Clients (Streaming):**
```
URL: http://localhost:8080/stream
Transport: Streamable HTTP
```

**Legacy MCP Clients:**
```
URL: http://localhost:8080/sse
Transport: Server-Sent Events
```

**Direct HTTP/curl (Non-MCP):**
```bash
curl -X POST http://localhost:8080/json/submitJob \
  -H "Content-Type: application/json" \
  -d '{"job_config": {...}}'

# Health check (on SERVER_PORT)
curl http://localhost:8080/health
```

## Quick Start

### Prerequisites

- Go 1.22+
- Nomad 1.10+ with Docker driver
- Consul for configuration storage
- Vault for secret management
- Docker registry for image storage
- Buildah-compatible Nomad clients

### Installation

1. **Clone and Build**
   ```bash
   git clone <repository-url>
   cd nomad-mcp-builder
   go mod tidy
   go build -o jobforge-service ./cmd/server
   ```

2. **Configuration**
   
   First, check your cluster configuration:
   ```bash
   # Check Consul datacenter name
   consul members
   
   # Check Nomad region name  
   nomad status
   # or
   curl http://your-nomad:4646/v1/regions
   ```
   
   Set environment variables or use Consul KV:
   ```bash
   export NOMAD_ADDR=http://your-nomad:4646
   export CONSUL_HTTP_ADDR=your-consul:8500
   export CONSUL_DATACENTER=your-datacenter  # Default: dc1, check with 'consul members'
   export NOMAD_REGION=your-region           # Default: global, check with 'nomad status'
   export NOMAD_DATACENTERS=cluster          # Default: cluster, check with 'nomad node status'
   export VAULT_ADDR=http://your-vault:8200
   export SERVER_PORT=8080
   
   # Registry configuration
   export REGISTRY_URL=localhost:5000        # For local registry on port 5000
   # export REGISTRY_URL=registry-1.docker.io  # For Docker Hub
   # export REGISTRY_URL=10.0.1.12:5000       # For registry on specific host:port
   export REGISTRY_TEMP_PREFIX=temp
   # Registry credentials not needed for public registries
   ```

3. **Run the Service**
   ```bash
   ./jobforge-service
   ```

## Configuration

### CLI YAML Configuration

The CLI tool supports YAML job configurations with a two-file approach:

#### Global Configuration (`deploy/global.yaml`)

Shared settings across all builds:

```yaml
owner: myteam
repo_url: https://github.com/myorg/myservice.git
git_credentials_path: secret/nomad/jobs/git-credentials
dockerfile_path: Dockerfile
image_name: myservice
registry_url: registry.cluster:5000/myapp
registry_credentials_path: secret/nomad/jobs/registry-credentials
```

#### Per-Build Configuration (`build.yaml`)

Build-specific overrides:

```yaml
git_ref: feature/new-feature
image_tags:
  - test
  - dev
test_entry_point: true
```

#### Merging Behavior

- Per-build values **override** global values for any non-zero field
- Arrays (like `image_tags`) are completely replaced, not merged
- The CLI automatically increments patch version and adds branch-aware tag

#### Usage

```bash
# With both global and per-build configs
jobforge submit-job -global deploy/global.yaml -config build.yaml

# With only per-build config (must include all required fields)
jobforge submit-job -config build.yaml

# Add extra tags in addition to auto-generated version tag
jobforge submit-job -config build.yaml --image-tags "latest,stable"
```

#### Version Management

The CLI automatically manages semantic versioning:

```bash
# Current version stored in deploy/version.yaml
# File format:
# version:
#   major: 0
#   minor: 1
#   patch: 5

# View current version
jobforge version-info
# Output:
#   Version: 0.1.5
#   Tag: v0.1.5
#   Branch: feature-new-feature
#   Branch Tag: feature-new-feature-v0.1.5

# Manual version bumps
jobforge version-major 1  # Sets version to 1.0.0
jobforge version-minor 2  # Sets version to 0.2.0

# Auto-increment on submit
# Each 'submit-job' automatically increments patch version
# and adds branch-aware tag to image_tags
```

### Server Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_HOST` | `0.0.0.0` | Server bind address |
| `SERVER_PORT` | `8080` | Server port |
| `CORS_ORIGIN` | `*` | CORS Access-Control-Allow-Origin header (use `*` for all origins or specific domain like `http://localhost:6274`) |
| `NOMAD_ADDR` | `http://localhost:4646` | Nomad API address |
| `NOMAD_REGION` | `global` | Nomad region name |
| `NOMAD_DATACENTERS` | `cluster` | Nomad datacenters (comma-separated) |
| `CONSUL_HTTP_ADDR` | `localhost:8500` | Consul API address |
| `CONSUL_DATACENTER` | `dc1` | Consul datacenter name |
| `VAULT_ADDR` | `http://localhost:8200` | Vault API address |
| `BUILD_TIMEOUT` | `30m` | Maximum build duration |
| `TEST_TIMEOUT` | `15m` | Maximum test duration |
| `METRICS_PORT` | `9090` | Prometheus metrics port |
| `REGISTRY_URL` | _(empty)_ | Docker registry URL (e.g., `docker.io`, `localhost:5000`, `registry.example.com:5000`) |
| `REGISTRY_TEMP_PREFIX` | `temp` | Prefix for temporary images in registry |
| `REGISTRY_USERNAME` | _(empty)_ | Registry username (optional for public registries) |
| `REGISTRY_PASSWORD` | _(empty)_ | Registry password (optional for public registries) |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `MCP_LOG_LEVEL` | `0` | MCP protocol verbose logging: `0`=compact (default), `1`=verbose with full request/response JSON |

### Consul Configuration

The service stores configuration in Consul KV at `jobforge-service/config/`:

```bash
consul kv put jobforge-service/config/build_timeout "45m"
consul kv put jobforge-service/config/test_timeout "20m"
consul kv put jobforge-service/config/default_resource_limits/cpu "1000"
consul kv put jobforge-service/config/default_resource_limits/memory "2048"
consul kv put jobforge-service/config/default_resource_limits/disk "10240"
```

### Vault Secrets

Store credentials in Vault for Git and registry access.

**Note**: This service requires Vault KV v2 (default in newer Vault versions). The code uses `.Data.data.` paths in templates which are specific to KV v2.

```bash
# Check if you're using KV v2 (should show "version: 2")
vault kv metadata secret/

# Git credentials (KV v2)
vault kv put secret/nomad/jobs/git-credentials \
  username="your-git-user" \
  password="your-git-token" \
  ssh_key="$(cat ~/.ssh/id_rsa)"

# Registry credentials (KV v2) - ONLY needed for private registries
# For Docker Hub public images, you can skip this step
vault kv put secret/nomad/jobs/registry-credentials \
  username="your-registry-user" \
  password="your-registry-password"

# Verify secrets were stored correctly
vault kv get secret/nomad/jobs/git-credentials
# Only if you created registry credentials:
# vault kv get secret/nomad/jobs/registry-credentials
```

## Nomad Client Setup

### Required Configuration

Nomad clients must be configured to support rootless Buildah:

1. **User Namespace Setup**
   ```bash
   # Add to /etc/subuid and /etc/subgid
   echo "build:10000:65536" >> /etc/subuid
   echo "build:10000:65536" >> /etc/subgid
   ```

2. **Create Build User**
   ```bash
   sudo useradd -r -s /bin/false build
   sudo usermod -aG docker build  # If using Docker driver
   ```

3. **Fuse Device Access**
   ```bash
   # Ensure /dev/fuse is accessible
   sudo chmod 666 /dev/fuse
   ```

4. **Build Cache Directory**
   ```bash
   sudo mkdir -p /opt/nomad/data/buildah-cache
   sudo chown -R build:build /opt/nomad/data/buildah-cache
   ```

### Nomad Client Configuration

Add to your Nomad client configuration:

```hcl
client {
  enabled = true
  
  host_volume "buildah-cache" {
    path      = "/opt/nomad/data/buildah-cache"
    read_only = false
  }
}

plugin "docker" {
  config {
    allow_privileged = false
    allow_caps = ["SYS_ADMIN"]  # Required for some builds
  }
}
```


## Command Line Tool

The project includes a `jobforge` CLI tool that provides the same functionality as the web API in a convenient command-line interface.

### Building the CLI Tool

```bash
# Build the CLI tool
go build -o jobforge ./cmd/jobforge
```

### CLI Usage

```
jobforge - CLI client for nomad build service

Usage:
  jobforge [flags] <command> [args...]

Commands:
  submit-job <json>     Submit a new build job (JSON from arg or stdin)
  get-status <job-id>   Get status of a job
  get-logs <job-id> [phase]  Get logs for a job (optional phase: build, test, publish)
  kill-job <job-id>     Kill a running job
  cleanup <job-id>      Clean up resources for a job
  get-history [limit] [offset]  Get job history (optional limit, optional offset)

Flags:
  -h, --help           Show this help message
  -u, --url <url>      Service URL (default: http://localhost:8080)
                       Can also be set via JOB_SERVICE_URL environment variable
```

### CLI Examples

#### Submit a Build Job

**From command line argument:**
```bash
jobforge submit-job '{
  "owner": "myorg",
  "repo_url": "https://github.com/myorg/myapp.git",
  "git_ref": "main",
  "dockerfile_path": "Dockerfile",
  "image_name": "myapp",
  "image_tags": ["v1.0", "latest"],
  "registry_url": "registry.cluster:5000/myapp",
  "test_entry_point": true
}'
```

**From stdin (useful for scripts):**
```bash
echo '{
  "owner": "myorg",
  "repo_url": "https://github.com/myorg/myapp.git",
  "git_ref": "main",
  "dockerfile_path": "Dockerfile",
  "image_name": "myapp",
  "image_tags": ["v1.0"],
  "registry_url": "registry.cluster:5000/myapp"
}' | jobforge submit-job
```

**From file:**
```bash
cat job-config.json | jobforge submit-job
```

#### Check Job Status

```bash
jobforge get-status abc123-def456-789
```

#### Get Job Logs

```bash
# Get all logs
jobforge get-logs abc123-def456-789

# Get logs for specific phase
jobforge get-logs abc123-def456-789 build
jobforge get-logs abc123-def456-789 test
jobforge get-logs abc123-def456-789 publish
```

#### Kill a Running Job

```bash
jobforge kill-job abc123-def456-789
```

#### Clean Up Job Resources

```bash
jobforge cleanup abc123-def456-789
```

#### Get Job History

```bash
# Get last 10 jobs
jobforge get-history

# Get specific number of jobs
jobforge get-history 20

# Get jobs with offset (pagination)
jobforge get-history 10 20
```

### Service Discovery Integration

The CLI tool automatically works with service discovery:

```bash
# Using Consul service discovery
consul catalog service jobforge-service
# Service Address: 10.0.1.13:21855

# Set environment variable for convenience
export JOB_SERVICE_URL=http://10.0.1.13:21855

# Now all CLI commands will use the discovered service
jobforge get-history
```

### CLI Integration Examples

#### CI/CD Pipeline Usage

```bash
#!/bin/bash
# Build and deploy script

# Set service URL from environment
export JOB_SERVICE_URL=${BUILD_SERVICE_URL}

# Submit job from JSON file
JOB_ID=$(cat build-config.json | jobforge submit-job | jq -r '.job_id')
echo "Build job submitted: $JOB_ID"

# Wait for completion
while true; do
  STATUS=$(jobforge get-status $JOB_ID | jq -r '.status')
  echo "Current status: $STATUS"

  if [[ "$STATUS" == "SUCCEEDED" ]]; then
    echo "Build completed successfully!"
    break
  elif [[ "$STATUS" == "FAILED" ]]; then
    echo "Build failed! Getting logs..."
    jobforge get-logs $JOB_ID
    exit 1
  fi

  sleep 30
done

# Clean up
jobforge cleanup $JOB_ID
```

#### Monitoring Script

```bash
#!/bin/bash
# Monitor build service

export JOB_SERVICE_URL=http://10.0.1.13:21855

echo "Recent build history:"
jobforge get-history 5

echo -e "\nRunning jobs:"
jobforge get-history 20 | jq '.jobs[] | select(.status == "BUILDING" or .status == "TESTING" or .status == "PUBLISHING") | {id: .id, status: .status, owner: .config.owner}'
```

### Go Library Usage

The CLI tool is built on a reusable Go client library at `pkg/client`. You can use this library in your own Go applications:

```go
package main

import (
    "fmt"
    "nomad-mcp-builder/pkg/client"
    "nomad-mcp-builder/pkg/types"
)

func main() {
    // Create client
    c := client.NewClient("http://10.0.1.13:21855")

    // Submit job
    jobConfig := &types.JobConfig{
        Owner:     "myorg",
        RepoURL:   "https://github.com/myorg/myapp.git",
        GitRef:    "main",
        ImageName: "myapp",
        ImageTags: []string{"v1.0"},
        RegistryURL: "registry.cluster:5000/myapp",
    }

    response, err := c.SubmitJob(jobConfig)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Job submitted: %s\n", response.JobID)

    // Check status
    status, err := c.GetStatus(response.JobID)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Status: %s\n", status.Status)
}
```

## Resource Configuration

Jobs can specify custom resource limits using the optional `resource_limits` parameter. If not specified, the following defaults are used:

### Default Resource Limits by Phase

- **Build Phase** (most resource-intensive):
  - CPU: 1000 MHz
  - Memory: 2048 MB (2 GB)
  - Disk: 10240 MB (10 GB)

- **Test Phase** (moderate resources):
  - CPU: 500 MHz
  - Memory: 1024 MB (1 GB)
  - Disk: 2048 MB (2 GB)

- **Publish Phase** (minimal resources):
  - CPU: 500 MHz
  - Memory: 1024 MB (1 GB)
  - Disk: 2048 MB (2 GB)

### Custom Resource Configuration

You can override the default resource limits in two ways:

#### 1. Global Resource Limits (Legacy)

Apply the same resource limits to all phases:

```json
{
  "job_config": {
    "owner": "myorg",
    "repo_url": "https://github.com/myorg/myapp.git",
    "image_name": "myapp",
    "image_tags": ["latest"],
    "registry_url": "registry.cluster:5000/myapp",
    "resource_limits": {
      "cpu": "2000",     // 2000 MHz (2 CPU cores)
      "memory": "4096",  // 4096 MB (4 GB RAM)
      "disk": "20480"    // 20480 MB (20 GB disk)
    }
  }
}
```

#### 2. Per-Phase Resource Limits (Recommended)

Specify different resource limits for each phase of the build process:

```json
{
  "job_config": {
    "owner": "myorg",
    "repo_url": "https://github.com/myorg/myapp.git",
    "image_name": "myapp",
    "image_tags": ["latest"],
    "registry_url": "registry.cluster:5000/myapp",
    "resource_limits": {
      "build": {
        "cpu": "4000",     // 4000 MHz (4 CPU cores) - build needs more resources
        "memory": "8192",  // 8192 MB (8 GB RAM)
        "disk": "40960"    // 40960 MB (40 GB disk)
      },
      "test": {
        "cpu": "1500",     // 1500 MHz (1.5 CPU cores)
        "memory": "3072",  // 3072 MB (3 GB RAM)
        "disk": "10240"    // 10240 MB (10 GB disk)
      },
      "publish": {
        "cpu": "800",      // 800 MHz
        "memory": "1536",  // 1536 MB (1.5 GB RAM)
        "disk": "5120"     // 5120 MB (5 GB disk)
      }
    }
  }
}
```

#### 3. Mixed Configuration

You can combine global limits with per-phase overrides:

```json
{
  "job_config": {
    "resource_limits": {
      "cpu": "2000",     // Global fallback for all phases
      "memory": "4096",  // Global fallback for all phases
      "build": {
        "cpu": "6000"    // Override only CPU for build phase
        // Memory and disk will use global values
      },
      "test": {
        "memory": "2048" // Override only memory for test phase
        // CPU and disk will use global values
      }
    }
  }
}
```

**Resource Resolution Priority:**
1. **Per-phase specific values** (highest priority)
2. **Global/legacy values** (fallback)
3. **System defaults** (final fallback)

This allows fine-grained control where resource-intensive build phases can have higher limits while test and publish phases use more conservative allocations.

### Recommended Resource Configurations by Application Type

#### 🚀 Simple Applications (Node.js, Python, Go)
**Global limits** (apply to all phases):
```json
"resource_limits": {
  "cpu": "1000",     // 1 GHz CPU
  "memory": "2048",  // 2 GB RAM
  "disk": "10240"    // 10 GB disk
}
```

#### 🔧 Complex Applications (Java, .NET, C++)
**Global limits** for moderate complexity:
```json
"resource_limits": {
  "cpu": "2000",     // 2 GHz CPU
  "memory": "4096",  // 4 GB RAM
  "disk": "20480"    // 20 GB disk
}
```

#### 🤖 Large Applications (ML/Data, Multi-stage builds)
**Per-phase limits** for maximum control:
```json
"resource_limits": {
  "build": {
    "cpu": "4000",     // 4 GHz CPU for compilation
    "memory": "8192",  // 8 GB RAM for dependencies
    "disk": "40960"    // 40 GB for large packages
  },
  "test": {
    "cpu": "2000",     // 2 GHz CPU for tests
    "memory": "4096",  // 4 GB RAM for test data
    "disk": "20480"    // 20 GB for test artifacts
  },
  "publish": {
    "cpu": "1000",     // 1 GHz CPU for registry push
    "memory": "2048",  // 2 GB RAM for image layers
    "disk": "10240"    // 10 GB for temporary files
  }
}
```

#### ⚡ Resource Guidelines by Image Size
- **Small images** (< 500 MB): Use simple application settings
- **Medium images** (500 MB - 2 GB): Use complex application settings
- **Large images** (> 2 GB): Use large application settings with per-phase limits

#### 💡 Performance Tips
- **CPU**: Build phase typically needs 2-4x more CPU than test/publish phases
- **Memory**: Increase memory for applications with many dependencies (Node.js, Maven)
- **Disk**: Allow extra disk space for layer caching (improves subsequent build performance)

## Usage Examples

### Submit a Build Job

**Note**: The `image_name` field is now **required** and specifies the name of the Docker image (e.g., "myapp", "web-server"). The final image will be tagged as `registry_url/image_name:tag`.

```bash
curl -X POST http://localhost:8080/json/submitJob \
  -H "Content-Type: application/json" \
  -d '{
    "job_config": {
      "owner": "developer",
      "repo_url": "https://github.com/user/app.git",
      "git_ref": "main",
      "git_credentials_path": "secret/nomad/jobs/git-credentials",
      "dockerfile_path": "Dockerfile",
      "image_name": "myapp",
      "image_tags": ["latest", "v1.0.0"],
      "registry_url": "docker.io/user",
      "registry_credentials_path": "secret/nomad/jobs/registry-credentials",
      "test_commands": [
        "/app/run-tests.sh",
        "/app/integration-tests.sh"
      ]
    }
  }'
```

### Check Job Status

```bash
# RESTful endpoint (recommended)
curl http://localhost:8080/json/job/550e8400-e29b-41d4-a716-446655440000/status

# Legacy POST endpoint
curl -X POST http://localhost:8080/json/getStatus \
  -H "Content-Type: application/json" \
  -d '{"job_id": "550e8400-e29b-41d4-a716-446655440000"}'
```

### Stream Logs via WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/json/streamLogs?job_id=550e8400-e29b-41d4-a716-446655440000');
ws.onmessage = function(event) {
  const log = JSON.parse(event.data);
  console.log(`[${log.phase}] ${log.message}`);
};
```

## Testing with MCP Inspector

You can test the MCP endpoints using the [MCP Inspector](https://github.com/modelcontextprotocol/inspector):

1. **Start your build service:**
   ```bash
   ./jobforge-service
   ```

2. **Open MCP Inspector** in your browser

3. **Connect to the service:**
   - **URL:** `http://localhost:8080/mcp`
   - **Transport:** Simple JSON-RPC (recommended for Inspector)

   **Alternative:** For streaming support, use `http://localhost:8080/stream`

4. **Available MCP Tools:**
   - `submitJob` - Submit a new build job
   - `getStatus` - Check job status  
   - `getLogs` - Retrieve job logs
   - `killJob` - Terminate a running job
   - `cleanup` - Clean up job resources
   - `getHistory` - Get build history
   - `purgeFailedJob` - Remove zombie/dead jobs from Nomad

5. **Example MCP Tool Call:**
   ```json
   {
     "name": "submitJob",
     "arguments": {
       "repo_url": "https://github.com/example/repo.git",
       "image_name": "myapp",
       "registry_url": "registry.example.com",
       "image_tags": ["latest", "v1.0.0"],
       "test_commands": ["npm test", "npm run lint"]
     }
   }
   ```

The MCP Inspector will show you the available tools, their schemas, and allow you to test tool calls interactively.

## Monitoring

### Prometheus Metrics

The service exposes comprehensive metrics:

- `build_duration_seconds` - Build phase timing
- `test_duration_seconds` - Test phase timing  
- `publish_duration_seconds` - Publish phase timing
- `job_success_rate` - Success rate by time window
- `concurrent_jobs_total` - Current running jobs
- `resource_usage` - CPU/memory consumption

### Grafana Dashboard

Example Grafana queries:

```promql
# Average build time by status
avg(build_duration_seconds) by (status)

# Job success rate over 24h
job_success_rate{window="24h"}

# Current resource utilization
resource_usage{resource_type="cpu"}
```

## Deployment

### Building Docker Image

First, build and push a Docker image:

```bash
# Build the binary
go build -o jobforge-service ./cmd/server

# Create Dockerfile (example)
cat > Dockerfile << 'EOF'
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY jobforge-service .
EXPOSE 8080 9090
CMD ["./jobforge-service"]
EOF

# Build and push image
docker build -t your-registry:5000/jobforge-service:latest .
docker push your-registry:5000/jobforge-service:latest
```

### Deploying with Nomad

1. **Update the job file variables**:
   ```bash
   # Edit jobforge-service.nomad and update:
   # - datacenters = ["your-datacenter"] 
   # - region = "your-region"
   # - REGISTRY_URL in env section
   ```

2. **Set deployment variables**:
   ```bash
   export REGISTRY_URL=your-registry:5000
   ```

3. **Deploy to Nomad**:
   ```bash
   # Plan the deployment
   nomad job plan jobforge-service.nomad
   
   # Deploy the service
   nomad job run jobforge-service.nomad
   
   # Check status
   nomad job status jobforge-service
   ```

4. **Verify service registration**:
   ```bash
   # Check Consul services
   consul catalog services
   
   # Check specific service
   consul catalog service jobforge-service
   consul catalog service jobforge-service-metrics
   ```

5. **Configure Prometheus** to discover the service:
   ```yaml
   scrape_configs:
     - job_name: 'jobforge-service'
       consul_sd_configs:
         - server: 'your-consul:8500'
           services: ['jobforge-service-metrics']
       relabel_configs:
         - source_labels: [__meta_consul_service_metadata_metrics_path]
           target_label: __metrics_path__
           regex: (.+)
   ```

### Service Endpoints

Once deployed, the service will be available at:
- **API**: `http://service-ip:8080` (MCP endpoints, configurable via SERVER_PORT)
- **Health**: `http://service-ip:8080/health` (on SERVER_PORT)
- **Metrics**: `http://service-ip:9090/metrics` (on METRICS_PORT, configurable via METRICS_PORT)

### Scaling

To scale the service:
```bash
# Update count in jobforge-service.nomad
count = 3

# Redeploy
nomad job run jobforge-service.nomad
```

## Development

### Running Tests

```bash
go test ./...
```

### Development Environment

```bash
# Start dependencies with Docker Compose
docker-compose up -d consul vault nomad

# Run in development mode
go run ./cmd/server
```

### Building Docker Image

```bash
docker build -t jobforge-service:latest .
```

## Troubleshooting

### Common Issues

1. **Permission Denied Errors**
   - Ensure user namespaces are configured
   - Check /dev/fuse permissions
   - Verify build cache directory ownership

2. **Build Timeouts**
   - Increase timeout values in configuration
   - Check Nomad resource allocation
   - Monitor build cache utilization

3. **Registry Authentication**
   - Verify Vault secret paths
   - Check registry credential format
   - Test manual registry access

### Debug Mode

Enable debug logging:
```bash
export LOG_LEVEL=debug
./jobforge-service
```

#### MCP Protocol Verbose Logging

For debugging MCP protocol communication issues, enable verbose logging to capture full request/response JSON:

```bash
export MCP_LOG_LEVEL=1
./jobforge-service
```

**Log Levels:**
- `MCP_LOG_LEVEL=0` (default): Compact structured logs with method names and IDs
- `MCP_LOG_LEVEL=1`: Verbose logging including:
  - Full raw request JSON
  - Tool name extraction for `tools/call` requests
  - Full raw response JSON
  - Useful for debugging client integration issues

**Note:** This setting is independent of `LOG_LEVEL` and only affects MCP protocol logging. Use it when troubleshooting MCP client connections or tool invocations.

### Log Analysis

Key log patterns to watch:
- `"Job submitted successfully"` - Successful job creation
- `"Build job submitted to Nomad"` - Phase transitions
- `"Health check failed"` - Infrastructure issues

## Security Considerations

- All secrets managed via Vault
- Rootless container execution
- No raw credential handling
- Network isolation during tests
- Automatic resource cleanup

## Testing

### Unit Tests

Run the basic unit tests:

```bash
go test ./pkg/... ./internal/... -v
```

### Integration Tests

The project includes comprehensive integration tests that:

- Discover the service via Consul service discovery
- Submit real build jobs using the hello-world test repository
- Monitor job progress until completion
- Retrieve and save build/test logs
- Generate detailed test reports with pass/fail status

#### Running Integration Tests

**Prerequisites:**
- Nomad cluster running with the jobforge-service deployed
- Consul accessible for service discovery
- Registry accessible at `registry.cluster:5000`

**Run the full integration test:**

```bash
# Set Consul address if not default
export CONSUL_HTTP_ADDR="10.0.1.12:8500"

# Run the comprehensive integration test (15 minute timeout)
go test -v ./test/integration -run TestConsulDiscoveryAndBuildWorkflow -timeout 15m
```

**Test Output:**

The test automatically creates a `test_results/` directory with:

- `build_logs_<job-id>.txt` - Complete build phase logs
- `test_logs_<job-id>.txt` - Complete test phase logs  
- `test_result_<job-id>.json` - JSON summary with test results

**Example test result:**

```json
{
  "job_id": "abc123-def456",
  "build_success": true,
  "test_success": true,
  "build_logs": ["STEP 1/4: FROM alpine:latest", "..."],
  "test_logs": ["Running entry point test", "..."],
  "timestamp": "2025-09-06T18:45:00Z",
  "duration": "2m15s"
}
```

#### Manual Testing with curl

You can also test manually after discovering the service:

```bash
# Discover service URL
consul catalog service jobforge-service

# Or use Consul API to get the service endpoint
SERVICE_URL=$(curl -s http://${CONSUL_HTTP_ADDR:-localhost:8500}/v1/catalog/service/jobforge-service | jq -r '.[0] | "\(.ServiceAddress):\(.ServicePort)"')

# Submit test job (replace with discovered URL or use SERVICE_URL)
curl -X POST http://${SERVICE_URL:-localhost:8080}/json/submitJob \
  -H "Content-Type: application/json" \
  -d '{
    "job_config": {
      "owner": "test",
      "repo_url": "https://github.com/geraldthewes/docker-build-hello-world.git",
      "git_ref": "main",
      "dockerfile_path": "Dockerfile",
      "image_name": "helloworld",
      "image_tags": ["hello-world-test"],
      "registry_url": "registry.cluster:5000",
      "test_entry_point": true
    }
  }'

# Check status (use returned job_id) - RESTful endpoint
curl http://${SERVICE_URL:-localhost:8080}/json/job/<job-id>/status

# Get logs when complete - RESTful endpoint
curl http://${SERVICE_URL:-localhost:8080}/json/job/<job-id>/logs
```

### Test Configuration

The integration test is configurable via environment variables:

- `CONSUL_HTTP_ADDR` - Consul address (default: `10.0.1.12:8500`)
- Test timeout is set to 15 minutes to allow for complete build cycles

### Webhook Integration Tests

The project includes webhook notification tests that verify the complete webhook delivery system. These tests are **opt-in** due to network requirements.

#### Network Requirements

Webhook tests require bidirectional network connectivity:
- The test creates a local webhook receiver
- The Nomad cluster must be able to reach the test machine's IP
- Works automatically with VPN setups (e.g., WireGuard, OpenVPN)

#### Running Webhook Tests

```bash
# Enable webhook tests (auto-detects correct network interface)
ENABLE_WEBHOOK_TESTS=true go test ./test/integration -v -run TestWebhookNotifications

# Override IP selection if auto-detection fails
ENABLE_WEBHOOK_TESTS=true WEBHOOK_TEST_IP=10.0.6.17 go test ./test/integration -v -run TestWebhookNotifications
```

#### How IP Selection Works

The test intelligently selects the correct network interface using a 3-strategy approach:

1. **Environment Override** (highest priority):
   ```bash
   export WEBHOOK_TEST_IP=10.0.6.17
   ```

2. **Auto-Detection** (recommended):
   - Dials the discovered service URL
   - Uses the local interface that can reach the service
   - Example: Service at `10.0.1.16` → Selects VPN interface `10.0.6.17`

3. **Network Scan Fallback**:
   - Searches for interfaces in `10.0.x.x` range
   - Useful for cluster network setups

4. **Default Route Fallback**:
   - Uses default route if all else fails

#### Example Network Setup

**Typical VPN Configuration:**
```
Developer Machine:
├─ WiFi: 192.168.0.149 (home network)
├─ VPN:  10.0.6.17 (WireGuard - cluster access)
└─ Docker: 172.17.0.1 (local)

Nomad Cluster: 10.0.1.x network

Webhook Test Flow:
Service (10.0.1.16) → VPN Bridge → Test Receiver (10.0.6.17:8889)
```

#### What the Test Validates

The webhook test verifies:
- ✅ Webhook receiver starts and binds correctly
- ✅ Build job submission with webhook configuration
- ✅ Webhook delivery for all job phases (build/test/publish)
- ✅ HMAC-SHA256 signature authentication
- ✅ Custom header propagation
- ✅ Job completion notification
- ✅ Proper error handling and retries

#### Troubleshooting Webhook Tests

**Test skips immediately:**
```bash
# Make sure to enable the test
ENABLE_WEBHOOK_TESTS=true go test ./test/integration -run TestWebhookNotifications
```

**Webhooks not received (check server logs):**
```bash
# View webhook delivery attempts
nomad alloc logs -stderr <alloc-id> | grep -i webhook

# Look for "context deadline exceeded" errors
# This indicates network connectivity issues
```

**Wrong IP selected:**
```bash
# Check available interfaces
ip addr

# Manually specify the correct IP
WEBHOOK_TEST_IP=10.0.6.17 ENABLE_WEBHOOK_TESTS=true go test ./test/integration -run TestWebhookNotifications
```

**Network Connectivity Test:**
```bash
# From your machine, verify you can reach the cluster
ping 10.0.1.16

# From the cluster, verify it can reach your machine (if possible)
# The webhook test will show selected IP in its output
```

#### Why Webhook Tests Are Opt-In

Webhook tests are disabled by default because:
- They require network accessibility from the Nomad cluster
- They depend on VPN or network bridge configuration
- They take longer to run (15-20 seconds)
- Most development workflows don't require webhook testing

For CI/CD environments with proper network setup, enable them in your pipeline configuration.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Run integration tests to verify changes
5. Submit a pull request

## License

[Specify License]

## Support

For issues and questions:
- Check troubleshooting guide above
- Review logs with debug enabled
- Submit GitHub issues with full context
