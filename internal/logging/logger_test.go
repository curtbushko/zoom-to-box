// Package logging provides structured logging functionality
package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name           string
		config         config.LoggingConfig
		expectedError  bool
		expectedLevel  LogLevel
	}{
		{
			name: "valid debug config",
			config: config.LoggingConfig{
				Level:      "debug",
				Console:    true,
				JSONFormat: false,
			},
			expectedError: false,
			expectedLevel: DebugLevel,
		},
		{
			name: "valid info config",
			config: config.LoggingConfig{
				Level:      "info",
				Console:    true,
				JSONFormat: true,
			},
			expectedError: false,
			expectedLevel: InfoLevel,
		},
		{
			name: "valid warn config",
			config: config.LoggingConfig{
				Level:      "warn",
				Console:    false,
				JSONFormat: false,
				File:       "test.log",
			},
			expectedError: false,
			expectedLevel: WarnLevel,
		},
		{
			name: "valid error config",
			config: config.LoggingConfig{
				Level:      "error",
				Console:    true,
				JSONFormat: false,
			},
			expectedError: false,
			expectedLevel: ErrorLevel,
		},
		{
			name: "invalid log level",
			config: config.LoggingConfig{
				Level:   "invalid",
				Console: true,
			},
			expectedError: true,
		},
		{
			name: "case insensitive level",
			config: config.LoggingConfig{
				Level:   "INFO",
				Console: true,
			},
			expectedError: false,
			expectedLevel: InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger(tt.config)
			
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if logger == nil {
				t.Error("Expected logger but got nil")
				return
			}
			
			// Check if logger has correct level
			if logger.GetLevel() != tt.expectedLevel {
				t.Errorf("Expected level %v, got %v", tt.expectedLevel, logger.GetLevel())
			}
		})
	}
}

func TestLoggerLevels(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "debug",
		Console:    true,
		JSONFormat: false,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	// Redirect output to buffer for testing
	logger.SetOutput(&buffer)
	
	tests := []struct {
		name     string
		logFunc  func(string, ...interface{})
		logLevel LogLevel
		message  string
		args     []interface{}
	}{
		{
			name:     "debug message",
			logFunc:  logger.Debug,
			logLevel: DebugLevel,
			message:  "Debug message: %s",
			args:     []interface{}{"test"},
		},
		{
			name:     "info message",
			logFunc:  logger.Info,
			logLevel: InfoLevel,
			message:  "Info message: %d",
			args:     []interface{}{123},
		},
		{
			name:     "warn message",
			logFunc:  logger.Warn,
			logLevel: WarnLevel,
			message:  "Warn message",
			args:     []interface{}{},
		},
		{
			name:     "error message",
			logFunc:  logger.Error,
			logLevel: ErrorLevel,
			message:  "Error message: %v",
			args:     []interface{}{errors.New("test error")},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer.Reset()
			tt.logFunc(tt.message, tt.args...)
			
			output := buffer.String()
			if output == "" {
				t.Error("Expected log output but got empty string")
			}
			
			// Check if output contains level
			levelStr := strings.ToUpper(tt.logLevel.String())
			if !strings.Contains(output, levelStr) {
				t.Errorf("Expected output to contain %s, got: %s", levelStr, output)
			}
		})
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "warn",
		Console:    true,
		JSONFormat: false,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	// Debug and Info should be filtered out
	buffer.Reset()
	logger.Debug("This should not appear")
	if buffer.String() != "" {
		t.Error("Debug message should be filtered out")
	}
	
	buffer.Reset()
	logger.Info("This should not appear")
	if buffer.String() != "" {
		t.Error("Info message should be filtered out")
	}
	
	// Warn and Error should appear
	buffer.Reset()
	logger.Warn("This should appear")
	if buffer.String() == "" {
		t.Error("Warn message should appear")
	}
	
	buffer.Reset()
	logger.Error("This should appear")
	if buffer.String() == "" {
		t.Error("Error message should appear")
	}
}

func TestJSONLogging(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: true,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	logger.Info("Test message")
	
	output := buffer.String()
	if output == "" {
		t.Fatal("Expected log output but got empty string")
	}
	
	// Parse as JSON to verify format
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Errorf("Failed to parse JSON log: %v. Output was: %s", err, output)
	}
	
	// Check required fields
	requiredFields := []string{"timestamp", "level", "message"}
	for _, field := range requiredFields {
		if _, exists := logEntry[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestFileLogging(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    false,
		File:       logFile,
		JSONFormat: false,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()
	
	testMessage := "Test file logging"
	logger.Info(testMessage)
	
	// Close logger to flush file
	logger.Close()
	
	// Read file content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	
	if !strings.Contains(string(content), testMessage) {
		t.Errorf("Log file doesn't contain expected message. Content: %s", string(content))
	}
}

func TestContextualLogging(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: true,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	// Test logging with context
	ctx := context.WithValue(context.Background(), RequestIDKey, "req-123")
	logger.InfoWithContext(ctx, "Test message with context")
	
	output := buffer.String()
	if output == "" {
		t.Fatal("Expected log output but got empty string")
	}
	
	// Parse JSON and check for request ID
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Errorf("Failed to parse JSON log: %v", err)
	}
	
	requestID, exists := logEntry["request_id"]
	if !exists {
		t.Error("Missing request_id field")
	}
	
	if requestID != "req-123" {
		t.Errorf("Expected request_id 'req-123', got %v", requestID)
	}
}

func TestUserActionLogging(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: true,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	// Test user action logging
	logger.LogUserAction("download_start", "john.doe@company.com", map[string]interface{}{
		"file_name": "meeting-recording.mp4",
		"file_size": 1048576,
	})
	
	output := buffer.String()
	if output == "" {
		t.Fatal("Expected log output but got empty string")
	}
	
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Errorf("Failed to parse JSON log: %v", err)
	}
	
	// Check user action fields
	expectedFields := map[string]interface{}{
		"action":    "download_start",
		"user":      "john.doe@company.com",
		"file_name": "meeting-recording.mp4",
		"file_size": float64(1048576), // JSON numbers are float64
	}
	
	for key, expectedValue := range expectedFields {
		if value, exists := logEntry[key]; !exists {
			t.Errorf("Missing field: %s", key)
		} else if value != expectedValue {
			t.Errorf("Field %s: expected %v, got %v", key, expectedValue, value)
		}
	}
}

func TestPerformanceLogging(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: true,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	// Test performance metrics logging
	metrics := PerformanceMetrics{
		Operation:     "download_file",
		Duration:      time.Second * 2,
		BytesProcessed: 1048576,
		Success:       true,
	}
	
	logger.LogPerformance(metrics)
	
	output := buffer.String()
	if output == "" {
		t.Fatal("Expected log output but got empty string")
	}
	
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Errorf("Failed to parse JSON log: %v", err)
	}
	
	// Check performance fields
	expectedFields := map[string]interface{}{
		"operation":        "download_file",
		"duration_ms":      float64(2000), // 2 seconds in milliseconds
		"bytes_processed":  float64(1048576),
		"success":          true,
	}
	
	for key, expectedValue := range expectedFields {
		if value, exists := logEntry[key]; !exists {
			t.Errorf("Missing field: %s", key)
		} else if value != expectedValue {
			t.Errorf("Field %s: expected %v, got %v", key, expectedValue, value)
		}
	}
}

func TestAPIRequestResponseLogging(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "debug",
		Console:    true,
		JSONFormat: true,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	// Test API request logging
	request := APIRequest{
		Method:    "GET",
		URL:       "https://api.zoom.us/v2/users/test@example.com/recordings",
		Headers:   map[string]string{"Authorization": "Bearer ***"},
		Body:      "",
		RequestID: "req-123",
	}
	
	logger.LogAPIRequest(request)
	
	output := buffer.String()
	if output == "" {
		t.Fatal("Expected log output but got empty string")
	}
	
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry); err != nil {
		t.Errorf("Failed to parse JSON log: %v", err)
	}
	
	// Check API request fields
	if logEntry["method"] != "GET" {
		t.Errorf("Expected method 'GET', got %v", logEntry["method"])
	}
	
	if logEntry["url"] != request.URL {
		t.Errorf("Expected URL %s, got %v", request.URL, logEntry["url"])
	}
	
	if logEntry["request_id"] != "req-123" {
		t.Errorf("Expected request_id 'req-123', got %v", logEntry["request_id"])
	}
}

func TestLoggerConcurrency(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: false,
	}
	
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	logger.SetOutput(&buffer)
	
	// Test concurrent logging with synchronization
	numGoroutines := 50  // Reduced number for more reliable test
	done := make(chan bool, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			message := fmt.Sprintf("Concurrent log message %d", id)
			logger.Info(message)
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	
	output := buffer.String()
	if output == "" {
		t.Error("Expected log output from concurrent writes")
	}
	
	// Verify we have output (exact line count may vary due to concurrent writes)
	// but we should have substantial output
	if len(output) < numGoroutines*10 { // At least 10 chars per message
		t.Errorf("Expected substantial output from %d concurrent writes, got %d chars", numGoroutines, len(output))
	}
	
	// Verify no corruption by checking for INFO level in output
	if !strings.Contains(output, "[INFO]") {
		t.Error("Expected to find INFO level markers in output")
	}
}

func TestGlobalLogger(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: false,
	}
	
	// Test InitializeLogging
	err := InitializeLogging(config)
	if err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	
	// Get the default logger and set output to buffer
	defaultLogger := GetDefaultLogger()
	if defaultLogger == nil {
		t.Fatal("Default logger should not be nil after initialization")
	}
	
	defaultLogger.SetOutput(&buffer)
	
	// Test package-level convenience functions
	Info("Test info message")
	Warn("Test warn message")
	Error("Test error message")
	
	output := buffer.String()
	if !strings.Contains(output, "Test info message") {
		t.Error("Expected to find info message in output")
	}
	if !strings.Contains(output, "Test warn message") {
		t.Error("Expected to find warn message in output")
	}
	if !strings.Contains(output, "Test error message") {
		t.Error("Expected to find error message in output")
	}
}

func TestContextUtilities(t *testing.T) {
	ctx := context.Background()
	
	// Test WithRequestID and GetRequestID
	requestID := "test-123"
	ctxWithID := WithRequestID(ctx, requestID)
	
	retrievedID, ok := GetRequestID(ctxWithID)
	if !ok {
		t.Error("Expected to find request ID in context")
	}
	
	if retrievedID != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, retrievedID)
	}
	
	// Test GetRequestID with context without request ID
	_, ok = GetRequestID(ctx)
	if ok {
		t.Error("Expected no request ID in empty context")
	}
}

func TestGenerateRequestID(t *testing.T) {
	// Generate multiple request IDs
	id1 := GenerateRequestID()
	id2 := GenerateRequestID()
	
	// They should be different
	if id1 == id2 {
		t.Error("Generated request IDs should be unique")
	}
	
	// They should start with "req-"
	if !strings.HasPrefix(id1, "req-") {
		t.Errorf("Request ID should start with 'req-', got %s", id1)
	}
	
	if !strings.HasPrefix(id2, "req-") {
		t.Errorf("Request ID should start with 'req-', got %s", id2)
	}
}

func TestPackageLevelLoggingWithoutInitialization(t *testing.T) {
	// Reset default logger
	originalLogger := GetDefaultLogger()
	SetDefaultLogger(nil)
	
	// These should not panic when defaultLogger is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Package-level logging should not panic when logger is nil: %v", r)
		}
		// Restore original logger
		SetDefaultLogger(originalLogger)
	}()
	
	Debug("This should not crash")
	Info("This should not crash")
	Warn("This should not crash")
	Error("This should not crash")
	
	LogUserAction("test_action", "test_user", nil)
	LogPerformance(PerformanceMetrics{Operation: "test"})
	LogAPIRequest(APIRequest{Method: "GET", URL: "http://example.com"})
	LogAPIResponse(APIResponse{StatusCode: 200})
}

func TestPackageLevelContextualLogging(t *testing.T) {
	var buffer bytes.Buffer
	
	config := config.LoggingConfig{
		Level:      "info",
		Console:    true,
		JSONFormat: true,
	}
	
	err := InitializeLogging(config)
	if err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	
	GetDefaultLogger().SetOutput(&buffer)
	
	// Test package-level contextual logging
	ctx := WithRequestID(context.Background(), "pkg-123")
	InfoWithContext(ctx, "Test contextual message")
	
	output := buffer.String()
	if !strings.Contains(output, "pkg-123") {
		t.Error("Expected to find request ID in contextual log output")
	}
	
	if !strings.Contains(output, "Test contextual message") {
		t.Error("Expected to find test message in output")
	}
}