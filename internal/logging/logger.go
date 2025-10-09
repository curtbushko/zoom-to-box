// Package logging provides structured logging functionality for zoom-to-box
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

// LogLevel represents the severity level of a log entry
type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	default:
		return "unknown"
	}
}

// RequestIDKey is the context key for request IDs
type contextKey string

const RequestIDKey contextKey = "request_id"

// Logger defines the interface for logging operations
type Logger interface {
	// Basic logging methods
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	
	// Contextual logging methods
	DebugWithContext(ctx context.Context, format string, args ...interface{})
	InfoWithContext(ctx context.Context, format string, args ...interface{})
	WarnWithContext(ctx context.Context, format string, args ...interface{})
	ErrorWithContext(ctx context.Context, format string, args ...interface{})
	
	// Specialized logging methods
	LogUserAction(action string, user string, metadata map[string]interface{})
	LogPerformance(metrics PerformanceMetrics)
	LogAPIRequest(request APIRequest)
	LogAPIResponse(response APIResponse)
	
	// Configuration and control methods
	GetLevel() LogLevel
	SetLevel(level LogLevel)
	SetOutput(w io.Writer)
	Close() error
}

// PerformanceMetrics represents performance data for logging
type PerformanceMetrics struct {
	Operation      string        `json:"operation"`
	Duration       time.Duration `json:"-"`
	BytesProcessed int64         `json:"bytes_processed"`
	Success        bool          `json:"success"`
	Error          string        `json:"error,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// APIRequest represents API request data for logging
type APIRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	RequestID string            `json:"request_id"`
	Timestamp time.Time         `json:"timestamp"`
}

// APIResponse represents API response data for logging
type APIResponse struct {
	StatusCode    int               `json:"status_code"`
	Headers       map[string]string `json:"headers,omitempty"`
	Body          string            `json:"body,omitempty"`
	RequestID     string            `json:"request_id"`
	Duration      time.Duration     `json:"-"`
	Timestamp     time.Time         `json:"timestamp"`
	Success       bool              `json:"success"`
	Error         string            `json:"error,omitempty"`
}

// loggerImpl implements the Logger interface
type loggerImpl struct {
	level      LogLevel
	jsonFormat bool
	writers    []io.Writer
	fileHandle *os.File
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	RequestID string                 `json:"request_id,omitempty"`
	Fields    map[string]interface{} `json:",inline,omitempty"`
}

// NewLogger creates a new Logger instance with the given configuration
func NewLogger(config config.LoggingConfig) (Logger, error) {
	level, err := parseLogLevel(config.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}
	
	logger := &loggerImpl{
		level:      level,
		jsonFormat: config.JSONFormat,
		writers:    []io.Writer{},
	}
	
	// Add console writer if enabled
	if config.Console {
		logger.writers = append(logger.writers, os.Stdout)
	}
	
	// Add file writer if configured
	if config.File != "" {
		file, err := os.OpenFile(config.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file %s: %w", config.File, err)
		}
		logger.fileHandle = file
		logger.writers = append(logger.writers, file)
	}
	
	return logger, nil
}

// parseLogLevel converts a string to LogLevel
func parseLogLevel(level string) (LogLevel, error) {
	switch strings.ToLower(level) {
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	default:
		return InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}
}

// log writes a log entry with the specified level and message
func (l *loggerImpl) log(level LogLevel, ctx context.Context, format string, args ...interface{}) {
	if level < l.level {
		return // Skip if level is below threshold
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     strings.ToUpper(level.String()),
		Message:   fmt.Sprintf(format, args...),
	}

	// Add request ID if available in context
	if ctx != nil {
		if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
			entry.RequestID = requestID
		}
	}

	l.writeEntry(entry)
}

// writeEntry writes a log entry to all configured writers
func (l *loggerImpl) writeEntry(entry LogEntry) {
	var output string
	
	if l.jsonFormat {
		data, _ := json.Marshal(entry)
		output = string(data) + "\n"
	} else {
		timestamp := entry.Timestamp.Format("2006-01-02T15:04:05Z")
		if entry.RequestID != "" {
			output = fmt.Sprintf("%s [%s] [%s] %s\n", timestamp, entry.Level, entry.RequestID, entry.Message)
		} else {
			output = fmt.Sprintf("%s [%s] %s\n", timestamp, entry.Level, entry.Message)
		}
	}
	
	for _, writer := range l.writers {
		writer.Write([]byte(output))
	}
}

// writeStructuredEntry writes a structured log entry with additional fields
func (l *loggerImpl) writeStructuredEntry(level LogLevel, message string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     strings.ToUpper(level.String()),
		Message:   message,
		Fields:    fields,
	}
	
	var output string
	
	if l.jsonFormat {
		// Flatten the fields into the entry for JSON format
		entryMap := map[string]interface{}{
			"timestamp": entry.Timestamp,
			"level":     entry.Level,
			"message":   entry.Message,
		}
		
		for key, value := range fields {
			entryMap[key] = value
		}
		
		data, _ := json.Marshal(entryMap)
		output = string(data) + "\n"
	} else {
		// For text format, include key fields in the message
		timestamp := entry.Timestamp.Format("2006-01-02T15:04:05Z")
		fieldStr := ""
		if len(fields) > 0 {
			var pairs []string
			for key, value := range fields {
				pairs = append(pairs, fmt.Sprintf("%s=%v", key, value))
			}
			fieldStr = " " + strings.Join(pairs, " ")
		}
		output = fmt.Sprintf("%s [%s] %s%s\n", timestamp, entry.Level, message, fieldStr)
	}
	
	for _, writer := range l.writers {
		writer.Write([]byte(output))
	}
}

// Debug logs a debug message
func (l *loggerImpl) Debug(format string, args ...interface{}) {
	l.log(DebugLevel, nil, format, args...)
}

// Info logs an info message
func (l *loggerImpl) Info(format string, args ...interface{}) {
	l.log(InfoLevel, nil, format, args...)
}

// Warn logs a warning message
func (l *loggerImpl) Warn(format string, args ...interface{}) {
	l.log(WarnLevel, nil, format, args...)
}

// Error logs an error message
func (l *loggerImpl) Error(format string, args ...interface{}) {
	l.log(ErrorLevel, nil, format, args...)
}

// DebugWithContext logs a debug message with context
func (l *loggerImpl) DebugWithContext(ctx context.Context, format string, args ...interface{}) {
	l.log(DebugLevel, ctx, format, args...)
}

// InfoWithContext logs an info message with context
func (l *loggerImpl) InfoWithContext(ctx context.Context, format string, args ...interface{}) {
	l.log(InfoLevel, ctx, format, args...)
}

// WarnWithContext logs a warning message with context
func (l *loggerImpl) WarnWithContext(ctx context.Context, format string, args ...interface{}) {
	l.log(WarnLevel, ctx, format, args...)
}

// ErrorWithContext logs an error message with context
func (l *loggerImpl) ErrorWithContext(ctx context.Context, format string, args ...interface{}) {
	l.log(ErrorLevel, ctx, format, args...)
}

// LogUserAction logs user actions with metadata
func (l *loggerImpl) LogUserAction(action string, user string, metadata map[string]interface{}) {
	fields := map[string]interface{}{
		"action": action,
		"user":   user,
	}
	
	// Add metadata fields
	for key, value := range metadata {
		fields[key] = value
	}
	
	l.writeStructuredEntry(InfoLevel, fmt.Sprintf("User action: %s", action), fields)
}

// LogPerformance logs performance metrics
func (l *loggerImpl) LogPerformance(metrics PerformanceMetrics) {
	fields := map[string]interface{}{
		"operation":       metrics.Operation,
		"duration_ms":     metrics.Duration.Milliseconds(),
		"bytes_processed": metrics.BytesProcessed,
		"success":         metrics.Success,
	}
	
	if metrics.Error != "" {
		fields["error"] = metrics.Error
	}
	
	// Add metadata fields
	for key, value := range metrics.Metadata {
		fields[key] = value
	}
	
	message := fmt.Sprintf("Performance: %s completed in %v", metrics.Operation, metrics.Duration)
	l.writeStructuredEntry(InfoLevel, message, fields)
}

// LogAPIRequest logs API requests
func (l *loggerImpl) LogAPIRequest(request APIRequest) {
	// Set timestamp if not provided
	if request.Timestamp.IsZero() {
		request.Timestamp = time.Now().UTC()
	}
	
	fields := map[string]interface{}{
		"method":     request.Method,
		"url":        request.URL,
		"request_id": request.RequestID,
		"timestamp":  request.Timestamp,
	}
	
	// Add headers if present (but sanitize sensitive ones)
	if len(request.Headers) > 0 {
		sanitizedHeaders := make(map[string]string)
		for key, value := range request.Headers {
			if strings.ToLower(key) == "authorization" {
				sanitizedHeaders[key] = "***"
			} else {
				sanitizedHeaders[key] = value
			}
		}
		fields["headers"] = sanitizedHeaders
	}
	
	// Add body if present (truncated for large bodies)
	if request.Body != "" {
		if len(request.Body) > 1000 {
			fields["body"] = request.Body[:1000] + "... (truncated)"
		} else {
			fields["body"] = request.Body
		}
	}
	
	message := fmt.Sprintf("API Request: %s %s", request.Method, request.URL)
	l.writeStructuredEntry(DebugLevel, message, fields)
}

// LogAPIResponse logs API responses
func (l *loggerImpl) LogAPIResponse(response APIResponse) {
	// Set timestamp if not provided
	if response.Timestamp.IsZero() {
		response.Timestamp = time.Now().UTC()
	}
	
	fields := map[string]interface{}{
		"status_code": response.StatusCode,
		"request_id":  response.RequestID,
		"duration_ms": response.Duration.Milliseconds(),
		"timestamp":   response.Timestamp,
		"success":     response.Success,
	}
	
	if response.Error != "" {
		fields["error"] = response.Error
	}
	
	// Add headers if present
	if len(response.Headers) > 0 {
		fields["headers"] = response.Headers
	}
	
	// Add body if present (truncated for large bodies)
	if response.Body != "" {
		if len(response.Body) > 1000 {
			fields["body"] = response.Body[:1000] + "... (truncated)"
		} else {
			fields["body"] = response.Body
		}
	}
	
	message := fmt.Sprintf("API Response: %d (%v)", response.StatusCode, response.Duration)
	l.writeStructuredEntry(DebugLevel, message, fields)
}

// GetLevel returns the current log level
func (l *loggerImpl) GetLevel() LogLevel {
	return l.level
}

// SetLevel sets the log level
func (l *loggerImpl) SetLevel(level LogLevel) {
	l.level = level
}

// SetOutput sets the output writer (mainly for testing)
func (l *loggerImpl) SetOutput(w io.Writer) {
	l.writers = []io.Writer{w}
}

// Close closes the logger and any open file handles
func (l *loggerImpl) Close() error {
	if l.fileHandle != nil {
		return l.fileHandle.Close()
	}
	return nil
}

// Global logger instance for package-level convenience functions
var defaultLogger Logger

// SetDefaultLogger sets the global default logger
func SetDefaultLogger(logger Logger) {
	defaultLogger = logger
}

// GetDefaultLogger returns the global default logger
func GetDefaultLogger() Logger {
	return defaultLogger
}

// InitializeLogging initializes the global logger with the provided configuration
func InitializeLogging(config config.LoggingConfig) error {
	logger, err := NewLogger(config)
	if err != nil {
		return err
	}
	
	SetDefaultLogger(logger)
	return nil
}

// Package-level convenience functions that use the default logger
// These functions are thread-safe and can be used throughout the application

// Debug logs a debug message using the default logger
func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, args...)
	}
}

// Info logs an info message using the default logger
func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, args...)
	}
}

// Warn logs a warning message using the default logger
func Warn(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(format, args...)
	}
}

// Error logs an error message using the default logger
func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, args...)
	}
}

// DebugWithContext logs a debug message with context using the default logger
func DebugWithContext(ctx context.Context, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.DebugWithContext(ctx, format, args...)
	}
}

// InfoWithContext logs an info message with context using the default logger
func InfoWithContext(ctx context.Context, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.InfoWithContext(ctx, format, args...)
	}
}

// WarnWithContext logs a warning message with context using the default logger
func WarnWithContext(ctx context.Context, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.WarnWithContext(ctx, format, args...)
	}
}

// ErrorWithContext logs an error message with context using the default logger
func ErrorWithContext(ctx context.Context, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.ErrorWithContext(ctx, format, args...)
	}
}

// LogUserAction logs user actions using the default logger
func LogUserAction(action string, user string, metadata map[string]interface{}) {
	if defaultLogger != nil {
		defaultLogger.LogUserAction(action, user, metadata)
	}
}

// LogPerformance logs performance metrics using the default logger
func LogPerformance(metrics PerformanceMetrics) {
	if defaultLogger != nil {
		defaultLogger.LogPerformance(metrics)
	}
}

// LogAPIRequest logs API requests using the default logger
func LogAPIRequest(request APIRequest) {
	if defaultLogger != nil {
		defaultLogger.LogAPIRequest(request)
	}
}

// LogAPIResponse logs API responses using the default logger
func LogAPIResponse(response APIResponse) {
	if defaultLogger != nil {
		defaultLogger.LogAPIResponse(response)
	}
}

// WithRequestID creates a context with a request ID
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID extracts the request ID from a context
func GetRequestID(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(RequestIDKey).(string)
	return requestID, ok
}

// GenerateRequestID generates a simple request ID (timestamp-based)
// For production use, consider using a more robust UUID library
func GenerateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}