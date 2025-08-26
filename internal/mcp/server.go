package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	
	// Register MCP endpoints
	mux.HandleFunc("/mcp/submitJob", s.handleSubmitJob)
	mux.HandleFunc("/mcp/getStatus", s.handleGetStatus)
	mux.HandleFunc("/mcp/getLogs", s.handleGetLogs)
	mux.HandleFunc("/mcp/streamLogs", s.handleStreamLogs)
	mux.HandleFunc("/mcp/killJob", s.handleKillJob)
	mux.HandleFunc("/mcp/cleanup", s.handleCleanup)
	mux.HandleFunc("/mcp/getHistory", s.handleGetHistory)
	
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
			// Cleanup old job history (older than 30 days)
			if err := s.storage.CleanupOldHistory(30 * 24 * time.Hour); err != nil {
				s.logger.WithError(err).Warn("Failed to cleanup old job history")
			}
			
			// Cleanup zombie jobs
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