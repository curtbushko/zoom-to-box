// Package download provides retry logic tests for enhanced error handling
package download

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

func TestRetryStrategy(t *testing.T) {
	tests := []struct {
		name               string
		config             RetryConfig
		errorType          ErrorType
		attempt            int
		expectedDelay      time.Duration
		expectedShouldRetry bool
	}{
		{
			name: "exponential backoff for network error",
			config: RetryConfig{
				MaxAttempts:   3,
				BaseDelay:     100 * time.Millisecond,
				MaxDelay:      5 * time.Second,
				Multiplier:    2.0,
				Jitter:        false,
				RetryableErrors: []ErrorType{ErrorTypeNetwork, ErrorTypeTimeout},
			},
			errorType:          ErrorTypeNetwork,
			attempt:            1,
			expectedDelay:      200 * time.Millisecond, // 100ms * 2^1
			expectedShouldRetry: true,
		},
		{
			name: "max delay cap",
			config: RetryConfig{
				MaxAttempts:   5,
				BaseDelay:     1 * time.Second,
				MaxDelay:      3 * time.Second,
				Multiplier:    2.0,
				Jitter:        false,
				RetryableErrors: []ErrorType{ErrorTypeNetwork},
			},
			errorType:          ErrorTypeNetwork,
			attempt:            3,
			expectedDelay:      3 * time.Second, // Capped at MaxDelay
			expectedShouldRetry: true,
		},
		{
			name: "non-retryable error",
			config: RetryConfig{
				MaxAttempts:   3,
				BaseDelay:     100 * time.Millisecond,
				RetryableErrors: []ErrorType{ErrorTypeNetwork},
			},
			errorType:          ErrorTypeAuth,
			attempt:            1,
			expectedDelay:      0,
			expectedShouldRetry: false,
		},
		{
			name: "max attempts exceeded",
			config: RetryConfig{
				MaxAttempts:   2,
				BaseDelay:     100 * time.Millisecond,
				RetryableErrors: []ErrorType{ErrorTypeNetwork},
			},
			errorType:          ErrorTypeNetwork,
			attempt:            2,
			expectedDelay:      0,
			expectedShouldRetry: false,
		},
		{
			name: "rate limit specific delay",
			config: RetryConfig{
				MaxAttempts:   3,
				BaseDelay:     100 * time.Millisecond,
				RetryableErrors: []ErrorType{ErrorTypeRateLimit},
				RateLimitDelay: 30 * time.Second,
			},
			errorType:          ErrorTypeRateLimit,
			attempt:            1,
			expectedDelay:      30 * time.Second,
			expectedShouldRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := NewRetryStrategy(tt.config)
			
			delay, shouldRetry := strategy.CalculateDelay(tt.errorType, tt.attempt)
			
			if shouldRetry != tt.expectedShouldRetry {
				t.Errorf("Expected shouldRetry %v, got %v", tt.expectedShouldRetry, shouldRetry)
			}
			
			if shouldRetry && delay != tt.expectedDelay {
				t.Errorf("Expected delay %v, got %v", tt.expectedDelay, delay)
			}
		})
	}
}

func TestRetryStrategyWithJitter(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   3,
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      5 * time.Second,
		Multiplier:    2.0,
		Jitter:        true,
		JitterPercent: 25, // ±25%
		RetryableErrors: []ErrorType{ErrorTypeNetwork},
	}

	strategy := NewRetryStrategy(config)
	
	// Test multiple calculations to ensure jitter is applied
	baseExpected := 200 * time.Millisecond // 100ms * 2^1
	minExpected := time.Duration(float64(baseExpected) * 0.75) // -25%
	maxExpected := time.Duration(float64(baseExpected) * 1.25) // +25%
	
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delay, shouldRetry := strategy.CalculateDelay(ErrorTypeNetwork, 1)
		if !shouldRetry {
			t.Error("Should retry network error")
		}
		delays[i] = delay
		
		if delay < minExpected || delay > maxExpected {
			t.Errorf("Delay %v outside jitter range [%v, %v]", delay, minExpected, maxExpected)
		}
	}
	
	// Verify that not all delays are the same (jitter is working)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("All delays are the same, jitter not working")
	}
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedType  ErrorType
	}{
		{
			name:         "network error",
			err:          errors.New("dial tcp: network unreachable"),
			expectedType: ErrorTypeNetwork,
		},
		{
			name:         "timeout error",
			err:          context.DeadlineExceeded,
			expectedType: ErrorTypeTimeout,
		},
		{
			name:         "http 429 rate limit",
			err:          &zoom.HTTPError{StatusCode: 429},
			expectedType: ErrorTypeRateLimit,
		},
		{
			name:         "http 401 auth error",
			err:          &zoom.HTTPError{StatusCode: 401},
			expectedType: ErrorTypeAuth,
		},
		{
			name:         "http 403 forbidden",
			err:          &zoom.HTTPError{StatusCode: 403},
			expectedType: ErrorTypeAuth,
		},
		{
			name:         "http 500 server error",
			err:          &zoom.HTTPError{StatusCode: 500},
			expectedType: ErrorTypeServer,
		},
		{
			name:         "http 502 bad gateway",
			err:          &zoom.HTTPError{StatusCode: 502},
			expectedType: ErrorTypeServer,
		},
		{
			name:         "http 400 client error",
			err:          &zoom.HTTPError{StatusCode: 400},
			expectedType: ErrorTypeClient,
		},
		{
			name:         "unknown error",
			err:          errors.New("unknown error"),
			expectedType: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorType := ClassifyError(tt.err)
			if errorType != tt.expectedType {
				t.Errorf("Expected error type %v, got %v", tt.expectedType, errorType)
			}
		})
	}
}

func TestRetryableErrorStrategies(t *testing.T) {
	config := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		Multiplier:  2.0,
		RetryableErrors: []ErrorType{
			ErrorTypeNetwork,
			ErrorTypeTimeout,
			ErrorTypeServer,
			ErrorTypeRateLimit,
		},
		NetworkErrorDelay:  200 * time.Millisecond,
		TimeoutErrorDelay:  1 * time.Second,
		ServerErrorDelay:   500 * time.Millisecond,
		RateLimitDelay:     30 * time.Second,
	}

	strategy := NewRetryStrategy(config)

	tests := []struct {
		errorType     ErrorType
		expectedDelay time.Duration
	}{
		{ErrorTypeNetwork, 200 * time.Millisecond},
		{ErrorTypeTimeout, 1 * time.Second},
		{ErrorTypeServer, 500 * time.Millisecond},
		{ErrorTypeRateLimit, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(string(tt.errorType), func(t *testing.T) {
			delay, shouldRetry := strategy.CalculateDelay(tt.errorType, 0)
			
			if !shouldRetry {
				t.Errorf("Should retry %v error", tt.errorType)
			}
			
			if delay != tt.expectedDelay {
				t.Errorf("Expected delay %v, got %v", tt.expectedDelay, delay)
			}
		})
	}
}

func TestRetryExecutor(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   3,
		BaseDelay:     10 * time.Millisecond, // Short delay for fast tests
		Multiplier:    2.0,
		RetryableErrors: []ErrorType{ErrorTypeNetwork, ErrorTypeServer},
	}

	t.Run("successful after retry", func(t *testing.T) {
		strategy := NewRetryStrategy(config)
		executor := NewRetryExecutor(strategy)

		attempt := 0
		operation := func() error {
			attempt++
			if attempt < 3 {
				return &zoom.HTTPError{StatusCode: 500} // Server error, retryable
			}
			return nil // Success on 3rd attempt
		}

		err := executor.Execute(context.Background(), operation)
		if err != nil {
			t.Errorf("Expected success after retries, got error: %v", err)
		}

		if attempt != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempt)
		}
	})

	t.Run("non-retryable error", func(t *testing.T) {
		strategy := NewRetryStrategy(config)
		executor := NewRetryExecutor(strategy)

		attempt := 0
		operation := func() error {
			attempt++
			return &zoom.HTTPError{StatusCode: 401} // Auth error, not retryable
		}

		err := executor.Execute(context.Background(), operation)
		if err == nil {
			t.Error("Expected error for non-retryable failure")
		}

		if attempt != 1 {
			t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempt)
		}
	})

	t.Run("max attempts exceeded", func(t *testing.T) {
		strategy := NewRetryStrategy(config)
		executor := NewRetryExecutor(strategy)

		attempt := 0
		operation := func() error {
			attempt++
			return &zoom.HTTPError{StatusCode: 500} // Always fail with retryable error
		}

		err := executor.Execute(context.Background(), operation)
		if err == nil {
			t.Error("Expected error after max attempts")
		}

		if attempt != config.MaxAttempts {
			t.Errorf("Expected %d attempts, got %d", config.MaxAttempts, attempt)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		strategy := NewRetryStrategy(config)
		executor := NewRetryExecutor(strategy)

		ctx, cancel := context.WithCancel(context.Background())
		
		attempt := 0
		operation := func() error {
			attempt++
			if attempt == 2 {
				cancel() // Cancel after first retry
			}
			return &zoom.HTTPError{StatusCode: 500}
		}

		err := executor.Execute(ctx, operation)
		if err == nil {
			t.Error("Expected error due to context cancellation")
		}

		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})
}

func TestRetryMetrics(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:   3,
		BaseDelay:     1 * time.Millisecond,
		Multiplier:    2.0,
		RetryableErrors: []ErrorType{ErrorTypeNetwork, ErrorTypeServer},
	}

	strategy := NewRetryStrategy(config)
	executor := NewRetryExecutor(strategy)

	// Test successful retry with metrics
	attemptCount := 0
	operation := func() error {
		attemptCount++
		// Fail first attempt, succeed second
		if attemptCount == 1 {
			return &zoom.HTTPError{StatusCode: 500}
		}
		return nil
	}

	start := time.Now()
	err := executor.Execute(context.Background(), operation)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	metrics := executor.GetMetrics()
	if metrics.TotalAttempts != 2 {
		t.Errorf("Expected 2 total attempts, got %d", metrics.TotalAttempts)
	}

	if metrics.TotalDuration == 0 {
		t.Error("Expected non-zero total duration")
	}

	if metrics.TotalDuration > duration+10*time.Millisecond {
		t.Errorf("Total duration %v seems too long compared to measured %v", metrics.TotalDuration, duration)
	}

	if metrics.LastError == nil {
		t.Error("Expected last error to be recorded")
	}
}

func TestRetryConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      RetryConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: RetryConfig{
				MaxAttempts: 3,
				BaseDelay:   100 * time.Millisecond,
				Multiplier:  2.0,
				RetryableErrors: []ErrorType{ErrorTypeNetwork},
			},
			expectError: false,
		},
		{
			name: "zero max attempts",
			config: RetryConfig{
				MaxAttempts: 0,
				BaseDelay:   100 * time.Millisecond,
			},
			expectError: true,
		},
		{
			name: "negative base delay",
			config: RetryConfig{
				MaxAttempts: 3,
				BaseDelay:   -100 * time.Millisecond,
			},
			expectError: true,
		},
		{
			name: "invalid multiplier",
			config: RetryConfig{
				MaxAttempts: 3,
				BaseDelay:   100 * time.Millisecond,
				Multiplier:  0.5, // Should be >= 1.0
			},
			expectError: true,
		},
		{
			name: "max delay less than base delay",
			config: RetryConfig{
				MaxAttempts: 3,
				BaseDelay:   1 * time.Second,
				MaxDelay:    500 * time.Millisecond,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRetryConfig(tt.config)
			
			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}
		})
	}
}

func TestCircuitBreakerIntegration(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       3,
		BaseDelay:         1 * time.Millisecond,
		RetryableErrors:   []ErrorType{ErrorTypeNetwork, ErrorTypeServer},
		CircuitBreaker:    true,
		FailureThreshold:  3,
		RecoveryTimeout:   100 * time.Millisecond,
	}

	strategy := NewRetryStrategy(config)
	executor := NewRetryExecutor(strategy)

	// Cause circuit breaker to open by exceeding failure threshold
	failingOperation := func() error {
		return &zoom.HTTPError{StatusCode: 500}
	}

	// Execute failing operations to open circuit
	for i := 0; i < config.FailureThreshold; i++ {
		executor.Execute(context.Background(), failingOperation)
	}

	// Next operation should fail fast due to open circuit
	start := time.Now()
	err := executor.Execute(context.Background(), failingOperation)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error due to open circuit")
	}

	// Should fail fast (no retry delays)
	if duration > 10*time.Millisecond {
		t.Errorf("Circuit breaker should fail fast, took %v", duration)
	}

	// Wait for recovery timeout
	time.Sleep(config.RecoveryTimeout + 10*time.Millisecond)

	// Circuit should now allow attempts again
	successOperation := func() error {
		return nil
	}

	err = executor.Execute(context.Background(), successOperation)
	if err != nil {
		t.Errorf("Expected success after recovery timeout, got %v", err)
	}
}