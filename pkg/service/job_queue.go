package service

import (
	"fmt"
	"sync"
	"time"

	nanabushv1 "github.com/dasmlab/iskoces/pkg/proto/v1"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// TranslationJobStatus represents the status of a translation job.
type TranslationJobStatus string

const (
	JobStatusQueued    TranslationJobStatus = "queued"
	JobStatusProcessing TranslationJobStatus = "processing"
	JobStatusCompleted  TranslationJobStatus = "completed"
	JobStatusFailed     TranslationJobStatus = "failed"
)

// TranslationJob represents an asynchronous translation job.
type TranslationJob struct {
	ID            string
	RequestID     string // Client-provided job ID
	Status        TranslationJobStatus
	CreatedAt     time.Time
	StartedAt     *time.Time
	CompletedAt   *time.Time
	Error         string
	
	// Request data
	Primitive     nanabushv1.PrimitiveType
	Title         string
	Document      *nanabushv1.DocumentContent
	SourceLang    string
	TargetLang    string
	
	// Result data
	TranslatedTitle    string
	TranslatedMarkdown string
	TokensUsed         int64
	InferenceTime      float64
	
	// Progress tracking
	ProgressPercent int32
	ProgressMessage string
	
	// Mutex for thread-safe access
	mu sync.RWMutex
}

// JobQueue manages asynchronous translation jobs.
type JobQueue struct {
	jobs      map[string]*TranslationJob
	jobsMu    sync.RWMutex
	logger    *logrus.Logger
	processor *JobProcessor
}

// NewJobQueue creates a new job queue.
func NewJobQueue(logger *logrus.Logger) *JobQueue {
	return &JobQueue{
		jobs:   make(map[string]*TranslationJob),
		logger: logger,
	}
}

// SetProcessor sets the job processor for this queue.
func (q *JobQueue) SetProcessor(processor *JobProcessor) {
	q.processor = processor
}

// CreateJob creates a new translation job and returns its ID.
func (q *JobQueue) CreateJob(req *nanabushv1.TranslateRequest) (string, error) {
	jobID := uuid.New().String()
	
	job := &TranslationJob{
		ID:         jobID,
		RequestID:  req.JobId,
		Status:     JobStatusQueued,
		CreatedAt:  time.Now(),
		Primitive:  req.Primitive,
		SourceLang: req.SourceLanguage,
		TargetLang: req.TargetLanguage,
	}
	
	// Store document data
	if req.Primitive == nanabushv1.PrimitiveType_PRIMITIVE_TITLE {
		job.Title = req.GetTitle()
	} else if req.Primitive == nanabushv1.PrimitiveType_PRIMITIVE_DOC_TRANSLATE {
		job.Document = req.GetDoc()
		if job.Document != nil {
			job.Title = job.Document.Title
		}
	}
	
	q.jobsMu.Lock()
	q.jobs[jobID] = job
	q.jobsMu.Unlock()
	
	q.logger.WithFields(logrus.Fields{
		"job_id":     jobID,
		"request_id": req.JobId,
		"primitive":  req.Primitive.String(),
	}).Info("Created translation job")
	
	// Start processing asynchronously if processor is set
	if q.processor != nil {
		go q.processor.ProcessJob(job)
	}
	
	return jobID, nil
}

// GetJob retrieves a job by ID.
func (q *JobQueue) GetJob(jobID string) (*TranslationJob, error) {
	q.jobsMu.RLock()
	defer q.jobsMu.RUnlock()
	
	job, exists := q.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	
	return job, nil
}

// UpdateJobStatus updates the status of a job.
func (j *TranslationJob) UpdateStatus(status TranslationJobStatus, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	
	j.Status = status
	j.ProgressMessage = message
	
	now := time.Now()
	switch status {
	case JobStatusProcessing:
		if j.StartedAt == nil {
			j.StartedAt = &now
		}
	case JobStatusCompleted, JobStatusFailed:
		if j.CompletedAt == nil {
			j.CompletedAt = &now
		}
	}
}

// UpdateProgress updates the progress of a job.
func (j *TranslationJob) UpdateProgress(percent int32, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	
	j.ProgressPercent = percent
	j.ProgressMessage = message
}

// SetError sets the error message for a failed job.
func (j *TranslationJob) SetError(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	
	j.Error = err.Error()
	j.Status = JobStatusFailed
	now := time.Now()
	j.CompletedAt = &now
}

// SetResult sets the translation result for a completed job.
func (j *TranslationJob) SetResult(title, markdown string, tokens int64, inferenceTime float64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	
	j.TranslatedTitle = title
	j.TranslatedMarkdown = markdown
	j.TokensUsed = tokens
	j.InferenceTime = inferenceTime
	j.Status = JobStatusCompleted
	now := time.Now()
	j.CompletedAt = &now
	j.ProgressPercent = 100
}

// GetStatus returns a copy of the job status (thread-safe).
func (j *TranslationJob) GetStatus() (TranslationJobStatus, string, int32) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	
	return j.Status, j.ProgressMessage, j.ProgressPercent
}

// CleanupOldJobs removes jobs older than the specified duration.
func (q *JobQueue) CleanupOldJobs(maxAge time.Duration) {
	q.jobsMu.Lock()
	defer q.jobsMu.Unlock()
	
	now := time.Now()
	removed := 0
	
	for id, job := range q.jobs {
		// Only remove completed or failed jobs that are old
		if (job.Status == JobStatusCompleted || job.Status == JobStatusFailed) {
			if job.CompletedAt != nil && now.Sub(*job.CompletedAt) > maxAge {
				delete(q.jobs, id)
				removed++
			}
		}
	}
	
	if removed > 0 {
		q.logger.WithFields(logrus.Fields{
			"removed": removed,
			"remaining": len(q.jobs),
		}).Info("Cleaned up old translation jobs")
	}
}

