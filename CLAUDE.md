# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the Nomad Build Service, a lightweight, stateless MCP-based server written in Golang. The service enables coding agents to submit Docker image build jobs remotely using Nomad as the backend infrastructure. It orchestrates builds, tests, and publishes using Buildah for daemonless image building.

**Current Status:** This repository contains only a PRD (Product Requirements Document) - the actual implementation has not yet been created.

## Architecture (Planned)

The system will consist of:

- **MCP Server**: Stateless Go application that translates MCP requests into Nomad API calls
- **Nomad Jobs**: Three phases of ephemeral batch jobs:
  1. Build phase: Uses Buildah to build Docker images from Git repos
  2. Test phase: Runs test commands against the built image 
  3. Publish phase: Pushes successful builds to Docker registries
- **Build Caching**: Persistent host volume for Buildah layer caching at `/opt/nomad/data/buildah-cache`

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

## MCP API Endpoints (Planned)

The service will expose these MCP endpoints:
- `submitJob`: Submit build request with Git repo, credentials refs, test commands
- `getStatus`: Poll job status (`PENDING`, `BUILDING`, `TESTING`, `PUBLISHING`, `SUCCEEDED`, `FAILED`)
- `getLogs`: Retrieve phase-specific logs for debugging
- `killJob`: Terminate running jobs
- Cleanup endpoint for resource management

## Development Workflow (To Be Implemented)

Since this is a new project, you'll need to:

1. Initialize Go module: `go mod init nomad-mcp-builder`
2. Set up project structure with proper separation of concerns
3. Implement MCP server handling
4. Create Nomad job templates for build/test/publish phases
5. Add comprehensive logging for agent debugging
6. Implement proper error handling and cleanup

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
# Replace <SERVICE_URL> with the URL from Step 1
curl -X POST http://<SERVICE_URL>/mcp/submitJob \
  -H "Content-Type: application/json" \
  -d '{
    "jobConfig": {
      "owner": "claude-test",
      "repoURL": "https://github.com/geraldthewes/bd",
      "gitRef": "main", 
      "dockerfilePath": "Dockerfile",
      "imageTags": ["test"],
      "registryURL": "registry.cluster:5000/bdtemp",
      "testCommands": ["echo test"]
    }
  }'
```

#### Step 3: Monitor Job Progress

```bash
# Get the job ID from the response and monitor it
nomad job status build-<job-id>
nomad alloc logs -f <alloc-id>  # Follow build progress and check for errors

# Check final status
curl http://<SERVICE_URL>/mcp/getStatus -H "Content-Type: application/json" -d '{"jobID":"<job-id>"}'
```

**Do not assume your changes work without testing them immediately!**

## Dependencies

Requires:
- Running Nomad cluster with Docker driver
- Nomad-Vault integration for secrets
- Persistent volume access for build caching
- For GPU builds: GPU drivers and device plugins on Nomad clients