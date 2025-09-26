package zoom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

// TestHTTPClientAuthIntegration tests integration between HTTP client and authentication
func TestHTTPClientAuthIntegration(t *testing.T) {
	apiCallCount := 0
	
	// Mock server that requires Bearer token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			// OAuth token endpoint
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "test_bearer_token_123",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "recording:read user:read"
			}`))
			return
		}

		// API endpoint - check for Bearer token
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test_bearer_token_123" {
			w.WriteHeader(401)
			w.Write([]byte(`{"code": 124, "message": "Invalid access token"}`))
			return
		}

		apiCallCount++
		
		// Simulate rate limiting on first call only
		if apiCallCount == 1 {
			w.Header().Set("Retry-After", "1") 
			w.WriteHeader(429)
			w.Write([]byte(`{"code": 429, "message": "Rate limit exceeded"}`))
			return
		}

		// Success response on retry
		w.WriteHeader(200)
		w.Write([]byte(`{"meetings": [], "total_records": 0}`))
	}))
	defer server.Close()

	// Create configuration
	cfg := config.ZoomConfig{
		AccountID:    "test_account",
		ClientID:     "test_client",
		ClientSecret: "test_secret",
		BaseURL:      server.URL,
	}

	// Create authenticator
	auth := NewServerToServerAuth(cfg)

	// Create HTTP client with retry logic
	downloadConfig := config.DownloadConfig{
		TimeoutSeconds: 10,
		RetryAttempts:  2,
	}
	httpConfig := HTTPClientConfigFromDownloadConfig(downloadConfig)
	retryClient := NewRetryHTTPClient(httpConfig)

	// Create authenticated client that uses retry client internally
	authenticatedRetryClient := NewAuthenticatedRetryClient(retryClient, auth)

	// Test request that will:
	// 1. Get OAuth token via auth system
	// 2. Make API request with Bearer token
	// 3. Handle rate limit (429) with retry automatically
	// 4. Succeed on second attempt
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/users/test@example.com/recordings", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Make request - should handle rate limit with retry internally
	resp, err := authenticatedRetryClient.Do(req)
	if err != nil {
		t.Fatalf("Request should succeed after rate limit retry: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("✅ HTTP client and authentication integration test completed successfully")
}

// TestRetryClientWithAuthentication tests the retry client configuration integration
func TestRetryClientWithAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			// OAuth token endpoint - always succeeds
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "valid_token",
				"token_type": "Bearer",
				"expires_in": 3600
			}`))
			return
		}

		// API endpoint - always returns success
		w.WriteHeader(200)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	cfg := config.ZoomConfig{
		AccountID:    "test_account", 
		ClientID:     "test_client",
		ClientSecret: "test_secret",
		BaseURL:      server.URL,
	}

	auth := NewServerToServerAuth(cfg)
	
	// Test configuration integration
	downloadConfig := config.DownloadConfig{
		TimeoutSeconds: 5,
		RetryAttempts:  2,
	}
	httpConfig := HTTPClientConfigFromDownloadConfig(downloadConfig)
	retryClient := NewRetryHTTPClient(httpConfig)
	authenticatedRetryClient := NewAuthenticatedRetryClient(retryClient, auth)

	// Verify configuration was applied correctly
	if httpConfig.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", httpConfig.Timeout)
	}
	if httpConfig.MaxRetries != 2 {
		t.Errorf("Expected max retries 2, got %d", httpConfig.MaxRetries)
	}

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Should succeed
	resp, err := authenticatedRetryClient.Do(req)
	if err != nil {
		t.Fatalf("Request should succeed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("✅ Retry client with authentication configuration test completed successfully")
}