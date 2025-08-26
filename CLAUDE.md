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

## Dependencies

Requires:
- Running Nomad cluster with Docker driver
- Nomad-Vault integration for secrets
- Persistent volume access for build caching
- For GPU builds: GPU drivers and device plugins on Nomad clients