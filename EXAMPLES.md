# API Gateway Examples

## Basic Setup

### 1. Build the Gateway

```bash
cd ~/Documents/Code/api-gateway
go build -o api-gateway .
```

### 2. Start Services (in separate terminals)

```bash
# Terminal 1: Gateway
./api-gateway -mode gateway -port 8080 -backends http://localhost:8081,http://localhost:8082

# Terminal 2: Backend 1
./api-gateway -mode backend -port 8081 -name "Backend-1"

# Terminal 3: Backend 2
./api-gateway -mode backend -port 8082 -name "Backend-2"

# Terminal 4: Testing
# Use this terminal for running tests
```

## Example 1: Basic Request Routing

### Scenario
Make a simple request to the gateway and verify it's proxied to a backend.

### Commands

```bash
# Single request to user endpoint
curl http://localhost:8080/api/user?id=123
```

### Expected Output

```json
{
  "status": "ok",
  "message": "User data",
  "backend": "http://localhost:8081",
  "timestamp": "2026-02-10T12:30:00Z",
  "path": "/api/user",
  "method": "GET",
  "echo": {
    "id": "123",
    "name": "Test User",
    "email": "user@example.com"
  }
}
```

## Example 2: Load Balancing

### Scenario
Make multiple requests and observe how they're distributed across backends.

### Commands

```bash
# Make 6 requests
for i in {1..6}; do
  echo "Request $i:"
  curl -s http://localhost:8080/api/data | jq .backend
done
```

### Expected Output

Notice the alternating pattern:
```
Request 1: "http://localhost:8081"
Request 2: "http://localhost:8082"
Request 3: "http://localhost:8081"
Request 4: "http://localhost:8082"
Request 5: "http://localhost:8081"
Request 6: "http://localhost:8082"
```

### Explanation
The gateway uses round-robin load balancing, distributing requests evenly.

## Example 3: Authentication

### Scenario
Test API key authentication - allow some keys, reject others.

### Commands

```bash
# Request without API key (should work - keys are optional)
curl http://localhost:8080/api/user

# Request with valid API key (should work)
curl -H "X-API-Key: key-admin" http://localhost:8080/api/user

# Request with invalid API key (should fail with 401)
curl -H "X-API-Key: invalid-key" http://localhost:8080/api/user
```

### Expected Output

```bash
# Valid key or no key: 200 OK with response
# Invalid key: 401 Unauthorized
# {"error":"Unauthorized: invalid API key"}
```

### Testing with Client

```bash
# Automated auth test
go run . -mode client -cmd auth
```

## Example 4: Rate Limiting

### Scenario
Exceed the per-IP rate limit and receive 429 responses.

### Setup

First, start the gateway with a lower rate limit for testing:

```bash
./api-gateway -mode gateway -port 8080 \
  -backends http://localhost:8081,http://localhost:8082 \
  -rate-limit 10  # Only 10 requests per minute per IP
```

### Commands

```bash
# Hammer the gateway with requests
for i in {1..20}; do
  status=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/user)
  echo "Request $i: $status"
done
```

### Expected Output

```
Request 1: 200
Request 2: 200
Request 3: 200
Request 4: 200
Request 5: 200
Request 6: 200
Request 7: 200
Request 8: 200
Request 9: 200
Request 10: 200
Request 11: 429  # Rate limited!
Request 12: 429  # Rate limited!
...
```

### Testing with Client

```bash
# Automated rate limit test
go run . -mode client -cmd rate-limit -count 150
```

## Example 5: Per-Key Rate Limiting

### Scenario
Different API keys have independent rate limits.

### Commands

```bash
# Start gateway with different limits
./api-gateway -mode gateway -port 8080 \
  -backends http://localhost:8081,http://localhost:8082 \
  -rate-limit 100 \
  -key-rate-limit 20  # Keys limited to 20/minute

# Request with key (uses key limit, not IP limit)
for i in {1..30}; do
  status=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "X-API-Key: key-admin" \
    http://localhost:8080/api/user)
  echo "Request $i: $status"
  sleep 0.01
done
```

### Expected Output

Around request 20, you'll see 429 responses (key limit exceeded).

## Example 6: Health Checks and Failover

### Scenario
Simulate a backend failure and observe automatic failover.

### Setup

Terminal 1: Start gateway and watch health checks

```bash
./api-gateway -mode gateway -port 8080 \
  -backends http://localhost:8081,http://localhost:8082
```

Terminal 2: Start Backend 1
```bash
./api-gateway -mode backend -port 8081 -name "Backend-1"
```

Terminal 3: Start Backend 2
```bash
./api-gateway -mode backend -port 8082 -name "Backend-2"
```

Terminal 4: Watch the routing

```bash
# Continuously show which backend handles requests
watch -n 0.5 'curl -s http://localhost:8080/api/data | jq .backend'
```

Terminal 5: Simulate failure

```bash
# Wait ~10 seconds, then kill Backend-1
sleep 10
pkill -f "port 8081"
```

### Expected Output

Terminal 4 should show:
```
1. Initially alternates between both backends
2. After killing Backend-1, all requests go to Backend-2
3. When you restart Backend-1, requests alternate again
```

## Example 7: Logging and Monitoring

### Scenario
Monitor all requests and analyze patterns.

### Commands

```bash
# Terminal 1: Start watching logs
tail -f gateway.log | jq .

# Terminal 2: Make some requests
for i in {1..5}; do
  curl -s http://localhost:8080/api/user?id=$i > /dev/null
done
```

### Expected Output in Logs

```json
{
  "timestamp": "2026-02-10T12:35:00Z",
  "method": "GET",
  "path": "/api/user",
  "client_ip": "127.0.0.1",
  "api_key": "",
  "status_code": 200,
  "response_time_ms": "4.23",
  "backend": "http://localhost:8081",
  "error": ""
}
```

### Log Analysis Examples

```bash
# Count requests by backend
jq -r '.backend' gateway.log | sort | uniq -c

# Find all rate-limited requests
jq 'select(.status_code == 429)' gateway.log

# Find all 401 unauthorized
jq 'select(.status_code == 401)' gateway.log

# Calculate average response time
jq '.response_time_ms | tonumber' gateway.log | \
  awk '{sum+=$1; n++} END {print "Avg:", sum/n, "ms"}'

# Show slowest requests
jq -S 'sort_by(.response_time_ms | tonumber) | reverse | .[0:10]' gateway.log

# Count by status code
jq '.status_code' gateway.log | sort | uniq -c
```

## Example 8: POST Request with Body

### Scenario
Send a POST request with JSON body.

### Commands

```bash
# Echo the request body
curl -X POST http://localhost:8080/api/echo \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello from Gateway", "user": "Alice"}'
```

### Expected Output

```json
{
  "status": "ok",
  "message": "Echo response",
  "backend": "http://localhost:8081",
  "timestamp": "2026-02-10T12:40:00Z",
  "path": "/api/echo",
  "method": "POST",
  "echo": {
    "message": "Hello from Gateway",
    "user": "Alice"
  }
}
```

## Example 9: Slow Endpoint Testing

### Scenario
Test how the gateway handles slow backends.

### Commands

```bash
# Time how long the slow endpoint takes
time curl http://localhost:8080/api/slow
```

### Expected Output

```
real	0m0.510s  # ~500ms (backend delay)
user	0m0.003s
sys	0m0.005s
```

### Concurrent Slow Requests

```bash
# Make 5 slow requests in parallel
for i in {1..5}; do
  curl http://localhost:8080/api/slow > /dev/null 2>&1 &
done
wait
echo "Done"
```

## Example 10: Using the Test Client

### Scenario
Use the built-in test client for various test scenarios.

### Commands

```bash
# Check gateway health
go run . -mode client -cmd health

# Test echo endpoint (3 requests)
go run . -mode client -cmd echo -count 3

# Test user endpoint with API key
go run . -mode client -cmd user -key key-admin -count 5

# Test authentication variations
go run . -mode client -cmd auth

# Test rate limiting
go run . -mode client -cmd rate-limit -count 200

# Test all endpoints
make test
```

## Advanced Examples

### Example 11: Custom Rate Limits

```bash
# Allow higher rate limits for testing
./api-gateway -mode gateway \
  -port 8080 \
  -backends http://localhost:8081,http://localhost:8082 \
  -rate-limit 1000 \
  -key-rate-limit 10000
```

### Example 12: Multiple Backends

```bash
# Add a third backend
./api-gateway -mode backend -port 8083 -name "Backend-3"

# Update gateway to use all three
./api-gateway -mode gateway \
  -port 8080 \
  -backends http://localhost:8081,http://localhost:8082,http://localhost:8083
```

Then make requests to see round-robin across 3 backends:
```bash
for i in {1..9}; do
  curl -s http://localhost:8080/api/data | jq .backend
done
```

Output should show: 8081, 8082, 8083, 8081, 8082, 8083, 8081, 8082, 8083

### Example 13: Stress Testing

Using Apache Bench (if installed):

```bash
# 1000 requests, 100 concurrent
ab -n 1000 -c 100 http://localhost:8080/api/user

# Shows: Requests per second, avg response time, min/max times
```

Using simple shell:

```bash
# Make 100 concurrent requests
for i in {1..100}; do
  curl -s http://localhost:8080/api/user > /dev/null &
done
wait
echo "100 concurrent requests completed"
```

## Troubleshooting Examples

### Issue: "Connection refused"

```bash
# Check if services are running
lsof -i :8080  # Gateway
lsof -i :8081  # Backend 1
lsof -i :8082  # Backend 2

# If not running, start them again
```

### Issue: Health checks failing

```bash
# Check backend is responding to health checks
curl http://localhost:8081/health

# Should return: {"status":"healthy","backend":"Backend-1"}
```

### Issue: Rate limiting too strict

```bash
# Start with higher limits
./api-gateway -mode gateway \
  -rate-limit 10000 \
  -key-rate-limit 100000
```

### Issue: Can't see round-robin behavior

```bash
# Make sure both backends are healthy
curl http://localhost:8080/health

# Should show: "healthy_backends": 2

# If not, restart a backend:
./api-gateway -mode backend -port 8082 -name "Backend-2"
```

## Best Practices

1. **Always check gateway health**: `curl http://localhost:8080/health`
2. **Monitor logs**: `tail -f gateway.log | jq .`
3. **Use API keys for rate limit tracking**: Easier to see per-app limits
4. **Test with realistic payloads**: Small vs large request bodies
5. **Verify loadbalancing**: Check backend names in responses
6. **Watch for 429s under load**: Expected behavior when limits hit
7. **Restart services cleanly**: `pkill -f api-gateway` before starting over

---

For more detailed information, see:
- `README.md` - Full documentation
- `TESTING.md` - Comprehensive testing guide
- `ARCHITECTURE.md` - Technical details
