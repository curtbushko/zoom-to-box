// Package download provides download manager with resume support for zoom-to-box
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DownloadManager defines the interface for download operations
type DownloadManager interface {
	Download(ctx context.Context, req DownloadRequest, progressCallback ProgressCallback) (*DownloadResult, error)
}

// DownloadConfig holds configuration for the download manager
type DownloadConfig struct {
	ChunkSize     int           // Size of each download chunk in bytes
	RetryAttempts int           // Number of retry attempts for failed downloads
	RetryDelay    time.Duration // Delay between retry attempts
	UserAgent     string        // User agent string for HTTP requests
	Timeout       time.Duration // HTTP request timeout
}

// DownloadRequest represents a single download request
type DownloadRequest struct {
	ID          string                 // Unique identifier for this download
	URL         string                 // Source URL to download from
	Destination string                 // Local file path to save to
	FileSize    int64                  // Expected file size in bytes (for progress tracking)
	Headers     map[string]string      // Additional HTTP headers
	Metadata    map[string]interface{} // Additional metadata for tracking
}

// ProgressUpdate represents download progress information
type ProgressUpdate struct {
	DownloadID      string                 // ID of the download
	BytesDownloaded int64                  // Total bytes downloaded so far
	TotalBytes      int64                  // Total expected bytes
	Speed           float64                // Current download speed in bytes/second
	ETA             time.Duration          // Estimated time to completion
	State           DownloadState          // Current download state
	Error           error                  // Error if download failed
	Metadata        map[string]interface{} // Additional progress metadata
	Timestamp       time.Time              // When this update was generated
}

// DownloadResult represents the result of a completed download
type DownloadResult struct {
	DownloadID      string                 // ID of the download
	BytesDownloaded int64                  // Total bytes successfully downloaded
	Duration        time.Duration          // Total download duration
	AverageSpeed    float64                // Average download speed in bytes/second
	Resumed         bool                   // Whether download was resumed from partial
	RetryCount      int                    // Number of retries that occurred
	Success         bool                   // Whether download completed successfully
	Error           error                  // Error if download failed
	Metadata        map[string]interface{} // Final metadata
	Timestamp       time.Time              // When download completed
}

// DownloadStatus represents current status of an active download
type DownloadStatus struct {
	Request     DownloadRequest
	Progress    ProgressUpdate
	StartTime   time.Time
	RetryCount  int
	LastAttempt time.Time
}

// DownloadState represents the current state of a download
type DownloadState int

const (
	DownloadStateQueued DownloadState = iota
	DownloadStateDownloading
	DownloadStatePaused
	DownloadStateCompleted
	DownloadStateFailed
	DownloadStateCancelled
)

func (s DownloadState) String() string {
	switch s {
	case DownloadStateQueued:
		return "queued"
	case DownloadStateDownloading:
		return "downloading"
	case DownloadStatePaused:
		return "paused"
	case DownloadStateCompleted:
		return "completed"
	case DownloadStateFailed:
		return "failed"
	case DownloadStateCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// ProgressCallback is called when download progress changes
type ProgressCallback func(update ProgressUpdate)

// downloadManagerImpl implements the DownloadManager interface
type downloadManagerImpl struct {
	config     DownloadConfig
	httpClient *http.Client
}

// NewDownloadManager creates a new download manager with the given configuration
func NewDownloadManager(config DownloadConfig) DownloadManager {
	// Set default values
	if config.ChunkSize <= 0 {
		config.ChunkSize = 64 * 1024 // 64KB chunks
	}
	if config.RetryAttempts < 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.UserAgent == "" {
		config.UserAgent = "zoom-to-box/1.0"
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit redirects to prevent infinite loops
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return &downloadManagerImpl{
		config:     config,
		httpClient: httpClient,
	}
}

// Download performs a download with resume support and retry logic
func (dm *downloadManagerImpl) Download(ctx context.Context, req DownloadRequest, progressCallback ProgressCallback) (*DownloadResult, error) {
	// Generate ID if not provided
	if req.ID == "" {
		req.ID = fmt.Sprintf("download_%d", time.Now().UnixNano())
	}

	startTime := time.Now()

	// Execute download with retry logic
	for attempt := 0; attempt <= dm.config.RetryAttempts; attempt++ {
		// Attempt download
		result, err := dm.performDownload(ctx, req, startTime, progressCallback)
		if err == nil {
			// Success
			result.RetryCount = attempt
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Check if we should retry
		if attempt >= dm.config.RetryAttempts {
			// Final attempt failed
			return &DownloadResult{
				DownloadID:      req.ID,
				BytesDownloaded: 0,
				Duration:        time.Since(startTime),
				AverageSpeed:    0,
				Resumed:         false,
				RetryCount:      attempt,
				Success:         false,
				Error:           err,
				Metadata:        req.Metadata,
				Timestamp:       time.Now(),
			}, err
		}

		// Wait before retry
		select {
		case <-time.After(dm.config.RetryDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("download failed after %d attempts", dm.config.RetryAttempts)
}

// performDownload performs a single download attempt with resume support
func (dm *downloadManagerImpl) performDownload(ctx context.Context, req DownloadRequest, startTime time.Time, progressCallback ProgressCallback) (*DownloadResult, error) {

	// Check if file already exists and get current size
	var currentSize int64 = 0
	var resumed bool = false
	
	if fileInfo, err := os.Stat(req.Destination); err == nil {
		currentSize = fileInfo.Size()
		if currentSize > 0 {
			resumed = true
		}
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(req.Destination), 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set user agent
	httpReq.Header.Set("User-Agent", dm.config.UserAgent)

	// Add custom headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Add Range header for resume if needed
	if currentSize > 0 {
		httpReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", currentSize))
	}

	// Send progress update: downloading
	if progressCallback != nil {
		progressCallback(ProgressUpdate{
			DownloadID:      req.ID,
			BytesDownloaded: currentSize,
			TotalBytes:      req.FileSize,
			Speed:           0,
			ETA:             0,
			State:           DownloadStateDownloading,
			Metadata:        req.Metadata,
			Timestamp:       time.Now(),
		})
	}

	// Make HTTP request
	resp, err := dm.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Validate partial content response
	if currentSize > 0 && resp.StatusCode != 206 {
		// Server doesn't support range requests, start over
		currentSize = 0
		resumed = false
	}

	// Open/create destination file
	var file *os.File
	if currentSize > 0 && resumed {
		// Append to existing file
		file, err = os.OpenFile(req.Destination, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open file for append: %w", err)
		}
	} else {
		// Create new file
		file, err = os.OpenFile(req.Destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}
		currentSize = 0
		resumed = false
	}
	defer file.Close()

	// Download with progress tracking
	downloadStartTime := time.Now()
	lastProgressTime := downloadStartTime
	bytesAtLastProgress := currentSize

	buffer := make([]byte, dm.config.ChunkSize)
	totalDownloaded := currentSize

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Read chunk
		n, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		if n == 0 {
			break // EOF
		}

		// Write chunk
		written, err := file.Write(buffer[:n])
		if err != nil {
			return nil, fmt.Errorf("failed to write to file: %w", err)
		}

		totalDownloaded += int64(written)

		// Update progress periodically
		now := time.Now()
		if progressCallback != nil && (now.Sub(lastProgressTime) >= 500*time.Millisecond || err == io.EOF) {
			// Calculate speed
			elapsed := now.Sub(lastProgressTime).Seconds()
			if elapsed > 0 {
				speed := float64(totalDownloaded-bytesAtLastProgress) / elapsed

				// Calculate ETA
				var eta time.Duration
				if speed > 0 && req.FileSize > totalDownloaded {
					eta = time.Duration(float64(req.FileSize-totalDownloaded)/speed) * time.Second
				}

				// Send progress update
				progressCallback(ProgressUpdate{
					DownloadID:      req.ID,
					BytesDownloaded: totalDownloaded,
					TotalBytes:      req.FileSize,
					Speed:           speed,
					ETA:             eta,
					State:           DownloadStateDownloading,
					Metadata:        req.Metadata,
					Timestamp:       now,
				})

				lastProgressTime = now
				bytesAtLastProgress = totalDownloaded
			}
		}

		if err == io.EOF {
			break
		}
	}

	// Ensure file is flushed
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("failed to sync file: %w", err)
	}

	// Calculate final statistics
	duration := time.Since(downloadStartTime)
	averageSpeed := float64(totalDownloaded-currentSize) / duration.Seconds()

	// Send final progress update
	if progressCallback != nil {
		progressCallback(ProgressUpdate{
			DownloadID:      req.ID,
			BytesDownloaded: totalDownloaded,
			TotalBytes:      req.FileSize,
			Speed:           0,
			ETA:             0,
			State:           DownloadStateCompleted,
			Metadata:        req.Metadata,
			Timestamp:       time.Now(),
		})
	}

	return &DownloadResult{
		DownloadID:      req.ID,
		BytesDownloaded: totalDownloaded,
		Duration:        duration,
		AverageSpeed:    averageSpeed,
		Resumed:         resumed,
		RetryCount:      0, // Will be set by caller
		Success:         true,
		Error:           nil,
		Metadata:        req.Metadata,
		Timestamp:       time.Now(),
	}, nil
}