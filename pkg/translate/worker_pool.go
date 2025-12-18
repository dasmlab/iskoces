package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// WorkerPool manages a pool of Python translation workers using Unix domain sockets.
// This provides fast, local communication without HTTP overhead.
type WorkerPool struct {
	engine        EngineType
	pythonPath    string
	scriptPath    string
	workers       []*TranslationWorker
	workerMu      sync.RWMutex
	maxWorkers    int
	socketDir     string
	logger        *logrus.Logger
	metrics       *MetricsCollector
	requestQueue  chan *TranslationRequest
	workerReady   chan *TranslationWorker
	shutdown      chan struct{}
	wg            sync.WaitGroup
}

// TranslationWorker represents a single Python subprocess worker.
type TranslationWorker struct {
	id           int
	process      *exec.Cmd
	socketPath   string
	listener     net.Listener
	conn         net.Conn
	mu           sync.Mutex
	busy         bool
	lastUsed     time.Time
	logger       *logrus.Entry // Use Entry for structured logging with fields
	pool         *WorkerPool
}

// TranslationRequest represents a translation request sent to a worker.
type TranslationRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// TranslationResponse represents a response from a worker.
type TranslationResponse struct {
	Success        bool   `json:"success"`
	TranslatedText string `json:"translated_text,omitempty"`
	Error          string `json:"error,omitempty"`
}

// NewWorkerPool creates a new worker pool for Python translation workers.
func NewWorkerPool(engine EngineType, maxWorkers int, logger *logrus.Logger) (*WorkerPool, error) {
	if logger == nil {
		logger = logrus.New()
	}

	// Use /tmp for socket directory (works in Kubernetes)
	socketDir := "/tmp/iskoces-workers"
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket directory: %w", err)
	}

	pool := &WorkerPool{
		engine:       engine,
		pythonPath:   "python3",
		scriptPath:   "/app/scripts/translate_worker.py",
		maxWorkers:   maxWorkers,
		socketDir:    socketDir,
		logger:       logger,
		metrics:      NewMetricsCollector(nil, string(engine)), // Will be set after pool creation
		requestQueue: make(chan *TranslationRequest, 100), // Buffered queue
		workerReady: make(chan *TranslationWorker, maxWorkers),
		shutdown:     make(chan struct{}),
	}

	// Set metrics pool reference
	pool.metrics = NewMetricsCollector(pool, string(engine))

	// Start worker manager
	pool.wg.Add(1)
	go pool.manageWorkers()

	// Start metrics updater
	pool.wg.Add(1)
	go pool.updateMetricsLoop()

	// Pre-start workers
	for i := 0; i < maxWorkers; i++ {
		if err := pool.startWorker(i); err != nil {
			logger.WithError(err).Warn("Failed to start initial worker, will retry")
		}
	}

	return pool, nil
}

// manageWorkers manages the worker pool lifecycle.
func (p *WorkerPool) manageWorkers() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.shutdown:
			return
		case <-ticker.C:
			// Health check and restart dead workers
			p.healthCheckWorkers()
		}
	}
}

// updateMetricsLoop periodically updates metrics.
func (p *WorkerPool) updateMetricsLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(5 * time.Second) // Update every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-p.shutdown:
			return
		case <-ticker.C:
			p.metrics.UpdateMetrics()
			// Try to update worker memory if possible
			p.updateWorkerMemory()
		}
	}
}

// updateWorkerMemory attempts to get memory usage for each worker from /proc.
func (p *WorkerPool) updateWorkerMemory() {
	p.workerMu.RLock()
	defer p.workerMu.RUnlock()

	for _, worker := range p.workers {
		if worker.process != nil && worker.process.Process != nil {
			pid := worker.process.Process.Pid
			memoryBytes := p.getProcessMemory(pid)
			if memoryBytes > 0 {
				p.metrics.UpdateWorkerMemory(worker.id, memoryBytes)
			}
		}
	}
}

// getProcessMemory reads memory usage from /proc/[pid]/status (Linux).
// Returns memory in bytes, or 0 if unavailable.
func (p *WorkerPool) getProcessMemory(pid int) int64 {
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return 0 // Not available (e.g., not on Linux or process died)
	}

	// Parse VmRSS (Resident Set Size) from status file
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// Value is in KB, convert to bytes
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return kb * 1024 // Convert KB to bytes
				}
			}
		}
	}

	return 0
}

// startWorker starts a new Python worker subprocess.
func (p *WorkerPool) startWorker(id int) error {
	p.workerMu.Lock()
	defer p.workerMu.Unlock()

	socketPath := filepath.Join(p.socketDir, fmt.Sprintf("worker-%d.sock", id))

	// Remove old socket if it exists
	os.Remove(socketPath)

	// Start Python worker with Unix socket server
	// The Python script will listen on the socket
	cmd := exec.Command(p.pythonPath, p.scriptPath, "--socket", socketPath)
	cmd.Stderr = os.Stderr // Log errors to stderr

	workerLogger := p.logger.WithField("worker_id", id)
	worker := &TranslationWorker{
		id:         id,
		process:    cmd,
		socketPath: socketPath,
		logger:     workerLogger,
		pool:       p,
		lastUsed:   time.Now(),
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start worker %d: %w", id, err)
	}

	// Wait a moment for socket to be created
	time.Sleep(100 * time.Millisecond)

	// Verify socket exists
	if _, err := os.Stat(socketPath); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("worker %d socket not created: %w", id, err)
	}

	p.workers = append(p.workers, worker)
	p.workerReady <- worker

	worker.logger.Info("Worker started")
	p.metrics.RecordWorkerStart(id)

	// Monitor worker process
	go worker.monitor()

	return nil
}

// monitor monitors the worker process and restarts it if it dies.
func (w *TranslationWorker) monitor() {
	err := w.process.Wait()
	w.logger.WithError(err).Warn("Worker process exited")

	// Mark as dead
	w.mu.Lock()
	w.busy = false
	w.conn = nil
	w.mu.Unlock()

	// Record restart
	w.pool.metrics.RecordWorkerRestart(w.id)

	// Restart worker
	time.Sleep(1 * time.Second)
	if err := w.pool.startWorker(w.id); err != nil {
		w.logger.WithError(err).Error("Failed to restart worker")
	}
}

// healthCheckWorkers checks worker health and restarts dead ones.
func (p *WorkerPool) healthCheckWorkers() {
	p.workerMu.RLock()
	workers := make([]*TranslationWorker, len(p.workers))
	copy(workers, p.workers)
	p.workerMu.RUnlock()

	for _, worker := range workers {
		worker.mu.Lock()
		processState := worker.process.ProcessState
		worker.mu.Unlock()

		if processState != nil && processState.Exited() {
			p.logger.WithField("worker_id", worker.id).Warn("Worker is dead, restarting")
			// Remove from pool
			p.workerMu.Lock()
			for i, w := range p.workers {
				if w.id == worker.id {
					p.workers = append(p.workers[:i], p.workers[i+1:]...)
					break
				}
			}
			p.workerMu.Unlock()
			// Restart
			p.startWorker(worker.id)
		}
	}
}

// Translate translates text using an available worker from the pool.
func (p *WorkerPool) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	startTime := time.Now()
	requestSize := len(text)

	// Get available worker (with metrics)
	waitStart := time.Now()
	var worker *TranslationWorker
	select {
	case worker = <-p.workerReady:
		// Got a worker
		p.metrics.RecordQueueWait(time.Since(waitStart))
	case <-ctx.Done():
		p.metrics.RecordTranslationRequest(time.Since(startTime), false, requestSize, 0)
		return "", ctx.Err()
	case <-time.After(10 * time.Second):
		p.metrics.RecordTranslationRequest(time.Since(startTime), false, requestSize, 0)
		return "", fmt.Errorf("timeout waiting for available worker")
	}

	// Mark worker as busy
	worker.mu.Lock()
	worker.busy = true
	worker.lastUsed = time.Now()
	worker.mu.Unlock()

	// Return worker when done
	defer func() {
		worker.mu.Lock()
		worker.busy = false
		worker.mu.Unlock()
		p.workerReady <- worker
	}()

	// Connect to worker socket (with metrics)
	socketStart := time.Now()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: worker.socketPath, Net: "unix"})
	socketDuration := time.Since(socketStart)
	if err != nil {
		p.metrics.RecordSocketConnection(worker.id, socketDuration, false)
		p.metrics.RecordTranslationRequest(time.Since(startTime), false, requestSize, 0)
		return "", fmt.Errorf("failed to connect to worker socket: %w", err)
	}
	defer conn.Close()
	p.metrics.RecordSocketConnection(worker.id, socketDuration, true)

	// Set timeout
	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	// Send request
	req := &TranslationRequest{
		Text:       text,
		SourceLang: sourceLang,
		TargetLang: targetLang,
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		p.metrics.RecordTranslationRequest(time.Since(startTime), false, requestSize, 0)
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(conn)
	var resp TranslationResponse
	if err := decoder.Decode(&resp); err != nil {
		if err == io.EOF {
			p.metrics.RecordTranslationRequest(time.Since(startTime), false, requestSize, 0)
			return "", fmt.Errorf("worker connection closed")
		}
		p.metrics.RecordTranslationRequest(time.Since(startTime), false, requestSize, 0)
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	responseSize := len(resp.TranslatedText)
	success := resp.Success
	p.metrics.RecordTranslationRequest(time.Since(startTime), success, requestSize, responseSize)

	if !success {
		return "", fmt.Errorf("translation failed: %s", resp.Error)
	}

	return resp.TranslatedText, nil
}

// CheckHealth verifies the worker pool is healthy.
func (p *WorkerPool) CheckHealth(ctx context.Context) error {
	// Try a simple translation
	_, err := p.Translate(ctx, "test", "en", "fr")
	return err
}

// SupportedLanguages returns supported language codes.
func (p *WorkerPool) SupportedLanguages(ctx context.Context) ([]string, error) {
	// Common languages supported by Argos/LibreTranslate
	return []string{
		"en", "es", "fr", "de", "it", "pt", "ru", "zh", "ja", "ko",
		"ar", "hi", "tr", "pl", "nl", "sv", "da", "fi", "no", "cs",
		"ro", "hu", "bg", "hr", "sk", "sl", "et", "lv", "lt", "el",
	}, nil
}

// Close shuts down the worker pool.
func (p *WorkerPool) Close() error {
	close(p.shutdown)

	p.workerMu.Lock()
	for _, worker := range p.workers {
		if worker.process != nil {
			worker.process.Process.Kill()
		}
		os.Remove(worker.socketPath)
	}
	p.workerMu.Unlock()

	p.wg.Wait()
	return nil
}

