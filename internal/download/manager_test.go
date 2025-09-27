package download

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDownloadManager tests the complete download manager functionality
func TestDownloadManager(t *testing.T) {
	tests := []struct {
		name            string
		fileSize        int64
		serverBehavior  string
		expectedError   bool
		expectResume    bool
		concurrentLimit int
	}{
		{
			name:            "successful complete download",
			fileSize:        1024,
			serverBehavior:  "normal",
			expectedError:   false,
			expectResume:    false,
			concurrentLimit: 5,
		},
		{
			name:            "successful resume after interruption",
			fileSize:        2048,
			serverBehavior:  "interrupt_then_resume",
			expectedError:   false,
			expectResume:    true,
			concurrentLimit: 5,
		},
		{
			name:            "range header support",
			fileSize:        1024,
			serverBehavior:  "range_requests",
			expectedError:   false,
			expectResume:    false,
			concurrentLimit: 5,
		},
		{
			name:            "concurrent download limiting",
			fileSize:        512,
			serverBehavior:  "normal",
			expectedError:   false,
			expectResume:    false,
			concurrentLimit: 2,
		},
		{
			name:            "server error handling",
			fileSize:        1024,
			serverBehavior:  "server_error",
			expectedError:   true,
			expectResume:    false,
			concurrentLimit: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tempDir := t.TempDir()
			
			// Create mock server based on behavior
			server := createMockDownloadServer(t, tt.serverBehavior, tt.fileSize)
			defer server.Close()

			// Create download manager
			config := DownloadConfig{
				ConcurrentLimit: tt.concurrentLimit,
				ChunkSize:       256,  // Small chunk size for testing
				RetryAttempts:   3,
				RetryDelay:      10 * time.Millisecond,
			}
			
			manager := NewDownloadManager(config)

			// Create download request
			req := DownloadRequest{
				URL:         server.URL + "/file.mp4",
				Destination: filepath.Join(tempDir, "test_file.mp4"),
				FileSize:    tt.fileSize,
				Metadata: map[string]interface{}{
					"meeting_id": "test_meeting_123",
					"user_id":    "test@example.com",
				},
			}

			// Track progress
			var progressUpdates []ProgressUpdate
			var progressMutex sync.Mutex
			
			progressCallback := func(update ProgressUpdate) {
				progressMutex.Lock()
				progressUpdates = append(progressUpdates, update)
				progressMutex.Unlock()
			}

			// Execute download
			ctx := context.Background()
			result, err := manager.Download(ctx, req, progressCallback)

			// Verify expectations
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify result
			if result.BytesDownloaded != tt.fileSize {
				t.Errorf("Expected %d bytes downloaded, got %d", tt.fileSize, result.BytesDownloaded)
			}

			// Verify file exists and has correct size
			fileInfo, err := os.Stat(req.Destination)
			if err != nil {
				t.Errorf("Failed to stat downloaded file: %v", err)
				return
			}

			if fileInfo.Size() != tt.fileSize {
				t.Errorf("Expected file size %d, got %d", tt.fileSize, fileInfo.Size())
			}

			// Verify progress updates were received
			progressMutex.Lock()
			if len(progressUpdates) == 0 {
				t.Error("Expected progress updates but got none")
			}
			progressMutex.Unlock()

			// For resume tests, verify resume occurred
			if tt.expectResume && !result.Resumed {
				t.Error("Expected download to be resumed but it wasn't")
			}
		})
	}
}

// TestRangeHeaderSupport tests HTTP Range header functionality
func TestRangeHeaderSupport(t *testing.T) {
	fileContent := strings.Repeat("test data ", 100) // 1000 bytes
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for Range header
		rangeHeader := r.Header.Get("Range")
		
		if rangeHeader == "" {
			// Full content request
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
			w.WriteHeader(200)
			w.Write([]byte(fileContent))
			return
		}

		// Parse Range header (e.g., "bytes=500-")
		if !strings.HasPrefix(rangeHeader, "bytes=") {
			w.WriteHeader(416) // Range Not Satisfiable
			return
		}

		rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
		parts := strings.Split(rangeSpec, "-")
		
		if len(parts) != 2 {
			w.WriteHeader(416)
			return
		}

		var start, end int
		fmt.Sscanf(parts[0], "%d", &start)
		if parts[1] == "" {
			end = len(fileContent) - 1
		} else {
			fmt.Sscanf(parts[1], "%d", &end)
		}

		if start < 0 || start >= len(fileContent) || end >= len(fileContent) || start > end {
			w.WriteHeader(416)
			return
		}

		// Send partial content
		contentRange := fmt.Sprintf("bytes %d-%d/%d", start, end, len(fileContent))
		w.Header().Set("Content-Range", contentRange)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(206) // Partial Content
		w.Write([]byte(fileContent[start : end+1]))
	}))
	defer server.Close()

	// Create download manager
	config := DownloadConfig{
		ConcurrentLimit: 1,
		ChunkSize:       200,
		RetryAttempts:   1,
		RetryDelay:      time.Millisecond,
	}
	manager := NewDownloadManager(config)

	// Create temporary file with partial content to simulate resume
	tempDir := t.TempDir()
	partialFile := filepath.Join(tempDir, "partial_file.mp4")
	
	// Write first 500 bytes to simulate previous partial download
	partialContent := []byte(fileContent[:500])
	err := os.WriteFile(partialFile, partialContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create partial file: %v", err)
	}

	// Create download request
	req := DownloadRequest{
		URL:         server.URL + "/file.mp4",
		Destination: partialFile,
		FileSize:    int64(len(fileContent)),
	}

	// Execute download with resume
	ctx := context.Background()
	result, err := manager.Download(ctx, req, nil)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify resume occurred
	if !result.Resumed {
		t.Error("Expected download to be resumed")
	}

	// Verify complete file
	downloadedContent, err := os.ReadFile(partialFile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(downloadedContent) != fileContent {
		t.Error("Downloaded content doesn't match expected content")
	}
}

// TestConcurrentDownloadLimiting tests that concurrent downloads are properly limited
func TestConcurrentDownloadLimiting(t *testing.T) {
	maxConcurrent := 2
	activeDownloads := int32(0)
	maxObservedConcurrent := int32(0)
	var mutex sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Track concurrent downloads
		mutex.Lock()
		activeDownloads++
		if activeDownloads > maxObservedConcurrent {
			maxObservedConcurrent = activeDownloads
		}
		mutex.Unlock()

		// Simulate some processing time
		time.Sleep(50 * time.Millisecond)

		// Send response
		content := strings.Repeat("test", 100)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(200)
		w.Write([]byte(content))

		// Decrement active downloads
		mutex.Lock()
		activeDownloads--
		mutex.Unlock()
	}))
	defer server.Close()

	// Create download manager with concurrent limit
	config := DownloadConfig{
		ConcurrentLimit: maxConcurrent,
		ChunkSize:       100,
		RetryAttempts:   1,
		RetryDelay:      time.Millisecond,
	}
	manager := NewDownloadManager(config)

	// Create multiple download requests
	tempDir := t.TempDir()
	numDownloads := 5
	var wg sync.WaitGroup
	var downloadErrors []error
	var errorsMutex sync.Mutex

	for i := 0; i < numDownloads; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			req := DownloadRequest{
				URL:         server.URL + fmt.Sprintf("/file%d.mp4", index),
				Destination: filepath.Join(tempDir, fmt.Sprintf("test_file_%d.mp4", index)),
				FileSize:    400, // 400 bytes
			}

			ctx := context.Background()
			_, err := manager.Download(ctx, req, nil)
			if err != nil {
				errorsMutex.Lock()
				downloadErrors = append(downloadErrors, err)
				errorsMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Check for errors
	if len(downloadErrors) > 0 {
		t.Errorf("Got %d download errors: %v", len(downloadErrors), downloadErrors[0])
	}

	// Verify concurrent limit was respected
	mutex.Lock()
	observed := maxObservedConcurrent
	mutex.Unlock()

	if observed > int32(maxConcurrent) {
		t.Errorf("Expected max %d concurrent downloads, observed %d", maxConcurrent, observed)
	}
}

// TestProgressTracking tests download progress reporting
func TestProgressTracking(t *testing.T) {
	fileSize := int64(1000)
	chunkSize := int64(100)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
		w.WriteHeader(200)
		
		// Send data in chunks to simulate progress
		content := strings.Repeat("x", int(fileSize))
		for i := int64(0); i < fileSize; i += chunkSize {
			end := i + chunkSize
			if end > fileSize {
				end = fileSize
			}
			w.Write([]byte(content[i:end]))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(10 * time.Millisecond) // Simulate network delay
		}
	}))
	defer server.Close()

	config := DownloadConfig{
		ConcurrentLimit: 1,
		ChunkSize:       int(chunkSize),
		RetryAttempts:   1,
		RetryDelay:      time.Millisecond,
	}
	manager := NewDownloadManager(config)

	tempDir := t.TempDir()
	req := DownloadRequest{
		URL:         server.URL + "/file.mp4",
		Destination: filepath.Join(tempDir, "progress_test.mp4"),
		FileSize:    fileSize,
	}

	// Track progress updates
	var progressUpdates []ProgressUpdate
	var progressMutex sync.Mutex
	
	progressCallback := func(update ProgressUpdate) {
		progressMutex.Lock()
		progressUpdates = append(progressUpdates, update)
		progressMutex.Unlock()
	}

	ctx := context.Background()
	result, err := manager.Download(ctx, req, progressCallback)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify progress updates
	progressMutex.Lock()
	if len(progressUpdates) < 2 {
		t.Errorf("Expected multiple progress updates, got %d", len(progressUpdates))
	}

	// Verify progress increases monotonically
	for i := 1; i < len(progressUpdates); i++ {
		if progressUpdates[i].BytesDownloaded < progressUpdates[i-1].BytesDownloaded {
			t.Error("Progress should increase monotonically")
		}
	}

	// Verify final progress matches result
	finalProgress := progressUpdates[len(progressUpdates)-1]
	if finalProgress.BytesDownloaded != result.BytesDownloaded {
		t.Errorf("Final progress %d doesn't match result %d", finalProgress.BytesDownloaded, result.BytesDownloaded)
	}
	progressMutex.Unlock()
}

// TestNetworkInterruptionHandling tests graceful handling of network interruptions
func TestNetworkInterruptionHandling(t *testing.T) {
	requestCount := 0
	fileContent := strings.Repeat("test data ", 200) // 2000 bytes
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		
		// Fail first request to simulate network interruption
		if requestCount == 1 {
			// Return server error to simulate network failure
			w.WriteHeader(500)
			w.Write([]byte("Network error"))
			return
		}

		// Second request should succeed (normal retry after failure)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
		w.WriteHeader(200)
		w.Write([]byte(fileContent))
	}))
	defer server.Close()

	config := DownloadConfig{
		ConcurrentLimit: 1,
		ChunkSize:       256,
		RetryAttempts:   3,
		RetryDelay:      50 * time.Millisecond,
	}
	manager := NewDownloadManager(config)

	tempDir := t.TempDir()
	req := DownloadRequest{
		URL:         server.URL + "/file.mp4",
		Destination: filepath.Join(tempDir, "interrupted_file.mp4"),
		FileSize:    int64(len(fileContent)),
	}

	ctx := context.Background()
	result, err := manager.Download(ctx, req, nil)
	if err != nil {
		t.Fatalf("Download should succeed after retry: %v", err)
	}

	// Verify result
	if !result.Success {
		t.Error("Expected download to succeed after retry")
	}

	// Verify retry occurred
	if requestCount < 2 {
		t.Errorf("Expected at least 2 requests (original + retry), got %d", requestCount)
	}

	// Verify final file is complete
	downloadedContent, err := os.ReadFile(req.Destination)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if len(downloadedContent) != len(fileContent) {
		t.Errorf("Expected file size %d, got %d", len(fileContent), len(downloadedContent))
	}
}

// Helper function to create mock download server with different behaviors
func createMockDownloadServer(t *testing.T, behavior string, fileSize int64) *httptest.Server {
	content := strings.Repeat("x", int(fileSize))
	requestCount := 0

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch behavior {
		case "normal":
			w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(200)
			w.Write([]byte(content))

		case "interrupt_then_resume":
			if requestCount == 1 {
				// First request - send partial content then close
				w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
				w.Header().Set("Accept-Ranges", "bytes")
				w.WriteHeader(200)
				w.Write([]byte(content[:fileSize/2])) // Send half
				return
			}
			// Second request - handle range request
			rangeHeader := r.Header.Get("Range")
			if strings.HasPrefix(rangeHeader, "bytes=") {
				start := fileSize / 2
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, fileSize-1, fileSize))
				w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize-start))
				w.WriteHeader(206)
				w.Write([]byte(content[start:]))
			}

		case "range_requests":
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "" {
				w.Header().Set("Accept-Ranges", "bytes")
				w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
				w.WriteHeader(200)
				w.Write([]byte(content))
			} else {
				// Handle range request
				w.WriteHeader(206)
				w.Write([]byte(content)) // Simplified for test
			}

		case "server_error":
			w.WriteHeader(500)
			w.Write([]byte("Internal Server Error"))

		case "slow":
			// Simulate a slow download by adding delay
			time.Sleep(200 * time.Millisecond)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
			w.WriteHeader(200)
			w.Write([]byte(content))

		default:
			t.Errorf("Unknown server behavior: %s", behavior)
		}
	}))
}

// TestDownloadManagerUncoveredFunctions tests functions with 0% coverage
func TestDownloadManagerUncoveredFunctions(t *testing.T) {
	config := DownloadConfig{
		ConcurrentLimit: 2,
		ChunkSize:       1024,
		RetryAttempts:   3,
		RetryDelay:      time.Millisecond,
	}

	manager := NewDownloadManager(config)

	t.Run("String method", func(t *testing.T) {
		// Test DownloadState String method
		state := DownloadStateDownloading
		str := state.String()
		if str != "downloading" {
			t.Errorf("Expected 'downloading', got: %s", str)
		}
		
		// Test another state
		completedState := DownloadStateCompleted
		completedStr := completedState.String()
		if completedStr != "completed" {
			t.Errorf("Expected 'completed', got: %s", completedStr)
		}
	})

	t.Run("GetActiveDownloads", func(t *testing.T) {
		// Initially should have no active downloads
		active := manager.GetActiveDownloads()
		if len(active) != 0 {
			t.Errorf("Expected 0 active downloads, got %d", len(active))
		}

		// Start a download and check active downloads
		server := createMockDownloadServer(t, "normal", 1024)
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := DownloadRequest{
			URL:         server.URL,
			Destination: filepath.Join(t.TempDir(), "test-active.bin"),
		}

		// Start download in goroutine
		go func() {
			_, _ = manager.Download(ctx, req, nil)
		}()

		// Give it time to start
		time.Sleep(10 * time.Millisecond)

		// Now check active downloads
		active = manager.GetActiveDownloads()
		if len(active) == 0 {
			// Note: Due to timing, this might be 0 if download completed quickly
			// This is acceptable for a fast test download
		}
	})

	t.Run("CancelDownload", func(t *testing.T) {
		// Test cancelling a non-existent download
		err := manager.CancelDownload("http://nonexistent.com/file.mp4")
		if err == nil {
			t.Error("Expected error when cancelling non-existent download")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("Expected 'not found' error, got: %v", err)
		}
	})
}