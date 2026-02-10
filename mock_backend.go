package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// MockBackend represents a test backend server
type MockBackend struct {
	port    int
	name    string
	healthy bool
	mux     *http.ServeMux
}

// Response is a standard API response
type Response struct {
	Status    string      `json:"status"`
	Message   string      `json:"message"`
	Backend   string      `json:"backend"`
	Timestamp string      `json:"timestamp"`
	Path      string      `json:"path"`
	Method    string      `json:"method"`
	Echo      interface{} `json:"echo,omitempty"`
}

// NewMockBackend creates a new mock backend
func NewMockBackend(port int, name string) *MockBackend {
	return &MockBackend{
		port:    port,
		name:    name,
		healthy: true,
		mux:     http.NewServeMux(),
	}
}

// Start starts the mock backend server
func (mb *MockBackend) Start() error {
	mb.mux.HandleFunc("/health", mb.handleHealth)
	mb.mux.HandleFunc("/api/echo", mb.handleEcho)
	mb.mux.HandleFunc("/api/user", mb.handleUser)
	mb.mux.HandleFunc("/api/data", mb.handleData)
	mb.mux.HandleFunc("/api/slow", mb.handleSlow)
	mb.mux.HandleFunc("/", mb.handleRoot)

	addr := fmt.Sprintf("localhost:%d", mb.port)
	log.Printf("[%s] Starting on %s", mb.name, addr)

	server := &http.Server{
		Addr:    addr,
		Handler: mb.mux,
	}

	return server.ListenAndServe()
}

// handleHealth returns health status
func (mb *MockBackend) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !mb.healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"backend": mb.name,
	})
}

// handleEcho echoes the request back
func (mb *MockBackend) handleEcho(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	resp := Response{
		Status:    "ok",
		Message:   "Echo response",
		Backend:   mb.name,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Path:      r.URL.Path,
		Method:    r.Method,
		Echo:      json.RawMessage(body),
	}

	json.NewEncoder(w).Encode(resp)
}

// handleUser returns user data
func (mb *MockBackend) handleUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID := r.URL.Query().Get("id")
	if userID == "" {
		userID = "123"
	}

	resp := Response{
		Status:    "ok",
		Message:   "User data",
		Backend:   mb.name,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Path:      r.URL.Path,
		Method:    r.Method,
		Echo: map[string]interface{}{
			"id":    userID,
			"name":  "Test User",
			"email": "user@example.com",
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// handleData returns sample data
func (mb *MockBackend) handleData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	data := []map[string]interface{}{
		{"id": 1, "value": "first", "backend": mb.name},
		{"id": 2, "value": "second", "backend": mb.name},
		{"id": 3, "value": "third", "backend": mb.name},
	}

	resp := map[string]interface{}{
		"status":    "ok",
		"backend":   mb.name,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	}

	json.NewEncoder(w).Encode(resp)
}

// handleSlow simulates a slow endpoint
func (mb *MockBackend) handleSlow(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	delay := 500 * time.Millisecond
	time.Sleep(delay)

	resp := Response{
		Status:    "ok",
		Message:   "Slow response",
		Backend:   mb.name,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Path:      r.URL.Path,
		Method:    r.Method,
	}

	json.NewEncoder(w).Encode(resp)
}

// handleRoot handles root path
func (mb *MockBackend) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := Response{
		Status:    "ok",
		Message:   "Welcome to backend",
		Backend:   mb.name,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Path:      r.URL.Path,
		Method:    r.Method,
	}

	json.NewEncoder(w).Encode(resp)
}

// MockBackendMain runs a mock backend
func MockBackendMain() {
	port := flag.Int("port", 8081, "Backend port")
	name := flag.String("name", "Backend-1", "Backend name")
	flag.Parse()

	backend := NewMockBackend(*port, *name)
	if err := backend.Start(); err != nil {
		log.Fatalf("Backend error: %v", err)
	}
}
