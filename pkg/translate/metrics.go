package translate

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Worker pool metrics
	workerPoolActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_pool_active_workers",
			Help: "Number of active translation workers in the pool",
		},
		[]string{"engine"},
	)

	workerPoolTotalWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_pool_total_workers",
			Help: "Total number of workers (active + idle) in the pool",
		},
		[]string{"engine"},
	)

	workerPoolBusyWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_pool_busy_workers",
			Help: "Number of workers currently processing requests",
		},
		[]string{"engine"},
	)

	workerPoolIdleWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_pool_idle_workers",
			Help: "Number of idle workers available for requests",
		},
		[]string{"engine"},
	)

	// Translation request metrics
	translationRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iskoces_translation_requests_total",
			Help: "Total number of translation requests",
		},
		[]string{"engine", "status"},
	)

	translationRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iskoces_translation_request_duration_seconds",
			Help:    "Duration of translation requests in seconds",
			Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0},
		},
		[]string{"engine", "status"},
	)

	translationRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iskoces_translation_request_size_bytes",
			Help:    "Size of translation request text in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000},
		},
		[]string{"engine"},
	)

	translationResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iskoces_translation_response_size_bytes",
			Help:    "Size of translation response text in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000},
		},
		[]string{"engine"},
	)

	// Worker lifecycle metrics
	workerStartsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iskoces_worker_starts_total",
			Help: "Total number of worker process starts",
		},
		[]string{"engine", "worker_id"},
	)

	workerRestartsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iskoces_worker_restarts_total",
			Help: "Total number of worker process restarts",
		},
		[]string{"engine", "worker_id"},
	)

	workerUptime = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_uptime_seconds",
			Help: "Uptime of each worker in seconds",
		},
		[]string{"engine", "worker_id"},
	)

	// Queue metrics
	workerQueueLength = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_queue_length",
			Help: "Current length of the worker request queue",
		},
		[]string{"engine"},
	)

	workerQueueWaitTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iskoces_worker_queue_wait_seconds",
			Help:    "Time spent waiting for an available worker",
			Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.0, 5.0},
		},
		[]string{"engine"},
	)

	// Socket communication metrics
	socketConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iskoces_socket_connections_total",
			Help: "Total number of Unix socket connections to workers",
		},
		[]string{"engine", "worker_id", "status"},
	)

	socketConnectionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iskoces_socket_connection_duration_seconds",
			Help:    "Duration of socket connections in seconds",
			Buckets: []float64{0.01, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
		[]string{"engine", "worker_id"},
	)

	// Memory metrics (if available)
	workerMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "iskoces_worker_memory_usage_bytes",
			Help: "Memory usage of worker processes in bytes",
		},
		[]string{"engine", "worker_id"},
	)
)

// MetricsCollector collects and updates metrics for the worker pool.
type MetricsCollector struct {
	pool   *WorkerPool
	engine string
	mu     sync.RWMutex
}

// NewMetricsCollector creates a new metrics collector for a worker pool.
func NewMetricsCollector(pool *WorkerPool, engine string) *MetricsCollector {
	return &MetricsCollector{
		pool:   pool,
		engine: engine,
	}
}

// UpdateMetrics updates all worker pool metrics.
func (mc *MetricsCollector) UpdateMetrics() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Get pool stats
	mc.pool.workerMu.RLock()
	totalWorkers := len(mc.pool.workers)
	busyWorkers := 0
	idleWorkers := 0
	activeWorkers := 0

	workerUptimes := make(map[int]float64)
	workerStartTimes := make(map[int]time.Time)

	for _, worker := range mc.pool.workers {
		worker.mu.Lock()
		if worker.busy {
			busyWorkers++
		} else {
			idleWorkers++
		}

		// Check if process is running
		if worker.process != nil && worker.process.ProcessState == nil {
			activeWorkers++
			// Calculate uptime
			if !worker.lastUsed.IsZero() {
				uptime := time.Since(worker.lastUsed).Seconds()
				workerUptimes[worker.id] = uptime
				workerStartTimes[worker.id] = worker.lastUsed
			}
		}
		worker.mu.Unlock()
	}
	mc.pool.workerMu.RUnlock()

	// Update metrics
	workerPoolTotalWorkers.WithLabelValues(mc.engine).Set(float64(totalWorkers))
	workerPoolActiveWorkers.WithLabelValues(mc.engine).Set(float64(activeWorkers))
	workerPoolBusyWorkers.WithLabelValues(mc.engine).Set(float64(busyWorkers))
	workerPoolIdleWorkers.WithLabelValues(mc.engine).Set(float64(idleWorkers))
	workerQueueLength.WithLabelValues(mc.engine).Set(float64(len(mc.pool.requestQueue)))

	// Update worker uptimes
	for workerID, uptime := range workerUptimes {
		workerUptime.WithLabelValues(mc.engine, fmt.Sprintf("%d", workerID)).Set(uptime)
	}
}

// RecordTranslationRequest records metrics for a translation request.
func (mc *MetricsCollector) RecordTranslationRequest(duration time.Duration, success bool, requestSize, responseSize int) {
	status := "success"
	if !success {
		status = "error"
	}

	translationRequestsTotal.WithLabelValues(mc.engine, status).Inc()
	translationRequestDuration.WithLabelValues(mc.engine, status).Observe(duration.Seconds())
	translationRequestSize.WithLabelValues(mc.engine).Observe(float64(requestSize))
	translationResponseSize.WithLabelValues(mc.engine).Observe(float64(responseSize))
}

// RecordWorkerStart records a worker start event.
func (mc *MetricsCollector) RecordWorkerStart(workerID int) {
	workerStartsTotal.WithLabelValues(mc.engine, fmt.Sprintf("%d", workerID)).Inc()
}

// RecordWorkerRestart records a worker restart event.
func (mc *MetricsCollector) RecordWorkerRestart(workerID int) {
	workerRestartsTotal.WithLabelValues(mc.engine, fmt.Sprintf("%d", workerID)).Inc()
}

// RecordQueueWait records time spent waiting for an available worker.
func (mc *MetricsCollector) RecordQueueWait(duration time.Duration) {
	workerQueueWaitTime.WithLabelValues(mc.engine).Observe(duration.Seconds())
}

// RecordSocketConnection records socket connection metrics.
func (mc *MetricsCollector) RecordSocketConnection(workerID int, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	socketConnectionsTotal.WithLabelValues(mc.engine, fmt.Sprintf("%d", workerID), status).Inc()
	socketConnectionDuration.WithLabelValues(mc.engine, fmt.Sprintf("%d", workerID)).Observe(duration.Seconds())
}

// UpdateWorkerMemory updates memory usage for a worker (if available).
func (mc *MetricsCollector) UpdateWorkerMemory(workerID int, memoryBytes int64) {
	workerMemoryUsage.WithLabelValues(mc.engine, fmt.Sprintf("%d", workerID)).Set(float64(memoryBytes))
}

