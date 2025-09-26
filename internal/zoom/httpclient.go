// Package zoom provides HTTP client with retry logic for Zoom API interactions
package zoom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

// HTTPClientConfig holds configuration for the retry HTTP client
type HTTPClientConfig struct {
	Timeout         time.Duration // Request timeout
	MaxRetries      int           // Maximum number of retries
	RetryWaitMin    time.Duration // Minimum wait time between retries
	RetryWaitMax    time.Duration // Maximum wait time between retries
	RetryableStatus []int         // HTTP status codes that should trigger retries
	FollowRedirects bool          // Whether to follow redirects
	MaxRedirects    int           // Maximum number of redirects to follow
}

// HTTPClientConfigFromDownloadConfig creates HTTPClientConfig from DownloadConfig
func HTTPClientConfigFromDownloadConfig(cfg config.DownloadConfig) HTTPClientConfig {
	return HTTPClientConfig{
		Timeout:         cfg.TimeoutDuration(),
		MaxRetries:      cfg.RetryAttempts,
		RetryWaitMin:    500 * time.Millisecond,
		RetryWaitMax:    5 * time.Second,
		RetryableStatus: []int{429, 500, 502, 503, 504},
		FollowRedirects: true,
		MaxRedirects:    10,
	}
}

// RetryHTTPClient is an HTTP client with retry logic and exponential backoff
type RetryHTTPClient struct {
	client *http.Client
	config HTTPClientConfig
}

// NewRetryHTTPClient creates a new HTTP client with retry logic
func NewRetryHTTPClient(config HTTPClientConfig) *RetryHTTPClient {
	// Set defaults if not provided
	if config.RetryWaitMin == 0 {
		config.RetryWaitMin = 500 * time.Millisecond
	}
	if config.RetryWaitMax == 0 {
		config.RetryWaitMax = 5 * time.Second
	}
	if len(config.RetryableStatus) == 0 {
		config.RetryableStatus = []int{429, 500, 502, 503, 504}
	}
	if config.MaxRedirects == 0 {
		config.MaxRedirects = 10
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}

	// Configure redirect policy
	if !config.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("too many redirects: %d", len(via))
			}
			return nil
		}
	}

	return &RetryHTTPClient{
		client: client,
		config: config,
	}
}

// ZoomAPIError represents a Zoom API error response
type ZoomAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *ZoomAPIError) Error() string {
	return fmt.Sprintf("zoom API error %d: %s", e.Code, e.Message)
}

// HTTPError represents a general HTTP error
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP error %d: %s", e.StatusCode, e.Status)
}

// Do executes an HTTP request with retry logic
func (c *RetryHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		// Clone the request for retry attempts
		reqClone := c.cloneRequest(req)

		resp, err = c.client.Do(reqClone)
		if err != nil {
			// Network errors should be retried
			if attempt < c.config.MaxRetries {
				c.waitForRetry(attempt, 0, "")
				continue
			}
			return nil, fmt.Errorf("request failed after %d attempts: %w", attempt+1, err)
		}

		// Check if we should retry based on status code
		if c.shouldRetry(resp.StatusCode) {
			// Read response body for error details
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if attempt < c.config.MaxRetries {
				retryAfter := c.parseRetryAfter(resp)
				c.waitForRetry(attempt, retryAfter, resp.Header.Get("Retry-After"))
				continue
			}

			// Max retries exceeded - return appropriate error
			zoomErr := c.parseZoomError(resp.StatusCode, body)
			if zoomErr != nil {
				return nil, zoomErr
			}
			return nil, &HTTPError{
				StatusCode: resp.StatusCode,
				Status:     resp.Status,
				Body:       string(body),
			}
		}

		// Check for other non-2xx status codes that should return errors
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			zoomErr := c.parseZoomError(resp.StatusCode, body)
			if zoomErr != nil {
				return nil, zoomErr
			}
			return nil, &HTTPError{
				StatusCode: resp.StatusCode,
				Status:     resp.Status,
				Body:       string(body),
			}
		}

		// Success case
		return resp, nil
	}

	return resp, err
}

// cloneRequest creates a copy of the HTTP request for retries
func (c *RetryHTTPClient) cloneRequest(req *http.Request) *http.Request {
	reqClone := req.Clone(req.Context())
	return reqClone
}

// shouldRetry determines if a request should be retried based on status code
func (c *RetryHTTPClient) shouldRetry(statusCode int) bool {
	for _, retryableStatus := range c.config.RetryableStatus {
		if statusCode == retryableStatus {
			return true
		}
	}
	return false
}

// parseZoomError attempts to parse a Zoom API error response
func (c *RetryHTTPClient) parseZoomError(statusCode int, body []byte) *ZoomAPIError {
	if len(body) == 0 {
		return nil
	}

	// Try to parse as JSON
	var zoomErr ZoomAPIError
	if err := json.Unmarshal(body, &zoomErr); err != nil {
		return nil
	}

	// Validate that it looks like a Zoom error
	if zoomErr.Code == 0 && zoomErr.Message == "" {
		return nil
	}

	zoomErr.Status = statusCode
	return &zoomErr
}

// parseRetryAfter parses the Retry-After header and returns the wait duration
func (c *RetryHTTPClient) parseRetryAfter(resp *http.Response) time.Duration {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}

	// Try parsing as seconds
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP date
	if t, err := http.ParseTime(retryAfter); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}

	return 0
}

// waitForRetry implements exponential backoff with jitter
func (c *RetryHTTPClient) waitForRetry(attempt int, retryAfter time.Duration, retryAfterHeader string) {
	var waitTime time.Duration

	// If we have a Retry-After header, respect it
	if retryAfter > 0 {
		waitTime = retryAfter
		// Cap at maximum wait time
		if waitTime > c.config.RetryWaitMax {
			waitTime = c.config.RetryWaitMax
		}
	} else {
		// Exponential backoff: 2^attempt * base + jitter
		base := float64(c.config.RetryWaitMin)
		exponential := base * math.Pow(2, float64(attempt))
		
		// Add jitter (±25% of the calculated time)
		jitter := exponential * 0.25 * (rand.Float64()*2 - 1)
		waitTime = time.Duration(exponential + jitter)
		
		// Cap at maximum wait time
		if waitTime > c.config.RetryWaitMax {
			waitTime = c.config.RetryWaitMax
		}
		if waitTime < c.config.RetryWaitMin {
			waitTime = c.config.RetryWaitMin
		}
	}

	time.Sleep(waitTime)
}

// Client returns the underlying HTTP client
func (c *RetryHTTPClient) Client() *http.Client {
	return c.client
}

// CheckConnectivity performs a basic connectivity check
func (c *RetryHTTPClient) CheckConnectivity(ctx context.Context, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create connectivity check request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("connectivity check failed: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// GetWithRetry performs a GET request with retry logic
func (c *RetryHTTPClient) GetWithRetry(ctx context.Context, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return c.Do(req)
}

// PostWithRetry performs a POST request with retry logic
func (c *RetryHTTPClient) PostWithRetry(ctx context.Context, url string, body io.Reader, contentType string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	
	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return c.Do(req)
}

// AuthenticatedRetryClient combines retry logic with authentication
type AuthenticatedRetryClient struct {
	retryClient *RetryHTTPClient
	auth        Authenticator
}

// NewAuthenticatedRetryClient creates a client with both retry logic and authentication
func NewAuthenticatedRetryClient(retryClient *RetryHTTPClient, auth Authenticator) *AuthenticatedRetryClient {
	return &AuthenticatedRetryClient{
		retryClient: retryClient,
		auth:        auth,
	}
}

// Do executes an HTTP request with both authentication and retry logic
func (c *AuthenticatedRetryClient) Do(req *http.Request) (*http.Response, error) {
	// Get access token
	token, err := c.auth.GetAccessToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to get access token for request: %w", err)
	}

	// Add Authorization header
	req.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)

	// Execute request with retry logic
	return c.retryClient.Do(req)
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for Zoom API errors that are retryable
	if zoomErr, ok := err.(*ZoomAPIError); ok {
		retryableCodes := []int{429, 500, 502, 503, 504}
		for _, code := range retryableCodes {
			if zoomErr.Status == code {
				return true
			}
		}
		return false
	}

	// Check for HTTP errors that are retryable
	if httpErr, ok := err.(*HTTPError); ok {
		retryableCodes := []int{429, 500, 502, 503, 504}
		for _, code := range retryableCodes {
			if httpErr.StatusCode == code {
				return true
			}
		}
		return false
	}

	// Network errors are generally retryable
	errStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"network is unreachable",
		"temporary failure",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}