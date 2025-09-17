# Nomad Build Service

A lightweight, stateless, MCP-based server written in Go that enables coding agents to submit Docker image build jobs remotely using Nomad as the backend infrastructure.

## Features

- **MCP Protocol Support**: Full compliance with Model Context Protocol for agent communication
- **Three-Phase Build Pipeline**: Build → Test → Publish workflow orchestration
- **WebSocket Log Streaming**: Real-time log access during builds
- **Rootless Buildah Integration**: Secure, daemonless container building
- **Private Registry Workflow**: Intermediate image handling via private registries
- **Consul/Vault Integration**: Configuration and secret management
- **Prometheus Metrics**: Comprehensive monitoring and observability
- **Build History**: Optional job tracking for debugging

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Agent     │───▶│ MCP Server  │───▶│   Nomad     │
│             │    │             │    │  Cluster    │
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

## MCP Transport

This server now supports **both** standard MCP transports and custom HTTP/JSON endpoints:

### Standard MCP Protocol Support ✅

- **JSON-RPC over HTTP** at `/mcp` endpoint
- **Streamable HTTP transport** at `/mcp/stream` endpoint  
- **Full MCP 2024-11-05 compliance** with tools/list, tools/call, initialize
- **Compatible with MCP Inspector** and standard MCP clients
- **Future-proof transport** using modern bidirectional HTTP streaming

### Custom HTTP/JSON API (Legacy)

- **HTTP POST requests** with JSON payloads at `/mcp/*` paths
- **Standard HTTP responses** with JSON results  
- **WebSocket connections** for real-time log streaming at `/mcp/streamLogs`
- **RESTful endpoints** for direct curl/Postman testing

### Connection Methods

**For MCP Inspector (Recommended):**
- URL: `http://localhost:8080/mcp/stream`
- Transport: Streamable HTTP (modern, bidirectional)

**For Standard MCP Clients:**
- URL: `http://localhost:8080/mcp`  
- Transport: JSON-RPC over HTTP (simple request/response)

**For Direct HTTP Testing:**
- Use existing `/mcp/submitJob`, `/mcp/getStatus` etc. endpoints

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
   go build -o nomad-build-service ./cmd/server
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
   ./nomad-build-service
   ```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_HOST` | `0.0.0.0` | Server bind address |
| `SERVER_PORT` | `8080` | Server port |
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

### Consul Configuration

The service stores configuration in Consul KV at `nomad-build-service/config/`:

```bash
consul kv put nomad-build-service/config/build_timeout "45m"
consul kv put nomad-build-service/config/test_timeout "20m"
consul kv put nomad-build-service/config/default_resource_limits/cpu "1000"
consul kv put nomad-build-service/config/default_resource_limits/memory "2048"
consul kv put nomad-build-service/config/default_resource_limits/disk "10240"
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

## API Endpoints

### MCP Endpoints

- `POST /mcp/submitJob` - Submit a new build job
- `POST /mcp/getStatus` - Get job status (legacy)
- `POST /mcp/getLogs` - Get job logs (legacy)
- `GET /mcp/job/{jobID}/status` - Get job status (RESTful)
- `GET /mcp/job/{jobID}/logs` - Get job logs (RESTful)
- `GET /mcp/streamLogs?job_id=<id>` - WebSocket log streaming
- `POST /mcp/killJob` - Terminate a job
- `POST /mcp/cleanup` - Cleanup resources
- `POST /mcp/getHistory` - Get job history

### Health Endpoints

- `GET /health` - Service health check
- `GET /ready` - Readiness probe

### Metrics

- `GET /metrics` - Prometheus metrics (default port 9090)

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

You can override these defaults by including a `resource_limits` object in your job submission:

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

**Note**: Resource limits apply to all phases (build, test, publish). Consider your workload requirements when setting custom limits.

## Usage Examples

### Submit a Build Job

**Note**: The `image_name` field is now **required** and specifies the name of the Docker image (e.g., "myapp", "web-server"). The final image will be tagged as `registry_url/image_name:tag`.

```bash
curl -X POST http://localhost:8080/mcp/submitJob \
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
curl http://localhost:8080/mcp/job/550e8400-e29b-41d4-a716-446655440000/status

# Legacy POST endpoint
curl -X POST http://localhost:8080/mcp/getStatus \
  -H "Content-Type: application/json" \
  -d '{"job_id": "550e8400-e29b-41d4-a716-446655440000"}'
```

### Stream Logs via WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/mcp/streamLogs?job_id=550e8400-e29b-41d4-a716-446655440000');
ws.onmessage = function(event) {
  const log = JSON.parse(event.data);
  console.log(`[${log.phase}] ${log.message}`);
};
```

## Testing with MCP Inspector

You can test the MCP endpoints using the [MCP Inspector](https://github.com/modelcontextprotocol/inspector):

1. **Start your build service:**
   ```bash
   ./nomad-build-service
   ```

2. **Open MCP Inspector** in your browser

3. **Connect to the service:**
   - **URL:** `http://localhost:8080/mcp/stream`
   - **Transport:** Streamable HTTP

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
go build -o nomad-build-service ./cmd/server

# Create Dockerfile (example)
cat > Dockerfile << 'EOF'
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY nomad-build-service .
EXPOSE 8080 9090 8081
CMD ["./nomad-build-service"]
EOF

# Build and push image
docker build -t your-registry:5000/nomad-build-service:latest .
docker push your-registry:5000/nomad-build-service:latest
```

### Deploying with Nomad

1. **Update the job file variables**:
   ```bash
   # Edit nomad-build-service.nomad and update:
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
   nomad job plan nomad-build-service.nomad
   
   # Deploy the service
   nomad job run nomad-build-service.nomad
   
   # Check status
   nomad job status nomad-build-service
   ```

4. **Verify service registration**:
   ```bash
   # Check Consul services
   consul catalog services
   
   # Check specific service
   consul catalog service nomad-build-service
   consul catalog service nomad-build-service-metrics
   ```

5. **Configure Prometheus** to discover the service:
   ```yaml
   scrape_configs:
     - job_name: 'nomad-build-service'
       consul_sd_configs:
         - server: 'your-consul:8500'
           services: ['nomad-build-service-metrics']
       relabel_configs:
         - source_labels: [__meta_consul_service_metadata_metrics_path]
           target_label: __metrics_path__
           regex: (.+)
   ```

### Service Endpoints

Once deployed, the service will be available at:
- **API**: `http://service-ip:8080` (MCP endpoints)  
- **Health**: `http://service-ip:8081/health`
- **Metrics**: `http://service-ip:9090/metrics` (Prometheus)

### Scaling

To scale the service:
```bash
# Update count in nomad-build-service.nomad
count = 3

# Redeploy
nomad job run nomad-build-service.nomad
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
docker build -t nomad-build-service:latest .
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
./nomad-build-service
```

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
- Nomad cluster running with the nomad-build-service deployed
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
consul catalog service nomad-build-service

# Or use Consul API to get the service endpoint
SERVICE_URL=$(curl -s http://${CONSUL_HTTP_ADDR:-localhost:8500}/v1/catalog/service/nomad-build-service | jq -r '.[0] | "\(.ServiceAddress):\(.ServicePort)"')

# Submit test job (replace with discovered URL or use SERVICE_URL)
curl -X POST http://${SERVICE_URL:-localhost:8080}/mcp/submitJob \
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
curl http://${SERVICE_URL:-localhost:8080}/mcp/job/<job-id>/status

# Get logs when complete - RESTful endpoint
curl http://${SERVICE_URL:-localhost:8080}/mcp/job/<job-id>/logs
```

### Test Configuration

The integration test is configurable via environment variables:

- `CONSUL_HTTP_ADDR` - Consul address (default: `10.0.1.12:8500`)
- Test timeout is set to 15 minutes to allow for complete build cycles

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