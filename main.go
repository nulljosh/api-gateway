package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Config holds gateway configuration
type Config struct {
	Port                int
	Backends            []string
	RateLimitPerIP      int
	RateLimitPerKey     int
	HealthCheckInterval time.Duration
	APIKeys             map[string]bool
}

// LoadBalancer implements round-robin load balancing
type LoadBalancer struct {
	backends []*Backend
	current  int
	mu       sync.Mutex
}

// Backend represents an upstream server
type Backend struct {
	URL   *url.URL
	Proxy *httputil.ReverseProxy
	Alive bool
	mu    sync.Mutex
}

// RateLimiter implements per-IP and per-key rate limiting
type RateLimiter struct {
	ipLimits  map[string]*TokenBucket
	keyLimits map[string]*TokenBucket
	mu        sync.RWMutex
}

// TokenBucket for rate limiting
type TokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
}

// RequestLogger logs all requests and responses
type RequestLogger struct {
	file *os.File
	mu   sync.Mutex
}

// LogEntry represents a logged request/response
type LogEntry struct {
	Timestamp    string `json:"timestamp"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	ClientIP     string `json:"client_ip"`
	APIKey       string `json:"api_key,omitempty"`
	StatusCode   int    `json:"status_code"`
	ResponseTime string `json:"response_time_ms"`
	Backend      string `json:"backend"`
	Error        string `json:"error,omitempty"`
}

// Gateway is the main API gateway
type Gateway struct {
	config      *Config
	lb          *LoadBalancer
	rateLimiter *RateLimiter
	logger      *RequestLogger
	mux         *http.ServeMux
}

// NewGateway creates a new gateway instance
func NewGateway(config *Config) (*Gateway, error) {
	lb := &LoadBalancer{
		backends: make([]*Backend, 0),
	}

	// Initialize backends
	for _, backendURL := range config.Backends {
		parsedURL, err := url.Parse(backendURL)
		if err != nil {
			return nil, fmt.Errorf("invalid backend URL: %s", backendURL)
		}

		backend := &Backend{
			URL:   parsedURL,
			Proxy: httputil.NewSingleHostReverseProxy(parsedURL),
			Alive: true,
		}
		lb.backends = append(lb.backends, backend)
	}

	// Create logger
	logFile, err := os.OpenFile("gateway.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	logger := &RequestLogger{file: logFile}

	g := &Gateway{
		config: config,
		lb:     lb,
		rateLimiter: &RateLimiter{
			ipLimits:  make(map[string]*TokenBucket),
			keyLimits: make(map[string]*TokenBucket),
		},
		logger: logger,
		mux:    http.NewServeMux(),
	}

	// Setup routes
	g.mux.HandleFunc("/health", g.handleHealth)
	g.mux.HandleFunc("/", g.handleRequest)

	return g, nil
}

// Start starts the gateway server
func (g *Gateway) Start() error {
	// Start health checks
	go g.healthCheckLoop()

	log.Printf("Gateway starting on :%d", g.config.Port)
	log.Printf("Routing to backends: %v", g.config.Backends)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", g.config.Port),
		Handler:      g.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	return server.ListenAndServe()
}

// handleHealth returns gateway health status
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	g.lb.mu.Lock()
	healthy := 0
	for _, b := range g.lb.backends {
		if b.Alive {
			healthy++
		}
	}
	g.lb.mu.Unlock()

	status := map[string]interface{}{
		"status":           "ok",
		"healthy_backends": healthy,
		"total_backends":   len(g.lb.backends),
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleRequest handles all proxy requests
func (g *Gateway) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		g.handleHealth(w, r)
		return
	}

	startTime := time.Now()
	clientIP := strings.Split(r.RemoteAddr, ":")[0]

	// Log entry
	logEntry := LogEntry{
		Timestamp: startTime.UTC().Format(time.RFC3339),
		Method:    r.Method,
		Path:      r.URL.Path,
		ClientIP:  clientIP,
	}

	// Auth middleware: check API key if required
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		logEntry.APIKey = apiKey
		if !g.config.APIKeys[apiKey] {
			logEntry.StatusCode = http.StatusUnauthorized
			logEntry.Error = "invalid API key"
			g.logger.Log(logEntry)
			http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
			return
		}
	}

	// Rate limiting
	if !g.rateLimiter.Allow(clientIP, apiKey, g.config) {
		logEntry.StatusCode = http.StatusTooManyRequests
		logEntry.Error = "rate limit exceeded"
		g.logger.Log(logEntry)
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Get healthy backend
	backend := g.lb.Next()
	if backend == nil {
		logEntry.StatusCode = http.StatusServiceUnavailable
		logEntry.Error = "no healthy backends available"
		g.logger.Log(logEntry)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	logEntry.Backend = backend.URL.String()

	// Wrap response writer to capture status code
	wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	// Forward request
	backend.Proxy.ServeHTTP(wrapped, r)

	// Log response
	logEntry.StatusCode = wrapped.statusCode
	logEntry.ResponseTime = fmt.Sprintf("%.2f", time.Since(startTime).Milliseconds())

	g.logger.Log(logEntry)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.written {
		w.statusCode = statusCode
		w.written = true
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// LoadBalancer.Next returns next healthy backend
func (lb *LoadBalancer) Next() *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.backends) == 0 {
		return nil
	}

	// Find next alive backend
	start := lb.current
	for i := 0; i < len(lb.backends); i++ {
		idx := (start + i) % len(lb.backends)
		if lb.backends[idx].Alive {
			lb.current = (idx + 1) % len(lb.backends)
			return lb.backends[idx]
		}
	}

	return nil
}

// RateLimiter.Allow checks if request is allowed
func (rl *RateLimiter) Allow(ip, key string, config *Config) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check IP limit
	if !rl.allowIP(ip, config.RateLimitPerIP) {
		return false
	}

	// Check key limit
	if key != "" && !rl.allowKey(key, config.RateLimitPerKey) {
		return false
	}

	return true
}

func (rl *RateLimiter) allowIP(ip string, limit int) bool {
	bucket, exists := rl.ipLimits[ip]
	if !exists {
		bucket = &TokenBucket{
			tokens:     float64(limit),
			capacity:   float64(limit),
			refillRate: float64(limit) / 60.0, // per second
			lastRefill: time.Now(),
		}
		rl.ipLimits[ip] = bucket
	}

	bucket.refill()
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

func (rl *RateLimiter) allowKey(key string, limit int) bool {
	bucket, exists := rl.keyLimits[key]
	if !exists {
		bucket = &TokenBucket{
			tokens:     float64(limit),
			capacity:   float64(limit),
			refillRate: float64(limit) / 60.0,
			lastRefill: time.Now(),
		}
		rl.keyLimits[key] = bucket
	}

	bucket.refill()
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}
	return false
}

// TokenBucket.refill adds tokens based on time elapsed
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.capacity, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// healthCheckLoop periodically checks backend health
func (g *Gateway) healthCheckLoop() {
	ticker := time.NewTicker(g.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		g.lb.mu.Lock()
		backends := make([]*Backend, len(g.lb.backends))
		copy(backends, g.lb.backends)
		g.lb.mu.Unlock()

		for _, backend := range backends {
			go g.checkBackendHealth(backend)
		}
	}
}

// checkBackendHealth performs a health check on a backend
func (g *Gateway) checkBackendHealth(backend *Backend) {
	healthURL := fmt.Sprintf("%s/health", backend.URL.String())

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(healthURL)

	backend.mu.Lock()
	wasAlive := backend.Alive

	if err != nil || (resp != nil && resp.StatusCode != http.StatusOK) {
		backend.Alive = false
		if wasAlive {
			log.Printf("Backend %s is now unhealthy", backend.URL.String())
		}
	} else {
		backend.Alive = true
		if !wasAlive {
			log.Printf("Backend %s is now healthy", backend.URL.String())
		}
	}
	backend.mu.Unlock()

	if resp != nil {
		resp.Body.Close()
	}
}

// RequestLogger.Log logs a request entry
func (rl *RequestLogger) Log(entry LogEntry) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	data, _ := json.Marshal(entry)
	rl.file.WriteString(string(data) + "\n")
}

// Close closes the gateway
func (g *Gateway) Close() error {
	return g.logger.file.Close()
}

func main() {
	mode := flag.String("mode", "gateway", "gateway, backend, or client")
	port := flag.Int("port", 8080, "Port (gateway: 8080, backend: 8081+)")
	backends := flag.String("backends", "http://localhost:8081,http://localhost:8082", "Comma-separated backend URLs")
	rateLimit := flag.Int("rate-limit", 100, "Requests per minute per IP")
	keyRateLimit := flag.Int("key-rate-limit", 1000, "Requests per minute per API key")
	name := flag.String("name", "Backend", "Backend name")
	flag.Parse()

	switch *mode {
	case "backend":
		runBackend(*port, *name)
	case "client":
		clientMain()
	default:
		runGateway(*port, *backends, *rateLimit, *keyRateLimit)
	}
}

func runGateway(port int, backendsStr string, rateLimit, keyRateLimit int) {
	// Create API keys
	apiKeys := make(map[string]bool)
	apiKeys["key-test-1"] = true
	apiKeys["key-test-2"] = true
	apiKeys["key-admin"] = true

	// Parse backends
	backendsList := strings.Split(backendsStr, ",")

	config := &Config{
		Port:                port,
		Backends:            backendsList,
		RateLimitPerIP:      rateLimit,
		RateLimitPerKey:     keyRateLimit,
		HealthCheckInterval: 10 * time.Second,
		APIKeys:             apiKeys,
	}

	gateway, err := NewGateway(config)
	if err != nil {
		log.Fatalf("Failed to create gateway: %v", err)
	}

	if err := gateway.Start(); err != nil {
		log.Fatalf("Gateway error: %v", err)
	}
}

func runBackend(port int, name string) {
	backend := NewMockBackend(port, name)
	if err := backend.Start(); err != nil {
		log.Fatalf("Backend error: %v", err)
	}
}
