// Package box provides upload functionality with status tracking
package box

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/logging"
	"github.com/curtbushko/zoom-to-box/internal/tracking"
)

// UploadManager defines the interface for Box upload operations
type UploadManager interface {
	// Upload operations
	UploadFile(ctx context.Context, localPath, videoOwner, downloadID string) (*UploadResult, error)
	UploadFileWithProgress(ctx context.Context, localPath, videoOwner, downloadID string, progressCallback UploadProgressCallback) (*UploadResult, error)

	// Resume operations
	UploadWithResume(ctx context.Context, localPath, videoOwner, downloadID string, statusTracker download.StatusTracker) (*UploadResult, error)

	// Email mapping support - upload using separate Zoom and Box emails
	UploadFileWithEmailMapping(ctx context.Context, localPath, zoomEmail, boxEmail, downloadID string, progressCallback UploadProgressCallback) (*UploadResult, error)

	// Bulk operations
	UploadPendingFiles(ctx context.Context, statusTracker download.StatusTracker) (*UploadSummary, error)

	// Validation
	ValidateUploadedFile(ctx context.Context, fileID string, expectedSize int64) (bool, error)

	// Configuration
	SetBaseFolderID(folderID string)
	GetBaseFolderID() string

	// Client access
	GetBoxClient() BoxClient

	// CSV Tracking
	SetGlobalCSVTracker(tracker tracking.CSVTracker)
	SetUserCSVTracker(tracker tracking.CSVTracker)
	TrackUploadWithTime(zoomUser, fileName string, fileSize int64, uploadDate time.Time, processingTime time.Duration)

	// Upload with processing time
	UploadFileWithEmailMappingWithTime(ctx context.Context, localPath, zoomEmail, boxEmail, downloadID string, progressCallback UploadProgressCallback, processingTime time.Duration, trackingZoomEmail string, fileSize int64) (*UploadResult, error)
}

// UploadProgressCallback is called during file upload to report progress
type UploadProgressCallback func(uploaded int64, total int64, phase UploadPhase)

// UploadPhase represents the current phase of upload
type UploadPhase string

const (
	PhaseCreatingFolders UploadPhase = "creating_folders"
	PhaseUploadingFile   UploadPhase = "uploading_file"
	PhaseCompleted       UploadPhase = "completed"
	PhaseFailed          UploadPhase = "failed"
)

// UploadResult represents the result of a Box upload operation
type UploadResult struct {
	Success    bool          `json:"success"`
	FileID     string        `json:"file_id,omitempty"`
	FolderID   string        `json:"folder_id,omitempty"`
	FileName   string        `json:"file_name"`
	FileSize   int64         `json:"file_size"`
	UploadDate time.Time     `json:"upload_date"`
	RetryCount int           `json:"retry_count"`
	Error      error         `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// UploadSummary represents a summary of bulk upload operations
type UploadSummary struct {
	TotalFiles   int             `json:"total_files"`
	SuccessCount int             `json:"success_count"`
	FailureCount int             `json:"failure_count"`
	SkippedCount int             `json:"skipped_count"`
	Results      []*UploadResult `json:"results"`
	Duration     time.Duration   `json:"duration"`
	Errors       []error         `json:"errors,omitempty"`
}

// boxUploadManager implements the UploadManager interface
type boxUploadManager struct {
	client            BoxClient
	baseFolderID      string
	maxRetries        int
	globalCSVTracker  tracking.CSVTracker
	userCSVTracker    tracking.CSVTracker
}

// NewUploadManager creates a new Box upload manager
// The base folder is initially set to root (0), but should be set to the user's
// zoom folder ID using SetBaseFolderID() before uploading files.
// Example: uploadManager.SetBaseFolderID(zoomFolderID)
// This allows uploads to go to: <zoomFolder>/<year>/<month>/<day>/
func NewUploadManager(client BoxClient) UploadManager {
	return &boxUploadManager{
		client:       client,
		baseFolderID: RootFolderID, // Will be set to user's zoom folder before uploads
		maxRetries:   3,
	}
}

// SetBaseFolderID sets the base folder ID for uploads
func (um *boxUploadManager) SetBaseFolderID(folderID string) {
	if folderID == "" {
		folderID = RootFolderID
	}
	um.baseFolderID = folderID
}

// GetBaseFolderID returns the current base folder ID
func (um *boxUploadManager) GetBaseFolderID() string {
	return um.baseFolderID
}

// GetBoxClient returns the underlying Box client
func (um *boxUploadManager) GetBoxClient() BoxClient {
	return um.client
}

// SetGlobalCSVTracker sets the global CSV tracker for tracking all uploads
func (um *boxUploadManager) SetGlobalCSVTracker(tracker tracking.CSVTracker) {
	um.globalCSVTracker = tracker
}

// SetUserCSVTracker sets the user-specific CSV tracker for tracking user uploads
func (um *boxUploadManager) SetUserCSVTracker(tracker tracking.CSVTracker) {
	um.userCSVTracker = tracker
}

// UploadFile uploads a single file to Box without progress tracking
func (um *boxUploadManager) UploadFile(ctx context.Context, localPath, videoOwner, downloadID string) (*UploadResult, error) {
	return um.UploadFileWithProgress(ctx, localPath, videoOwner, downloadID, nil)
}

// UploadFileWithProgress uploads a single file to Box with progress tracking
func (um *boxUploadManager) UploadFileWithProgress(ctx context.Context, localPath, videoOwner, downloadID string, progressCallback UploadProgressCallback) (*UploadResult, error) {
	startTime := time.Now()

	result := &UploadResult{
		FileName:   filepath.Base(localPath),
		UploadDate: startTime,
	}

	// Extract folder path from the local file path
	// The local path structure is: <baseDir>/<user>/<year>/<month>/<day>/<filename>
	// We want to preserve the same structure in Box: <user>/<year>/<month>/<day>
	folderPath := extractFolderPathFromLocalPath(localPath)

	// Report progress - creating folders
	if progressCallback != nil {
		progressCallback(0, 0, PhaseCreatingFolders)
	}

	// Create folder structure using service account
	// The service account is co-owner of the zoom folder and can create subfolders
	folder, err := CreateFolderPath(um.client, folderPath, um.baseFolderID)
	if err != nil {
		err = fmt.Errorf("failed to create folder structure: %w", err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}

	result.FolderID = folder.ID

	// Report progress - uploading file
	if progressCallback != nil {
		progressCallback(0, 0, PhaseUploadingFile)
	}

	// Create upload progress callback
	var uploadProgressCallback ProgressCallback
	if progressCallback != nil {
		uploadProgressCallback = func(uploaded, total int64) {
			progressCallback(uploaded, total, PhaseUploadingFile)
		}
	}

	// Upload the file using service account
	file, err := um.client.UploadFileWithProgress(localPath, folder.ID, result.FileName, uploadProgressCallback)
	if err != nil {
		err = fmt.Errorf("failed to upload file as user: %w", err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}

	result.FileID = file.ID
	result.FileSize = file.Size
	result.Success = true

	result.Duration = time.Since(startTime)

	// Report progress - completed
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseCompleted)
	}

	logging.LogUserAction("box_upload_completed", videoOwner, map[string]interface{}{
		"file_id":     result.FileID,
		"file_name":   result.FileName,
		"file_size":   result.FileSize,
		"folder_id":   result.FolderID,
		"duration_ms": result.Duration.Milliseconds(),
	})

	// Track upload in CSV files if trackers are configured
	um.trackUpload(videoOwner, result.FileName, result.FileSize, result.UploadDate, 0)

	return result, nil
}

// UploadFileWithEmailMapping uploads a file using separate Zoom and Box emails
// zoomEmail is used for logging/metadata, boxEmail is used for Box folder structure
func (um *boxUploadManager) UploadFileWithEmailMapping(ctx context.Context, localPath, zoomEmail, boxEmail, downloadID string, progressCallback UploadProgressCallback) (*UploadResult, error) {
	startTime := time.Now()

	result := &UploadResult{
		FileName:   filepath.Base(localPath),
		UploadDate: startTime,
	}

	// Validate both emails
	if zoomEmail == "" {
		err := fmt.Errorf("zoom email cannot be empty")
		result.Error = err
		return result, err
	}
	if boxEmail == "" {
		err := fmt.Errorf("box email cannot be empty")
		result.Error = err
		return result, err
	}

	// Extract folder path from the local file path
	// The local path structure is: <baseDir>/<user>/<year>/<month>/<day>/<filename>
	// We want to preserve the same structure in Box: <user>/<year>/<month>/<day>
	folderPath := extractFolderPathFromLocalPath(localPath)

	// Report progress - creating folders
	if progressCallback != nil {
		progressCallback(0, 0, PhaseCreatingFolders)
	}

	// Create folder structure using service account
	// The service account is co-owner of the zoom folder and can create subfolders
	folder, err := CreateFolderPath(um.client, folderPath, um.baseFolderID)
	if err != nil {
		err = fmt.Errorf("failed to create folder structure for box email %s: %w", boxEmail, err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}

	result.FolderID = folder.ID

	// Report progress - uploading file
	if progressCallback != nil {
		progressCallback(0, 0, PhaseUploadingFile)
	}

	// Create upload progress callback
	var uploadProgressCallback ProgressCallback
	if progressCallback != nil {
		uploadProgressCallback = func(uploaded, total int64) {
			progressCallback(uploaded, total, PhaseUploadingFile)
		}
	}

	// Upload the file using service account
	file, err := um.client.UploadFileWithProgress(localPath, folder.ID, result.FileName, uploadProgressCallback)
	if err != nil {
		err = fmt.Errorf("failed to upload file as user: %w", err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}

	result.FileID = file.ID
	result.FileSize = file.Size
	result.Success = true

	result.Duration = time.Since(startTime)

	// Report progress - completed
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseCompleted)
	}

	// Log using both emails for context
	logging.LogUserAction("box_upload_completed_with_mapping", zoomEmail, map[string]interface{}{
		"zoom_email":  zoomEmail,
		"box_email":   boxEmail,
		"file_id":     result.FileID,
		"file_name":   result.FileName,
		"file_size":   result.FileSize,
		"folder_id":   result.FolderID,
		"duration_ms": result.Duration.Milliseconds(),
	})

	// Note: Tracking is NOT done here - caller is responsible for tracking with accurate processing time
	// See processor.go line 375 for main file tracking and uploadToBox for metadata tracking

	return result, nil
}

// UploadFileWithEmailMappingWithTime uploads a file using separate Zoom and Box emails with processing time tracking
func (um *boxUploadManager) UploadFileWithEmailMappingWithTime(ctx context.Context, localPath, zoomEmail, boxEmail, downloadID string, progressCallback UploadProgressCallback, processingTime time.Duration, trackingZoomEmail string, fileSize int64) (*UploadResult, error) {
	startTime := time.Now()

	result := &UploadResult{
		FileName:   filepath.Base(localPath),
		UploadDate: startTime,
	}

	// Validate both emails
	if zoomEmail == "" {
		err := fmt.Errorf("zoom email cannot be empty")
		result.Error = err
		return result, err
	}
	if boxEmail == "" {
		err := fmt.Errorf("box email cannot be empty")
		result.Error = err
		return result, err
	}

	// Extract folder path from the local file path
	folderPath := extractFolderPathFromLocalPath(localPath)

	// Report progress - creating folders
	if progressCallback != nil {
		progressCallback(0, 0, PhaseCreatingFolders)
	}

	// Create folder structure using service account
	folder, err := CreateFolderPath(um.client, folderPath, um.baseFolderID)
	if err != nil {
		err = fmt.Errorf("failed to create folder structure for box email %s: %w", boxEmail, err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}

	result.FolderID = folder.ID

	// Report progress - uploading file
	if progressCallback != nil {
		progressCallback(0, 0, PhaseUploadingFile)
	}

	// Create upload progress callback
	var uploadProgressCallback ProgressCallback
	if progressCallback != nil {
		uploadProgressCallback = func(uploaded, total int64) {
			progressCallback(uploaded, total, PhaseUploadingFile)
		}
	}

	// Upload the file using service account
	file, err := um.client.UploadFileWithProgress(localPath, folder.ID, result.FileName, uploadProgressCallback)
	if err != nil {
		err = fmt.Errorf("failed to upload file as user: %w", err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}

	result.FileID = file.ID
	result.FileSize = file.Size
	result.Success = true

	result.Duration = time.Since(startTime)

	// Report progress - completed
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseCompleted)
	}

	// Log using both emails for context
	logging.LogUserAction("box_upload_completed_with_mapping_and_time", trackingZoomEmail, map[string]interface{}{
		"zoom_email":             zoomEmail,
		"box_email":              boxEmail,
		"file_id":                result.FileID,
		"file_name":              result.FileName,
		"file_size":              result.FileSize,
		"folder_id":              result.FolderID,
		"duration_ms":            result.Duration.Milliseconds(),
		"processing_time_seconds": int64(processingTime.Seconds()),
	})

	// Track upload with processing time using actual uploaded file size from Box
	um.trackUpload(trackingZoomEmail, result.FileName, result.FileSize, result.UploadDate, processingTime)

	return result, nil
}

// UploadPendingFiles uploads all pending files from the status tracker
func (um *boxUploadManager) UploadPendingFiles(ctx context.Context, statusTracker download.StatusTracker) (*UploadSummary, error) {
	startTime := time.Now()

	summary := &UploadSummary{
		Results: make([]*UploadResult, 0),
		Errors:  make([]error, 0),
	}

	// Get pending uploads
	pendingUploads := statusTracker.GetPendingBoxUploads()
	summary.TotalFiles = len(pendingUploads)

	logging.Info("Starting bulk Box upload for %d files", summary.TotalFiles)

	// Upload each file
	for downloadID, entry := range pendingUploads {
		// Check if upload should be retried
		if !download.ShouldRetryBoxUpload(entry, um.maxRetries) {
			summary.SkippedCount++
			logging.Info("Skipping upload for %s (max retries exceeded)", downloadID)
			continue
		}

		// Mark upload started
		statusTracker.MarkBoxUploadStarted(downloadID, um.baseFolderID)

		// Upload the file with resume support
		result, err := um.UploadWithResume(ctx, entry.FilePath, entry.VideoOwner, downloadID, statusTracker)
		if err != nil {
			summary.FailureCount++
			summary.Errors = append(summary.Errors, err)
			statusTracker.MarkBoxUploadFailed(downloadID, err.Error())

			logging.LogUserAction("box_upload_failed", entry.VideoOwner, map[string]interface{}{
				"download_id": downloadID,
				"file_path":   entry.FilePath,
				"error":       err.Error(),
			})
		} else {
			summary.SuccessCount++
			statusTracker.MarkBoxUploadCompleted(downloadID, result.FileID)
		}

		summary.Results = append(summary.Results, result)
	}

	summary.Duration = time.Since(startTime)

	logging.Info("Bulk Box upload completed: %d success, %d failed, %d skipped in %v",
		summary.SuccessCount, summary.FailureCount, summary.SkippedCount, summary.Duration)

	return summary, nil
}

// createFolderStructure creates the necessary folder structure for the upload with proper permissions
func (um *boxUploadManager) createFolderStructure(ctx context.Context, folderPath string) (*Folder, error) {
	return CreateFolderPath(um.client, folderPath, um.baseFolderID)
}

// Helper functions

// extractFolderPathFromLocalPath extracts the folder structure from a local file path
// Local path structure: <baseDir>/<user>/<year>/<month>/<day>/<filename>
// Returns: <year>/<month>/<day>
// Note: The username is NOT included because baseFolderID is already set to the zoom folder
func extractFolderPathFromLocalPath(localPath string) string {
	// Get the directory part of the path (remove filename)
	dir := filepath.Dir(localPath)

	// Split the path into components
	parts := strings.Split(filepath.ToSlash(dir), "/")

	// We need to extract the last 3 components: year/month/day
	// Start from the end and take the last 3 parts
	if len(parts) >= 3 {
		// Get the last 3 components: year, month, day
		relevantParts := parts[len(parts)-3:]
		return strings.Join(relevantParts, "/")
	}

	// If we don't have enough parts, return the entire directory path
	// This shouldn't happen in normal operation
	return dir
}

// createDateBasedFolderPath creates a date-based folder path for the given username and date
// If username is empty, returns just the date-based path (for when baseFolderID is user's root)
// Note: This function is deprecated and should not be used for new uploads.
// Use extractFolderPathFromLocalPath instead to preserve the download directory structure.
func createDateBasedFolderPath(username string, date time.Time) string {
	utcDate := date.UTC()
	datePath := fmt.Sprintf("%04d/%02d/%02d",
		utcDate.Year(),
		utcDate.Month(),
		utcDate.Day())

	if username == "" {
		return datePath
	}

	return fmt.Sprintf("%s/%s", username, datePath)
}

// UploadWithResume uploads a file with support for resuming interrupted uploads
func (um *boxUploadManager) UploadWithResume(ctx context.Context, localPath, videoOwner, downloadID string, statusTracker download.StatusTracker) (*UploadResult, error) {
	// Check if upload already exists
	boxInfo, err := statusTracker.GetBoxUploadStatus(downloadID)
	if err == nil && boxInfo != nil {
		// Check if upload was completed successfully
		if boxInfo.Uploaded && boxInfo.FileID != "" {
			// IMPORTANT: Validate the file actually exists in Box before skipping upload
			// This prevents the bug where local status says "uploaded" but file doesn't exist in Box
			valid, validateErr := um.ValidateUploadedFile(ctx, boxInfo.FileID, 0)
			if validateErr != nil {
				// Error during validation - log and proceed with re-upload
				logging.Warn("Failed to validate existing upload for %s (file ID: %s): %v - will re-upload",
					downloadID, boxInfo.FileID, validateErr)
			} else if valid {
				// Upload already exists in Box and is valid - skip upload
				logging.Info("File already exists in Box for %s (file ID: %s) - skipping upload",
					downloadID, boxInfo.FileID)
				return &UploadResult{
					Success:    true,
					FileID:     boxInfo.FileID,
					FolderID:   boxInfo.FolderID,
					FileName:   filepath.Base(localPath),
					UploadDate: boxInfo.UploadDate,
					Duration:   0, // No upload time since it was already done
				}, nil
			} else {
				// File doesn't exist in Box or validation failed - need to re-upload
				logging.Warn("Existing upload validation failed for %s (file ID: %s) - will re-upload",
					downloadID, boxInfo.FileID)
			}
		}

		// Check if we should retry failed uploads
		if boxInfo.UploadError != "" && !download.ShouldRetryBoxUpload(download.DownloadEntry{Box: boxInfo}, um.maxRetries) {
			return nil, fmt.Errorf("upload for %s exceeded max retries (%d): %s", downloadID, um.maxRetries, boxInfo.UploadError)
		}
	}

	// Proceed with new upload
	progressCallback := func(uploaded, total int64, phase UploadPhase) {
		logging.Debug("Upload progress for %s: %d/%d bytes (%s)", downloadID, uploaded, total, phase)
	}

	result, err := um.UploadFileWithProgress(ctx, localPath, videoOwner, downloadID, progressCallback)

	// Update status tracker
	if err != nil {
		statusTracker.MarkBoxUploadFailed(downloadID, err.Error())
	} else {
		statusTracker.MarkBoxUploadCompleted(downloadID, result.FileID)
	}

	return result, err
}

// ValidateUploadedFile validates that a file exists in Box and matches expected criteria
func (um *boxUploadManager) ValidateUploadedFile(ctx context.Context, fileID string, expectedSize int64) (bool, error) {
	if fileID == "" {
		return false, fmt.Errorf("file ID cannot be empty")
	}

	// Get file information from Box
	file, err := um.client.GetFile(fileID)
	if err != nil {
		// File doesn't exist or is inaccessible
		logging.Debug("File validation failed for %s: %v", fileID, err)
		return false, nil
	}

	// Check file size if provided
	if expectedSize > 0 && file.Size != expectedSize {
		logging.Debug("File size mismatch for %s: expected %d, got %d", fileID, expectedSize, file.Size)
		return false, nil
	}

	// File exists and size matches (if checked)
	return true, nil
}

// trackUpload records an upload to both global and user CSV trackers if they are configured
func (um *boxUploadManager) trackUpload(zoomUser, fileName string, fileSize int64, uploadDate time.Time, processingTime time.Duration) {
	entry := tracking.UploadEntry{
		ZoomUser:       zoomUser,
		FileName:       fileName,
		RecordingSize:  fileSize,
		UploadDate:     uploadDate,
		ProcessingTime: processingTime,
	}

	// Track in global CSV if configured
	if um.globalCSVTracker != nil {
		if err := um.globalCSVTracker.TrackUpload(entry); err != nil {
			logging.Warn("Failed to track upload in global CSV: %v", err)
		}
	}

	// Track in user CSV if configured
	if um.userCSVTracker != nil {
		if err := um.userCSVTracker.TrackUpload(entry); err != nil {
			logging.Warn("Failed to track upload in user CSV: %v", err)
		}
	}
}

// TrackUploadWithTime is a public method to track uploads with processing time
func (um *boxUploadManager) TrackUploadWithTime(zoomUser, fileName string, fileSize int64, uploadDate time.Time, processingTime time.Duration) {
	um.trackUpload(zoomUser, fileName, fileSize, uploadDate, processingTime)
}

