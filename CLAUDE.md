# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the Nomad Build Service, a lightweight, stateless MCP-based server written in Golang. The service enables coding agents to submit Docker image build jobs remotely using Nomad as the backend infrastructure. It orchestrates builds, tests, and publishes using Buildah for daemonless image building.

**For complete project requirements and architecture details, see [PRD.md](PRD.md).**

**Current Status:** Fully implemented and operational. The service provides a complete build-test-publish pipeline with improved Docker-native test execution.

## Architecture (Current Implementation)

The system consists of:

- **MCP Server**: Stateless Go application that translates MCP requests into Nomad API calls
- **Nomad Jobs**: Three phases of ephemeral batch jobs:
  1. **Build phase**: Uses Buildah to build Docker images from Git repos and push to temporary registry location
  2. **Test phase**: Creates separate Nomad jobs using Docker driver directly to run the built image (no buildah complexity)
  3. **Publish phase**: Uses Buildah to retag and push final images to target registries
- **Build Caching**: Persistent host volume for Buildah layer caching at `/opt/nomad/data/buildah-cache`

**Key Innovation**: Test phase now uses Nomad's native Docker driver instead of buildah commands, providing better isolation, cleaner logs, and parallel test execution capability.

## Key Technical Requirements

### Technology Stack (from PRD)
- **Go 1.22+** 
- **Nomad 1.10+** with Vault integration
- **Buildah** (latest stable via `quay.io/buildah/stable`)
- **MCP Protocol** for agent communication

### Key Libraries to Use
- `github.com/hashicorp/nomad/api` - Nomad API client
- `github.com/sirupsen/logrus` - Logging
- Standard `net/http` or `gorilla/websocket` - HTTP/WebSocket handling

### Security Requirements
- Buildah must run in rootless mode
- All secrets handled via Nomad Vault integration - server never handles raw credentials
- Stateless design for horizontal scaling

### API Validation Requirements
- **CRITICAL**: Both the web interface (`/mcp/submitJob`) and the MCP interface (`tools/call` with `submitJob`) must have identical parameter validation
- Always call `validateJobConfig()` in both interfaces to ensure consistent validation
- Required parameters must match between both interfaces (owner, repo_url, git_ref, dockerfile_path, image_name, image_tags, registry_url)
- This prevents runtime errors like "invalid reference format" when Docker image names are malformed due to missing parameters

## MCP API Endpoints (Planned)

The service will expose these MCP endpoints:
- `submitJob`: Submit build request with Git repo, credentials refs, test commands
- `getStatus`: Poll job status (`PENDING`, `BUILDING`, `TESTING`, `PUBLISHING`, `SUCCEEDED`, `FAILED`)
- `getLogs`: Retrieve phase-specific logs for debugging
- `killJob`: Terminate running jobs
- Cleanup endpoint for resource management

## Development Workflow

**CRITICAL: Service Discovery**
- The service runs in Nomad, NOT locally
- NEVER use `localhost:8080` for testing
- Always discover the service address using Consul:
  ```bash
  # Get service address and port
  consul catalog services
  consul catalog service nomad-build-service
  ```
- Use the discovered address for curl commands, e.g.:
  ```bash
  curl -X POST http://10.0.1.12:31183/mcp/submitJob \
    -H "Content-Type: application/json" \
    -d '{"jobConfig": {...}}'
  ```

**Development Steps:**
1. Build: `make build`
2. Deploy: `REGISTRY_URL=registry.cluster:5000 make nomad-restart`
3. Discover service: `consul catalog service nomad-build-service`
4. Test with discovered address: `curl -X POST http://<discovered-ip>:<discovered-port>/mcp/submitJob`

**Do not assume your changes work without testing them immediately!**

## Critical Design Considerations

- **Atomicity**: Build-test-publish is treated as single atomic operation
- **Logging**: Must provide detailed, accessible logs for agent self-correction
- **Networking**: Test phase needs network access for external services
- **Caching**: Implement Buildah layer caching for performance
- **Cleanup**: Automatic garbage collection of jobs and temporary images

## Testing Strategy

Plan for:
- Unit tests for MCP handlers
- Integration tests using mocked Nomad API
- End-to-end test with actual hello-world Docker image build

## Development and Deployment Workflow

### Making Code Changes and Testing Fixes

When making fixes to the codebase, follow this workflow to build, deploy, and test changes:

1. **Make your code changes**
2. **Build and push the Docker image:**
   ```bash
   make docker-push
   ```
   This builds the image and pushes it to `registry.cluster:5000/nomad-build-service:latest`

3. **Deploy the updated service:**
   ```bash
   make nomad-restart
   ```
   This restarts the Nomad job to pull and run the latest image

4. **Verify the deployment worked:**
   ```bash
   nomad job status nomad-build-service
   nomad alloc logs -stderr <alloc-id>  # Check for any startup errors
   ```

### Job Management

**IMPORTANT**: The Nomad job for this service is managed by Terraform in a separate repository, NOT by this codebase.

The service can be restarted to pull new images using:
```bash
make nomad-restart
# or directly: nomad job restart -yes nomad-build-service
```

### Testing Your Changes

**CRITICAL**: After deploying via the above workflow, you MUST immediately test that your changes work by submitting a test build job.

#### Step 1: Find the Service URL

The service registers itself in Consul with dynamic ports. Find the current URL:

```bash
# Method 1: Check Consul service registry
curl -s http://10.0.1.12:8500/v1/catalog/service/nomad-build-service | jq -r '.[0] | "\(.ServiceAddress):\(.ServicePort)"'

# Method 2: Check Nomad allocation 
nomad job status nomad-build-service  # Get allocation ID
nomad alloc status <alloc-id>         # Look for "Allocation Addresses" section
```

#### Step 2: Submit Test Build Job

Use this exact test command with the discovered URL:

```bash
# Replace <SERVICE_URL> with the URL from Step 1, or use environment variable
SERVICE_URL=${SERVICE_URL:-$(curl -s http://${CONSUL_HTTP_ADDR:-10.0.1.12:8500}/v1/catalog/service/nomad-build-service | jq -r '.[0] | "\(.ServiceAddress):\(.ServicePort)"')}

curl -X POST http://${SERVICE_URL}/mcp/submitJob \
  -H "Content-Type: application/json" \
  -d '{
    "job_config": {
      "owner": "test",
      "repo_url": "https://github.com/geraldthewes/docker-build-hello-world.git",
      "git_ref": "main",
      "dockerfile_path": "Dockerfile",
      "image_tags": ["hello-world-test"],
      "registry_url": "registry.cluster:5000/helloworld",
      "test_entry_point": true
    }
  }'
```

#### Step 3: Monitor Job Progress

```bash
# Get the job ID from the response and monitor it
nomad job status build-<job-id>
nomad alloc logs -f <alloc-id>  # Follow build progress and check for errors

# Check final status (RESTful endpoint)
curl http://${SERVICE_URL}/mcp/job/<job-id>/status

# Get logs (RESTful endpoint)  
curl http://${SERVICE_URL}/mcp/job/<job-id>/logs
```

**Do not assume your changes work without testing them immediately!**

## Dependencies

Requires:
- Running Nomad cluster with Docker driver
- Nomad-Vault integration for secrets
- Persistent volume access for build caching
- For GPU builds: GPU drivers and device plugins on Nomad clients