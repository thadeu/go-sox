# Production Deployment Guide

## System Resource Limits

When running go-sox in production with high concurrent load (100+ parallel conversions), you must increase system resource limits.

### Understanding Resource Usage

Each `StreamConverter` creates:
- 1 SoX process
- 3 file descriptors (stdin, stdout, stderr)

For 100 parallel conversions:
- 100 processes
- 300 file descriptors

### Linux: Increase ulimit Persistently

#### 1. System-Wide Limits

Edit `/etc/security/limits.conf`:

```bash
sudo nano /etc/security/limits.conf
```

Add these lines (replace `username` with your app user):

```
# File descriptors
username soft nofile 65536
username hard nofile 65536

# Processes
username soft nproc 4096
username hard nproc 4096

# For systemd services, use '*' instead of username
* soft nofile 65536
* hard nofile 65536
* soft nproc 4096
* hard nproc 4096
```

#### 2. Systemd Service Limits

If running as systemd service, add to your `.service` file:

```ini
[Service]
LimitNOFILE=65536
LimitNPROC=4096
```

Example `/etc/systemd/system/myapp.service`:

```ini
[Unit]
Description=My SIP Application
After=network.target

[Service]
Type=simple
User=myapp
WorkingDirectory=/opt/myapp
ExecStart=/opt/myapp/bin/server
Restart=always

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

# Environment
Environment="SOX_MAX_WORKERS=500"

[Install]
WantedBy=multi-user.target
```

Reload and restart:

```bash
sudo systemctl daemon-reload
sudo systemctl restart myapp
```

#### 3. PAM Limits (for SSH sessions)

Edit `/etc/pam.d/common-session`:

```bash
sudo nano /etc/pam.d/common-session
```

Add:

```
session required pam_limits.so
```

#### 4. Kernel Limits

Edit `/etc/sysctl.conf`:

```bash
sudo nano /etc/sysctl.conf
```

Add:

```
# Maximum number of open files
fs.file-max = 2097152

# Maximum number of processes
kernel.pid_max = 4194304
```

Apply changes:

```bash
sudo sysctl -p
```

#### 5. Verify Limits

After reboot or re-login:

```bash
# Check file descriptor limits
ulimit -n

# Check process limits
ulimit -u

# Check all limits
ulimit -a

# For running process
cat /proc/<PID>/limits
```

### Docker Deployments

Add to `docker-compose.yml`:

```yaml
version: '3.8'
services:
  app:
    image: myapp:latest
    ulimits:
      nofile:
        soft: 65536
        hard: 65536
      nproc:
        soft: 4096
        hard: 4096
    environment:
      - SOX_MAX_WORKERS=500
```

Or for `docker run`:

```bash
docker run \
  --ulimit nofile=65536:65536 \
  --ulimit nproc=4096:4096 \
  -e SOX_MAX_WORKERS=500 \
  myapp:latest
```

### Kubernetes Deployments

Add to pod spec:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
spec:
  containers:
  - name: app
    image: myapp:latest
    env:
    - name: SOX_MAX_WORKERS
      value: "500"
    resources:
      limits:
        cpu: "4"
        memory: "8Gi"
      requests:
        cpu: "2"
        memory: "4Gi"
    securityContext:
      capabilities:
        add:
        - SYS_RESOURCE
```

## Configuration

### Environment Variables

```bash
# Maximum concurrent SoX conversions (default: 500)
export SOX_MAX_WORKERS=500
```

### Worker Pool Configuration

```go
// Use environment variable
pool := sox.NewPool() // reads SOX_MAX_WORKERS

// Or explicit limit
pool := sox.NewPoolWithLimit(500)
```

### Recommended Settings by Load

| Concurrent Calls | SOX_MAX_WORKERS | File Descriptors | Processes |
|-----------------|-----------------|------------------|-----------|
| 50              | 100             | 4096            | 2048      |
| 100             | 200             | 8192            | 4096      |
| 200             | 400             | 16384           | 8192      |
| 500             | 1000            | 32768           | 16384     |

## Monitoring

### Check Active Resources

```go
monitor := sox.GetMonitor()
stats := monitor.GetStats()

fmt.Printf("Active processes: %d\n", stats.ActiveProcesses)
fmt.Printf("Total conversions: %d\n", stats.TotalConversions)
fmt.Printf("Failed conversions: %d\n", stats.FailedConversions)
fmt.Printf("Success rate: %.2f%%\n", stats.SuccessRate)
fmt.Printf("Oldest process age: %v\n", stats.OldestProcessAge)
```

### Prometheus Metrics Example

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    activeProcesses = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "sox_active_processes",
        Help: "Number of active SoX processes",
    })
    
    totalConversions = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "sox_conversions_total",
        Help: "Total number of conversions",
    })
    
    failedConversions = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "sox_conversions_failed_total",
        Help: "Total number of failed conversions",
    })
)

func updateMetrics() {
    stats := sox.GetMonitor().GetStats()
    activeProcesses.Set(float64(stats.ActiveProcesses))
    totalConversions.Add(float64(stats.TotalConversions))
    failedConversions.Add(float64(stats.FailedConversions))
}
```

## Circuit Breaker Configuration

For production resiliency:

```go
// Conservative settings for production
circuitBreaker := sox.NewCircuitBreakerWithConfig(
    10,                  // maxFailures before opening
    60 * time.Second,    // resetTimeout
    5,                   // halfOpenRequests
)

retryConfig := sox.RetryConfig{
    MaxAttempts:     3,
    InitialBackoff:  200 * time.Millisecond,
    MaxBackoff:      10 * time.Second,
    BackoffMultiple: 2.0,
}

converter := sox.NewResilientConverter(input, output).
    WithCircuitBreaker(circuitBreaker).
    WithRetryConfig(retryConfig).
    WithPool(pool)
```

## Troubleshooting

### "Too many open files" Error

```bash
# Check current limits
ulimit -n

# Check system-wide
cat /proc/sys/fs/file-max

# Increase and verify
sudo sysctl -w fs.file-max=2097152
sudo sysctl -p
```

### "Cannot fork: Resource temporarily unavailable"

```bash
# Check process limits
ulimit -u

# Check running processes
ps -eLf | wc -l

# Increase limits (see sections above)
```

### Memory Issues

Each SoX process uses ~2-5MB RAM. For 500 concurrent:
- Expected RAM: 1-2.5GB
- Recommended: 4GB+ available

### Orphaned Processes

Monitor for zombie processes:

```bash
# Check for zombie SoX processes
ps aux | grep 'sox' | grep '<defunct>'

# Kill orphaned processes
pkill -9 sox
```

In code, always use defer:

```go
stream := sox.NewStreamConverter(input, output)
defer stream.Close() // Always cleanup

stream.Start()
// ... use stream
```

## Testing Under Load

```bash
# Run stress tests
go test -v -run TestPooledConversions

# With custom concurrency
SOX_MAX_WORKERS=100 go test -v -run TestParallelConversions

# Benchmark parallel performance
go test -bench=BenchmarkPooled -benchtime=30s
```

## Health Checks

Implement health endpoint:

```go
func healthCheck(w http.ResponseWriter, r *http.Request) {
    stats := sox.GetMonitor().GetStats()
    
    status := "healthy"
    if stats.ActiveProcesses > pool.MaxWorkers() * 0.9 {
        status = "degraded"
    }
    if stats.SuccessRate < 95.0 {
        status = "unhealthy"
    }
    
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": status,
        "active_processes": stats.ActiveProcesses,
        "success_rate": stats.SuccessRate,
    })
}
```

