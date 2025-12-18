# Iskoces Metrics

Iskoces exposes comprehensive Prometheus metrics for monitoring the translation service, worker pool, and job processing.

## Metrics Endpoint

All metrics are exposed at:
```
http://iskoces-service.iskoces.svc:5000/metrics
```

## Worker Pool Metrics

### Worker Status

- **`iskoces_worker_pool_total_workers`** (Gauge)
  - Total number of workers (active + idle) in the pool
  - Labels: `engine` (libretranslate/argos)

- **`iskoces_worker_pool_active_workers`** (Gauge)
  - Number of active (running) workers
  - Labels: `engine`

- **`iskoces_worker_pool_busy_workers`** (Gauge)
  - Number of workers currently processing requests
  - Labels: `engine`

- **`iskoces_worker_pool_idle_workers`** (Gauge)
  - Number of idle workers available for requests
  - Labels: `engine`

### Worker Lifecycle

- **`iskoces_worker_starts_total`** (Counter)
  - Total number of worker process starts
  - Labels: `engine`, `worker_id`

- **`iskoces_worker_restarts_total`** (Counter)
  - Total number of worker process restarts (indicates failures)
  - Labels: `engine`, `worker_id`

- **`iskoces_worker_uptime_seconds`** (Gauge)
  - Uptime of each worker in seconds
  - Labels: `engine`, `worker_id`

- **`iskoces_worker_memory_usage_bytes`** (Gauge)
  - Memory usage (RSS) of worker processes in bytes
  - Labels: `engine`, `worker_id`
  - Note: Only available on Linux (reads from `/proc/[pid]/status`)

## Translation Request Metrics

### Request Volume

- **`iskoces_translation_requests_total`** (Counter)
  - Total number of translation requests
  - Labels: `engine`, `status` (success/error)

### Request Performance

- **`iskoces_translation_request_duration_seconds`** (Histogram)
  - Duration of translation requests in seconds
  - Buckets: 0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0
  - Labels: `engine`, `status`

- **`iskoces_translation_request_size_bytes`** (Histogram)
  - Size of translation request text in bytes
  - Buckets: 100, 500, 1KB, 5KB, 10KB, 50KB, 100KB, 500KB
  - Labels: `engine`

- **`iskoces_translation_response_size_bytes`** (Histogram)
  - Size of translation response text in bytes
  - Buckets: 100, 500, 1KB, 5KB, 10KB, 50KB, 100KB, 500KB
  - Labels: `engine`

## Queue Metrics

- **`iskoces_worker_queue_length`** (Gauge)
  - Current length of the worker request queue
  - Labels: `engine`
  - High values indicate worker saturation

- **`iskoces_worker_queue_wait_seconds`** (Histogram)
  - Time spent waiting for an available worker
  - Buckets: 0.001, 0.01, 0.1, 0.5, 1.0, 2.0, 5.0
  - Labels: `engine`
  - High values indicate need for more workers

## Socket Communication Metrics

- **`iskoces_socket_connections_total`** (Counter)
  - Total number of Unix socket connections to workers
  - Labels: `engine`, `worker_id`, `status` (success/error)

- **`iskoces_socket_connection_duration_seconds`** (Histogram)
  - Duration of socket connections in seconds
  - Buckets: 0.01, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0
  - Labels: `engine`, `worker_id`

## Example Queries

### Worker Pool Health

```promql
# Worker utilization
iskoces_worker_pool_busy_workers / iskoces_worker_pool_total_workers

# Worker restart rate (indicates instability)
rate(iskoces_worker_restarts_total[5m])
```

### Translation Performance

```promql
# Request success rate
rate(iskoces_translation_requests_total{status="success"}[5m]) / 
rate(iskoces_translation_requests_total[5m])

# P95 request duration
histogram_quantile(0.95, 
  rate(iskoces_translation_request_duration_seconds_bucket[5m])
)

# Average request size
rate(iskoces_translation_request_size_bytes_sum[5m]) / 
rate(iskoces_translation_request_size_bytes_count[5m])
```

### Queue Saturation

```promql
# Queue length (should be low)
iskoces_worker_queue_length

# Average wait time
histogram_quantile(0.95,
  rate(iskoces_worker_queue_wait_seconds_bucket[5m])
)
```

### Memory Usage

```promql
# Total worker memory usage
sum(iskoces_worker_memory_usage_bytes) by (engine)

# Average memory per worker
avg(iskoces_worker_memory_usage_bytes) by (engine)
```

## Grafana Dashboard

Recommended panels:

1. **Worker Pool Status**
   - Active workers (gauge)
   - Busy vs Idle workers (stacked area)
   - Worker restart rate (line)

2. **Translation Performance**
   - Request rate (line)
   - Success rate (gauge)
   - P50/P95/P99 latency (line)
   - Request size distribution (histogram)

3. **Queue Health**
   - Queue length (gauge)
   - Wait time percentiles (line)

4. **Resource Usage**
   - Memory usage per worker (line)
   - Total memory usage (gauge)

## Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: iskoces
    rules:
      - alert: HighWorkerRestartRate
        expr: rate(iskoces_worker_restarts_total[5m]) > 0.1
        annotations:
          summary: "Worker pool is unstable"

      - alert: HighQueueWaitTime
        expr: histogram_quantile(0.95, rate(iskoces_worker_queue_wait_seconds_bucket[5m])) > 2
        annotations:
          summary: "Workers are saturated"

      - alert: LowSuccessRate
        expr: |
          rate(iskoces_translation_requests_total{status="success"}[5m]) / 
          rate(iskoces_translation_requests_total[5m]) < 0.95
        annotations:
          summary: "Translation success rate is low"

      - alert: HighMemoryUsage
        expr: sum(iskoces_worker_memory_usage_bytes) > 2e9  # 2GB
        annotations:
          summary: "Worker pool memory usage is high"
```

