package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	
	"nomad-mcp-builder/internal/config"
	"nomad-mcp-builder/internal/nomad"
	"nomad-mcp-builder/internal/storage"
	"nomad-mcp-builder/pkg/types"
)

// Server represents the MCP server
type Server struct {
	config     *config.Config
	nomadClient *nomad.Client
	storage    *storage.ConsulStorage
	logger     *logrus.Logger
	
	// WebSocket connections for log streaming
	wsConnections map[string][]*websocket.Conn
	wsMutex       sync.RWMutex
	
	// WebSocket upgrader
	upgrader websocket.Upgrader
}

// NewServer creates a new MCP server
func NewServer(cfg *config.Config, nomadClient *nomad.Client, storage *storage.ConsulStorage) *Server {
	return &Server{
		config:        cfg,
		nomadClient:   nomadClient,
		storage:       storage,
		logger:        logrus.New(),
		wsConnections: make(map[string][]*websocket.Conn),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
	}
}

// Start starts the MCP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	
	// Register MCP endpoints (HTTP/JSON API)
	mux.HandleFunc("/mcp/submitJob", s.handleSubmitJob)
	mux.HandleFunc("/mcp/getStatus", s.handleGetStatus)
	mux.HandleFunc("/mcp/getLogs", s.handleGetLogs)
	mux.HandleFunc("/mcp/streamLogs", s.handleStreamLogs)
	mux.HandleFunc("/mcp/killJob", s.handleKillJob)
	mux.HandleFunc("/mcp/cleanup", s.handleCleanup)
	mux.HandleFunc("/mcp/getHistory", s.handleGetHistory)
	
	// Register RESTful endpoints
	mux.HandleFunc("/mcp/job/", s.handleJobResource)
	
	// Register Standard MCP Protocol endpoints
	mux.HandleFunc("/mcp", s.handleMCPRequest)     // JSON-RPC over HTTP
	mux.HandleFunc("/mcp/stream", s.handleMCPStream) // Streamable HTTP transport
	
	// Health check endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	
	s.logger.WithField("address", server.Addr).Info("Starting MCP server")
	
	// Start background cleanup routine
	go s.backgroundCleanup(ctx)
	
	return server.ListenAndServe()
}

// handleSubmitJob handles job submission requests
func (s *Server) handleSubmitJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req types.SubmitJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest, err.Error())
		return
	}
	
	// Validate required fields
	if err := validateJobConfig(&req.JobConfig); err != nil {
		s.writeErrorResponse(w, "Job configuration validation failed", http.StatusBadRequest, err.Error())
		return
	}
	
	// Create new job
	job, err := s.nomadClient.CreateJob(&req.JobConfig)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create job")
		s.writeErrorResponse(w, "Failed to create job", http.StatusInternalServerError, err.Error())
		return
	}
	
	// Store job in Consul
	if err := s.storage.StoreJob(job); err != nil {
		s.logger.WithError(err).Error("Failed to store job")
		s.writeErrorResponse(w, "Failed to store job", http.StatusInternalServerError, err.Error())
		return
	}
	
	response := types.SubmitJobResponse{
		JobID:  job.ID,
		Status: job.Status,
	}
	
	s.writeJSONResponse(w, response)
	s.logger.WithField("job_id", job.ID).Info("Job submitted successfully")
}

// handleGetStatus handles status requests
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req types.GetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest, err.Error())
		return
	}
	
	// Get job from storage
	job, err := s.storage.GetJob(req.JobID)
	if err != nil {
		s.writeErrorResponse(w, "Job not found", http.StatusNotFound, err.Error())
		return
	}
	
	// Update job status from Nomad
	updatedJob, err := s.nomadClient.UpdateJobStatus(job)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to update job status from Nomad")
		// Continue with cached status
	} else {
		job = updatedJob
		// Update storage with latest status
		if err := s.storage.UpdateJob(job); err != nil {
			s.logger.WithError(err).Warn("Failed to update job in storage")
		}
	}
	
	response := types.GetStatusResponse{
		JobID:   job.ID,
		Status:  job.Status,
		Metrics: job.Metrics,
		Error:   job.Error,
	}
	
	s.writeJSONResponse(w, response)
}

// handleGetLogs handles log retrieval requests
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req types.GetLogsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest, err.Error())
		return
	}
	
	// Get job from storage
	job, err := s.storage.GetJob(req.JobID)
	if err != nil {
		s.writeErrorResponse(w, "Job not found", http.StatusNotFound, err.Error())
		return
	}
	
	// Get latest logs from Nomad
	logs, err := s.nomadClient.GetJobLogs(job)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get logs from Nomad")
		// Return cached logs
		logs = job.Logs
	} else {
		// Update job with latest logs
		job.Logs = logs
		if err := s.storage.UpdateJob(job); err != nil {
			s.logger.WithError(err).Warn("Failed to update job logs in storage")
		}
	}
	
	response := types.GetLogsResponse{
		JobID: job.ID,
		Logs:  logs,
	}
	
	s.writeJSONResponse(w, response)
}

// handleStreamLogs handles WebSocket log streaming
func (s *Server) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id parameter required", http.StatusBadRequest)
		return
	}
	
	// Upgrade to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.WithError(err).Error("Failed to upgrade to WebSocket")
		return
	}
	defer conn.Close()
	
	// Add connection to tracking
	s.wsMutex.Lock()
	s.wsConnections[jobID] = append(s.wsConnections[jobID], conn)
	s.wsMutex.Unlock()
	
	// Remove connection when done
	defer func() {
		s.wsMutex.Lock()
		connections := s.wsConnections[jobID]
		for i, c := range connections {
			if c == conn {
				s.wsConnections[jobID] = append(connections[:i], connections[i+1:]...)
				break
			}
		}
		if len(s.wsConnections[jobID]) == 0 {
			delete(s.wsConnections, jobID)
		}
		s.wsMutex.Unlock()
	}()
	
	// Stream logs
	s.streamJobLogs(conn, jobID)
}

// handleKillJob handles job termination requests
func (s *Server) handleKillJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req types.KillJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest, err.Error())
		return
	}
	
	// Get job from storage
	job, err := s.storage.GetJob(req.JobID)
	if err != nil {
		s.writeErrorResponse(w, "Job not found", http.StatusNotFound, err.Error())
		return
	}
	
	// Kill job in Nomad
	err = s.nomadClient.KillJob(job)
	success := err == nil
	
	var message string
	if success {
		message = "Job killed successfully"
		job.Status = types.StatusFailed
		job.Error = "Job killed by user"
		job.FinishedAt = &[]time.Time{time.Now()}[0]
		s.storage.UpdateJob(job)
	} else {
		message = fmt.Sprintf("Failed to kill job: %v", err)
	}
	
	response := types.KillJobResponse{
		JobID:   req.JobID,
		Success: success,
		Message: message,
	}
	
	s.writeJSONResponse(w, response)
}

// handleCleanup handles cleanup requests
func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req types.CleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest, err.Error())
		return
	}
	
	var cleanedJobs []string
	var err error
	
	if req.All {
		cleanedJobs, err = s.cleanupZombieJobs()
	} else if req.JobID != "" {
		err = s.cleanupSingleJob(req.JobID)
		if err == nil {
			cleanedJobs = []string{req.JobID}
		}
	}
	
	success := err == nil
	message := "Cleanup completed"
	if !success {
		message = fmt.Sprintf("Cleanup failed: %v", err)
	}
	
	response := types.CleanupResponse{
		Success:     success,
		CleanedJobs: cleanedJobs,
		Message:     message,
	}
	
	s.writeJSONResponse(w, response)
}

// handleGetHistory handles job history requests
func (s *Server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req types.GetHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest, err.Error())
		return
	}
	
	// Set defaults
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	
	jobs, total, err := s.storage.GetJobHistory(req.Limit, req.Offset)
	if err != nil {
		s.writeErrorResponse(w, "Failed to get job history", http.StatusInternalServerError, err.Error())
		return
	}
	
	response := types.GetHistoryResponse{
		Jobs:  jobs,
		Total: total,
	}
	
	s.writeJSONResponse(w, response)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	services := make(map[string]string)
	
	// Check Nomad connectivity
	if err := s.nomadClient.Health(); err != nil {
		services["nomad"] = "unhealthy"
	} else {
		services["nomad"] = "healthy"
	}
	
	// Check Consul connectivity
	if err := s.storage.Health(); err != nil {
		services["consul"] = "unhealthy"
	} else {
		services["consul"] = "healthy"
	}
	
	// Overall health status
	status := "healthy"
	for _, serviceStatus := range services {
		if serviceStatus != "healthy" {
			status = "unhealthy"
			break
		}
	}
	
	response := types.HealthResponse{
		Status:    status,
		Services:  services,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	
	if status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	s.writeJSONResponse(w, response)
}

// handleReady handles readiness probe requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Simple readiness check - service is ready if it can respond
	response := map[string]string{
		"status": "ready",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	s.writeJSONResponse(w, response)
}

// Helper methods

func (s *Server) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

func (s *Server) writeErrorResponse(w http.ResponseWriter, message string, code int, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	
	response := types.ErrorResponse{
		Error:   message,
		Code:    code,
		Details: details,
	}
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.WithError(err).Error("Failed to encode error response")
	}
}

func (s *Server) streamJobLogs(conn *websocket.Conn, jobID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	var lastLogCount int
	
	for {
		select {
		case <-ticker.C:
			job, err := s.storage.GetJob(jobID)
			if err != nil {
				s.logger.WithError(err).Warn("Failed to get job for log streaming")
				continue
			}
			
			// Get updated logs from Nomad
			logs, err := s.nomadClient.GetJobLogs(job)
			if err != nil {
				continue
			}
			
			// Send new log entries
			totalLogs := len(logs.Build) + len(logs.Test) + len(logs.Publish)
			if totalLogs > lastLogCount {
				// Send build logs
				for i := lastLogCount; i < len(logs.Build) && i >= 0; i++ {
					msg := types.StreamLogsMessage{
						JobID:     jobID,
						Phase:     "build",
						Timestamp: time.Now().Format(time.RFC3339),
						Level:     "INFO",
						Message:   logs.Build[i],
					}
					if err := conn.WriteJSON(msg); err != nil {
						return
					}
				}
				
				// Send test logs
				buildLogCount := len(logs.Build)
				for i := max(0, lastLogCount-buildLogCount); i < len(logs.Test); i++ {
					msg := types.StreamLogsMessage{
						JobID:     jobID,
						Phase:     "test",
						Timestamp: time.Now().Format(time.RFC3339),
						Level:     "INFO",
						Message:   logs.Test[i],
					}
					if err := conn.WriteJSON(msg); err != nil {
						return
					}
				}
				
				// Send publish logs
				testLogStart := buildLogCount + len(logs.Test)
				for i := max(0, lastLogCount-testLogStart); i < len(logs.Publish); i++ {
					msg := types.StreamLogsMessage{
						JobID:     jobID,
						Phase:     "publish",
						Timestamp: time.Now().Format(time.RFC3339),
						Level:     "INFO",
						Message:   logs.Publish[i],
					}
					if err := conn.WriteJSON(msg); err != nil {
						return
					}
				}
				
				lastLogCount = totalLogs
			}
			
			// Stop streaming if job is finished
			if job.Status == types.StatusSucceeded || job.Status == types.StatusFailed {
				return
			}
		}
	}
}

func (s *Server) backgroundCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get configurable retention period, default to 7 days
			retentionDays := s.config.Build.LogRetentionDays
			if retentionDays <= 0 {
				retentionDays = 7 // Default to 7 days
			}
			
			// Only cleanup old job history automatically - normal cleanup should be done explicitly
			if err := s.storage.CleanupOldHistory(time.Duration(retentionDays) * 24 * time.Hour); err != nil {
				s.logger.WithError(err).Warn("Failed to cleanup old job history")
			}
			
			// Cleanup zombie jobs (jobs running longer than 24 hours without updates)
			if _, err := s.cleanupZombieJobs(); err != nil {
				s.logger.WithError(err).Warn("Failed to cleanup zombie jobs")
			}
		}
	}
}

func (s *Server) cleanupZombieJobs() ([]string, error) {
	// Implementation for cleaning up zombie/orphaned jobs
	// This would involve querying Nomad for running jobs and comparing with stored jobs
	return []string{}, nil // Placeholder
}

func (s *Server) cleanupSingleJob(jobID string) error {
	job, err := s.storage.GetJob(jobID)
	if err != nil {
		return err
	}
	
	return s.nomadClient.CleanupJob(job)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MCP Protocol Handlers

// handleMCPRequest processes standard MCP JSON-RPC requests
func (s *Server) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var mcpReq MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&mcpReq); err != nil {
		response := NewMCPErrorResponse(nil, MCPErrorParseError, "Parse error", err.Error())
		s.writeMCPResponse(w, response)
		return
	}

	var response MCPResponse
	switch mcpReq.Method {
	case "tools/list":
		response = s.handleMCPToolsList(mcpReq)
	case "tools/call":
		response = s.handleMCPToolsCall(mcpReq)
	case "initialize":
		response = s.handleMCPInitialize(mcpReq)
	default:
		response = NewMCPErrorResponse(mcpReq.ID, MCPErrorMethodNotFound, "Method not found", mcpReq.Method)
	}

	s.writeMCPResponse(w, response)
}

// handleMCPStream provides streamable HTTP transport for MCP Inspector and modern clients
func (s *Server) handleMCPStream(w http.ResponseWriter, r *http.Request) {
	// Set headers for streamable HTTP transport
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")

	// Handle preflight requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Handle bidirectional streaming
	decoder := json.NewDecoder(r.Body)
	encoder := json.NewEncoder(w)

	for {
		var mcpReq MCPRequest
		if err := decoder.Decode(&mcpReq); err != nil {
			if err.Error() != "EOF" {
				s.logger.WithError(err).Warn("Error decoding MCP stream request")
			}
			break
		}

		var response MCPResponse
		switch mcpReq.Method {
		case "tools/list":
			response = s.handleMCPToolsList(mcpReq)
		case "tools/call":
			response = s.handleMCPToolsCall(mcpReq)
		case "initialize":
			response = s.handleMCPInitialize(mcpReq)
		case "ping":
			response = s.handleMCPPing(mcpReq)
		default:
			response = NewMCPErrorResponse(mcpReq.ID, MCPErrorMethodNotFound, "Method not found", mcpReq.Method)
		}

		// Send response immediately over the stream
		if err := encoder.Encode(response); err != nil {
			s.logger.WithError(err).Error("Failed to encode MCP stream response")
			break
		}
		flusher.Flush()
	}
}

// handleMCPInitialize handles MCP initialization
func (s *Server) handleMCPInitialize(req MCPRequest) MCPResponse {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "nomad-build-service",
			"version": "2.0.0",
		},
	}
	return NewMCPResponse(req.ID, result)
}

// handleMCPPing handles ping requests for keep-alive
func (s *Server) handleMCPPing(req MCPRequest) MCPResponse {
	result := map[string]interface{}{
		"status": "pong",
	}
	return NewMCPResponse(req.ID, result)
}

// handleMCPToolsList handles tools/list requests
func (s *Server) handleMCPToolsList(req MCPRequest) MCPResponse {
	tools := GetTools()
	result := ToolListResult{Tools: tools}
	return NewMCPResponse(req.ID, result)
}

// handleMCPToolsCall handles tools/call requests by translating to internal API
func (s *Server) handleMCPToolsCall(req MCPRequest) MCPResponse {
	params, ok := req.Params.(map[string]interface{})
	if !ok {
		return NewMCPErrorResponse(req.ID, MCPErrorInvalidParams, "Invalid params", nil)
	}

	toolName, ok := params["name"].(string)
	if !ok {
		return NewMCPErrorResponse(req.ID, MCPErrorInvalidParams, "Tool name required", nil)
	}

	arguments, ok := params["arguments"].(map[string]interface{})
	if !ok {
		arguments = make(map[string]interface{})
	}

	switch toolName {
	case "submitJob":
		return s.mcpSubmitJob(req.ID, arguments)
	case "getStatus":
		return s.mcpGetStatus(req.ID, arguments)
	case "getLogs":
		return s.mcpGetLogs(req.ID, arguments)
	case "killJob":
		return s.mcpKillJob(req.ID, arguments)
	case "cleanup":
		return s.mcpCleanup(req.ID, arguments)
	case "getHistory":
		return s.mcpGetHistory(req.ID, arguments)
	case "purgeFailedJob":
		return s.mcpPurgeFailedJob(req.ID, arguments)
	default:
		return NewMCPErrorResponse(req.ID, MCPErrorMethodNotFound, "Tool not found", toolName)
	}
}

// MCP Tool implementations (translate to existing internal methods)

func (s *Server) mcpSubmitJob(id interface{}, args map[string]interface{}) MCPResponse {
	// Convert MCP arguments to internal job config
	var jobConfig types.JobConfig

	if owner, ok := args["owner"].(string); ok {
		jobConfig.Owner = owner
	}
	if repoURL, ok := args["repo_url"].(string); ok {
		jobConfig.RepoURL = repoURL
	}
	if gitRef, ok := args["git_ref"].(string); ok {
		jobConfig.GitRef = gitRef
	} else {
		jobConfig.GitRef = "main"
	}
	if gitCreds, ok := args["git_credentials_path"].(string); ok {
		jobConfig.GitCredentialsPath = gitCreds
	} else {
		jobConfig.GitCredentialsPath = "secret/nomad/jobs/git-credentials"
	}
	if dockerfile, ok := args["dockerfile_path"].(string); ok {
		jobConfig.DockerfilePath = dockerfile
	} else {
		jobConfig.DockerfilePath = "Dockerfile"
	}
	if regURL, ok := args["registry_url"].(string); ok {
		jobConfig.RegistryURL = regURL
	}
	if regCreds, ok := args["registry_credentials_path"].(string); ok {
		jobConfig.RegistryCredentialsPath = regCreds
	} else {
		jobConfig.RegistryCredentialsPath = "secret/nomad/jobs/registry-credentials"
	}

	// Convert image tags
	if tagsInterface, ok := args["image_tags"].([]interface{}); ok {
		for _, tag := range tagsInterface {
			if tagStr, ok := tag.(string); ok {
				jobConfig.ImageTags = append(jobConfig.ImageTags, tagStr)
			}
		}
	}

	// Convert test commands
	if testCmdsInterface, ok := args["test_commands"].([]interface{}); ok {
		for _, cmd := range testCmdsInterface {
			if cmdStr, ok := cmd.(string); ok {
				jobConfig.TestCommands = append(jobConfig.TestCommands, cmdStr)
			}
		}
	}

	// Create job using existing logic
	job, err := s.nomadClient.CreateJob(&jobConfig)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Failed to create job", err.Error())
	}

	// Store job
	if err := s.storage.StoreJob(job); err != nil {
		s.logger.WithError(err).Warn("Failed to store job in storage")
	}

	result := ToolCallResult{
		Content: NewMCPJSONContent(map[string]interface{}{
			"job_id": job.ID,
			"status": job.Status,
		}),
	}
	return NewMCPResponse(id, result)
}

func (s *Server) mcpGetStatus(id interface{}, args map[string]interface{}) MCPResponse {
	jobID, ok := args["job_id"].(string)
	if !ok {
		return NewMCPErrorResponse(id, MCPErrorInvalidParams, "job_id required", nil)
	}

	job, err := s.storage.GetJob(jobID)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Job not found", err.Error())
	}

	// Update status from Nomad
	if updatedJob, err := s.nomadClient.UpdateJobStatus(job); err == nil {
		job = updatedJob
		s.storage.UpdateJob(job) // Update storage
	}

	result := ToolCallResult{
		Content: NewMCPJSONContent(map[string]interface{}{
			"job_id": job.ID,
			"status": job.Status,
			"error":  job.Error,
			"phase":  job.FailedPhase,
		}),
	}
	return NewMCPResponse(id, result)
}

func (s *Server) mcpGetLogs(id interface{}, args map[string]interface{}) MCPResponse {
	jobID, ok := args["job_id"].(string)
	if !ok {
		return NewMCPErrorResponse(id, MCPErrorInvalidParams, "job_id required", nil)
	}

	job, err := s.storage.GetJob(jobID)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Job not found", err.Error())
	}

	logs, err := s.nomadClient.GetJobLogs(job)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Failed to get logs", err.Error())
	}

	phase, _ := args["phase"].(string)
	var result interface{}
	
	switch phase {
	case "build":
		result = logs.Build
	case "test":
		result = logs.Test
	case "publish":
		result = logs.Publish
	default:
		result = logs
	}

	toolResult := ToolCallResult{
		Content: NewMCPJSONContent(result),
	}
	return NewMCPResponse(id, toolResult)
}

func (s *Server) mcpKillJob(id interface{}, args map[string]interface{}) MCPResponse {
	jobID, ok := args["job_id"].(string)
	if !ok {
		return NewMCPErrorResponse(id, MCPErrorInvalidParams, "job_id required", nil)
	}

	job, err := s.storage.GetJob(jobID)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Job not found", err.Error())
	}

	if err := s.nomadClient.KillJob(job); err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Failed to kill job", err.Error())
	}

	// Update job status
	job.Status = types.StatusFailed
	job.Error = "Job killed by user"
	s.storage.UpdateJob(job)

	result := ToolCallResult{
		Content: NewMCPTextContent("Job terminated successfully"),
	}
	return NewMCPResponse(id, result)
}

func (s *Server) mcpCleanup(id interface{}, args map[string]interface{}) MCPResponse {
	jobID, ok := args["job_id"].(string)
	if !ok {
		return NewMCPErrorResponse(id, MCPErrorInvalidParams, "job_id required", nil)
	}

	job, err := s.storage.GetJob(jobID)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Job not found", err.Error())
	}

	if err := s.nomadClient.CleanupJob(job); err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Failed to cleanup job", err.Error())
	}

	result := ToolCallResult{
		Content: NewMCPTextContent("Job resources cleaned up successfully"),
	}
	return NewMCPResponse(id, result)
}

func (s *Server) mcpGetHistory(id interface{}, args map[string]interface{}) MCPResponse {
	limit := 10
	if limitFloat, ok := args["limit"].(float64); ok {
		limit = int(limitFloat)
	}
	
	offset := 0 // Default offset for MCP interface

	history, total, err := s.storage.GetJobHistory(limit, offset)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Failed to get history", err.Error())
	}
	
	// Filter by owner if specified (post-filtering for simplicity)
	if ownerFilter, ok := args["owner"].(string); ok && ownerFilter != "" {
		var filteredHistory []types.JobHistory
		for _, job := range history {
			if job.Config.Owner == ownerFilter {
				filteredHistory = append(filteredHistory, job)
			}
		}
		history = filteredHistory
	}

	result := ToolCallResult{
		Content: NewMCPJSONContent(map[string]interface{}{
			"jobs":  history,
			"total": total,
		}),
	}
	return NewMCPResponse(id, result)
}

func (s *Server) mcpPurgeFailedJob(id interface{}, args map[string]interface{}) MCPResponse {
	jobID, ok := args["job_id"].(string)
	if !ok {
		return NewMCPErrorResponse(id, MCPErrorInvalidParams, "job_id required", nil)
	}

	job, err := s.storage.GetJob(jobID)
	if err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Job not found", err.Error())
	}

	// Use the new CleanupFailedJobs method to purge from Nomad
	if err := s.nomadClient.CleanupFailedJobs(job); err != nil {
		return NewMCPErrorResponse(id, MCPErrorInternalError, "Failed to purge job from Nomad", err.Error())
	}

	result := ToolCallResult{
		Content: NewMCPTextContent(fmt.Sprintf("Job %s purged successfully from Nomad", jobID)),
	}
	return NewMCPResponse(id, result)
}

// Helper method to write MCP responses
func (s *Server) writeMCPResponse(w http.ResponseWriter, response MCPResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.WithError(err).Error("Failed to encode MCP response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// validateJobConfig validates the job configuration and returns an error if validation fails
func validateJobConfig(config *types.JobConfig) error {
	// Required fields
	if config.Owner == "" {
		return fmt.Errorf("owner is required")
	}
	if config.RepoURL == "" {
		return fmt.Errorf("repo_url is required")
	}
	if config.GitRef == "" {
		return fmt.Errorf("git_ref is required")
	}
	if config.DockerfilePath == "" {
		return fmt.Errorf("dockerfile_path is required")
	}
	if len(config.ImageTags) == 0 {
		return fmt.Errorf("image_tags is required and cannot be empty")
	}
	if config.RegistryURL == "" {
		return fmt.Errorf("registry_url is required")
	}
	
	// Validate at least one testing mode is specified if test_commands is empty
	if len(config.TestCommands) == 0 && !config.TestEntryPoint {
		// This is allowed - no testing will be performed
	}
	
	// Optional fields (git_credentials_path, registry_credentials_path, test_entry_point, test_commands)
	// are allowed to be empty
	
	return nil
}

// handleJobResource handles RESTful job resource endpoints
// Routes: GET /mcp/job/{jobID}/status and GET /mcp/job/{jobID}/logs
func (s *Server) handleJobResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Parse URL path: /mcp/job/{jobID}/{resource}
	path := strings.TrimPrefix(r.URL.Path, "/mcp/job/")
	parts := strings.Split(path, "/")
	
	if len(parts) != 2 {
		http.Error(w, "Invalid path format. Expected: /mcp/job/{jobID}/{status|logs}", http.StatusBadRequest)
		return
	}
	
	jobID := parts[0]
	resource := parts[1]
	
	if jobID == "" {
		http.Error(w, "Job ID is required", http.StatusBadRequest)
		return
	}
	
	switch resource {
	case "status":
		s.handleJobStatus(w, r, jobID)
	case "logs":
		s.handleJobLogs(w, r, jobID)
	default:
		http.Error(w, "Invalid resource. Expected: status or logs", http.StatusBadRequest)
	}
}

// handleJobStatus handles GET /mcp/job/{jobID}/status
func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	job, err := s.storage.GetJob(jobID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get job")
		s.writeErrorResponse(w, "Failed to get job", http.StatusInternalServerError, "")
		return
	}
	
	if job == nil {
		s.writeErrorResponse(w, "Job not found", http.StatusNotFound, "")
		return
	}
	
	// Update job status before returning
	updatedJob, err := s.nomadClient.UpdateJobStatus(job)
	if err != nil {
		s.logger.WithError(err).Error("Failed to update job status")
		// Continue with existing job data rather than failing
		updatedJob = job
	}
	
	response := types.GetStatusResponse{
		JobID:   updatedJob.ID,
		Status:  updatedJob.Status,
		Metrics: updatedJob.Metrics,
		Error:   updatedJob.Error,
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.WithError(err).Error("Failed to encode status response")
		s.writeErrorResponse(w, "Failed to encode response", http.StatusInternalServerError, "")
	}
}

// handleJobLogs handles GET /mcp/job/{jobID}/logs
func (s *Server) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string) {
	job, err := s.storage.GetJob(jobID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get job")
		s.writeErrorResponse(w, "Failed to get job", http.StatusInternalServerError, "")
		return
	}
	
	if job == nil {
		s.writeErrorResponse(w, "Job not found", http.StatusNotFound, "")
		return
	}
	
	logs, err := s.nomadClient.GetJobLogs(job)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get job logs")
		s.writeErrorResponse(w, "Failed to get job logs", http.StatusInternalServerError, "")
		return
	}
	
	response := types.GetLogsResponse{
		JobID: job.ID,
		Logs:  logs,
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.WithError(err).Error("Failed to encode logs response")
		s.writeErrorResponse(w, "Failed to encode response", http.StatusInternalServerError, "")
	}
}