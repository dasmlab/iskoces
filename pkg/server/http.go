package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dasmlab/iskoces/pkg/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// HTTPServer provides HTTP endpoints for translation job status and SSE progress updates.
type HTTPServer struct {
	jobQueue *service.JobQueue
	logger   *logrus.Logger
	port     int
}

// NewHTTPServer creates a new HTTP server for job status and SSE.
func NewHTTPServer(jobQueue *service.JobQueue, logger *logrus.Logger, port int) *HTTPServer {
	return &HTTPServer{
		jobQueue: jobQueue,
		logger:   logger,
		port:     port,
	}
}

// Start starts the HTTP server.
func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// Job status endpoint (GET /api/v1/jobs/:jobID)
	// SSE endpoint for job progress (GET /api/v1/jobs/:jobID/events)
	// Both handled by the same function which routes based on path
	mux.HandleFunc("/api/v1/jobs/", s.handleJobRequest)

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", s.port)
	s.logger.WithFields(logrus.Fields{
		"port": s.port,
	}).Info("Starting HTTP server for job status and SSE")

	return http.ListenAndServe(addr, mux)
}

// handleJobRequest handles both job status and SSE events based on the path.
func (s *HTTPServer) handleJobRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	path := r.URL.Path[len("/api/v1/jobs/"):]
	if path == "" {
		http.Error(w, "Job ID is required", http.StatusBadRequest)
		return
	}

	// Check if this is an SSE request
	isSSE := false
	jobID := path
	if len(path) > len("/events") && path[len(path)-len("/events"):] == "/events" {
		isSSE = true
		jobID = path[:len(path)-len("/events")]
	}

	// Get job from queue
	job, err := s.jobQueue.GetJob(jobID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Job not found: %v", err), http.StatusNotFound)
		return
	}

	if isSSE {
		s.handleJobEventsSSE(w, r, job)
	} else {
		s.handleJobStatusJSON(w, r, job)
	}
}

// handleJobStatusJSON returns the current status of a translation job as JSON.
func (s *HTTPServer) handleJobStatusJSON(w http.ResponseWriter, r *http.Request, job *service.TranslationJob) {
	// Get current status (thread-safe)
	status, message, progress := job.GetStatus()

	// Build response
	response := map[string]interface{}{
		"job_id":          job.ID,
		"request_id":      job.RequestID,
		"status":          string(status),
		"progress_percent": progress,
		"progress_message": message,
		"created_at":      job.CreatedAt.Format(time.RFC3339),
	}

	if job.StartedAt != nil {
		response["started_at"] = job.StartedAt.Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		response["completed_at"] = job.CompletedAt.Format(time.RFC3339)
	}
	if job.Error != "" {
		response["error"] = job.Error
	}

	// If completed, include results
	if status == service.JobStatusCompleted {
		response["translated_title"] = job.TranslatedTitle
		response["translated_markdown"] = job.TranslatedMarkdown
		response["tokens_used"] = job.TokensUsed
		response["inference_time"] = job.InferenceTime
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleJobEventsSSE provides Server-Sent Events (SSE) for job progress updates.
func (s *HTTPServer) handleJobEventsSSE(w http.ResponseWriter, r *http.Request, job *service.TranslationJob) {
	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a ticker to poll job status
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Send initial status
	s.sendSSEEvent(w, "status", job)

	// Poll for updates
	lastStatus := ""
	lastProgress := int32(-1)

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Get current status
			status, _, progress := job.GetStatus()

			// Send update if status or progress changed
			if string(status) != lastStatus || progress != lastProgress {
				s.sendSSEEvent(w, "status", job)
				lastStatus = string(status)
				lastProgress = progress

				// If job is completed or failed, send final event and close
				if status == service.JobStatusCompleted || status == service.JobStatusFailed {
					time.Sleep(100 * time.Millisecond) // Small delay to ensure final event is sent
					return
				}
			}
		}
	}
}

// sendSSEEvent sends a Server-Sent Event.
func (s *HTTPServer) sendSSEEvent(w http.ResponseWriter, eventType string, job *service.TranslationJob) {
	status, message, progress := job.GetStatus()

	event := map[string]interface{}{
		"job_id":          job.ID,
		"request_id":      job.RequestID,
		"status":          string(status),
		"progress_percent": progress,
		"progress_message": message,
		"timestamp":        time.Now().Format(time.RFC3339),
	}

	if job.Error != "" {
		event["error"] = job.Error
	}

	// If completed, include results
	if status == service.JobStatusCompleted {
		event["translated_title"] = job.TranslatedTitle
		event["translated_markdown"] = job.TranslatedMarkdown
		event["tokens_used"] = job.TokensUsed
		event["inference_time"] = job.InferenceTime
	}

	// Encode to JSON
	data, err := json.Marshal(event)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal SSE event")
		return
	}

	// Write SSE format: event: <type>\ndata: <json>\n\n
	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", string(data))

	// Flush to ensure data is sent immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// handleHealth provides a health check endpoint.
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

