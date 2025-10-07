# Product Requirements Document (PRD): Refactoring Nomad MCP Builder to a Library-Focused Build Tool

## Version History
- **Version 1.1**: Revised draft, October 07, 2025. Clarifies that the deploy/ subdirectory is managed in the target repository being built (e.g., the repo containing the code and Dockerfiles for the service, such as video-transcription-batch), not in the build tool repository itself.
- **Version 1.0**: Initial draft, October 07, 2025.
- **Author**: Grok 4 (AI-assisted proposal based on repository analysis and user feedback).
- **Status**: Draft.

## Overview
### Problem Statement
The current Nomad MCP Builder repository implements a lightweight, stateless MCP server in Go for submitting Docker image build jobs to a Nomad cluster. It supports MCP-compliant endpoints for tool calls (e.g., `submitJob`, `getStatus`, `getLogs`) with multiple transport methods (JSON-RPC over HTTP, streamable HTTP with chunked encoding per MCP 2025-03-26 spec, and legacy SSE per MCP 2024-11-05 spec). However, issues arise with coding agents, such as Qwen Coder 32B struggling with reliable tool calls and confusion over transport versions and specs. The job configuration uses JSON, and deployment/version management is distributed across Nomad jobs, environment variables, and Consul KV stores, lacking centralization.

This PRD proposes a refactor to shift away from MCP dependency, emphasizing the existing Go library (`pkg/client`) and CLI tool (`nomad-build`) for simpler, more reliable integrations. It introduces YAML for job configs, explicit phase splitting (build, test, deploy), and support for a centralized `deploy/` subdirectory *in the target repository being built* (e.g., the service repo like video-transcription-batch) for version management, build summaries, and history. This aligns with principles of treating deployment as code, making the target repo self-contained for agents, and embedding best practices for Git, Docker builds, and semantic versioning.

The build tool will clone the target repo during the build process, perform operations, and update the `deploy/` dir within that cloned workspace before potentially committing/pushing changes back (if configured). This ensures the deployment artifacts and history reside in the service repo itself, not the build tool repo.

### Goals
- Improve agent reliability by eliminating MCP protocol complexities.
- Enhance modularity and readability with YAML configs and phase separation.
- Centralize deployment logic and history in the target repo's `deploy/` dir for easy agent access and reproducibility.
- Maintain core functionality: Orchestrating Docker builds on Nomad using Buildah, with support for secrets via Vault and configs via Consul.
- Enable performance optimizations (e.g., direct publish without tests) as caller responsibilities.

### Non-Goals
- Adding new features like multi-repo support or advanced ML integrations (e.g., LakeFS for model versioning).
- Maintaining backward compatibility with MCP specs or transports.
- Changing the underlying infrastructure (Nomad, Buildah, Consul, Vault).
- Implementing the refactor itself—this PRD is for a coding agent to execute the changes.

## Scope
### In Scope
- Refactoring the server, library, and CLI to remove MCP dependencies.
- Updating job config syntax from JSON to YAML.
- Splitting build pipeline into explicit phases with caller-controlled orchestration.
- Adding logic to manage a `deploy/` subdirectory in the cloned target repo during builds.
- Updating documentation (README.md, PRD.m) and tests to reflect changes, including examples from target repos like video-transcription-batch. Thoroughly document the job specification.
- Ensuring semantic versioning best practices are embedded in deployment code within the target repo.

### Out of Scope
- UI enhancements or web interfaces.
- Integration with specific LLMs beyond general agent accessibility.
- Performance benchmarking or scaling optimizations beyond phase splitting.
- Automatic git commit/push from the build tool (optional extension; caller-responsible).

## Assumptions and Dependencies
- The coding agent has access to the build tool repository and can commit changes.
- The build command will be executed in the target repo, the build will be executed on a cloned copy in nomad. The output of the build will be stored in the local target repo.
- Nomad cluster, Consul, Vault, and Buildah remain available for testing.
- Go environment (e.g., Go 1.20+) for building and testing.
- Agents will use the library/CLI directly, not via MCP.
- No breaking changes to external dependencies like Nomad APIs.
- Target repos like video-transcription-batch already integrate with the build tool for Docker image creation (e.g., triggering builds on git push to produce tagged images).

## User Personas
- **Coding Agent Developer**: Builds and deploys using LLMs like Qwen; needs reliable, simple interfaces without protocol pitfalls.
- **DevOps Engineer**: Deploys services from target repos; benefits from centralized deployment code in the service repo.
- **AI Agent**: Interacts with target repo for automated builds; requires explicit, file-based history and configs in `deploy/`.

## Key Requirements
### Functional Requirements
1. **Remove MCP Dependency**
   - Eliminate MCP endpoints and transports (HTTP stream, SSE).
   - Refactor the server (`cmd/server`) to expose a simple HTTP API for job submission and management, using standard REST or gRPC if needed, but prioritize library/CLI usage.
   - Update the client library (`pkg/client`) to handle direct interactions without MCP wrappers.
   - Deprecate any MCP-specific code in tests (`test/integration`) but maintain the remaining tests


2. **Switch Job Config to YAML**
   - Change job specification format from JSON to YAML for better readability (supports comments, multi-line strings).
   - Example YAML structure (submitted to the build tool, referencing the target repo):
     ```
	 meta:
	   - purpose: Purpose of this build
     target:
       - image_name: video-transcription-batch
       - image_tag: v4.0.0
	   - registry_url: registry.cluster:5000/myapp
	   - registry_credentials_path: secret/nomad/jobs/registry-credentials
	   - webhooks_url: web hooks
	 build:
       - git_repo: https://github.com/geraldthewes/video-transcription-batch.git
       - git_ref: main
	   - git_credentials_path : secret/nomad/jobs/git-credentials
       - dockerfile_path: docker/Dockerfile
	   - resource_limits: 
	     - cpu: 2000
		 - memory: 4096
		 - disk: 20480
     test:
	   - test: true
	   - test_command: /app/run-tests.sh
	   - resource_limits: 
	     - cpu: 2000
		 - memory: 4096
		 - disk: 20480
	 publish:
	   - resource_limits: 
	     - cpu: 2000
		 - memory: 4096
		 - disk: 20480	 
     ```
   - Allow breaking the YAML file in a global one and a per run one
     -- Global
	      ```
     target:
       - image_name: video-transcription-batch
	   - registry_url: registry.cluster:5000/myapp
	   - registry_credentials_path: secret/nomad/jobs/registry-credentials
	 build:
       - git_repo: https://github.com/geraldthewes/video-transcription-batch.git
	   - git_credentials_path : secret/nomad/jobs/git-credentials
       - dockerfile_path: docker/Dockerfile
	   - resource_limits: 
	     - cpu: 2000
		 - memory: 4096
		 - disk: 20480
     test:
	   - resource_limits: 
	     - cpu: 2000
		 - memory: 4096
		 - disk: 20480
	 publish:
	   - resource_limits: 
	     - cpu: 2000
		 - memory: 4096
		 - disk: 20480	 
     ```
     -- per build configuration
     ```
	 meta:
	   - purpose: Purpose of this build
     target:
       - image_tag: v4.0.0
	   - webhooks_url: web hooks
	 build:
       - git_ref: main
     test:
	   - test: true
	   - test_command: /app/run-tests.sh
     ```
	 
   - Update CLI (`nomad-build`) to parse YAML inputs (e.g., `nomad-build submit -f job.yaml`). Errors are reported to the caller and required attributes checked
   - Library methods to accept YAML strings or files.

3. **Split Build Pipeline into Phases**
   - Define explicit phases: Build (using Buildah to create Docker image), Test (run container tests), Deploy (push to registry).
   - Make phases optional and caller-controlled; e.g., library calls like `client.Build(job)`, `client.Test(job)`, `client.Deploy(job)`.
   - Remove monolithic pipeline; if no tests, caller can chain build → deploy directly for optimization.
   - Update Nomad job dispatching to handle phase-specific tasks, cloning the target repo (e.g., video-transcription-batch) as needed.

4. **Support for Deploy Subdirectory in Target Repo**
   - Add build tool logic to create/manage `deploy/` dir *in the cloned target repository* (not the build tool repo).
   - Have the deploy tool create a subdirectory called 'builds' under `deploy/` and store all the logs and results in them. As in `deploy/builds/v0.0.13/`
      - Have a status.md that includes summary phase results
	  - For each phase store detail logs (stdout and stderr) as in build.md, test.md, deploy.md
   - Substructure in target repo's `deploy/`:
     - `config.yaml`: Global settings (e.g., registry, resources).
     - `builds/`: Per-build YAML files (e.g., `2025-10-07-v4.0.0.yaml`) with:
       - Build version (SemVer).
       - Git version/ref.
       - Purpose (e.g., "Update for unified S3 paths").
       - LLM-generated summary of build output.
     - `history.yaml` or `history.md`: Appended chronological log of all builds, including summaries for agent review.
   - Integrate LLM summarization: Add a subagent hook (e.g., library function `SummarizeOutput(logs string) string`) that prompts an LLM to condense build logs and writes to `deploy/builld-summary.md`.
   - Embed SemVer best practices: Include a `versioning.sh` or Go func to auto-bump versions based on Git commits (using Conventional Commits: feat → minor, fix → patch, breaking → major), executed within the target repo's context.

5. **Update Documentation and Tests**
   - Revise README.md to reflect new approach: Focus on library/CLI usage, YAML examples, phase examples, and how to use `deploy/` in target repos (with video-transcription-batch as an example).
   - If PRD.md exists (assuming it's a product doc), update it to match this PRD's goals.
   - Create detailed JobSpec.md instead of in README,md documentating the Job Spec in detail
   - CLAUDE.md (noted as empty or minimal; perhaps for Claude prompts)—expand if needed for LLM integration examples in target repos.
   - Enhance integration tests to cover YAML parsing, phase splitting, and `deploy/` dir writes in a mocked target repo.

### Non-Functional Requirements
- **Performance**: Phase splitting should allow skipping tests
- **Reliability**: Library/CLI must handle errors gracefully, with clear logging; handle cases where `deploy/` doesn't exist in target repo.
- **Security**: Retain Vault for secrets; no new vulnerabilities from refactor; ensure cloned repos are handled securely.
- **Compatibility**: Support existing Nomad job file (`nomad-build-service.nomad`); update for non-MCP API.
- **Usability**: YAML configs should include schema validation if possible; document target repo integration clearly.

## Implementation Notes
- **Step-by-Step for Coding Agent**:
  1. Clone build tool repo and create feature branch (e.g., `refactor-to-library-focus`).
  2. Remove MCP code: Delete transport handlers in server; refactor endpoints to simple HTTP.
  3. Implement YAML parsing: Use `gopkg.in/yaml.v3` in client and server.
  4. Split phases: Refactor job runner to modular functions; expose in library.
  5. Add target repo `deploy/` support: In build logic, after git clone, create/update `deploy/` files; include optional commit/push flags.
  6. Add summarization: Implement a placeholder LLM call (e.g., via HTTP to a model endpoint) and write to target `deploy/`.
  7. Update README: Rewrite usage sections with YAML examples, emphasizing target repo `deploy/` (reference video-transcription-batch structure).
  8. Run tests: Fix/adapt `go test ./internal/...` and integration tests, using mocked target repos.
  9. Commit and PR: Include this PRD as `docs/refactor-prd.md`.

- **Risks and Mitigations**:
  - Agent confusion on new API: Mitigate with comprehensive examples in README, including target repo flows.
  - YAML parsing errors: Add robust validation and error messages.
  - History file growth in target repo: Implement rotation or archiving for large histories.
  - Git access issues: Document requirements for build tool to have push access if updating target repo.

- **Success Metrics**:
  - Successful build/test/deploy via CLI/library without MCP, updating `deploy/` in target repo.
  - Agents can read/write to target repo's `deploy/` for history review.
  - Reduced issues with tool calls in Qwen-like models.

## Appendices
- **Inspiration Reference**: Based on standard PRD templates (e.g., problem/solution focus, requirements breakdown), inspired by AI-dev task examples emphasizing clear, actionable specs for agents.
- **Related Docs**: Current README.md (describes MCP setup), assumed PRD.md (if exists, for original product vision), CLAUDE.md (minimal; perhaps for prompt engineering). Example target repo: video-transcription-batch (uses build tool for Docker images, integrates with Nomad jobs).
