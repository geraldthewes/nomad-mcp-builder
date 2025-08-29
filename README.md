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
   export VAULT_ADDR=http://your-vault:8200
   export SERVER_PORT=8080
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
| `CONSUL_HTTP_ADDR` | `localhost:8500` | Consul API address |
| `CONSUL_DATACENTER` | `dc1` | Consul datacenter name |
| `VAULT_ADDR` | `http://localhost:8200` | Vault API address |
| `BUILD_TIMEOUT` | `30m` | Maximum build duration |
| `TEST_TIMEOUT` | `15m` | Maximum test duration |
| `METRICS_PORT` | `9090` | Prometheus metrics port |

### Consul Configuration

The service stores configuration in Consul KV at `nomad-build-service/config/`:

```bash
consul kv put nomad-build-service/config/build_timeout "45m"
consul kv put nomad-build-service/config/test_timeout "20m"
consul kv put nomad-build-service/config/default_resource_limits/cpu "2000"
consul kv put nomad-build-service/config/default_resource_limits/memory "4096"
```

### Vault Secrets

Store credentials in Vault for Git and registry access:

```bash
# Git credentials
vault kv put secret/nomad/jobs/git-credentials \
  username="your-git-user" \
  password="your-git-token" \
  ssh_key="$(cat ~/.ssh/id_rsa)"

# Registry credentials  
vault kv put secret/nomad/jobs/registry-credentials \
  username="your-registry-user" \
  password="your-registry-password"
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
- `POST /mcp/getStatus` - Get job status
- `POST /mcp/getLogs` - Get job logs
- `GET /mcp/streamLogs?job_id=<id>` - WebSocket log streaming
- `POST /mcp/killJob` - Terminate a job
- `POST /mcp/cleanup` - Cleanup resources
- `POST /mcp/getHistory` - Get job history

### Health Endpoints

- `GET /health` - Service health check
- `GET /ready` - Readiness probe

### Metrics

- `GET /metrics` - Prometheus metrics (default port 9090)

## Usage Examples

### Submit a Build Job

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
      "image_tags": ["latest", "v1.0.0"],
      "registry_url": "docker.io/user/app",
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

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Submit a pull request

## License

[Specify License]

## Support

For issues and questions:
- Check troubleshooting guide above
- Review logs with debug enabled
- Submit GitHub issues with full context