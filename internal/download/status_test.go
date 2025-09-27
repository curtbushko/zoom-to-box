// Package download provides status tracking tests for the download manager
package download

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewStatusTracker(t *testing.T) {
	tests := []struct {
		name         string
		statusFile   string
		expectedError bool
	}{
		{
			name:         "valid status file path",
			statusFile:   "downloads_status.json",
			expectedError: false,
		},
		{
			name:         "empty status file path",
			statusFile:   "",
			expectedError: true,
		},
		{
			name:         "status file in nested directory",
			statusFile:   "data/downloads/status.json",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			statusFile := ""
			if tt.statusFile != "" {
				statusFile = filepath.Join(tempDir, tt.statusFile)
			}

			tracker, err := NewStatusTracker(statusFile)

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

			if tracker == nil {
				t.Error("Expected tracker but got nil")
				return
			}

			// Cleanup
			tracker.Close()
		})
	}
}

func TestStatusFileCreation(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// File should be created
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		t.Error("Status file should be created")
	}

	// File should contain valid JSON with basic structure
	content, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}

	var status StatusFile
	if err := json.Unmarshal(content, &status); err != nil {
		t.Errorf("Status file should contain valid JSON: %v", err)
	}

	// Check basic structure
	if status.Version == "" {
		t.Error("Status file should have version")
	}

	if status.Downloads == nil {
		t.Error("Status file should have downloads map")
	}
}

func TestUpdateDownloadStatus(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// Test updating download status
	downloadStatus := DownloadEntry{
		Status:           StatusPending,
		FilePath:         "test/path/file.mp4",
		FileSize:         1048576,
		DownloadedSize:   0,
		LastAttempt:      time.Now().UTC(),
		MetadataDownloaded: false,
		RetryCount:       0,
	}

	err = tracker.UpdateDownloadStatus("test123", downloadStatus)
	if err != nil {
		t.Errorf("Failed to update download status: %v", err)
	}

	// Verify status was updated
	retrieved, exists := tracker.GetDownloadStatus("test123")
	if !exists {
		t.Error("Download status should exist after update")
	}

	if retrieved.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, retrieved.Status)
	}

	if retrieved.FilePath != downloadStatus.FilePath {
		t.Errorf("Expected file path %s, got %s", downloadStatus.FilePath, retrieved.FilePath)
	}
}

func TestDownloadStatusStates(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	testID := "status_test"
	baseStatus := DownloadEntry{
		FilePath:   "test.mp4",
		FileSize:   1000,
		LastAttempt: time.Now().UTC(),
	}

	// Test all status states
	states := []DownloadStatusType{
		StatusPending,
		StatusDownloading,
		StatusCompleted,
		StatusFailed,
		StatusPaused,
	}

	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			status := baseStatus
			status.Status = state

			err := tracker.UpdateDownloadStatus(testID, status)
			if err != nil {
				t.Errorf("Failed to update status to %s: %v", state, err)
			}

			retrieved, exists := tracker.GetDownloadStatus(testID)
			if !exists {
				t.Error("Download status should exist")
			}

			if retrieved.Status != state {
				t.Errorf("Expected status %s, got %s", state, retrieved.Status)
			}
		})
	}
}

func TestChecksumHandling(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// Test checksum setting and retrieval
	testData := "test file content"
	expectedChecksum := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(testData)))

	downloadStatus := DownloadEntry{
		Status:           StatusCompleted,
		FilePath:         "test.mp4",
		FileSize:         int64(len(testData)),
		DownloadedSize:   int64(len(testData)),
		Checksum:         expectedChecksum,
		LastAttempt:      time.Now().UTC(),
		MetadataDownloaded: true,
	}

	err = tracker.UpdateDownloadStatus("checksum_test", downloadStatus)
	if err != nil {
		t.Errorf("Failed to update download status: %v", err)
	}

	// Verify checksum
	retrieved, exists := tracker.GetDownloadStatus("checksum_test")
	if !exists {
		t.Error("Download status should exist")
	}

	if retrieved.Checksum != expectedChecksum {
		t.Errorf("Expected checksum %s, got %s", expectedChecksum, retrieved.Checksum)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// Test concurrent read/write operations
	numGoroutines := 50
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			downloadID := fmt.Sprintf("concurrent_%d", id)
			status := DownloadEntry{
				Status:      StatusPending,
				FilePath:    fmt.Sprintf("file_%d.mp4", id),
				FileSize:    int64(id * 1000),
				LastAttempt: time.Now().UTC(),
			}
			
			err := tracker.UpdateDownloadStatus(downloadID, status)
			if err != nil {
				t.Errorf("Concurrent update failed: %v", err)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			downloadID := fmt.Sprintf("concurrent_%d", id)
			
			// Try to read, may or may not exist depending on timing
			_, _ = tracker.GetDownloadStatus(downloadID)
		}(i)
	}

	wg.Wait()

	// Verify all writes completed
	allDownloads := tracker.GetAllDownloads()
	if len(allDownloads) != numGoroutines {
		t.Errorf("Expected %d downloads, got %d", numGoroutines, len(allDownloads))
	}
}

func TestStatusFilePersistence(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	// Create tracker and add some data
	tracker1, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}

	testStatus := DownloadEntry{
		Status:           StatusCompleted,
		FilePath:         "persistent/test.mp4",
		FileSize:         2048,
		DownloadedSize:   2048,
		Checksum:         "sha256:abc123",
		LastAttempt:      time.Now().UTC(),
		MetadataDownloaded: true,
	}

	err = tracker1.UpdateDownloadStatus("persist_test", testStatus)
	if err != nil {
		t.Errorf("Failed to update status: %v", err)
	}

	// Close first tracker
	tracker1.Close()

	// Create new tracker with same file
	tracker2, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create second status tracker: %v", err)
	}
	defer tracker2.Close()

	// Verify data persisted
	retrieved, exists := tracker2.GetDownloadStatus("persist_test")
	if !exists {
		t.Error("Persisted download should exist")
	}

	if retrieved.Status != StatusCompleted {
		t.Errorf("Expected status %s, got %s", StatusCompleted, retrieved.Status)
	}

	if retrieved.FilePath != testStatus.FilePath {
		t.Errorf("Expected file path %s, got %s", testStatus.FilePath, retrieved.FilePath)
	}
}

func TestCorruptedStatusFileRecovery(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	// Create corrupted file
	corruptedContent := `{"version": "1.0", "downloads": { invalid json }`
	err := os.WriteFile(statusFile, []byte(corruptedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create corrupted file: %v", err)
	}

	// Should handle corrupted file gracefully
	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Should handle corrupted file gracefully: %v", err)
	}
	defer tracker.Close()

	// Should be able to add new entries
	testStatus := DownloadEntry{
		Status:      StatusPending,
		FilePath:    "recovery/test.mp4",
		FileSize:    1024,
		LastAttempt: time.Now().UTC(),
	}

	err = tracker.UpdateDownloadStatus("recovery_test", testStatus)
	if err != nil {
		t.Errorf("Failed to update status after recovery: %v", err)
	}

	// Verify new file is valid
	retrieved, exists := tracker.GetDownloadStatus("recovery_test")
	if !exists {
		t.Error("Recovery download should exist")
	}

	if retrieved.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, retrieved.Status)
	}
}

func TestGetDownloadsByStatus(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// Add downloads with different statuses
	statuses := map[string]DownloadStatusType{
		"pending1":   StatusPending,
		"pending2":   StatusPending,
		"completed1": StatusCompleted,
		"failed1":    StatusFailed,
		"downloading1": StatusDownloading,
	}

	for id, status := range statuses {
		entry := DownloadEntry{
			Status:      status,
			FilePath:    fmt.Sprintf("%s.mp4", id),
			FileSize:    1024,
			LastAttempt: time.Now().UTC(),
		}
		
		err := tracker.UpdateDownloadStatus(id, entry)
		if err != nil {
			t.Errorf("Failed to update status: %v", err)
		}
	}

	// Test filtering by status
	pendingDownloads := tracker.GetDownloadsByStatus(StatusPending)
	if len(pendingDownloads) != 2 {
		t.Errorf("Expected 2 pending downloads, got %d", len(pendingDownloads))
	}

	completedDownloads := tracker.GetDownloadsByStatus(StatusCompleted)
	if len(completedDownloads) != 1 {
		t.Errorf("Expected 1 completed download, got %d", len(completedDownloads))
	}

	failedDownloads := tracker.GetDownloadsByStatus(StatusFailed)
	if len(failedDownloads) != 1 {
		t.Errorf("Expected 1 failed download, got %d", len(failedDownloads))
	}
}

func TestDeleteDownloadStatus(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// Add a download
	testStatus := DownloadEntry{
		Status:      StatusCompleted,
		FilePath:    "delete_test.mp4",
		FileSize:    1024,
		LastAttempt: time.Now().UTC(),
	}

	err = tracker.UpdateDownloadStatus("delete_test", testStatus)
	if err != nil {
		t.Errorf("Failed to update status: %v", err)
	}

	// Verify it exists
	_, exists := tracker.GetDownloadStatus("delete_test")
	if !exists {
		t.Error("Download should exist before deletion")
	}

	// Delete it
	err = tracker.DeleteDownloadStatus("delete_test")
	if err != nil {
		t.Errorf("Failed to delete download status: %v", err)
	}

	// Verify it's gone
	_, exists = tracker.GetDownloadStatus("delete_test")
	if exists {
		t.Error("Download should not exist after deletion")
	}
}

func TestStatusFileVersioning(t *testing.T) {
	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "status.json")

	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	// Read the file and check version
	content, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("Failed to read status file: %v", err)
	}

	var status StatusFile
	err = json.Unmarshal(content, &status)
	if err != nil {
		t.Errorf("Failed to parse status file: %v", err)
	}

	// Should have a version
	if status.Version == "" {
		t.Error("Status file should have version")
	}

	// Should have a last_updated timestamp
	if status.LastUpdated.IsZero() {
		t.Error("Status file should have last_updated timestamp")
	}
}

func TestCalculateChecksum(t *testing.T) {
	// Test the checksum calculation function
	testData := "test file content for checksum"
	
	// Create a temporary file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	
	err := os.WriteFile(testFile, []byte(testData), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate checksum
	checksum, err := CalculateFileChecksum(testFile)
	if err != nil {
		t.Errorf("Failed to calculate checksum: %v", err)
	}

	// Verify format
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("Checksum should start with 'sha256:', got %s", checksum)
	}

	// Verify it's consistent
	checksum2, err := CalculateFileChecksum(testFile)
	if err != nil {
		t.Errorf("Failed to calculate checksum second time: %v", err)
	}

	if checksum != checksum2 {
		t.Error("Checksum should be consistent")
	}

	// Verify it changes with different content
	err = os.WriteFile(testFile, []byte("different content"), 0644)
	if err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	checksum3, err := CalculateFileChecksum(testFile)
	if err != nil {
		t.Errorf("Failed to calculate checksum for modified file: %v", err)
	}

	if checksum == checksum3 {
		t.Error("Checksum should change when file content changes")
	}
}

func TestCreateDownloadEntry(t *testing.T) {
	req := DownloadRequest{
		ID:          "test123",
		URL:         "https://example.com/file.mp4",
		Destination: "/path/to/file.mp4",
		FileSize:    1048576,
		Metadata:    map[string]interface{}{"test": "value"},
	}

	entry := CreateDownloadEntry(req, StatusPending)

	if entry.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, entry.Status)
	}

	if entry.FilePath != req.Destination {
		t.Errorf("Expected file path %s, got %s", req.Destination, entry.FilePath)
	}

	if entry.FileSize != req.FileSize {
		t.Errorf("Expected file size %d, got %d", req.FileSize, entry.FileSize)
	}

	if entry.DownloadedSize != 0 {
		t.Errorf("Expected downloaded size 0, got %d", entry.DownloadedSize)
	}

	if !entry.MetadataDownloaded {
		// This is expected to be false initially
	}

	if entry.Metadata["test"] != "value" {
		t.Error("Metadata should be copied from request")
	}
}

func TestUpdateEntryFromProgress(t *testing.T) {
	entry := DownloadEntry{
		Status:         StatusDownloading,
		DownloadedSize: 0,
	}

	progress := ProgressUpdate{
		BytesDownloaded: 524288,
		State:          DownloadStateDownloading,
		Timestamp:      time.Now().UTC(),
	}

	updated := UpdateEntryFromProgress(entry, progress)

	if updated.DownloadedSize != progress.BytesDownloaded {
		t.Errorf("Expected downloaded size %d, got %d", progress.BytesDownloaded, updated.DownloadedSize)
	}

	if updated.Status != StatusDownloading {
		t.Errorf("Expected status %s, got %s", StatusDownloading, updated.Status)
	}

	// Test completed state
	progress.State = DownloadStateCompleted
	updated = UpdateEntryFromProgress(entry, progress)

	if updated.Status != StatusCompleted {
		t.Errorf("Expected status %s, got %s", StatusCompleted, updated.Status)
	}

	if updated.CompletedTime.IsZero() {
		t.Error("Completed time should be set")
	}
}

func TestUpdateEntryFromResult(t *testing.T) {
	entry := DownloadEntry{
		Status: StatusDownloading,
	}

	result := DownloadResult{
		BytesDownloaded: 1048576,
		Success:         true,
		RetryCount:      2,
		Duration:        time.Second * 30,
		AverageSpeed:    34952.5,
		Resumed:         true,
		Metadata:        map[string]interface{}{"final": "result"},
		Timestamp:       time.Now().UTC(),
	}

	updated := UpdateEntryFromResult(entry, result)

	if updated.Status != StatusCompleted {
		t.Errorf("Expected status %s, got %s", StatusCompleted, updated.Status)
	}

	if updated.DownloadedSize != result.BytesDownloaded {
		t.Errorf("Expected downloaded size %d, got %d", result.BytesDownloaded, updated.DownloadedSize)
	}

	if updated.RetryCount != result.RetryCount {
		t.Errorf("Expected retry count %d, got %d", result.RetryCount, updated.RetryCount)
	}

	// Check metadata merging
	if updated.Metadata["final"] != "result" {
		t.Error("Result metadata should be merged")
	}

	if updated.Metadata["duration_seconds"] != result.Duration.Seconds() {
		t.Error("Duration should be added to metadata")
	}

	if updated.Metadata["average_speed"] != result.AverageSpeed {
		t.Error("Average speed should be added to metadata")
	}

	if updated.Metadata["resumed"] != result.Resumed {
		t.Error("Resumed flag should be added to metadata")
	}
}

func TestShouldResumeDownload(t *testing.T) {
	tests := []struct {
		name     string
		status   DownloadStatusType
		lastTime time.Time
		expected bool
	}{
		{
			name:     "pending download",
			status:   StatusPending,
			expected: true,
		},
		{
			name:     "failed download",
			status:   StatusFailed,
			expected: true,
		},
		{
			name:     "paused download",
			status:   StatusPaused,
			expected: true,
		},
		{
			name:     "completed download",
			status:   StatusCompleted,
			expected: false,
		},
		{
			name:     "recent downloading",
			status:   StatusDownloading,
			lastTime: time.Now().Add(-1 * time.Minute),
			expected: false,
		},
		{
			name:     "stale downloading",
			status:   StatusDownloading,
			lastTime: time.Now().Add(-10 * time.Minute),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := DownloadEntry{
				Status:      tt.status,
				LastAttempt: tt.lastTime,
			}

			result := ShouldResumeDownload(entry)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetResumeOffset(t *testing.T) {
	tests := []struct {
		name           string
		entry          DownloadEntry
		expectedOffset int64
	}{
		{
			name: "partial download",
			entry: DownloadEntry{
				Status:         StatusPending,
				FileSize:       1048576,
				DownloadedSize: 524288,
			},
			expectedOffset: 524288,
		},
		{
			name: "completed download",
			entry: DownloadEntry{
				Status:         StatusCompleted,
				FileSize:       1048576,
				DownloadedSize: 1048576,
			},
			expectedOffset: 1048576, // Full file size
		},
		{
			name: "new download",
			entry: DownloadEntry{
				Status:         StatusPending,
				FileSize:       1048576,
				DownloadedSize: 0,
			},
			expectedOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := GetResumeOffset(tt.entry)
			if offset != tt.expectedOffset {
				t.Errorf("Expected offset %d, got %d", tt.expectedOffset, offset)
			}
		})
	}
}

func TestIsIntegrityValid(t *testing.T) {
	tests := []struct {
		name     string
		entry    DownloadEntry
		expected bool
	}{
		{
			name: "valid completed download",
			entry: DownloadEntry{
				Status:         StatusCompleted,
				FileSize:       1024,
				DownloadedSize: 1024,
			},
			expected: true,
		},
		{
			name: "incomplete download",
			entry: DownloadEntry{
				Status:         StatusCompleted,
				FileSize:       1024,
				DownloadedSize: 512,
			},
			expected: false,
		},
		{
			name: "not completed",
			entry: DownloadEntry{
				Status:         StatusDownloading,
				FileSize:       1024,
				DownloadedSize: 1024,
			},
			expected: false,
		},
		{
			name: "unknown file size",
			entry: DownloadEntry{
				Status:         StatusCompleted,
				FileSize:       0,
				DownloadedSize: 1024,
			},
			expected: true, // Size check skipped when FileSize is 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := IsIntegrityValid(tt.entry)
			if valid != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, valid)
			}
		})
	}
}

func TestNeedsChecksumVerification(t *testing.T) {
	tests := []struct {
		name     string
		entry    DownloadEntry
		expected bool
	}{
		{
			name: "completed without checksum",
			entry: DownloadEntry{
				Status:   StatusCompleted,
				Checksum: "",
			},
			expected: true,
		},
		{
			name: "completed with checksum",
			entry: DownloadEntry{
				Status:   StatusCompleted,
				Checksum: "sha256:abc123",
			},
			expected: false,
		},
		{
			name: "not completed",
			entry: DownloadEntry{
				Status:   StatusDownloading,
				Checksum: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needs := NeedsChecksumVerification(tt.entry)
			if needs != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, needs)
			}
		})
	}
}

// TestStatusUncoveredFunctions tests functions with 0% coverage
func TestStatusUncoveredFunctions(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewStatusTracker(filepath.Join(tempDir, "test_status.json"))
	if err != nil {
		t.Fatalf("Failed to create status tracker: %v", err)
	}
	defer tracker.Close()

	t.Run("GetIncompleteDownloads", func(t *testing.T) {
		// Add some test entries
		entry1 := CreateDownloadEntry(DownloadRequest{
			URL:         "http://example.com/file1.mp4",
			Destination: "file1.mp4",
		}, StatusPending)
		entry2 := CreateDownloadEntry(DownloadRequest{
			URL:         "http://example.com/file2.mp4", 
			Destination: "file2.mp4",
		}, StatusCompleted)

		tracker.UpdateDownloadStatus("id1", entry1)
		tracker.UpdateDownloadStatus("id2", entry2)

		incomplete := tracker.GetIncompleteDownloads()
		if len(incomplete) != 1 {
			t.Errorf("Expected 1 incomplete download, got %d", len(incomplete))
		}
		// Find the incomplete download (it should be the pending one)
		var foundIncomplete DownloadEntry
		for _, entry := range incomplete {
			if entry.Status == StatusPending {
				foundIncomplete = entry
				break
			}
		}
		if foundIncomplete.FilePath == "" {
			t.Error("Expected to find pending download in incomplete list")
		}
	})

	t.Run("VerifyFileChecksum", func(t *testing.T) {
		// Create a test file
		testFile := filepath.Join(tempDir, "test.txt")
		content := "test content"
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		// Calculate expected checksum
		expected, err := CalculateFileChecksum(testFile)
		if err != nil {
			t.Fatalf("Failed to calculate checksum: %v", err)
		}

		// Verify with correct checksum
		valid, err := VerifyFileChecksum(testFile, expected)
		if err != nil {
			t.Errorf("VerifyFileChecksum failed: %v", err)
		}
		if !valid {
			t.Error("Expected checksum to be valid")
		}

		// Verify with incorrect checksum
		valid, err = VerifyFileChecksum(testFile, "wrong_checksum")
		if err != nil {
			t.Errorf("VerifyFileChecksum failed: %v", err)
		}
		if valid {
			t.Error("Expected checksum to be invalid")
		}
	})

	t.Run("GetDownloadStatus with return values", func(t *testing.T) {
		req := DownloadRequest{
			URL:         "http://example.com/test.mp4",
			Destination: "test.mp4",
		}
		entry := CreateDownloadEntry(req, StatusCompleted)
		tracker.UpdateDownloadStatus("test_id", entry)

		// Test GetDownloadStatus returns both entry and bool
		retrievedEntry, exists := tracker.GetDownloadStatus("test_id")
		if !exists {
			t.Error("Expected entry to exist")
		}
		if retrievedEntry.FilePath != req.Destination {
			t.Errorf("Expected FilePath %s, got %s", req.Destination, retrievedEntry.FilePath)
		}

		// Test non-existent entry
		_, exists = tracker.GetDownloadStatus("nonexistent")
		if exists {
			t.Error("Expected entry to not exist")
		}
	})
}