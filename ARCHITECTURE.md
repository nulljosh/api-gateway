# API Gateway Architecture

## Overview

The API Gateway is a request routing and filtering layer that sits between clients and backend services. It provides:

1. **Request Routing** - Distribute traffic across multiple backends
2. **Load Balancing** - Round-robin distribution algorithm
3. **Authentication** - API key validation
4. **Rate Limiting** - Per-IP and per-key token bucket limiting
5. **Health Checks** - Monitor backend availability
6. **Logging** - Track all requests and responses

## Components

### 1. Main Gateway Server

**File**: `main.go`

**Responsibilities**:
- Accept client requests on port 8080
- Route requests to healthy backends
- Apply authentication and rate limiting
- Log all requests
- Manage backend health

**Key Functions**:
- `NewGateway()` - Initialize gateway with config
- `Start()` - Start HTTP server and health checks
- `handleRequest()` - Main request handler (middleware pipeline)
- `handleHealth()` - Health status endpoint

### 2. Load Balancer

**Type**: `LoadBalancer` in `main.go`

**Algorithm**: Round-robin with health-aware selection

**Key Methods**:
- `Next()` - Get next healthy backend
- Maintains current index for round-robin
- Skips unhealthy backends

**Concurrency**: Protected by `sync.Mutex` for thread-safe index updates

### 3. Rate Limiter

**Type**: `RateLimiter` in `main.go`

**Algorithm**: Token bucket per IP and per API key

**Key Methods**:
- `Allow()` - Check if request is allowed
- `allowIP()` - Check per-IP limit
- `allowKey()` - Check per-key limit
- `refill()` - Add tokens based on elapsed time

**Token Bucket Implementation**:
```go
type TokenBucket struct {
    tokens     float64    // Current tokens
    capacity   float64    // Max tokens (rate limit)
    refillRate float64    // Tokens added per second
    lastRefill time.Time  // Last time tokens were added
}
```

**Concurrency**: Protected by `sync.RWMutex`

### 4. Request Logger

**Type**: `RequestLogger` in `main.go`

**Format**: JSON lines (one JSON object per line)

**Logged Fields**:
- `timestamp` - Request timestamp (ISO 8601)
- `method` - HTTP method (GET, POST, etc.)
- `path` - Request path
- `client_ip` - Client IP address
- `api_key` - API key (if provided)
- `status_code` - HTTP status code returned
- `response_time_ms` - Time taken (milliseconds)
- `backend` - Which backend processed request
- `error` - Error message (if any)

**File**: `gateway.log` (appended to)

### 5. Mock Backends

**File**: `mock_backend.go`

**Endpoints Provided**:
- `GET /health` - Health status
- `GET /api/user?id=<id>` - User data
- `POST /api/echo` - Echo request body
- `GET /api/data` - Sample data array
- `GET /api/slow` - Simulated slow endpoint (500ms)

**Purpose**: Testing and demonstration

### 6. Test Client

**File**: `client.go`

**Commands**:
- `health` - Check gateway health
- `echo` - Test echo endpoint
- `user` - Get user data
- `data` - Get data (test load balancing)
- `slow` - Test slow endpoint
- `auth` - Test authentication
- `rate-limit` - Test rate limiting

## Request Flow

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Client Request Arrives                                   │
│    GET /api/user?id=123                                     │
│    Headers: X-API-Key: key-admin                            │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Parse Request                                            │
│    - Extract client IP                                      │
│    - Extract API key (if provided)                          │
│    - Create log entry                                       │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Authentication Middleware                                │
│    - Check if X-API-Key header is present                   │
│    - If present, validate against APIKeys map              │
│    - Return 401 if invalid                                 │
│    - Continue if valid or no key required                  │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Rate Limiting                                            │
│    - Check per-IP token bucket                             │
│    - Check per-key token bucket (if key provided)          │
│    - Return 429 if rate limit exceeded                     │
│    - Decrement token if allowed                            │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Load Balancing                                           │
│    - Get next healthy backend using round-robin            │
│    - Check backend.Alive flag                              │
│    - Skip unhealthy backends                               │
│    - Return 503 if no healthy backends                     │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Proxy Request                                            │
│    - Forward request to selected backend                   │
│    - Backend processes request                             │
│    - Capture response status code                          │
│    - Measure response time                                 │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. Log Request                                              │
│    - Write JSON log entry with all metadata                │
│    - Include status code, backend, response time           │
│    - Append to gateway.log                                 │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 8. Return Response to Client                                │
│    - HTTP status code                                       │
│    - Response body from backend                             │
│    - Original headers                                       │
└─────────────────────────────────────────────────────────────┘
```

## Concurrency Model

### Thread Safety

1. **LoadBalancer**: `sync.Mutex` protects `current` index
   - Used by round-robin algorithm
   - Low contention (one mutex for all requests)

2. **RateLimiter**: `sync.RWMutex` protects token buckets
   - Multiple readers for checking limits
   - Lock upgrade for creating new buckets
   - Per-IP and per-key maps

3. **Backend Health**: `sync.Mutex` per backend
   - Protects `Alive` flag
   - Health check goroutine updates status
   - Request handler reads status

4. **RequestLogger**: `sync.Mutex` protects file writes
   - Ensures log lines aren't interleaved
   - Multiple goroutines can log

### Goroutine Model

- **Main**: HTTP server accepts and routes requests
- **Health Checker**: Background goroutine checks backends every 10s
- **Proxy**: Reverse proxy goroutine forwards requests (std library)
- **Per Request**: Wrapped response writer captures status

## Data Structures

### Backend

```go
type Backend struct {
    URL   *url.URL                    // Backend base URL
    Proxy *httputil.ReverseProxy      // Reverse proxy
    Alive bool                         // Health status
    mu    sync.Mutex                   // Protects Alive
}
```

### LoadBalancer

```go
type LoadBalancer struct {
    backends []*Backend  // Array of available backends
    current  int         // Current index for round-robin
    mu       sync.Mutex  // Protects current
}
```

### RateLimiter

```go
type RateLimiter struct {
    ipLimits  map[string]*TokenBucket  // Per-IP limits
    keyLimits map[string]*TokenBucket  // Per-key limits
    mu        sync.RWMutex             // Protects maps
}
```

### TokenBucket

```go
type TokenBucket struct {
    tokens     float64   // Current tokens
    capacity   float64   // Maximum tokens
    refillRate float64   // Tokens per second
    lastRefill time.Time // Last refill time
}
```

## Performance Characteristics

### Time Complexity

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Route request | O(n) | n = number of backends (skip unhealthy) |
| Rate limit check | O(1) | Hash map lookup and refill |
| Load balance | O(n) | Worst case: iterate all backends |
| Auth check | O(1) | Hash map lookup |
| Log write | O(1) | File write (buffered) |

### Space Complexity

| Data | Space | Notes |
|------|-------|-------|
| Token buckets | O(ip_count + key_count) | Grows with unique IPs/keys |
| Backends | O(n) | Fixed size array |
| Request logs | O(1) | Append to file |

## Health Check Design

### Algorithm

```
Every 10 seconds:
  For each backend:
    Launch health check goroutine
    
Health check goroutine:
  1. HTTP GET to <backend>/health
  2. Timeout: 5 seconds
  3. If success (200 OK):
     - Mark Alive = true
     - Log status change if was unhealthy
  4. If failure (non-200, timeout, error):
     - Mark Alive = false
     - Log status change if was healthy
```

### Properties

- **Non-blocking**: Health checks run in background
- **Automatic failover**: Requests route away from unhealthy backends
- **Graceful degradation**: Works with reduced backend capacity
- **Fast detection**: ~10 second max before failover
- **Recovery detection**: Automatically re-enables healthy backends

## Rate Limiting Design

### Token Bucket Algorithm

```
Per IP/key:
  Initialize bucket = Capacity tokens
  
  On request:
    1. Refill bucket:
       elapsed = now - last_refill
       tokens = min(capacity, tokens + elapsed * refill_rate)
       last_refill = now
    
    2. Check limit:
       if tokens >= 1:
           tokens -= 1
           Allow request
       else:
           Deny request (429)
```

### Properties

- **Fair**: Each IP gets independent limit
- **Burstable**: Can make more requests per minute initially
- **Refillable**: Bucket refills over time
- **Smooth**: No artificial delays
- **Scalable**: O(1) check time

### Example

Rate limit: 100 requests/minute per IP

```
Initial state:
  capacity = 100
  refill_rate = 100/60 = 1.67 tokens/sec
  tokens = 100

Request 1: tokens = 99 
Request 2-100: tokens decrements each time

After 5 seconds (no requests):
  elapsed = 5
  tokens = min(100, 0 + 5*1.67) = 8.35
  Request 101:  (consume 1 token)
  tokens = 7.35
```

## Authentication Design

### API Key Validation

```
On request with X-API-Key header:
  1. Extract key from header
  2. Look up in APIKeys map
  3. If found and true: Allow
  4. If not found or false: Return 401 Unauthorized
```

### Properties

- **Simple**: Hash map lookup
- **Fast**: O(1) check time
- **Flexible**: Easy to add/remove keys
- **Optional**: Works with or without API key

### Extension Points

To support more complex auth:
- Replace map with database query
- Add token validation (JWT, etc.)
- Implement OAuth2 flow
- Add rate limiting per user instead of per key

## Load Balancing Design

### Round-Robin Algorithm

```
backends = [B1, B2, B3]
current = 0

Request 1:
  Check B1: healthy? → Yes
  current = 1
  return B1

Request 2:
  Check B2: healthy? → Yes
  current = 2
  return B2

Request 3:
  Check B3: healthy? → Yes
  current = 0
  return B3

Request 4:
  Check B1: healthy? → Yes
  current = 1
  return B1
```

### Health-Aware Selection

```
If backend is unhealthy:
  current = (current + 1) % len(backends)
  Check next backend
  Repeat until healthy backend found
  
If all backends unhealthy:
  Return nil
  Gateway returns 503 Service Unavailable
```

### Properties

- **Fair**: Equal distribution (when all healthy)
- **Efficient**: O(n) worst case
- **Adaptive**: Skips unhealthy backends
- **Stateful**: Remembers position
- **Thread-safe**: Mutex protects index

## Logging Design

### JSON Format

Each line is a complete JSON object:

```json
{
  "timestamp": "2026-02-10T12:23:45Z",
  "method": "GET",
  "path": "/api/user",
  "client_ip": "127.0.0.1",
  "api_key": "key-admin",
  "status_code": 200,
  "response_time_ms": "5.23",
  "backend": "http://localhost:8081",
  "error": ""
}
```

### Advantages

- **Parseable**: Each line is valid JSON
- **Structured**: Easy to parse and filter
- **Complete**: All metadata in one line
- **Queryable**: Tools like `jq` can filter

### File Handling

- **Append mode**: Multiple instances can log
- **Buffered**: File writes are buffered
- **Thread-safe**: Mutex protects file handle
- **Rolling**: File grows indefinitely (can add rotation)

## Scalability Considerations

### Vertical Scaling (Single Instance)

- Handle ~5000-10000 req/s on modern hardware
- Memory: ~10MB baseline + request buffer (~1KB per concurrent request)
- CPU: 1-2 cores saturates at ~5000 req/s
- Limited by Go runtime and reverse proxy overhead

### Horizontal Scaling

To scale beyond single instance:

1. **Multiple Gateways**: Run multiple instances
2. **Load Balancer**: Use DNS round-robin or hardware LB
3. **Shared Backend Pool**: All gateways route to same backends
4. **Sticky Sessions**: If needed for stateful backends

### Backend Scalability

- Add more backends by extending `-backends` flag
- Gateway distributes requests across all backends
- Health checks adapt to added/removed backends
- No performance degradation with more backends

## Future Enhancements

### Caching

Add response caching layer:
- Cache GET requests
- TTL-based expiration
- Cache invalidation on errors
- Significant performance improvement

### Metrics/Monitoring

Add Prometheus metrics:
- Request counts by endpoint
- Response time histograms
- Rate limit events
- Backend health status
- Gateway resource usage

### Circuit Breaker

Add circuit breaker per backend:
- Fail fast when backend is unreliable
- Gradual ramp-up on recovery
- Prevent cascading failures

### Request Transformation

- Rewrite URLs
- Add/modify headers
- Body transformation (JSON, XML)
- Request/response compression

### Advanced Load Balancing

- Weighted round-robin
- Least connections
- Least response time
- IP hash (sticky sessions)
- Random selection

### TLS/HTTPS

- Terminate TLS at gateway
- Backend communication options
- Certificate management
- Self-signed support
