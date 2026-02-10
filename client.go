package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// TestClient provides methods for testing the gateway
type TestClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewTestClient creates a new test client
func NewTestClient(baseURL, apiKey string) *TestClient {
	return &TestClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 10,
		},
	}
}

// Request makes an HTTP request to the gateway
func (tc *TestClient) Request(method, path string, body interface{}) (string, int, error) {
	url := tc.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return "", 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	if tc.apiKey != "" {
		req.Header.Set("X-API-Key", tc.apiKey)
	}

	resp, err := tc.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), resp.StatusCode, nil
}

// Get makes a GET request
func (tc *TestClient) Get(path string) (string, int, error) {
	return tc.Request("GET", path, nil)
}

// Post makes a POST request
func (tc *TestClient) Post(path string, body interface{}) (string, int, error) {
	return tc.Request("POST", path, body)
}

// ClientMain runs the test client
func clientMain() {
	cmd := flag.String("cmd", "health", "health|echo|user|data|slow|auth")
	endpoint := flag.String("endpoint", "http://localhost:8080", "Gateway endpoint")
	apiKey := flag.String("key", "", "API key")
	count := flag.Int("count", 1, "Number of requests")
	flag.Parse()

	client := NewTestClient(*endpoint, *apiKey)

	switch *cmd {
	case "health":
		testHealth(client)
	case "echo":
		testEcho(client, *count)
	case "user":
		testUser(client, *count)
	case "data":
		testData(client, *count)
	case "slow":
		testSlow(client, *count)
	case "auth":
		testAuth(client)
	case "rate-limit":
		testRateLimit(client, *count)
	default:
		fmt.Println("Unknown command")
	}
}

func testHealth(client *TestClient) {
	fmt.Println("Testing /health endpoint...")
	resp, code, err := client.Get("/health")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}
	fmt.Printf("Status: %d\n", code)
	fmt.Printf("Response: %s\n", resp)
}

func testEcho(client *TestClient, count int) {
	fmt.Printf("Testing /api/echo endpoint (%d requests)...\n", count)
	for i := 1; i <= count; i++ {
		body := map[string]interface{}{
			"message": fmt.Sprintf("Request %d", i),
		}
		resp, code, err := client.Post("/api/echo", body)
		if err != nil {
			log.Printf("Request %d error: %v", i, err)
			continue
		}
		fmt.Printf("[%d] Status: %d\n", i, code)
		if code == 200 {
			var result map[string]interface{}
			json.Unmarshal([]byte(resp), &result)
			fmt.Printf("    Backend: %v\n", result["backend"])
		}
	}
}

func testUser(client *TestClient, count int) {
	fmt.Printf("Testing /api/user endpoint (%d requests)...\n", count)
	for i := 1; i <= count; i++ {
		resp, code, err := client.Get(fmt.Sprintf("/api/user?id=%d", i))
		if err != nil {
			log.Printf("Request %d error: %v", i, err)
			continue
		}
		fmt.Printf("[%d] Status: %d\n", i, code)
		if code == 200 {
			var result map[string]interface{}
			json.Unmarshal([]byte(resp), &result)
			if echo, ok := result["echo"].(map[string]interface{}); ok {
				fmt.Printf("    User ID: %v, Backend: %v\n", echo["id"], result["backend"])
			}
		}
	}
}

func testData(client *TestClient, count int) {
	fmt.Printf("Testing /api/data endpoint (%d requests)...\n", count)
	for i := 1; i <= count; i++ {
		resp, code, err := client.Get("/api/data")
		if err != nil {
			log.Printf("Request %d error: %v", i, err)
			continue
		}
		fmt.Printf("[%d] Status: %d\n", i, code)
		if code == 200 {
			var result map[string]interface{}
			json.Unmarshal([]byte(resp), &result)
			fmt.Printf("    Backend: %v\n", result["backend"])
		}
	}
}

func testSlow(client *TestClient, count int) {
	fmt.Printf("Testing /api/slow endpoint (%d requests)...\n", count)
	for i := 1; i <= count; i++ {
		resp, code, err := client.Get("/api/slow")
		if err != nil {
			log.Printf("Request %d error: %v", i, err)
			continue
		}
		fmt.Printf("[%d] Status: %d\n", i, code)
		if code == 200 {
			var result map[string]interface{}
			json.Unmarshal([]byte(resp), &result)
			fmt.Printf("    Backend: %v\n", result["backend"])
		}
	}
}

func testAuth(client *TestClient) {
	fmt.Println("Testing authentication...")

	// Test without key
	fmt.Println("1. Request without key (should succeed):")
	resp, code, _ := client.Get("/api/user")
	fmt.Printf("   Status: %d\n", code)

	// Test with invalid key
	fmt.Println("2. Request with invalid key (should fail):")
	invalidClient := NewTestClient(client.baseURL, "invalid-key")
	resp, code, _ = invalidClient.Get("/api/user")
	fmt.Printf("   Status: %d\n", code)

	// Test with valid key
	fmt.Println("3. Request with valid key (should succeed):")
	validClient := NewTestClient(client.baseURL, "key-admin")
	resp, code, _ = validClient.Get("/api/user")
	fmt.Printf("   Status: %d\n", code)
	var result map[string]interface{}
	json.Unmarshal([]byte(resp), &result)
	if code == 200 {
		fmt.Printf("   Backend: %v\n", result["backend"])
	}
}

func testRateLimit(client *TestClient, count int) {
	fmt.Printf("Testing rate limiting (%d requests)...\n", count)
	successCount := 0
	for i := 1; i <= count; i++ {
		resp, code, err := client.Get("/api/user")
		if err != nil {
			log.Printf("Request %d error: %v", i, err)
			continue
		}
		if code == 200 {
			successCount++
		} else if code == 429 {
			fmt.Printf("[%d] Rate limited\n", i)
		}
	}
	fmt.Printf("Successful: %d/%d, Limited: %d/%d\n", successCount, count, count-successCount, count)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: go run . -mode client -cmd <command> [options]

Commands:
  health      - Check gateway health
  echo        - Echo request to backend
  user        - Get user data
  data        - Get sample data
  slow        - Test slow endpoint
  auth        - Test authentication
  rate-limit  - Test rate limiting

Options:
  -endpoint string  Gateway endpoint (default "http://localhost:8080")
  -key string       API key
  -count int        Number of requests (default 1)
  -cmd string       Command to run
`)
}
