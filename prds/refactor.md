# Product Requirements Document (PRD): Nomad Build Service Refactor

## Version History
- **Version 2.0**: Simplified refactor, October 07, 2025. Focus on removing MCP protocol complexity while preserving core server functionality. Major CLI enhancements for YAML support and local build management.
- **Version 1.2**: Updated draft, October 07, 2025. Incorporates refinements: YAML merging logic, unified deploy/ substructure, expanded SemVer integration, CLI enhancements, completed non-functional requirements, and minor fixes for clarity/consistency.
- **Version 1.1**: Revised draft, October 07, 2025. Clarifies that the deploy/ subdirectory is managed in the target repository being built.
- **Version 1.0**: Initial draft, October 07, 2025.
- **Author**: AI-assisted proposal based on repository analysis and user feedback.
- **Status**: Draft.

## Overview

### Problem Statement

The current Nomad Build Service implements a lightweight, stateless server in Go for submitting Docker image build jobs to a Nomad cluster. It supports multiple MCP transport protocols (JSON-RPC over HTTP, streamable HTTP with chunked encoding, and SSE) for compatibility with different coding agents. However, the MCP protocol transports (SSE and Streamable HTTP) add unnecessary complexity without providing significant value over simple JSON-RPC.

Additionally, the CLI tool currently only supports JSON job configurations, and managing build history and semantic versioning across multiple target repositories is cumbersome for coding agents.

### Goals

- **Simplify server**: Remove MCP-specific transports (SSE, Streamable HTTP), keeping only JSON-RPC over HTTP for simpler agent integration
- **Preserve core functionality**: Keep all existing server features (Prometheus metrics, Consul locking, graceful termination, webhooks, build history)
- **Enhance CLI tool**: Add YAML configuration support, version management, and local build history tracking
- **Improve agent experience**: Better documentation, clearer job specifications, easier version management

### Non-Goals

- Maintaining backward compatibility with MCP protocol transports
- Changing core server orchestration logic or phase handling
- Adding LLM summarization features (agent's responsibility)
- Automatic git commit/push from server component
- Changes to intermediate image handling or registry workflows
- Changes to existing validation logic

## Scope

### In Scope

**Server Component (Minimal Changes)**:
- Remove MCP transport implementations (SSE, Streamable HTTP)
- Keep JSON-RPC over HTTP as the only transport
- Preserve all existing features:
  - Prometheus metrics (FR9)
  - Build history in Consul KV (FR10)
  - Distributed locking via Consul (FR8.1)
  - Graceful job termination (FR6)
  - Webhook notifications
  - Current phase orchestration (build → test → deploy)
  - Intermediate image handling with `bdtemp-` naming

**CLI Tool (Major Enhancements)**:
- Add YAML configuration support (alongside or replacing JSON)
- Support global configuration file (`deploy/global.yaml`) + per-build override file
- Manage semantic versioning with automatic patch incrementing
- Use branch-aware tagging format: `branchname-vX.Y.Z`
- Create and manage local `deploy/builds/` directory structure in target repo
- Store build metadata, logs, and status locally for agent access
- Invoked from within target repository directory

**Documentation**:
- Create comprehensive `JobSpec.md` (JSON schema, YAML examples, validation rules, all fields documented)
- Create `command.md` or enhance CLI `--help` with complete usage guide for agents
- Update `README.md` to reflect simplified server and enhanced CLI
- Remove MCP-specific tests

### Out of Scope

- UI enhancements or web interfaces
- LLM integration for build summarization (deferred to calling agent)
- Git commit/push automation from server
- Changes to Nomad job orchestration logic
- Performance benchmarking beyond current capabilities
- Multi-repo support

## Assumptions and Dependencies

- Coding agent has access to build tool repository and can commit changes
- CLI tool is invoked from within the target repository directory
- Server component runs on Nomad cluster, accessed via Consul service discovery
- Nomad cluster, Consul, Vault, and Buildah remain available and configured as currently
- Go 1.22+ environment for building and testing
- No backward compatibility required for MCP transports

## User Personas

- **Coding Agent**: Needs simple, reliable build service without protocol complexity; benefits from YAML configs and local build history
- **DevOps Engineer**: Deploys services from target repos; needs clear version management and build tracking
- **AI Agent**: Requires explicit, file-based job specifications and build history for autonomous operation

## Key Requirements

### Functional Requirements

#### FR1: Server Component Changes (Minimal)

**Remove MCP Protocol Transports**:
- Remove SSE (Server-Sent Events) transport implementation
- Remove Streamable HTTP (chunked encoding) transport implementation
- Keep JSON-RPC over HTTP as the sole transport mechanism
- Update server startup and endpoint registration to reflect removal
- Remove MCP-specific code from `internal/mcp/` (transport handlers only, keep core logic)

**Preserve All Existing Functionality**:
- ✅ Keep Prometheus metrics endpoint (`/metrics`)
- ✅ Keep build history in Consul KV (`nomad-build-service/history/<job-id>`)
- ✅ Keep distributed locking via Consul for concurrency control
- ✅ Keep graceful job termination via `killJob` endpoint
- ✅ Keep webhook notification support (now explicitly configured in job config)
- ✅ Keep current phase orchestration (atomic build → test → deploy)
- ✅ Keep intermediate image handling (`<registry>/bdtemp-imagename:branch-job-id`)
- ✅ Keep existing validation logic (no changes)

**Existing Endpoints (Unchanged)**:
- `submitJob`: Accept job configuration (JSON or YAML), return job ID
- `getStatus`: Query job status and metrics
- `getLogs`: Retrieve phase-specific logs
- `killJob`: Graceful job termination
- `cleanup`: Resource cleanup
- `health`: Health check
- `ready`: Readiness probe

#### FR2: CLI Tool YAML Configuration Support

**Add YAML Configuration**:
- CLI tool must accept YAML job configurations (in addition to or replacing JSON)
- Support two configuration modes:
  1. **Single file**: Complete job config in one YAML file
  2. **Split files**: Global config (`deploy/global.yaml`) + per-build config (e.g., `build.yaml`)

**YAML Merging Logic**:
- Per-build settings **override** global settings (simple deep merge)
- No conflict detection needed - last value wins
- Example:
  ```yaml
  # deploy/global.yaml
  target:
    image_name: my-service
  registry_url: registry.cluster:5000/myapp
  registry_credentials_path: secret/nomad/jobs/registry-credentials
  build:
    git_repo: https://github.com/user/my-service.git
    git_credentials_path: secret/nomad/jobs/git-credentials
    dockerfile_path: docker/Dockerfile
    resource_limits:
      cpu: 2000
      memory: 4096
      disk: 20480
  test:
    resource_limits:
      cpu: 2000
      memory: 4096
      disk: 20480
  deploy:
    resource_limits:
      cpu: 2000
      memory: 4096
      disk: 20480
  ```

  ```yaml
  # build.yaml (per-build override)
  meta:
    purpose: "Fix authentication bug"
  target:
    image_tag: v1.2.3
  build:
    git_ref: feature/auth-fix
  test:
    test: true
    test_command: /app/run-tests.sh
  webhooks_url: https://my-webhook.example.com/notify
  ```

**CLI Command Updates**:
```bash
# Single file mode
nomad-build submit -config build.yaml

# Split file mode
nomad-build submit -global deploy/global.yaml -config build.yaml

# Existing commands remain unchanged
nomad-build get-status <job-id>
nomad-build get-logs <job-id> [phase]
nomad-build kill-job <job-id>
nomad-build cleanup <job-id>
```

**Validation**:
- Reuse existing server validation logic (already implemented)
- Validation happens on merged configuration
- Clear error messages for missing required fields

#### FR3: CLI Semantic Versioning Management

**Version Tracking**:
- CLI manages semantic versioning for builds
- Automatic patch-level incrementing on each build
- Branch-aware tagging format: `<branchname>-v<MAJOR>.<MINOR>.<PATCH>`
- Examples:
  - `main-v1.2.3`
  - `feature-auth-v0.1.5`
  - `release-2.0-v2.0.12`

**Version Commands**:
```bash
# Show current version for current branch
nomad-build version-info

# Manual version bumps (when requested by user)
nomad-build version-major <version>  # e.g., main-v2.0.0
nomad-build version-minor <version>  # e.g., main-v1.3.0

# Automatic patch increment on submit
nomad-build submit -config build.yaml  # Auto-increments patch
```

**Version Storage**:
- Store current version in `deploy/version.yaml`:
  ```yaml
  versions:
    main: v1.2.3
    feature-auth: v0.1.5
    release-2.0: v2.0.12
  ```

#### FR4: CLI Local Build History Management

**Deploy Directory Structure** (CLI creates and manages in target repo):
```
deploy/
├── global.yaml              # Global configuration
├── version.yaml             # Version tracking per branch
├── builds/                  # Per-build history
│   ├── main-v1.2.3/
│   │   ├── status.md        # Summary: phases, status, duration
│   │   ├── metadata.yaml    # Job config, timestamps, version, branch
│   │   ├── build.log        # Build phase logs (stdout + stderr)
│   │   ├── test.log         # Test phase logs (if applicable)
│   │   └── deploy.log       # Deploy phase logs (if applicable)
│   ├── main-v1.2.4/
│   │   └── ...
│   └── feature-auth-v0.1.5/
│       └── ...
└── history.md               # Chronological summary of all builds
```

**File Formats**:

**`deploy/builds/<version>/status.md`**:
```markdown
# Build Status: main-v1.2.3

**Status**: SUCCESS
**Started**: 2025-10-07T14:23:45Z
**Completed**: 2025-10-07T14:28:12Z
**Duration**: 4m27s

## Phases
- Build: ✅ SUCCESS (2m15s)
- Test: ✅ SUCCESS (1m30s)
- Deploy: ✅ SUCCESS (42s)

## Image
- Registry: registry.cluster:5000/myapp
- Tags: main-v1.2.3, latest
```

**`deploy/builds/<version>/metadata.yaml`**:
```yaml
version: main-v1.2.3
branch: main
job_id: build-abc123def456
started_at: 2025-10-07T14:23:45Z
completed_at: 2025-10-07T14:28:12Z
status: SUCCESS
purpose: "Fix authentication bug"
git_commit: abc123def456789...
job_config:
  # Full merged job configuration
  target:
    image_name: my-service
    image_tag: v1.2.3
  # ... rest of config
phases:
  build:
    status: SUCCESS
    started_at: 2025-10-07T14:23:45Z
    completed_at: 2025-10-07T14:26:00Z
    duration: 2m15s
  test:
    status: SUCCESS
    started_at: 2025-10-07T14:26:00Z
    completed_at: 2025-10-07T14:27:30Z
    duration: 1m30s
  deploy:
    status: SUCCESS
    started_at: 2025-10-07T14:27:30Z
    completed_at: 2025-10-07T14:28:12Z
    duration: 42s
```

**`deploy/history.md`**:
```markdown
# Build History

## main-v1.2.4 (2025-10-07 15:30:12 UTC)
**Status**: SUCCESS | **Duration**: 4m12s | **Purpose**: Add retry logic

## main-v1.2.3 (2025-10-07 14:28:12 UTC)
**Status**: SUCCESS | **Duration**: 4m27s | **Purpose**: Fix authentication bug

## main-v1.2.2 (2025-10-07 10:15:33 UTC)
**Status**: FAILED | **Duration**: 3m05s | **Purpose**: Update dependencies
**Error**: Build phase failed - compilation error in auth.go
```

**CLI Behavior**:
- After job submission, CLI polls server for status and logs
- Upon completion (success or failure), writes results to `deploy/builds/<version>/`
- Appends entry to `deploy/history.md`
- Stores complete logs for each phase in separate `.log` files

#### FR5: Documentation Updates

**Create `JobSpec.md`**:
- Comprehensive job configuration specification
- JSON schema definition
- YAML examples (global + per-build)
- All fields documented with:
  - Name and type
  - Required vs optional
  - Default values
  - Validation rules
  - Examples
- Separate sections for:
  - Metadata fields (`meta`, `target`)
  - Build configuration (`build`)
  - Test configuration (`test`)
  - Deploy configuration (`deploy`)
  - Resource limits
  - Credentials and secrets

**Create `command.md` or Enhance CLI `--help`**:
- Complete CLI command reference for coding agents
- All commands with examples:
  - `submit`: Job submission with YAML examples
  - `version-info`: Version tracking
  - `version-major`, `version-minor`: Manual version bumps
  - `get-status`: Status checking
  - `get-logs`: Log retrieval
  - `kill-job`: Job termination
  - `cleanup`: Cleanup operations
- Service discovery via Consul
- Environment variables (`NOMAD_BUILD_URL`)
- Integration examples

**Update `README.md`**:
- Remove MCP transport documentation
- Focus on JSON-RPC over HTTP
- Add YAML configuration examples
- Document CLI usage patterns
- Explain `deploy/` directory structure
- Add version management workflow
- Update testing instructions

**Update `CLAUDE.md`**:
- Reflect simplified server architecture
- Document new CLI capabilities
- Update testing workflow
- Remove MCP-specific guidance

#### FR6: Test Updates

**Remove MCP-Specific Tests**:
- Remove tests for SSE transport
- Remove tests for Streamable HTTP transport
- Keep JSON-RPC tests

**Add CLI Tests**:
- YAML parsing (single file and split file modes)
- Configuration merging (global + per-build)
- Version management (increment, tracking)
- Build history file creation
- Integration test with end-to-end CLI workflow

**Preserve Existing Tests**:
- Server core functionality tests
- Job orchestration tests
- Validation tests
- Integration tests (adapted for JSON-RPC only)

### Non-Functional Requirements

#### NFR1: Compatibility
- Server changes must not affect core build orchestration
- Existing JSON job configs should continue to work (server-side)
- CLI should support both JSON and YAML (if JSON support retained)
- All existing Nomad job templates remain compatible

#### NFR2: Performance
- Server performance unchanged (removing transports may improve slightly)
- CLI overhead minimal (<500ms for version tracking and file writes)
- YAML parsing performance acceptable (<100ms for typical configs)

#### NFR3: Security
- All existing security features preserved:
  - Vault integration for secrets
  - Rootless Buildah execution
  - Consul distributed locking
  - No secrets in logs or build history files

#### NFR4: Reliability
- Server reliability unchanged
- CLI must handle errors gracefully:
  - Missing `deploy/` directory (create if needed)
  - Network errors during polling
  - Corrupted version files (reset with warning)
- Atomic file writes to prevent corruption

#### NFR5: Usability
- YAML configs more readable than JSON (comments, multi-line strings)
- Clear error messages for validation failures
- Comprehensive documentation for agents
- Local build history easily browsable by agents

## Implementation Plan

### Phase 1: Server Simplification
1. Remove SSE transport code from `internal/mcp/` or server endpoints
2. Remove Streamable HTTP transport code
3. Update server initialization to only register JSON-RPC endpoints
4. Remove MCP-specific tests
5. Update server documentation
6. Test server functionality with JSON-RPC only

### Phase 2: CLI YAML Support
1. Add YAML parsing library (`gopkg.in/yaml.v3`)
2. Implement configuration merging (global + per-build)
3. Update CLI `submit` command to accept `-global` and `-config` flags
4. Add validation (reuse server validation logic)
5. Add tests for YAML parsing and merging
6. Update CLI documentation

### Phase 3: CLI Version Management
1. Implement version tracking in `deploy/version.yaml`
2. Add automatic patch incrementing
3. Implement `version-info`, `version-major`, `version-minor` commands
4. Add branch detection from git
5. Generate branch-aware tags (`branchname-vX.Y.Z`)
6. Add tests for version management
7. Update documentation

### Phase 4: CLI Build History
1. Implement `deploy/builds/<version>/` directory creation
2. Poll server for job status and logs after submission
3. Write `status.md`, `metadata.yaml`, and phase logs
4. Implement `history.md` appending
5. Add error handling for file operations
6. Add tests for history management
7. Update documentation

### Phase 5: Documentation
1. Create comprehensive `JobSpec.md`
2. Create `command.md` or enhance `--help`
3. Update `README.md`
4. Update `CLAUDE.md`
5. Review all documentation for consistency

### Phase 6: Testing and Validation
1. Run all unit tests
2. Run integration tests against deployed server
3. Manual end-to-end test with CLI
4. Verify build history files created correctly
5. Verify version tracking works across branches
6. Verify YAML configuration merging works as expected

## Success Metrics

- ✅ Server runs without MCP transport code
- ✅ All existing server features functional (metrics, locking, graceful termination, webhooks)
- ✅ CLI accepts YAML configurations (single and split file modes)
- ✅ Configuration merging works correctly (per-build overrides global)
- ✅ Version tracking functional with automatic patch incrementing
- ✅ Build history files created and populated correctly
- ✅ All tests pass (unit and integration)
- ✅ Documentation complete and accurate
- ✅ End-to-end CLI workflow successful (submit → poll → write history)

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing integrations | No backward compatibility required; clean break acceptable |
| YAML parsing errors | Robust validation with clear error messages; comprehensive testing |
| Version file corruption | Atomic writes; reset with warning on corruption |
| Build history disk usage | Document history cleanup best practices; could add automatic rotation later |
| CLI polling overhead | Reasonable polling intervals (1-2 seconds); timeout after reasonable duration |

## Appendices

### A. Example YAML Configurations

**Global Configuration** (`deploy/global.yaml`):
```yaml
target:
  image_name: video-transcription-batch

registry_url: registry.cluster:5000/myapp
registry_credentials_path: secret/nomad/jobs/registry-credentials

build:
  git_repo: https://github.com/geraldthewes/video-transcription-batch.git
  git_credentials_path: secret/nomad/jobs/git-credentials
  dockerfile_path: docker/Dockerfile
  resource_limits:
    cpu: 2000
    memory: 4096
    disk: 20480

test:
  resource_limits:
    cpu: 2000
    memory: 4096
    disk: 20480

deploy:
  resource_limits:
    cpu: 2000
    memory: 4096
    disk: 20480
```

**Per-Build Configuration** (`build.yaml`):
```yaml
meta:
  purpose: "Add S3 unified path support"

target:
  image_tag: v4.0.1  # Overrides version (or auto-incremented)

build:
  git_ref: feature/s3-unified-paths  # Overrides branch

test:
  test: true
  test_command: /app/run-tests.sh

webhooks_url: https://my-webhook.example.com/notify
```

**Merged Configuration** (what gets sent to server):
```yaml
meta:
  purpose: "Add S3 unified path support"

target:
  image_name: video-transcription-batch
  image_tag: v4.0.1

registry_url: registry.cluster:5000/myapp
registry_credentials_path: secret/nomad/jobs/registry-credentials

build:
  git_repo: https://github.com/geraldthewes/video-transcription-batch.git
  git_ref: feature/s3-unified-paths
  git_credentials_path: secret/nomad/jobs/git-credentials
  dockerfile_path: docker/Dockerfile
  resource_limits:
    cpu: 2000
    memory: 4096
    disk: 20480

test:
  test: true
  test_command: /app/run-tests.sh
  resource_limits:
    cpu: 2000
    memory: 4096
    disk: 20480

deploy:
  resource_limits:
    cpu: 2000
    memory: 4096
    disk: 20480

webhooks_url: https://my-webhook.example.com/notify
```

### B. CLI Workflow Example

```bash
# From target repository directory (e.g., /path/to/video-transcription-batch)
cd /path/to/video-transcription-batch

# First time setup: create deploy directory with global config
mkdir -p deploy
cat > deploy/global.yaml << EOF
target:
  image_name: video-transcription-batch
registry_url: registry.cluster:5000/myapp
# ... rest of global config
EOF

# Create per-build config
cat > build.yaml << EOF
meta:
  purpose: "Fix authentication bug"
build:
  git_ref: feature/auth-fix
test:
  test: true
  test_command: /app/run-tests.sh
EOF

# Check current version
nomad-build version-info
# Output: Current version for branch 'feature/auth-fix': feature/auth-fix-v0.1.4
# Output: Next version will be: feature/auth-fix-v0.1.5

# Submit build (auto-increments patch version)
nomad-build submit -global deploy/global.yaml -config build.yaml
# Output: Job submitted: build-abc123def456
# Output: Version: feature/auth-fix-v0.1.5
# Output: Polling for status...
# Output: [BUILDING] Build phase in progress...
# Output: [TESTING] Test phase in progress...
# Output: [PUBLISHING] Deploy phase in progress...
# Output: [SUCCESS] Build completed in 4m27s
# Output: Results written to: deploy/builds/feature/auth-fix-v0.1.5/

# Check build results
cat deploy/builds/feature-auth-fix-v0.1.5/status.md
cat deploy/builds/feature-auth-fix-v0.1.5/build.log
cat deploy/history.md
```

### C. Related Documents

- Original PRD: `prds/PRD.md` - Full product requirements for current implementation
- README: Current MCP-based documentation
- CLAUDE.md: Current development guidance
- Test files: `test/integration/` and `test/unit/`
