package progress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/logging"
)

// mockLogger implements the logging.Logger interface for testing
type mockLogger struct {
	logs []logEntry
}

type logEntry struct {
	level   string
	message string
	context context.Context
	fields  map[string]interface{}
}

func (m *mockLogger) Debug(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "debug", message: format})
}

func (m *mockLogger) Info(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "info", message: format})
}

func (m *mockLogger) Warn(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "warn", message: format})
}

func (m *mockLogger) Error(format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "error", message: format})
}

func (m *mockLogger) DebugWithContext(ctx context.Context, format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "debug", message: format, context: ctx})
}

func (m *mockLogger) InfoWithContext(ctx context.Context, format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "info", message: format, context: ctx})
}

func (m *mockLogger) WarnWithContext(ctx context.Context, format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "warn", message: format, context: ctx})
}

func (m *mockLogger) ErrorWithContext(ctx context.Context, format string, args ...interface{}) {
	m.logs = append(m.logs, logEntry{level: "error", message: format, context: ctx})
}

func (m *mockLogger) LogUserAction(action string, user string, metadata map[string]interface{}) {
	m.logs = append(m.logs, logEntry{
		level:   "user_action",
		message: action,
		fields:  map[string]interface{}{"user": user, "metadata": metadata},
	})
}

func (m *mockLogger) LogPerformance(metrics logging.PerformanceMetrics) {
	m.logs = append(m.logs, logEntry{
		level:   "performance",
		message: metrics.Operation,
		fields:  map[string]interface{}{"metrics": metrics},
	})
}

func (m *mockLogger) LogAPIRequest(request logging.APIRequest) {
	m.logs = append(m.logs, logEntry{level: "api_request", message: request.URL})
}

func (m *mockLogger) LogAPIResponse(response logging.APIResponse) {
	m.logs = append(m.logs, logEntry{level: "api_response", message: response.RequestID})
}

func (m *mockLogger) GetLevel() logging.LogLevel { return logging.InfoLevel }
func (m *mockLogger) SetLevel(level logging.LogLevel) {}
func (m *mockLogger) SetOutput(w io.Writer) {}
func (m *mockLogger) Close() error { return nil }

func TestProgressReporter_BasicOperations(t *testing.T) {
	mockLog := &mockLogger{}
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false, // Disable for testing
		UpdateInterval:    10 * time.Millisecond,
		LogInterval:       50 * time.Millisecond,
		Writer:            buffer,
		EnableFileLogging: true,
		CompactMode:       true,
	}
	
	reporter := NewProgressReporter(config, mockLog)
	ctx := context.Background()
	
	// Test Start
	err := reporter.Start(ctx, 5)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	
	// Check initial summary
	summary := reporter.GetSummary()
	if summary.TotalItems != 5 {
		t.Errorf("Expected total items 5, got %d", summary.TotalItems)
	}
	if summary.CompletedDownloads != 0 {
		t.Errorf("Expected 0 completed downloads, got %d", summary.CompletedDownloads)
	}
	
	// Test UpdateDownload
	update := download.ProgressUpdate{
		DownloadID:      "test-1",
		BytesDownloaded: 512,
		TotalBytes:      1024,
		Speed:           100.0,
		ETA:             5 * time.Second,
		State:           download.DownloadStateDownloading,
		Timestamp:       time.Now(),
		Metadata:        map[string]interface{}{"filename": "test-file.mp4"},
	}
	
	reporter.UpdateDownload(update)
	
	// Test completion
	update.State = download.DownloadStateCompleted
	update.BytesDownloaded = 1024
	reporter.UpdateDownload(update)
	
	// Test skip
	reporter.AddSkipped(SkipReasonAlreadyExists, "existing-file.mp4", map[string]interface{}{
		"path": "/path/to/existing-file.mp4",
	})
	
	// Test error
	testErr := errors.New("network timeout")
	reporter.AddError("failed-file.mp4", testErr, map[string]interface{}{
		"url": "https://example.com/failed-file.mp4",
	})
	
	// Test Finish
	finalSummary := reporter.Finish()
	
	// Verify final summary
	if finalSummary.TotalItems != 5 {
		t.Errorf("Expected total items 5, got %d", finalSummary.TotalItems)
	}
	if finalSummary.CompletedDownloads != 1 {
		t.Errorf("Expected 1 completed download, got %d", finalSummary.CompletedDownloads)
	}
	if len(finalSummary.SkippedItems) != 1 {
		t.Errorf("Expected 1 skipped item, got %d", len(finalSummary.SkippedItems))
	}
	if len(finalSummary.ErrorItems) != 1 {
		t.Errorf("Expected 1 error item, got %d", len(finalSummary.ErrorItems))
	}
	if finalSummary.TotalBytesDownloaded != 1024 {
		t.Errorf("Expected 1024 bytes downloaded, got %d", finalSummary.TotalBytesDownloaded)
	}
	
	// Verify skipped item details
	skipped := finalSummary.SkippedItems[0]
	if skipped.Reason != SkipReasonAlreadyExists {
		t.Errorf("Expected skip reason already_exists, got %s", skipped.Reason)
	}
	if skipped.Item != "existing-file.mp4" {
		t.Errorf("Expected skipped item existing-file.mp4, got %s", skipped.Item)
	}
	
	// Verify error item details
	errorItem := finalSummary.ErrorItems[0]
	if errorItem.Item != "failed-file.mp4" {
		t.Errorf("Expected error item failed-file.mp4, got %s", errorItem.Item)
	}
	if errorItem.Error != testErr {
		t.Errorf("Expected error %v, got %v", testErr, errorItem.Error)
	}
	
	// Verify logging occurred
	if len(mockLog.logs) == 0 {
		t.Error("Expected logging to occur, but no logs found")
	}
	
	// Check for specific log types
	hasStartLog := false
	hasDownloadLog := false
	hasSkipLog := false
	hasErrorLog := false
	hasFinishLog := false
	
	for _, log := range mockLog.logs {
		switch log.level {
		case "info":
			if log.message == "Progress tracking started: %d items to process" {
				hasStartLog = true
			}
			if log.message == "Starting download: %s (%s)" {
				hasDownloadLog = true
			}
			if log.message == "Progress tracking completed: %d total, %d completed, %d failed, %d skipped" {
				hasFinishLog = true
			}
		case "warn":
			if log.message == "Skipping item: %s (reason: %s)" {
				hasSkipLog = true
			}
		case "error":
			if log.message == "Error processing item: %s - %v" {
				hasErrorLog = true
			}
		}
	}
	
	if !hasStartLog {
		t.Error("Expected start log not found")
	}
	if !hasDownloadLog {
		t.Error("Expected download log not found")
	}
	if !hasSkipLog {
		t.Error("Expected skip log not found")
	}
	if !hasErrorLog {
		t.Error("Expected error log not found")
	}
	if !hasFinishLog {
		t.Error("Expected finish log not found")
	}
}

func TestProgressReporter_SkipReasons(t *testing.T) {
	tests := []struct {
		reason   SkipReason
		expected string
	}{
		{SkipReasonAlreadyExists, "already_exists"},
		{SkipReasonInactiveUser, "inactive_user"},
		{SkipReasonUnsupportedFile, "unsupported_file"},
		{SkipReasonPermissionDenied, "permission_denied"},
		{SkipReasonMetaOnlyMode, "meta_only_mode"},
	}
	
	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			if test.reason.String() != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, test.reason.String())
			}
		})
	}
}

func TestSummary_GetSkippedByReason(t *testing.T) {
	summary := &Summary{
		SkippedItems: []SkippedItem{
			{Item: "file1.mp4", Reason: SkipReasonAlreadyExists},
			{Item: "file2.mp4", Reason: SkipReasonAlreadyExists},
			{Item: "file3.mp4", Reason: SkipReasonInactiveUser},
			{Item: "file4.mp4", Reason: SkipReasonUnsupportedFile},
			{Item: "file5.mp4", Reason: SkipReasonInactiveUser},
		},
	}
	
	skippedByReason := summary.GetSkippedByReason()
	
	if len(skippedByReason[SkipReasonAlreadyExists]) != 2 {
		t.Errorf("Expected 2 already exists items, got %d", len(skippedByReason[SkipReasonAlreadyExists]))
	}
	if len(skippedByReason[SkipReasonInactiveUser]) != 2 {
		t.Errorf("Expected 2 inactive user items, got %d", len(skippedByReason[SkipReasonInactiveUser]))
	}
	if len(skippedByReason[SkipReasonUnsupportedFile]) != 1 {
		t.Errorf("Expected 1 unsupported file item, got %d", len(skippedByReason[SkipReasonUnsupportedFile]))
	}
	if len(skippedByReason[SkipReasonPermissionDenied]) != 0 {
		t.Errorf("Expected 0 permission denied items, got %d", len(skippedByReason[SkipReasonPermissionDenied]))
	}
}

func TestProgressReporter_ConcurrentUpdates(t *testing.T) {
	mockLog := &mockLogger{}
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false,
		UpdateInterval:    10 * time.Millisecond,
		LogInterval:       50 * time.Millisecond,
		Writer:            buffer,
		EnableFileLogging: false,
	}
	
	reporter := NewProgressReporter(config, mockLog)
	ctx := context.Background()
	
	err := reporter.Start(ctx, 10)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	
	// Simulate concurrent download updates
	numWorkers := 5
	done := make(chan bool, numWorkers)
	
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			downloadID := fmt.Sprintf("download-%d", id)
			
			for progress := int64(0); progress <= 1000; progress += 100 {
				update := download.ProgressUpdate{
					DownloadID:      downloadID,
					BytesDownloaded: progress,
					TotalBytes:      1000,
					Speed:           float64(100 + id*10),
					State:           download.DownloadStateDownloading,
					Timestamp:       time.Now(),
					Metadata:        map[string]interface{}{"filename": fmt.Sprintf("file%d.mp4", id)},
				}
				
				if progress == 1000 {
					update.State = download.DownloadStateCompleted
				}
				
				reporter.UpdateDownload(update)
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}
	
	// Wait for all workers to complete
	for i := 0; i < numWorkers; i++ {
		<-done
	}
	
	summary := reporter.Finish()
	
	// Verify all downloads completed
	if summary.CompletedDownloads != numWorkers {
		t.Errorf("Expected %d completed downloads, got %d", numWorkers, summary.CompletedDownloads)
	}
	if summary.TotalBytesDownloaded != int64(numWorkers*1000) {
		t.Errorf("Expected %d bytes downloaded, got %d", numWorkers*1000, summary.TotalBytesDownloaded)
	}
}

func TestProgressReporter_WithoutLogger(t *testing.T) {
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false,
		Writer:            buffer,
		EnableFileLogging: false,
	}
	
	// Create reporter without logger
	reporter := NewProgressReporter(config, nil)
	ctx := context.Background()
	
	// Should not panic even without logger
	err := reporter.Start(ctx, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	
	update := download.ProgressUpdate{
		DownloadID:      "test",
		BytesDownloaded: 100,
		TotalBytes:      100,
		State:           download.DownloadStateCompleted,
		Timestamp:       time.Now(),
	}
	
	reporter.UpdateDownload(update)
	reporter.AddSkipped(SkipReasonAlreadyExists, "test", nil)
	reporter.AddError("test", errors.New("test error"), nil)
	
	summary := reporter.Finish()
	
	if summary == nil {
		t.Error("Expected summary to be returned even without logger")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d bytes", test.input), func(t *testing.T) {
			result := formatBytes(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{3661 * time.Second, "1h 1m"},
		{7200 * time.Second, "2h 0m"},
	}
	
	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := formatDuration(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestCreateProgressBar(t *testing.T) {
	tests := []struct {
		percent  float64
		width    int
		expected string
	}{
		{0, 10, "░░░░░░░░░░"},
		{50, 10, "█████░░░░░"},
		{100, 10, "██████████"},
		{25, 8, "██░░░░░░"},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("%.0f%% width %d", test.percent, test.width), func(t *testing.T) {
			result := createProgressBar(test.percent, test.width)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

// Benchmark tests
func BenchmarkProgressReporter_UpdateDownload(b *testing.B) {
	mockLog := &mockLogger{}
	config := ProgressConfig{
		ShowProgressBar:   false,
		EnableFileLogging: false,
	}
	
	reporter := NewProgressReporter(config, mockLog)
	ctx := context.Background()
	reporter.Start(ctx, b.N)
	
	update := download.ProgressUpdate{
		DownloadID:      "benchmark-test",
		BytesDownloaded: 100,
		TotalBytes:      1000,
		Speed:           100.0,
		State:           download.DownloadStateDownloading,
		Timestamp:       time.Now(),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		update.BytesDownloaded = int64(i * 10)
		reporter.UpdateDownload(update)
	}
}

func BenchmarkCreateProgressBar(b *testing.B) {
	for i := 0; i < b.N; i++ {
		createProgressBar(float64(i%101), 40)
	}
}

func BenchmarkFormatBytes(b *testing.B) {
	values := []int64{512, 1024, 1048576, 1073741824}
	for i := 0; i < b.N; i++ {
		formatBytes(values[i%len(values)])
	}
}