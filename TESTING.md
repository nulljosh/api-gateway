# Testing Guide

Complete testing guide for the API Gateway.

## Setup

### Prerequisites
- Go 1.21+
- Terminals (at least 4)

### Installation

```bash
cd ~/Documents/Code/api-gateway
go build -o api-gateway .
```

## Starting the Gateway Stack

### Terminal 1: Start Gateway

```bash
./api-gateway -mode gateway -port 8080 \
  -backends http://localhost:8081,http://localhost:8082 \
  -rate-limit 100 \
  -key-rate-limit 1000
```

Expected output:
```
2026/02/10 12:23:45 Gateway starting on :8080
2026/02/10 12:23:45 Routing to backends: [http://localhost:8081 http://localhost:8082]
```

### Terminal 2: Start Backend 1

```bash
./api-gateway -mode backend -port 8081 -name "Backend-1"
```

Expected output:
```
2026/02/10 12:23:46 [Backend-1] Starting on localhost:8081
```

### Terminal 3: Start Backend 2

```bash
./api-gateway -mode backend -port 8082 -name "Backend-2"
```

Expected output:
```
2026/02/10 12:23:47 [Backend-2] Starting on localhost:8082
```

### Terminal 4: Run Tests

Use the test client in this terminal:

```bash
# All tests at once
make test

# Or run individual tests below
```

## Unit Tests

### 1. Health Check

```bash
go run . -mode client -cmd health -endpoint http://localhost:8080
```

Expected response:
```
Testing /health endpoint...
Status: 200
Response: {"healthy_backends":2,"status":"ok","timestamp":"2026-02-10T12:23:50Z","total_backends":2}
```

 **Test**: Gateway reports both backends as healthy

### 2. Request Routing

```bash
go run . -mode client -cmd user -endpoint http://localhost:8080 -count 4
```

Expected output shows alternating backends:
```
[1] Status: 200
    User ID: 1, Backend: http://localhost:8081
[2] Status: 200
    User ID: 2, Backend: http://localhost:8082
[3] Status: 200
    User ID: 3, Backend: http://localhost:8081
[4] Status: 200
    User ID: 4, Backend: http://localhost:8082
```

 **Test**: Load balancing works correctly (round-robin)

### 3. Echo Endpoint

```bash
go run . -mode client -cmd echo -endpoint http://localhost:8080 -count 2
```

Expected response includes request body echoed back:
```
[1] Status: 200
    Backend: http://localhost:8081
[2] Status: 200
    Backend: http://localhost:8082
```

 **Test**: Requests are proxied correctly

### 4. Authentication

```bash
go run . -mode client -cmd auth -endpoint http://localhost:8080
```

Expected output:
```
Testing authentication...
1. Request without key (should succeed):
   Status: 200
2. Request with invalid key (should fail):
   Status: 401
3. Request with valid key (should succeed):
   Status: 200
   Backend: http://localhost:8081
```

 **Test**: Invalid keys are rejected (401), valid keys are accepted (200)

### 5. Rate Limiting (Per IP)

```bash
go run . -mode client -cmd rate-limit -endpoint http://localhost:8080 -count 150
```

Expected output shows some requests being rate limited:
```
Testing rate limiting (150 requests)...
[110] Rate limited
[111] Rate limited
[112] Rate limited
Successful: 100/150, Limited: 50/150
```

 **Test**: Requests are limited to ~100/minute per IP

### 6. Slow Endpoint

```bash
go run . -mode client -cmd slow -endpoint http://localhost:8080 -count 1
```

Expected: Each request takes ~500ms

```
Testing /api/slow endpoint (1 requests)...
[1] Status: 200
    Backend: http://localhost:8081
```

 **Test**: Slow backends are handled correctly

### 7. Request Logging

```bash
# While requests are being made, check the log in another terminal
tail -f gateway.log | jq .
```

Expected log entries:
```json
{
  "client_ip": "127.0.0.1",
  "method": "GET",
  "path": "/api/user",
  "status_code": 200,
  "response_time_ms": "5.23",
  "backend": "http://localhost:8081",
  "timestamp": "2026-02-10T12:23:50.123Z"
}
```

 **Test**: All requests are logged with status, backend, and response time

## Integration Tests

### Test 1: Load Balancing Under Load

```bash
go run . -mode client -cmd data -endpoint http://localhost:8080 -count 20
```

Verify output shows alternating backends in round-robin pattern.

### Test 2: Backend Failover

1. Start the test:
```bash
watch -n 0.1 'curl -s http://localhost:8080/api/user | jq .backend'
```

2. In another terminal, kill Backend-1:
```bash
pkill -f "port 8081"
```

3. Observe:
   - First few requests may fail or timeout
   - Gateway detects unhealthy backend
   - Requests route only to Backend-2

4. Restart Backend-1:
```bash
./api-gateway -mode backend -port 8081 -name "Backend-1"
```

5. Observe:
   - Gateway detects Backend-1 is healthy again
   - Requests start routing to both backends

 **Test**: Failover and recovery work correctly

### Test 3: API Key Rate Limiting

```bash
# Create a script to test per-key limits
for i in {1..1500}; do
  curl -H "X-API-Key: key-test-1" http://localhost:8080/api/user >/dev/null 2>&1
  [ $((i % 500)) -eq 0 ] && echo "Request $i"
done
```

Expected: Around request 1000, you'll see 429 responses (after limit is hit).

 **Test**: Per-key rate limiting works (1000 req/min default)

## Stress Tests

### Test 4: High Throughput

```bash
# Test 1000 requests in parallel
for i in {1..1000}; do
  curl http://localhost:8080/api/data &
done | wait
```

Monitor memory and CPU during test.

### Test 5: Concurrent Connections

```bash
# Use Apache Bench if available
ab -n 1000 -c 100 http://localhost:8080/api/user

# Or with wrk if available
wrk -t4 -c100 -d10s http://localhost:8080/api/user
```

## Log Analysis

### View last 10 requests:
```bash
tail -n 10 gateway.log | jq .
```

### Count requests by backend:
```bash
jq -r '.backend' gateway.log | sort | uniq -c
```

### Find rate-limited requests:
```bash
jq 'select(.status_code == 429)' gateway.log
```

### Find authentication failures:
```bash
jq 'select(.status_code == 401)' gateway.log
```

### Find slow requests (>100ms):
```bash
jq 'select(.response_time_ms > "100")' gateway.log
```

## Performance Metrics

### Measure average response time:
```bash
jq -r '.response_time_ms | tonumber' gateway.log | \
  awk '{sum+=$1; n++} END {print "Average:", sum/n, "ms"}'
```

### Count by status code:
```bash
jq -r '.status_code' gateway.log | sort | uniq -c
```

## Cleanup

```bash
# Kill all processes
pkill -f api-gateway

# Remove logs
rm gateway.log
```

## Common Issues

### Issue: "Connection refused" when connecting to backends

**Solution**: Make sure all backends are started before making requests.

### Issue: Health checks failing

**Solution**: Backends take a few seconds to start. Wait a moment before running tests.

### Issue: Rate limiting not working as expected

**Solution**: 
- Limits are per-IP and per-API-key
- Token bucket refills over time (per-minute limits)
- Run tests quickly to see rate limiting in action

### Issue: Round-robin not alternating

**Solution**: 
- Check that both backends are healthy: `curl http://localhost:8080/health`
- Load balancer only uses healthy backends
- If one backend is down, all requests go to the other

## Test Summary Checklist

- [ ] Health check returns correct backend status
- [ ] Requests alternate between backends (round-robin)
- [ ] Invalid API keys are rejected (401)
- [ ] Valid API keys are accepted (200)
- [ ] Rate limiting is enforced (429 when exceeded)
- [ ] All requests are logged
- [ ] Response times are measured correctly
- [ ] Failed backend is detected
- [ ] Requests failover to healthy backend
- [ ] Gateway continues operating with reduced backends
- [ ] Request routing works with multiple backends
- [ ] Load balancing distributes evenly

## Expected Results

All tests should pass with:
-  Proper request routing
-  Load balancing across backends
-  Authentication enforcement
-  Rate limiting enforcement
-  Health check functionality
-  Automatic failover
-  Comprehensive logging
