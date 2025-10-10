package zoom

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

// TestRetryHTTPClient tests the retry logic and configuration
func TestRetryHTTPClient(t *testing.T) {
	tests := []struct {
		name            string
		maxRetries      int
		serverResponses []serverResponse
		expectedError   bool
		expectedCalls   int
	}{
		{
			name:       "successful request on first try",
			maxRetries: 3,
			serverResponses: []serverResponse{
				{statusCode: 200, body: `{"success": true}`},
			},
			expectedError: false,
			expectedCalls: 1,
		},
		{
			name:       "success after transient failures",
			maxRetries: 3,
			serverResponses: []serverResponse{
				{statusCode: 500, body: `{"error": "server_error"}`},
				{statusCode: 502, body: `{"error": "bad_gateway"}`},
				{statusCode: 200, body: `{"success": true}`},
			},
			expectedError: false,
			expectedCalls: 3,
		},
		{
			name:       "max retries exceeded",
			maxRetries: 2,
			serverResponses: []serverResponse{
				{statusCode: 500, body: `{"error": "server_error"}`},
				{statusCode: 500, body: `{"error": "server_error"}`},
				{statusCode: 500, body: `{"error": "server_error"}`},
			},
			expectedError: true,
			expectedCalls: 3, // initial + 2 retries
		},
		{
			name:       "no retry for client errors",
			maxRetries: 3,
			serverResponses: []serverResponse{
				{statusCode: 400, body: `{"error": "bad_request"}`},
			},
			expectedError: true,
			expectedCalls: 1,
		},
		{
			name:       "retry for rate limits",
			maxRetries: 2,
			serverResponses: []serverResponse{
				{statusCode: 429, body: `{"error": "rate_limit_exceeded"}`, headers: map[string]string{"Retry-After": "1"}},
				{statusCode: 200, body: `{"success": true}`},
			},
			expectedError: false,
			expectedCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if callCount >= len(tt.serverResponses) {
					callCount++
					w.WriteHeader(500)
					w.Write([]byte(`{"error": "unexpected_call"}`))
					return
				}

				response := tt.serverResponses[callCount]
				callCount++

				// Set custom headers
				for key, value := range response.headers {
					w.Header().Set(key, value)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(response.statusCode)
				w.Write([]byte(response.body))
			}))
			defer server.Close()

			// Create retry client
			clientConfig := HTTPClientConfig{
				Timeout:         30 * time.Second,
				MaxRetries:      tt.maxRetries,
				RetryWaitMin:    10 * time.Millisecond,
				RetryWaitMax:    100 * time.Millisecond,
				RetryableStatus: []int{429, 500, 502, 503, 504},
			}

			client := NewRetryHTTPClient(clientConfig)
			
			// Make request
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/test", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			resp, err := client.Do(req)

			// Verify expectations
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if resp != nil {
					resp.Body.Close()
				}
			}

			if callCount != tt.expectedCalls {
				t.Errorf("Expected %d calls, got %d", tt.expectedCalls, callCount)
			}
		})
	}
}

type serverResponse struct {
	statusCode int
	body       string
	headers    map[string]string
}

// TestExponentialBackoff tests the exponential backoff timing
func TestExponentialBackoff(t *testing.T) {
	callTimes := []time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callTimes = append(callTimes, time.Now())
		w.WriteHeader(500) // Always fail to trigger retries
	}))
	defer server.Close()

	clientConfig := HTTPClientConfig{
		Timeout:      10 * time.Second,
		MaxRetries:   3,
		RetryWaitMin: 50 * time.Millisecond,
		RetryWaitMax: 200 * time.Millisecond,
	}

	client := NewRetryHTTPClient(clientConfig)
	
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	start := time.Now()
	_, err = client.Do(req)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error due to max retries exceeded")
	}

	// Should have made 4 calls total (initial + 3 retries)
	if len(callTimes) != 4 {
		t.Errorf("Expected 4 calls, got %d", len(callTimes))
	}

	// Verify exponential backoff timing
	if len(callTimes) >= 2 {
		firstBackoff := callTimes[1].Sub(callTimes[0])
		if firstBackoff < 50*time.Millisecond {
			t.Errorf("First backoff too short: %v", firstBackoff)
		}
		if firstBackoff > 300*time.Millisecond { // Allow some buffer
			t.Errorf("First backoff too long: %v", firstBackoff)
		}
	}

	// Total duration should be at least the sum of minimum wait times
	minExpectedDuration := 3 * 50 * time.Millisecond // 3 retries * min wait
	if duration < minExpectedDuration {
		t.Errorf("Total duration too short: %v (expected at least %v)", duration, minExpectedDuration)
	}
}

// TestRateLimitHandling tests HTTP 429 rate limit response handling
func TestRateLimitHandling(t *testing.T) {
	tests := []struct {
		name        string
		retryAfter  string
		expectDelay bool
	}{
		{
			name:        "retry-after header with seconds",
			retryAfter:  "2",
			expectDelay: true,
		},
		{
			name:        "retry-after header with date",
			retryAfter:  time.Now().Add(1 * time.Second).Format(http.TimeFormat),
			expectDelay: true,
		},
		{
			name:        "no retry-after header",
			retryAfter:  "",
			expectDelay: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				if callCount == 1 {
					// First call returns rate limit
					if tt.retryAfter != "" {
						w.Header().Set("Retry-After", tt.retryAfter)
					}
					w.WriteHeader(429)
					w.Write([]byte(`{"error": "rate_limit_exceeded"}`))
				} else {
					// Second call succeeds
					w.WriteHeader(200)
					w.Write([]byte(`{"success": true}`))
				}
			}))
			defer server.Close()

			clientConfig := HTTPClientConfig{
				Timeout:    10 * time.Second,
				MaxRetries: 1,
			}

			client := NewRetryHTTPClient(clientConfig)
			
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			start := time.Now()
			resp, err := client.Do(req)
			duration := time.Since(start)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if resp != nil {
				resp.Body.Close()
			}

			if tt.expectDelay {
				if duration < 500*time.Millisecond { // Should wait at least some time
					t.Errorf("Expected delay for retry-after, but request completed too quickly: %v", duration)
				}
			}

			if callCount != 2 {
				t.Errorf("Expected 2 calls (rate limit + retry), got %d", callCount)
			}
		})
	}
}

// TestRedirectHandling tests URL redirection support
func TestRedirectHandling(t *testing.T) {
	// Final destination server
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(200)
		w.Write([]byte("file content here"))
	}))
	defer finalServer.Close()

	// Redirect server
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL+"/file.mp4", http.StatusFound)
	}))
	defer redirectServer.Close()

	clientConfig := HTTPClientConfig{
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		FollowRedirects: true,
		MaxRedirects:    10,
	}

	client := NewRetryHTTPClient(clientConfig)
	
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", redirectServer.URL+"/download", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200 after redirect, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "file content here" {
		t.Errorf("Expected 'file content here', got %s", string(body))
	}

	// Verify final URL
	if !strings.Contains(resp.Request.URL.String(), "file.mp4") {
		t.Errorf("Expected final URL to contain 'file.mp4', got %s", resp.Request.URL.String())
	}
}

// TestZoomAPIErrorHandling tests Zoom-specific API error responses
func TestZoomAPIErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedError  string
		shouldRetry    bool
	}{
		{
			name:        "zoom authentication error",
			statusCode:  401,
			responseBody: `{"code": 124, "message": "Invalid access token"}`,
			expectedError: "zoom API error 124: Invalid access token",
			shouldRetry: false,
		},
		{
			name:        "zoom rate limit error",
			statusCode:  429,
			responseBody: `{"code": 429, "message": "Rate limit exceeded"}`,
			expectedError: "zoom API error 429: Rate limit exceeded",
			shouldRetry: true,
		},
		{
			name:        "zoom server error",
			statusCode:  500,
			responseBody: `{"code": 500, "message": "Internal server error"}`,
			expectedError: "zoom API error 500: Internal server error",
			shouldRetry: true,
		},
		{
			name:        "generic HTTP error",
			statusCode:  404,
			responseBody: `Not Found`,
			expectedError: "HTTP error 404",
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				if strings.Contains(tt.responseBody, "code") {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			clientConfig := HTTPClientConfig{
				Timeout:    10 * time.Second,
				MaxRetries: 2,
			}

			client := NewRetryHTTPClient(clientConfig)
			
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			_, err = client.Do(req)

			if err == nil {
				t.Error("Expected error for non-2xx status code")
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error to contain '%s', got '%s'", tt.expectedError, err.Error())
			}

			// Check retry behavior
			expectedCalls := 1
			if tt.shouldRetry {
				expectedCalls = 3 // initial + 2 retries
			}

			if callCount != expectedCalls {
				t.Errorf("Expected %d calls (retry=%t), got %d", expectedCalls, tt.shouldRetry, callCount)
			}
		})
	}
}

// TestTimeoutHandling tests timeout behavior
func TestTimeoutHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than client timeout
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer server.Close()

	clientConfig := HTTPClientConfig{
		Timeout:    100 * time.Millisecond, // Short timeout
		MaxRetries: 1,
	}

	client := NewRetryHTTPClient(clientConfig)
	
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	start := time.Now()
	_, err = client.Do(req)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error")
	}

	// Should timeout within reasonable bounds (allowing for retry + backoff)
	if duration > 1000*time.Millisecond {
		t.Errorf("Request took too long: %v", duration)
	}

	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// TestHTTPClientConfiguration tests client configuration integration
func TestHTTPClientConfiguration(t *testing.T) {
	// Use download config from configuration system
	downloadConfig := config.DownloadConfig{
		TimeoutSeconds: 10,
		RetryAttempts:  3,
	}

	clientConfig := HTTPClientConfigFromDownloadConfig(downloadConfig)

	expectedTimeout := 10 * time.Second
	if clientConfig.Timeout != expectedTimeout {
		t.Errorf("Expected timeout %v, got %v", expectedTimeout, clientConfig.Timeout)
	}

	if clientConfig.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", clientConfig.MaxRetries)
	}

	// Test that client is created successfully
	client := NewRetryHTTPClient(clientConfig)
	if client == nil {
		t.Error("Expected client to be created, got nil")
	}
}