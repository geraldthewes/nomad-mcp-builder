

# Product Requirements Document (PRD): Nomad Build Service (Version 2.0)

## 1\. Overview

### 1.1 Product Description

Nomad Build Service is a lightweight, stateless, MCP-based server written in Golang that enables coding agents to submit Docker image build jobs remotely. It orchestrates builds, tests, and publishes using Nomad as the backend infrastructure, ensuring all operations (server, builds, tests) run as Nomad jobs. The service uses Buildah for daemonless image building from Dockerfiles in git repos, supports test execution with network access, provides detailed build logs for error analysis, and publishes successful images to Docker private registries.

This product is designed for agentic code development, offloading resource-intensive builds to remote Nomad clusters while empowering agents to debug failures autonomously through accessible logs.

### 1.2 Target Audience

* **AI Coding Agents:** The primary user, interacting via the MCP protocol for secure, contextual, and automated build-test-deploy workflows.  
* **Developers:** Individuals building containerized applications environments who can leverage the service for standardized builds.

### 1.3 Business Goals

* Enable seamless, remote Docker image builds for agents without requiring local high-end compute resources or access to local GPUs.  
* Provide detailed, accessible logs to allow for autonomous self-correction by agents upon build or test failure.  
* Ensure integration with Nomad for orchestrated workloads.  
* Minimize dependencies by using daemonless build tools (Buildah) and avoiding complex external CI/CD platforms.

### 1.4 Agent Scenarios (User Stories)

* **As a coding agent, I want to submit my Git repository and have the service build a Docker image** so that I can build and test my application in a containerized environment without local dependencies. Applications dependencies shall be provided either directly or in consul and vault for secrets. The details of application building, testing, configuration and dependencies shall be provided in  a Job Configuration. The format of that job configuration shall be documented.  
* **As a coding agent, when a build fails, I need to receive detailed logs** so that I can identify the error in my Dockerfile or source code, attempt a fix, and resubmit the job.  
* **As a coding agent, after a successful build, I need the service to run my specified test commands against the new image** to verify my code changes work as expected.  
* **As a coding agent, when my tests fail, I need to access the test output logs** so that I can debug the application logic, push a fix, and trigger a new build and test run.

## 2\. Scope

### 2.1 In Scope

* MCP server for job submission, status queries, and log retrieval.  
* Nomad job orchestration for: repo cloning, image building (via Buildah), testing (running commands within the new image), and publishing.  
* Support for network access during the test phase (e.g., for connecting to databases or other services like S3).  
* Robust error handling with full, phase-specific logs accessible via MCP.  
* Secure credential handling using pre-populated Nomad Vault variables for Git and container registries.  
* Leveraging Buildah's layer caching via a persistent host volume to accelerate builds, especially for images with extensive dependencies (e.g., CUDA, Python packages).

### 2.2 Out of Scope

* **Advanced Volume Support (Initial Version):** Direct mounting of arbitrary host volumes during tests will be deferred to a future version to simplify the initial implementation.  
* **Full Git Integration:** The service will not integrate with Git webhooks or triggers; the agent is responsible for initiating a job by submitting a job configuration.  
* **Advanced Multi-User Authentication:** Authentication is handled at the infrastructure level by Nomad ACLs and secure MCP communication, not within the application itself.  
* **Non-Dockerfile Builds:** The service assumes the input is always a Git repository containing a Dockerfile.  
* **Integrated Image Scanning:** Security and vulnerability scanning are considered extensions to be added in the future.

## 3\. Features and Requirements

### 3.1 Functional Requirements

#### FR1: Job Submission

* The agent will commit their changes to a new build branch, publish their changes to the git repo before evoking the build. Further changes made until  the build succeeds and tests passes shall be made by the agent on the same branch.   
* The agent submits a build request via an MCP endpoint via a configuration descriptor to be documented.  
* The server validates the inputs and generates a unique job ID.  
* **Secrets Handling:** The agent passes references (e.g., `nomad/jobs/my-repo-secret`) to pre-populated secrets in Nomad's Vault for private Git repositories and container registries. The service itself does not handle raw credentials.  
* Once the build and the test passes, the branch will be merged by the agent back in the current development branch.

#### FR1.1: MCP API Contract (Example)

A build submission request from the agent could look like this:

{

  "owner": "John Doe",

  "repo\_url": "https://github.com/my-org/my-app.git",

  "git\_ref": "main",

  "git\_credentials\_path": "nomad/jobs/my-git-creds",

  "dockerfile\_path": "Dockerfile",

  "image\_tags": \["latest", "1.2.3"\],

  "registry\_url": "docker.io/my-org/my-app",

  "registry\_credentials\_path": "nomad/jobs/my-registry-creds",

  "test\_commands": \[

    "/app/run\_unit\_tests.sh",

    "/app/run\_integration\_tests.sh"

  \]

}

#### FR2: Build Phase

* A Nomad batch job is spawned using the `quay.io/buildah/stable` image.  
* The job task clones the specified Git repository and executes `buildah bud --file <path> --tag <temp> .`.  
* **Build Caching:** The job mounts a shared, persistent host volume (e.g., `/opt/nomad/data/buildah-cache`) to enable Buildah's layer caching, significantly speeding up subsequent builds. Instructions will be provided on how to configure.  
* If the build fails, the job terminates and logs the complete Buildah output for retrieval.


#### FR3: Test Phase

* If the build succeeds, a separate Nomad batch job is spawned to run tests.  
* The task uses the temporary image created in the build phase and executes each test command via `buildah run`.  
* **Networking:** The job can be configured with standard Nomad networking (`bridge` mode) to allow the tests to access external services (e.g., databases, S3, other APIs).  
* If any test command exits with a non-zero status code, the job is marked as failed, and the test logs (stdout/stderr) are captured.

#### FR4: Publish Phase

* If all tests pass, a final Nomad batch job pushes the image to the specified registry using `buildah push <temp> docker://<registry>/<repo>:<tag>`.  
* Authentication is handled by Nomad injecting the referenced registry credentials from Vault into the job's environment.

#### FR5: Logging and Monitoring

* **Real-time Status Updates:** The service should provide access to real-time status updates (e.g., `PENDING`, `BUILDING`, `TESTING`, `PUBLISHING`, `SUCCEEDED`, `FAILED`) via a polling `getStatus` endpoint.  
* **Accessible Logs:** The agent must be able to retrieve detailed, separated logs for each phase (build, test, publish) via a `getLogs` MCP endpoint using the job ID. This is critical for enabling the agent to diagnose and fix errors autonomously. The logs should be the raw output from the underlying Buildah commands. The logs should be accessible during the build and test process to interrupt if necessary.  
* Please be clear if any infrastructure is required for storing the logs (example prometheus.)  
* **Actionable Error Reporting:** On failure, the MCP response should clearly indicate the failed phase and provide direct access to its logs. The goal is to give the agent all necessary information to self-correct.

#### FR6: Ability to kill a job 

* **Killing a Job**: The agent or the user should be able to kill  a build or test job at any time.

#### FR7: Query Endpoints

* **`submitJob`:** Accepts a JSON payload (see FR1.1) and returns a job ID.  
* **`getStatus`:** Takes a job ID and returns the current status of the job.  
* **`getLogs`:** Takes a job ID and returns the logs, preferably structured by phase (e.g., `{"build": "...", "test": "..."}`).  
* **killJob**: takes a job ID and terminates the build or test run.  
* **Cleanup:** Cleanup any residual resources from this build

### 3.2 Non-Functional Requirements

#### 

#### NFR1: Best Practices

* Log everything: Log all tool calls, including inputs, outputs, and timestamps. Comprehensive logging is essential for troubleshooting and post-mortem analysis when things go wrong.  
* Handle timeouts: Assume that the agent or the underlying connection may time out. Tools should be resilient to interruptions and be able to resume or recover gracefully if preempted.  
* Plan for future custom dashboards: Use monitoring platforms with customizable dashboards to visualize key metrics like response times and task completion rates. This helps track performance and spot recurring issues.  
* Review and follow all current best practices for all the components of this service.  
* Verify and use the latest stable version of all components of this service.


#### NFR2: Performance

* The service should handle concurrent build submissions, with the actual build execution scaled by the Nomad cluster.  
* Build time should be optimized via layer caching.

#### NFR3: Security

* Buildah should be run in rootless mode to minimize container privileges.  
* Secrets for Git and registry access must be managed exclusively through Nomad's Vault integration. The server is stateless and never handles secrets directly.

#### NFR3: Reliability

* **Atomicity:** The entire build-test-publish workflow is treated as a single, atomic operation. If any phase fails, the entire job is considered failed. There is no mechanism to retry a single phase; the agent must resubmit the entire job.  
* **Cleanup:** Nomad batch jobs should be configured with a garbage collection policy (`gc.enabled = true`) to ensure that job specifications and allocations are automatically removed from the cluster after completion or failure. Temporary images must also be cleaned up. The agent should also be able to initiate a cleanup

#### NFR4: Usability

* The MCP API must be simple, with clear JSON request and response schemas.  
* Error messages in logs must be detailed and directly reflect the output from the underlying tools to aid agent debugging.

#### NFR5: Scalability

* **Stateless Server:** The Go server must be stateless, with all job state managed by Nomad. This allows the server to be deployed as a replicated service job in Nomad for high availability and horizontal scaling.  
* **Artifact Storage:** Larger artifacts required for builds or tests should be stored in an external system like S3, with smaller configuration details or state references stored in Consul if necessary.

#### NFR6: Compatibility

* **Go:** Version 1.22+  
* **Nomad:** Version 1.10+ (with Vault integration enabled)  
* **Buildah:** Latest stable version (`quay.io/buildah/stable`)  
* **MCP:** A modern Go implementation of the MCP protocol.

## 4\. Technical Stack

* **Language:** Golang  
* **Key Libraries:**  
  * Nomad API: `github.com/hashicorp/nomad/api`  
  * MCP: A stable Go MCP library for WebSocket/JSON communication.  
  * Logging: `github.com/sirupsen/logrus`  
  * HTTP/WebSockets: Standard `net/http` or a library like `gorilla/websocket`.  
* **Deployment:** The service itself will be deployed as a Dockerized application running as a Nomad service job, exposing its API port.  
* **Testing:** Unit tests for MCP handlers and integration tests using a mocked Nomad API.

## 5\. Architecture

* **Components:**  
  * **MCP Server:** A stateless Go application listening for agent requests. It acts as a control plane, translating MCP requests into Nomad API calls.  
  * **Nomad Client:** Integrated into the Go application to communicate with the Nomad cluster API.  
  * **Build/Test/Push Jobs:** Ephemeral Nomad batch jobs that execute the different phases using a Buildah task driver.  
* **Data Flow:**  
  1. Agent sends a `submitJob` request via MCP to the server.  
  2. The server validates the request and submits a "build" job to the Nomad API.  
  3. Upon successful completion of the build job, a "test" job is submitted.  
  4. Upon successful completion of the test job, a "publish" job to the docker registry is submitted.  
  5. The agent can poll for status or receive real-time updates and request logs for any phase at any time.  
* **Job Atomicity:** The server orchestrates the sequence of jobs. If any job in the sequence fails, the orchestration stops, and the overall job ID is marked as `FAILED`.

## 6\. Assumptions and Dependencies

* A running Nomad cluster is available and configured with the Docker driver.  
* Nomad clients have access to the persistent volume path for Buildah caching.  
* For GPU-dependent builds, relevant Nomad clients are configured with the necessary GPU drivers and device plugins.  
* The AI agent correctly implements an MCP client for communication.  
* Nomad is integrated with Vault for secret management.

## 7\. Risks and Mitigations

* **Risk:** Nomad API rate limiting under heavy load.  
  * **Mitigation:**None  
* **Risk:** Long-running or stuck jobs.  
  * **Mitigation:** Configure aggressive timeouts on all Nomad batch jobs to prevent them from consuming resources indefinitely. Allow override in the Job description for long jobs.


## 8\. Success Metrics

* Demonstrated ability of a test agent to successfully submit a job, receive logs from a failed build, and use those logs to trigger a corrected build.  
  * For example build and execute the hello-world docker image https://hub.docker.com/\_/hello-world

