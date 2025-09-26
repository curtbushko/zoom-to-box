package progress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
)

// mockDownloadManager implements the download.DownloadManager interface for testing
type mockDownloadManager struct {
	downloads map[string]*mockDownloadStatus
	results   map[string]*download.DownloadResult
	errors    map[string]error
}

type mockDownloadStatus struct {
	request download.DownloadRequest
	active  bool
}

func newMockDownloadManager() *mockDownloadManager {
	return &mockDownloadManager{
		downloads: make(map[string]*mockDownloadStatus),
		results:   make(map[string]*download.DownloadResult),
		errors:    make(map[string]error),
	}
}

func (m *mockDownloadManager) Download(ctx context.Context, req download.DownloadRequest, progressCallback download.ProgressCallback) (*download.DownloadResult, error) {
	// Store download
	m.downloads[req.ID] = &mockDownloadStatus{
		request: req,
		active:  true,
	}

	// Simulate progress updates
	if progressCallback != nil {
		// Start
		progressCallback(download.ProgressUpdate{
			DownloadID:      req.ID,
			BytesDownloaded: 0,
			TotalBytes:      req.FileSize,
			Speed:           0,
			State:           download.DownloadStateDownloading,
			Timestamp:       time.Now(),
			Metadata:        req.Metadata,
		})

		// Progress
		progressCallback(download.ProgressUpdate{
			DownloadID:      req.ID,
			BytesDownloaded: req.FileSize / 2,
			TotalBytes:      req.FileSize,
			Speed:           100.0,
			State:           download.DownloadStateDownloading,
			Timestamp:       time.Now(),
			Metadata:        req.Metadata,
		})

		// Complete or fail
		if err, hasError := m.errors[req.ID]; hasError {
			progressCallback(download.ProgressUpdate{
				DownloadID:      req.ID,
				BytesDownloaded: req.FileSize / 2,
				TotalBytes:      req.FileSize,
				State:           download.DownloadStateFailed,
				Error:           err,
				Timestamp:       time.Now(),
				Metadata:        req.Metadata,
			})
			return nil, err
		} else {
			progressCallback(download.ProgressUpdate{
				DownloadID:      req.ID,
				BytesDownloaded: req.FileSize,
				TotalBytes:      req.FileSize,
				State:           download.DownloadStateCompleted,
				Timestamp:       time.Now(),
				Metadata:        req.Metadata,
			})
		}
	}

	// Mark as inactive
	m.downloads[req.ID].active = false

	// Return result or error
	if err, hasError := m.errors[req.ID]; hasError {
		return nil, err
	}

	result := &download.DownloadResult{
		DownloadID:      req.ID,
		BytesDownloaded: req.FileSize,
		Duration:        100 * time.Millisecond,
		AverageSpeed:    float64(req.FileSize) / 0.1,
		Resumed:         false,
		RetryCount:      0,
		Success:         true,
		Error:           nil,
		Metadata:        req.Metadata,
		Timestamp:       time.Now(),
	}

	m.results[req.ID] = result
	return result, nil
}

func (m *mockDownloadManager) GetActiveDownloads() []download.DownloadStatus {
	var active []download.DownloadStatus
	for _, status := range m.downloads {
		if status.active {
			active = append(active, download.DownloadStatus{
				Request:   status.request,
				StartTime: time.Now(),
			})
		}
	}
	return active
}

func (m *mockDownloadManager) CancelDownload(downloadID string) error {
	if status, exists := m.downloads[downloadID]; exists {
		status.active = false
		return nil
	}
	return errors.New("download not found")
}

func (m *mockDownloadManager) SetError(downloadID string, err error) {
	m.errors[downloadID] = err
}

func TestDownloadProgressTracker_Success(t *testing.T) {
	// Setup
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false,
		EnableFileLogging: false,
		Writer:            buffer,
	}
	
	reporter := NewProgressReporter(config, nil) // No logger for test
	downloadMgr := newMockDownloadManager()
	tracker := NewDownloadProgressTracker(reporter, downloadMgr, nil)
	
	ctx := context.Background()
	reporter.Start(ctx, 1)
	
	// Test successful download
	req := download.DownloadRequest{
		ID:          "test-download-1",
		URL:         "https://example.com/file.mp4",
		Destination: "/tmp/file.mp4",
		FileSize:    1024,
		Metadata:    map[string]interface{}{"user": "test@example.com"},
	}
	
	result, err := tracker.StartDownloadWithProgress(ctx, req)
	
	// Verify success
	if err != nil {
		t.Fatalf("Expected successful download, got error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected download result, got nil")
	}
	if !result.Success {
		t.Error("Expected successful result")
	}
	if result.BytesDownloaded != 1024 {
		t.Errorf("Expected 1024 bytes downloaded, got %d", result.BytesDownloaded)
	}
	
	// Verify filename was added to metadata
	if filename, ok := req.Metadata["filename"]; !ok || filename != "file.mp4" {
		t.Errorf("Expected filename in metadata, got %v", req.Metadata["filename"])
	}
	
	// Check progress tracking
	summary := reporter.GetSummary()
	if summary.CompletedDownloads != 1 {
		t.Errorf("Expected 1 completed download, got %d", summary.CompletedDownloads)
	}
	if summary.FailedDownloads != 0 {
		t.Errorf("Expected 0 failed downloads, got %d", summary.FailedDownloads)
	}
}

func TestDownloadProgressTracker_Failure(t *testing.T) {
	// Setup
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false,
		EnableFileLogging: false,
		Writer:            buffer,
	}
	
	reporter := NewProgressReporter(config, nil)
	downloadMgr := newMockDownloadManager()
	tracker := NewDownloadProgressTracker(reporter, downloadMgr, nil)
	
	ctx := context.Background()
	reporter.Start(ctx, 1)
	
	// Setup download to fail
	downloadID := "test-download-fail"
	expectedError := errors.New("network timeout")
	downloadMgr.SetError(downloadID, expectedError)
	
	req := download.DownloadRequest{
		ID:          downloadID,
		URL:         "https://example.com/fail.mp4",
		Destination: "/tmp/fail.mp4",
		FileSize:    1024,
	}
	
	result, err := tracker.StartDownloadWithProgress(ctx, req)
	
	// Verify failure
	if err == nil {
		t.Fatal("Expected download to fail, but got no error")
	}
	if err != expectedError {
		t.Errorf("Expected specific error %v, got %v", expectedError, err)
	}
	if result != nil {
		t.Error("Expected nil result on failure")
	}
	
	// Check that error was added to progress tracking
	summary := reporter.GetSummary()
	if len(summary.ErrorItems) != 1 {
		t.Errorf("Expected 1 error item, got %d", len(summary.ErrorItems))
	}
	
	errorItem := summary.ErrorItems[0]
	if errorItem.Item != "fail.mp4" {
		t.Errorf("Expected error item 'fail.mp4', got '%s'", errorItem.Item)
	}
	if errorItem.Error != expectedError {
		t.Errorf("Expected error %v, got %v", expectedError, errorItem.Error)
	}
}

func TestProgressConfigBuilder(t *testing.T) {
	builder := NewProgressConfigBuilder()
	
	config := builder.
		WithVerbose(true).
		WithCompactMode(true).
		WithSpeedDisplay(false).
		WithETADisplay(false).
		WithFileLogging(false).
		WithLogInterval(10 * time.Second).
		Build()
	
	if !config.CompactMode {
		t.Error("Expected compact mode to be enabled")
	}
	if config.ShowSpeed {
		t.Error("Expected speed display to be disabled")
	}
	if config.ShowETA {
		t.Error("Expected ETA display to be disabled")
	}
	if config.EnableFileLogging {
		t.Error("Expected file logging to be disabled")
	}
	if config.LogInterval != 10*time.Second {
		t.Errorf("Expected log interval 10s, got %v", config.LogInterval)
	}
	if !config.ShowProgressBar {
		t.Error("Expected progress bar to be enabled by default")
	}
	if config.UpdateInterval != 500*time.Millisecond {
		t.Errorf("Expected update interval 500ms, got %v", config.UpdateInterval)
	}
}

func TestProgressBarConfigBuilder(t *testing.T) {
	builder := NewProgressBarConfigBuilder()
	
	config := builder.
		WithWidth(60).
		WithSpeedDisplay(false).
		WithETADisplay(false).
		WithElapsedDisplay(true).
		WithUnits("files").
		WithSpeedUnits("files/s").
		Build()
	
	if config.Width != 60 {
		t.Errorf("Expected width 60, got %d", config.Width)
	}
	if config.ShowSpeed {
		t.Error("Expected speed display to be disabled")
	}
	if config.ShowETA {
		t.Error("Expected ETA display to be disabled")
	}
	if !config.ShowElapsed {
		t.Error("Expected elapsed display to be enabled")
	}
	if config.Units != "files" {
		t.Errorf("Expected units 'files', got '%s'", config.Units)
	}
	if config.SpeedUnits != "files/s" {
		t.Errorf("Expected speed units 'files/s', got '%s'", config.SpeedUnits)
	}
	if !config.ShowPercent {
		t.Error("Expected percent to be shown by default")
	}
	if config.RefreshInterval != 100*time.Millisecond {
		t.Errorf("Expected refresh interval 100ms, got %v", config.RefreshInterval)
	}
}

func TestLoggingProgressReporter(t *testing.T) {
	// Setup base reporter
	mockLog := &mockLogger{}
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false,
		EnableFileLogging: false,
		Writer:            buffer,
	}
	
	baseReporter := NewProgressReporter(config, nil) // No logger for base
	loggingReporter := NewLoggingProgressReporter(baseReporter, mockLog)
	
	ctx := context.Background()
	
	// Test start with enhanced logging
	err := loggingReporter.Start(ctx, 2)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	
	// Verify start logging
	hasSessionStart := false
	for _, log := range mockLog.logs {
		if log.level == "user_action" && log.message == "session_start" {
			hasSessionStart = true
			break
		}
	}
	if !hasSessionStart {
		t.Error("Expected session start user action log")
	}
	
	// Test download updates with milestone logging
	update := download.ProgressUpdate{
		DownloadID:      "milestone-test",
		BytesDownloaded: 250,
		TotalBytes:      1000,
		Speed:           100.0,
		State:           download.DownloadStateDownloading,
		Timestamp:       time.Now(),
		Metadata:        make(map[string]interface{}),
	}
	
	loggingReporter.UpdateDownload(update)
	
	// Test 50% milestone
	update.BytesDownloaded = 500
	loggingReporter.UpdateDownload(update)
	
	// Test 100% completion
	update.BytesDownloaded = 1000
	update.State = download.DownloadStateCompleted
	loggingReporter.UpdateDownload(update)
	
	// Test skip with enhanced logging
	loggingReporter.AddSkipped(SkipReasonInactiveUser, "inactive-user-file.mp4", map[string]interface{}{
		"user": "inactive@example.com",
	})
	
	// Test error with enhanced logging
	testErr := errors.New("network error")
	loggingReporter.AddError("error-file.mp4", testErr, map[string]interface{}{
		"url": "https://example.com/error-file.mp4",
	})
	
	// Test finish with enhanced logging
	summary := loggingReporter.Finish()
	
	if summary == nil {
		t.Fatal("Expected summary from finish")
	}
	
	// Verify comprehensive logging occurred
	hasPerformanceLog := false
	hasCompletionLog := false
	
	for _, log := range mockLog.logs {
		if log.level == "performance" {
			hasPerformanceLog = true
		}
		if log.level == "info" && log.message == "Download session completed with progress tracking" {
			hasCompletionLog = true
		}
	}
	
	if !hasPerformanceLog {
		t.Error("Expected performance logging")
	}
	if !hasCompletionLog {
		t.Error("Expected completion logging")
	}
}

func TestGetMetrics(t *testing.T) {
	// Create a summary with test data
	startTime := time.Now().Add(-5 * time.Minute)
	summary := &Summary{
		TotalItems:          10,
		CompletedDownloads:  7,
		FailedDownloads:     2,
		SkippedItems:        []SkippedItem{{Item: "skipped.mp4", Reason: SkipReasonAlreadyExists}},
		ErrorItems:          []ErrorItem{{Item: "error.mp4", Error: errors.New("test error")}},
		TotalBytesDownloaded: 1048576,
		AverageSpeed:        1024.0,
		StartTime:           startTime,
		EndTime:             time.Now(),
		ActiveDownloads: map[string]*DownloadProgress{
			"active1": {Speed: 2048.0},
			"active2": {Speed: 1536.0},
		},
	}
	
	metrics := GetMetrics(summary)
	
	// Verify metrics
	if metrics.SessionStart != startTime {
		t.Errorf("Expected start time %v, got %v", startTime, metrics.SessionStart)
	}
	if metrics.TotalDownloads != 10 {
		t.Errorf("Expected total downloads 10, got %d", metrics.TotalDownloads)
	}
	if metrics.CompletedDownloads != 7 {
		t.Errorf("Expected completed downloads 7, got %d", metrics.CompletedDownloads)
	}
	if metrics.FailedDownloads != 2 {
		t.Errorf("Expected failed downloads 2, got %d", metrics.FailedDownloads)
	}
	if metrics.SkippedDownloads != 1 {
		t.Errorf("Expected skipped downloads 1, got %d", metrics.SkippedDownloads)
	}
	if metrics.TotalBytesProcessed != 1048576 {
		t.Errorf("Expected total bytes 1048576, got %d", metrics.TotalBytesProcessed)
	}
	if metrics.AverageSpeed != 1024.0 {
		t.Errorf("Expected average speed 1024.0, got %f", metrics.AverageSpeed)
	}
	if metrics.PeakSpeed != 2048.0 {
		t.Errorf("Expected peak speed 2048.0, got %f", metrics.PeakSpeed)
	}
	if metrics.ActiveConnections != 2 {
		t.Errorf("Expected active connections 2, got %d", metrics.ActiveConnections)
	}
}

func TestDownloadProgressTracker_ConcurrentDownloads(t *testing.T) {
	// Setup
	mockLog := &mockLogger{}
	buffer := &bytes.Buffer{}
	
	config := ProgressConfig{
		ShowProgressBar:   false,
		EnableFileLogging: false,
		Writer:            buffer,
	}
	
	reporter := NewProgressReporter(config, mockLog)
	downloadMgr := newMockDownloadManager()
	tracker := NewDownloadProgressTracker(reporter, downloadMgr, mockLog)
	
	ctx := context.Background()
	numDownloads := 5
	reporter.Start(ctx, numDownloads)
	
	// Start multiple downloads concurrently
	results := make(chan *download.DownloadResult, numDownloads)
	errors := make(chan error, numDownloads)
	
	for i := 0; i < numDownloads; i++ {
		go func(id int) {
			req := download.DownloadRequest{
				ID:          fmt.Sprintf("concurrent-%d", id),
				URL:         fmt.Sprintf("https://example.com/file%d.mp4", id),
				Destination: fmt.Sprintf("/tmp/file%d.mp4", id),
				FileSize:    1024 * int64(id+1),
			}
			
			result, err := tracker.StartDownloadWithProgress(ctx, req)
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}(i)
	}
	
	// Collect results
	successCount := 0
	errorCount := 0
	
	for i := 0; i < numDownloads; i++ {
		select {
		case result := <-results:
			if result != nil && result.Success {
				successCount++
			}
		case err := <-errors:
			if err != nil {
				errorCount++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent downloads")
		}
	}
	
	if successCount != numDownloads {
		t.Errorf("Expected %d successful downloads, got %d", numDownloads, successCount)
	}
	if errorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", errorCount)
	}
	
	// Verify final summary
	summary := reporter.Finish()
	if summary.CompletedDownloads != numDownloads {
		t.Errorf("Expected %d completed downloads in summary, got %d", numDownloads, summary.CompletedDownloads)
	}
}