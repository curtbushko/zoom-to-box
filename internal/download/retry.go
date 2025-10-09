// Package download provides enhanced retry logic and error handling for download operations
package download

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

// ErrorType represents different categories of errors for retry logic
type ErrorType string

const (
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeServer     ErrorType = "server"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeClient     ErrorType = "client"
	ErrorTypeUnknown    ErrorType = "unknown"
)

// RetryConfig holds configuration for retry strategies
type RetryConfig struct {
	// Basic retry configuration
	MaxAttempts     int           `json:"max_attempts"`
	BaseDelay       time.Duration `json:"base_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	Multiplier      float64       `json:"multiplier"`
	
	// Jitter configuration
	Jitter        bool    `json:"jitter"`
	JitterPercent int     `json:"jitter_percent"` // Percentage of jitter (0-100)
	
	// Error-specific configurations
	RetryableErrors   []ErrorType   `json:"retryable_errors"`
	NetworkErrorDelay time.Duration `json:"network_error_delay"`
	TimeoutErrorDelay time.Duration `json:"timeout_error_delay"`
	ServerErrorDelay  time.Duration `json:"server_error_delay"`
	RateLimitDelay    time.Duration `json:"rate_limit_delay"`
	
	// Circuit breaker configuration
	CircuitBreaker   bool          `json:"circuit_breaker"`
	FailureThreshold int           `json:"failure_threshold"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		BaseDelay:       500 * time.Millisecond,
		MaxDelay:        30 * time.Second,
		Multiplier:      2.0,
		Jitter:          true,
		JitterPercent:   25,
		RetryableErrors: []ErrorType{
			ErrorTypeNetwork,
			ErrorTypeTimeout,
			ErrorTypeServer,
			ErrorTypeRateLimit,
		},
		NetworkErrorDelay:  1 * time.Second,
		TimeoutErrorDelay:  2 * time.Second,
		ServerErrorDelay:   1 * time.Second,
		RateLimitDelay:     60 * time.Second,
		CircuitBreaker:     true,
		FailureThreshold:   5,
		RecoveryTimeout:    30 * time.Second,
	}
}

// ValidateRetryConfig validates a retry configuration
func ValidateRetryConfig(config RetryConfig) error {
	if config.MaxAttempts <= 0 {
		return fmt.Errorf("max_attempts must be greater than 0")
	}
	
	if config.BaseDelay < 0 {
		return fmt.Errorf("base_delay cannot be negative")
	}
	
	if config.Multiplier < 1.0 {
		return fmt.Errorf("multiplier must be >= 1.0")
	}
	
	if config.MaxDelay > 0 && config.MaxDelay < config.BaseDelay {
		return fmt.Errorf("max_delay cannot be less than base_delay")
	}
	
	if config.JitterPercent < 0 || config.JitterPercent > 100 {
		return fmt.Errorf("jitter_percent must be between 0 and 100")
	}
	
	return nil
}

// RetryStrategy defines how retries should be performed
type RetryStrategy interface {
	// CalculateDelay returns the delay before the next retry and whether to retry
	CalculateDelay(errorType ErrorType, attempt int) (time.Duration, bool)
	
	// IsRetryable checks if an error type is retryable
	IsRetryable(errorType ErrorType) bool
	
	// GetConfig returns the retry configuration
	GetConfig() RetryConfig
}

// retryStrategy implements the RetryStrategy interface
type retryStrategy struct {
	config RetryConfig
	random *rand.Rand
}

// NewRetryStrategy creates a new retry strategy with the given configuration
func NewRetryStrategy(config RetryConfig) RetryStrategy {
	return &retryStrategy{
		config: config,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// CalculateDelay calculates the delay before the next retry attempt
func (rs *retryStrategy) CalculateDelay(errorType ErrorType, attempt int) (time.Duration, bool) {
	// Check if we've exceeded max attempts
	if attempt >= rs.config.MaxAttempts {
		return 0, false
	}
	
	// Check if error type is retryable
	if !rs.IsRetryable(errorType) {
		return 0, false
	}
	
	// Calculate delay based on error type and configuration
	var delay time.Duration
	
	// Use error-specific delays if configured
	switch errorType {
	case ErrorTypeNetwork:
		if rs.config.NetworkErrorDelay > 0 {
			delay = rs.config.NetworkErrorDelay
		}
	case ErrorTypeTimeout:
		if rs.config.TimeoutErrorDelay > 0 {
			delay = rs.config.TimeoutErrorDelay
		}
	case ErrorTypeServer:
		if rs.config.ServerErrorDelay > 0 {
			delay = rs.config.ServerErrorDelay
		}
	case ErrorTypeRateLimit:
		if rs.config.RateLimitDelay > 0 {
			delay = rs.config.RateLimitDelay
		}
	}
	
	// If no error-specific delay, use exponential backoff
	if delay == 0 {
		delay = rs.calculateExponentialBackoff(attempt)
	}
	
	// Apply jitter if enabled
	if rs.config.Jitter {
		delay = rs.applyJitter(delay)
	}
	
	return delay, true
}

// calculateExponentialBackoff calculates exponential backoff delay
func (rs *retryStrategy) calculateExponentialBackoff(attempt int) time.Duration {
	if rs.config.BaseDelay == 0 {
		return time.Second // Default 1 second
	}
	
	// Calculate: base_delay * multiplier^attempt
	multiplier := rs.config.Multiplier
	if multiplier < 1.0 {
		multiplier = 2.0 // Default multiplier
	}
	
	delay := float64(rs.config.BaseDelay) * math.Pow(multiplier, float64(attempt))
	
	// Apply maximum delay cap
	if rs.config.MaxDelay > 0 && time.Duration(delay) > rs.config.MaxDelay {
		delay = float64(rs.config.MaxDelay)
	}
	
	return time.Duration(delay)
}

// applyJitter adds random jitter to reduce thundering herd effect
func (rs *retryStrategy) applyJitter(delay time.Duration) time.Duration {
	if rs.config.JitterPercent <= 0 {
		return delay
	}

	// Calculate jitter range
	jitterRange := float64(delay) * float64(rs.config.JitterPercent) / 100.0

	// Apply random jitter: delay ± jitterRange
	jitter := (rs.random.Float64() - 0.5) * 2 * jitterRange
	jitteredDelay := float64(delay) + jitter

	// Ensure delay is not negative
	if jitteredDelay < 0 {
		jitteredDelay = float64(delay) * 0.1 // Minimum 10% of original delay
	}

	return time.Duration(jitteredDelay)
}

// IsRetryable checks if an error type is configured as retryable
func (rs *retryStrategy) IsRetryable(errorType ErrorType) bool {
	for _, retryable := range rs.config.RetryableErrors {
		if retryable == errorType {
			return true
		}
	}
	return false
}

// GetConfig returns the retry configuration
func (rs *retryStrategy) GetConfig() RetryConfig {
	return rs.config
}

// ClassifyError classifies an error into an ErrorType for retry logic
func ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}
	
	// Check for context errors
	if err == context.DeadlineExceeded || err == context.Canceled {
		return ErrorTypeTimeout
	}
	
	// Check for network errors
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return ErrorTypeTimeout
		}
		return ErrorTypeNetwork
	}
	
	// Check for HTTP errors
	if httpErr, ok := err.(*zoom.HTTPError); ok {
		return ClassifyHTTPError(httpErr.StatusCode)
	}
	
	// Check for Zoom API errors
	if zoomErr, ok := err.(*zoom.ZoomAPIError); ok {
		return ClassifyHTTPError(zoomErr.Status)
	}
	
	// Check error message for common patterns
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "network") || strings.Contains(errMsg, "connection") {
		return ErrorTypeNetwork
	}
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline") {
		return ErrorTypeTimeout
	}
	if strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "forbidden") {
		return ErrorTypeAuth
	}
	
	return ErrorTypeUnknown
}

// ClassifyHTTPError classifies HTTP status codes into error types
func ClassifyHTTPError(statusCode int) ErrorType {
	switch {
	case statusCode == http.StatusTooManyRequests: // 429
		return ErrorTypeRateLimit
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden: // 401, 403
		return ErrorTypeAuth
	case statusCode >= 400 && statusCode < 500: // 4xx client errors
		return ErrorTypeClient
	case statusCode >= 500: // 5xx server errors
		return ErrorTypeServer
	default:
		return ErrorTypeUnknown
	}
}

// RetryMetrics holds metrics about retry operations
type RetryMetrics struct {
	TotalAttempts   int           `json:"total_attempts"`
	TotalDuration   time.Duration `json:"total_duration"`
	LastError       error         `json:"-"`
	LastErrorType   ErrorType     `json:"last_error_type"`
	SuccessAttempt  int           `json:"success_attempt"` // Which attempt succeeded (0 if failed)
}

// RetryExecutor executes operations with retry logic
type RetryExecutor interface {
	// Execute runs an operation with retry logic
	Execute(ctx context.Context, operation func() error) error
	
	// GetMetrics returns metrics about the last execution
	GetMetrics() RetryMetrics
	
	// GetAttemptCount returns the current attempt count (for testing)
	GetAttemptCount() int
	
	// Reset resets the executor state
	Reset()
}

// retryExecutor implements the RetryExecutor interface
type retryExecutor struct {
	strategy      RetryStrategy
	metrics       RetryMetrics
	attemptCount  int
	circuitBreaker *circuitBreaker
}

// NewRetryExecutor creates a new retry executor
func NewRetryExecutor(strategy RetryStrategy) RetryExecutor {
	executor := &retryExecutor{
		strategy: strategy,
	}
	
	// Initialize circuit breaker if enabled
	config := strategy.GetConfig()
	if config.CircuitBreaker {
		executor.circuitBreaker = newCircuitBreaker(config.FailureThreshold, config.RecoveryTimeout)
	}
	
	return executor
}

// Execute runs an operation with retry logic
func (re *retryExecutor) Execute(ctx context.Context, operation func() error) error {
	re.attemptCount = 0
	re.metrics = RetryMetrics{}
	start := time.Now()

	defer func() {
		re.metrics.TotalDuration = time.Since(start)
	}()

	for {
		// Check circuit breaker
		if re.circuitBreaker != nil && !re.circuitBreaker.AllowRequest() {
			return fmt.Errorf("circuit breaker open: too many failures")
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute operation
		re.attemptCount++
		currentAttempt := re.attemptCount

		err := operation()

		re.metrics.TotalAttempts = currentAttempt
		if err != nil {
			re.metrics.LastError = err
			re.metrics.LastErrorType = ClassifyError(err)
		}

		// Success
		if err == nil {
			re.metrics.SuccessAttempt = currentAttempt

			if re.circuitBreaker != nil {
				re.circuitBreaker.RecordSuccess()
			}
			return nil
		}
		
		// Record failure
		if re.circuitBreaker != nil {
			re.circuitBreaker.RecordFailure()
		}
		
		// Classify error and check if retryable
		errorType := ClassifyError(err)
		delay, shouldRetry := re.strategy.CalculateDelay(errorType, currentAttempt)
		
		if !shouldRetry {
			return fmt.Errorf("operation failed after %d attempts: %w", currentAttempt, err)
		}
		
		// Wait before retry
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// GetMetrics returns metrics about the last execution
func (re *retryExecutor) GetMetrics() RetryMetrics {
	return re.metrics
}

// GetAttemptCount returns the current attempt count
func (re *retryExecutor) GetAttemptCount() int {
	return re.attemptCount
}

// Reset resets the executor state
func (re *retryExecutor) Reset() {
	re.attemptCount = 0
	re.metrics = RetryMetrics{}

	if re.circuitBreaker != nil {
		re.circuitBreaker.Reset()
	}
}

// circuitBreaker implements a simple circuit breaker pattern
type circuitBreaker struct {
	failureThreshold int
	recoveryTimeout  time.Duration
	failureCount     int
	lastFailureTime  time.Time
	state            circuitState
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// newCircuitBreaker creates a new circuit breaker
func newCircuitBreaker(failureThreshold int, recoveryTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
		state:           circuitClosed,
	}
}

// AllowRequest checks if a request should be allowed
func (cb *circuitBreaker) AllowRequest() bool {
	switch cb.state {
	case circuitClosed:
		return true
	case circuitOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
			cb.state = circuitHalfOpen
			return true
		}
		return false
	case circuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *circuitBreaker) RecordSuccess() {
	cb.failureCount = 0
	cb.state = circuitClosed
}

// RecordFailure records a failed operation
func (cb *circuitBreaker) RecordFailure() {
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= cb.failureThreshold {
		cb.state = circuitOpen
	}
}

// Reset resets the circuit breaker
func (cb *circuitBreaker) Reset() {
	cb.failureCount = 0
	cb.state = circuitClosed
}