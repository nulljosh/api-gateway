# Configuration Guide

Detailed guide for configuring the API Gateway.

## Command-Line Flags

### Gateway Mode

```bash
./api-gateway -mode gateway [flags]
```

#### Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-port` | int | 8080 | Port to listen on |
| `-backends` | string | `http://localhost:8081,http://localhost:8082` | Comma-separated backend URLs |
| `-rate-limit` | int | 100 | Requests per minute per IP |
| `-key-rate-limit` | int | 1000 | Requests per minute per API key |
| `-mode` | string | `gateway` | `gateway`, `backend`, or `client` |

#### Examples:

```bash
# Default configuration
./api-gateway -mode gateway

# Custom port and backends
./api-gateway -mode gateway \
  -port 9000 \
  -backends http://service1.local:3000,http://service2.local:3000

# Strict rate limiting
./api-gateway -mode gateway \
  -rate-limit 50 \
  -key-rate-limit 500

# Permissive rate limiting
./api-gateway -mode gateway \
  -rate-limit 10000 \
  -key-rate-limit 100000

# Single backend
./api-gateway -mode gateway \
  -backends http://localhost:3000
```

### Backend Mode

```bash
./api-gateway -mode backend [flags]
```

#### Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-port` | int | 8080 | Port to listen on |
| `-name` | string | `Backend` | Backend identifier |
| `-mode` | string | `gateway` | `gateway`, `backend`, or `client` |

#### Examples:

```bash
# Start backend on port 8081
./api-gateway -mode backend -port 8081 -name "API-Service-1"

# Start backend on port 8082
./api-gateway -mode backend -port 8082 -name "API-Service-2"

# Start backend on port 8083
./api-gateway -mode backend -port 8083 -name "Legacy-Service"
```

### Client Mode

```bash
./api-gateway -mode client [flags]
```

#### Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-cmd` | string | `health` | Command to run |
| `-endpoint` | string | `http://localhost:8080` | Gateway endpoint |
| `-key` | string | `` | API key to use |
| `-count` | int | 1 | Number of requests to make |

#### Commands:

| Command | Purpose | Example |
|---------|---------|---------|
| `health` | Check gateway health | `-cmd health` |
| `echo` | Echo POST request | `-cmd echo -count 3` |
| `user` | Get user data | `-cmd user -count 5` |
| `data` | Get data (test load balancing) | `-cmd data` |
| `slow` | Test slow endpoint | `-cmd slow` |
| `auth` | Test authentication | `-cmd auth` |
| `rate-limit` | Test rate limiting | `-cmd rate-limit -count 150` |

#### Examples:

```bash
# Check health
./api-gateway -mode client -cmd health

# Test with 10 requests
./api-gateway -mode client -cmd user -count 10

# Test with API key
./api-gateway -mode client -cmd user -key key-admin

# Test against different gateway
./api-gateway -mode client -cmd health -endpoint http://api.local:8000

# Test rate limiting
./api-gateway -mode client -cmd rate-limit -count 200
```

## Configuration Recommendations

### Development

```bash
# Loose rate limits for testing
./api-gateway -mode gateway \
  -port 8080 \
  -backends http://localhost:8081,http://localhost:8082 \
  -rate-limit 10000 \
  -key-rate-limit 100000
```

### Testing

```bash
# Realistic rate limits for testing
./api-gateway -mode gateway \
  -port 8080 \
  -backends http://localhost:8081,http://localhost:8082 \
  -rate-limit 100 \
  -key-rate-limit 1000
```

### Production (Single Instance)

```bash
# Moderate rate limits
./api-gateway -mode gateway \
  -port 8080 \
  -backends http://api1.prod:8000,http://api2.prod:8000,http://api3.prod:8000 \
  -rate-limit 1000 \
  -key-rate-limit 10000
```

### Production (High Traffic)

```bash
# Stricter rate limits, more backends
./api-gateway -mode gateway \
  -port 8080 \
  -backends http://api1.prod:8000,http://api2.prod:8000,http://api3.prod:8000,http://api4.prod:8000 \
  -rate-limit 5000 \
  -key-rate-limit 50000
```

## API Key Configuration

### Current Implementation

API keys are hardcoded in the `runGateway()` function in `main.go`:

```go
apiKeys := make(map[string]bool)
apiKeys["key-test-1"] = true
apiKeys["key-test-2"] = true
apiKeys["key-admin"] = true
```

### To Add/Remove API Keys

1. Edit `main.go`
2. Find the `runGateway()` function
3. Add or remove entries from the `apiKeys` map

```go
// Add a new key
apiKeys["production-key-123"] = true

// Remove an old key
// (just delete the line)
```

4. Rebuild: `go build -o api-gateway .`

### Future Enhancement: Configuration File

To support external API key management:

```yaml
# config.yaml
gateway:
  port: 8080
  rate_limit: 100
  key_rate_limit: 1000
  
backends:
  - http://api1.local:8000
  - http://api2.local:8000
  
api_keys:
  - key-test-1
  - key-test-2
  - key-admin
  - production-key-xyz
```

Then load from file:
```go
config, err := LoadConfig("config.yaml")
```

## Rate Limit Configuration

### Understanding Token Buckets

Each IP/API key has a token bucket:

```
Capacity = Rate Limit (requests per minute)
Refill Rate = Capacity / 60 (tokens per second)

Example: 100 requests/minute
  Capacity = 100 tokens
  Refill Rate = 100/60 = 1.67 tokens/second
  
  If idle for 60 seconds: bucket refills to 100 tokens
  If idle for 30 seconds: bucket refills to ~50 tokens
```

### Rate Limit Recommendations

| Use Case | Per-IP | Per-Key |
|----------|--------|---------|
| Open API | 1000 | 10000 |
| Protected API | 100 | 1000 |
| Internal API | 10000 | 100000 |
| Strict API | 10 | 100 |
| Testing | 10000 | 100000 |

### Finding the Right Limits

1. **Measure baseline**: How many requests do normal users make?
2. **Add buffer**: Multiply by 1.5-2x for spikes
3. **Monitor logs**: Check for legitimate 429s
4. **Adjust gradually**: Increase/decrease by 50% at a time
5. **Set per-key limits**: Higher than per-IP (trusted clients)

## Backend Configuration

### Adding Backends

```bash
# Start new backend
./api-gateway -mode backend -port 8084 -name "Backend-4"

# Update gateway (restart required)
./api-gateway -mode gateway \
  -backends http://localhost:8081,http://localhost:8082,http://localhost:8083,http://localhost:8084
```

### Removing Backends

1. Let existing requests drain (up to 10 seconds)
2. Update `-backends` flag (restart gateway)
3. Gracefully stop the backend

### Backend URL Format

Backends must be valid HTTP URLs:

```
http://localhost:8080      ✓ Local HTTP
http://api.example.com     ✓ Domain name
http://192.168.1.1:8000    ✓ IP with port
https://secure.api.com     ✗ HTTPS (not yet supported)
localhost:8000             ✗ Missing scheme
http://api.example.com/api ✓ With path (path is kept)
```

## Health Check Configuration

Health checks are currently hardcoded:

- **Interval**: 10 seconds
- **Timeout**: 5 seconds
- **Endpoint**: `GET /health`
- **Success**: HTTP 200 OK

To customize, edit in `main.go`:

```go
// Change interval (currently 10 seconds)
HealthCheckInterval: 10 * time.Second

// In checkBackendHealth():
healthURL := fmt.Sprintf("%s/health", backend.URL.String())  // Customize endpoint
```

## Logging Configuration

### Log File

- **Location**: `./gateway.log` (relative to working directory)
- **Format**: JSON (one line per request)
- **Mode**: Append (adds to existing file)
- **Rotation**: Manual (not automated)

### Managing Log Files

```bash
# View recent logs
tail -n 100 gateway.log | jq .

# Archive old logs
mv gateway.log gateway.log.1
gzip gateway.log.1

# Clear logs (warning: deletes history)
> gateway.log

# Monitor live
tail -f gateway.log | jq .
```

### Future Enhancement: Log Rotation

Add automatic rotation:

```go
// Use lumberjack for rotating logs
import "github.com/natefinch/lumberjack.v2"

logFile := &lumberjack.Logger{
    Filename:   "gateway.log",
    MaxSize:    100,      // MB
    MaxBackups: 3,
    MaxAge:     28,       // days
}
```

## Environment-Specific Configurations

### Local Development

```bash
make gateway &
sleep 1
make backend1 &
make backend2 &
make test
```

### Docker-Friendly

```bash
./api-gateway \
  -mode gateway \
  -port ${PORT:-8080} \
  -backends ${BACKENDS:-http://localhost:8081,http://localhost:8082} \
  -rate-limit ${RATE_LIMIT:-100} \
  -key-rate-limit ${KEY_RATE_LIMIT:-1000}
```

### Docker Compose Example

```yaml
version: '3'
services:
  gateway:
    build: .
    ports:
      - "8080:8080"
    environment:
      - BACKENDS=http://backend1:8000,http://backend2:8000
      - RATE_LIMIT=100
    depends_on:
      - backend1
      - backend2

  backend1:
    build: .
    command: ./api-gateway -mode backend -port 8000 -name Backend-1
    expose:
      - "8000"

  backend2:
    build: .
    command: ./api-gateway -mode backend -port 8000 -name Backend-2
    expose:
      - "8000"
```

## Performance Tuning

### Linux System Tuning

```bash
# Increase file descriptor limit
ulimit -n 65536

# Tune TCP settings
sysctl -w net.ipv4.tcp_max_syn_backlog=5000
sysctl -w net.core.somaxconn=5000

# Allow more connections
sysctl -w net.ipv4.ip_local_port_range="1024 65535"
```

### Gateway Tuning

In `main.go`, adjust server timeouts:

```go
server := &http.Server{
    ReadTimeout:  15 * time.Second,   // Increase for slow clients
    WriteTimeout: 15 * time.Second,   // Increase for slow backends
    IdleTimeout:  60 * time.Second,   // Keep-alive timeout
}
```

## Monitoring Configuration

### Prometheus Integration (Future)

```go
// Add metrics handler
mux.HandleFunc("/metrics", promhttp.Handler())

// Track requests
duration := prometheus.NewHistogramVec(...)
status := prometheus.NewCounterVec(...)
```

### CloudWatch Integration (Future)

```go
import "github.com/aws/aws-cloudwatch-logs-for-go/..."

// Send logs to CloudWatch
cwl := logs.NewLogsClient()
```

## Security Configuration

### Current

- API key whitelist validation
- Rate limiting per IP and key

### Recommended Additions

```go
// IP whitelisting
var whitelist = []string{
    "10.0.0.0/8",      // Private network
    "203.0.113.0/24",  // Known clients
}

// IP blacklisting
var blacklist = []string{
    "192.0.2.1",       // Troublemaker
}

// Custom middleware
func ipFilterMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := getClientIP(r)
        if isBlacklisted(ip) {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Troubleshooting Configuration Issues

### Issue: "Address already in use"

```bash
# Find what's using the port
lsof -i :8080

# Kill the process
kill -9 <PID>

# Or use a different port
./api-gateway -mode gateway -port 8090
```

### Issue: Backends not responding

```bash
# Verify backend is running
curl http://localhost:8081/health

# Check firewall
sudo firewall-cmd --add-port=8081/tcp

# Check DNS (if using domain names)
nslookup api.example.com
```

### Issue: Rate limiting too aggressive

```bash
# Check current limits
# From logs, count 429 responses
jq 'select(.status_code == 429) | .client_ip' gateway.log | sort | uniq -c

# Increase limits
./api-gateway -mode gateway -rate-limit 500 -key-rate-limit 5000
```

---

For examples, see `EXAMPLES.md`
For implementation details, see `ARCHITECTURE.md`
