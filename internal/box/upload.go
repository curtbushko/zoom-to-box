// Package box provides upload functionality with status tracking and permission management
package box

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/logging"
)

// UploadManager defines the interface for Box upload operations
type UploadManager interface {
	// Upload operations
	// videoOwner should be the Box email address for proper permission management
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
}

// UploadProgressCallback is called during file upload to report progress
type UploadProgressCallback func(uploaded int64, total int64, phase UploadPhase)

// UploadPhase represents the current phase of upload
type UploadPhase string

const (
	PhaseCreatingFolders UploadPhase = "creating_folders"
	PhaseUploadingFile   UploadPhase = "uploading_file"
	PhaseSettingPermissions UploadPhase = "setting_permissions"
	PhaseCompleted       UploadPhase = "completed"
	PhaseFailed          UploadPhase = "failed"
)

// UploadResult represents the result of a Box upload operation
type UploadResult struct {
	Success        bool      `json:"success"`
	FileID         string    `json:"file_id,omitempty"`
	FolderID       string    `json:"folder_id,omitempty"`
	FileName       string    `json:"file_name"`
	FileSize       int64     `json:"file_size"`
	UploadDate     time.Time `json:"upload_date"`
	PermissionsSet bool      `json:"permissions_set"`
	PermissionIDs  []string  `json:"permission_ids,omitempty"`
	RetryCount     int       `json:"retry_count"`
	Error          error     `json:"error,omitempty"`
	Duration       time.Duration `json:"duration"`
}

// UploadSummary represents a summary of bulk upload operations
type UploadSummary struct {
	TotalFiles     int               `json:"total_files"`
	SuccessCount   int               `json:"success_count"`
	FailureCount   int               `json:"failure_count"`
	SkippedCount   int               `json:"skipped_count"`
	Results        []*UploadResult   `json:"results"`
	Duration       time.Duration     `json:"duration"`
	Errors         []error           `json:"errors,omitempty"`
}

// boxUploadManager implements the UploadManager interface
type boxUploadManager struct {
	client       BoxClient
	baseFolderID string
	maxRetries   int
	mutex        sync.RWMutex
}

// NewUploadManager creates a new Box upload manager
func NewUploadManager(client BoxClient, baseFolderID string) UploadManager {
	if baseFolderID == "" {
		baseFolderID = RootFolderID
	}
	
	return &boxUploadManager{
		client:       client,
		baseFolderID: baseFolderID,
		maxRetries:   3,
	}
}

// SetBaseFolderID sets the base folder ID for uploads
func (um *boxUploadManager) SetBaseFolderID(folderID string) {
	um.mutex.Lock()
	defer um.mutex.Unlock()
	
	if folderID == "" {
		folderID = RootFolderID
	}
	um.baseFolderID = folderID
}

// GetBaseFolderID returns the current base folder ID
func (um *boxUploadManager) GetBaseFolderID() string {
	um.mutex.RLock()
	defer um.mutex.RUnlock()
	
	return um.baseFolderID
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
	
	// Extract username from email (videoOwner)
	username := extractUsernameFromEmail(videoOwner)
	if username == "" {
		err := fmt.Errorf("invalid video owner email: %s", videoOwner)
		result.Error = err
		return result, err
	}
	
	// Create folder structure: <username>/<year>/<month>/<day>
	folderPath := createDateBasedFolderPath(username, startTime)
	
	// Report progress - creating folders
	if progressCallback != nil {
		progressCallback(0, 0, PhaseCreatingFolders)
	}
	
	// Create folder structure with user permissions
	folder, err := um.createFolderStructureWithPermissions(ctx, folderPath, videoOwner)
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
	
	// Upload the file
	file, err := um.client.UploadFileWithProgress(localPath, folder.ID, result.FileName, uploadProgressCallback)
	if err != nil {
		err = fmt.Errorf("failed to upload file: %w", err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}
	
	result.FileID = file.ID
	result.FileSize = file.Size
	result.Success = true
	
	// Report progress - setting permissions
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseSettingPermissions)
	}
	
	// Set permissions for the uploaded file
	permissionIDs, err := um.setFilePermissions(ctx, file.ID, videoOwner)
	if err != nil {
		// Log warning but don't fail the upload
		logging.Warn("Failed to set permissions for file %s: %v", file.ID, err)
		result.PermissionsSet = false
	} else {
		result.PermissionsSet = true
		result.PermissionIDs = permissionIDs
	}
	
	result.Duration = time.Since(startTime)
	
	// Report progress - completed
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseCompleted)
	}
	
	logging.LogUserAction("box_upload_completed", videoOwner, map[string]interface{}{
		"file_id":         result.FileID,
		"file_name":       result.FileName,
		"file_size":       result.FileSize,
		"folder_id":       result.FolderID,
		"permissions_set": result.PermissionsSet,
		"duration_ms":     result.Duration.Milliseconds(),
	})
	
	return result, nil
}

// UploadFileWithEmailMapping uploads a file using separate Zoom and Box emails
// zoomEmail is used for logging/metadata, boxEmail is used for Box folder structure and permissions
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
	
	// Extract username from Box email for folder structure
	username := extractUsernameFromEmail(boxEmail)
	if username == "" {
		err := fmt.Errorf("invalid box email: %s", boxEmail)
		result.Error = err
		return result, err
	}
	
	// Create folder structure: <box_username>/<year>/<month>/<day>
	folderPath := createDateBasedFolderPath(username, startTime)
	
	// Report progress - creating folders
	if progressCallback != nil {
		progressCallback(0, 0, PhaseCreatingFolders)
	}
	
	// Create folder structure with Box user permissions
	folder, err := um.createFolderStructureWithPermissions(ctx, folderPath, boxEmail)
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
	
	// Upload the file
	file, err := um.client.UploadFileWithProgress(localPath, folder.ID, result.FileName, uploadProgressCallback)
	if err != nil {
		err = fmt.Errorf("failed to upload file: %w", err)
		result.Error = err
		if progressCallback != nil {
			progressCallback(0, 0, PhaseFailed)
		}
		return result, err
	}
	
	result.FileID = file.ID
	result.FileSize = file.Size
	result.Success = true
	
	// Report progress - setting permissions
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseSettingPermissions)
	}
	
	// Set permissions for the uploaded file using Box email
	permissionIDs, err := um.setFilePermissions(ctx, file.ID, boxEmail)
	if err != nil {
		// Log warning but don't fail the upload
		logging.Warn("Failed to set permissions for file %s (box email: %s): %v", file.ID, boxEmail, err)
		result.PermissionsSet = false
	} else {
		result.PermissionsSet = true
		result.PermissionIDs = permissionIDs
	}
	
	result.Duration = time.Since(startTime)
	
	// Report progress - completed
	if progressCallback != nil {
		progressCallback(result.FileSize, result.FileSize, PhaseCompleted)
	}
	
	// Log using both emails for context
	logging.LogUserAction("box_upload_completed_with_mapping", zoomEmail, map[string]interface{}{
		"zoom_email":      zoomEmail,
		"box_email":       boxEmail,
		"file_id":         result.FileID,
		"file_name":       result.FileName,
		"file_size":       result.FileSize,
		"folder_id":       result.FolderID,
		"permissions_set": result.PermissionsSet,
		"duration_ms":     result.Duration.Milliseconds(),
	})
	
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
			
			if result.PermissionsSet {
				statusTracker.MarkBoxPermissionsSet(downloadID, result.PermissionIDs)
			}
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

// createFolderStructureWithPermissions creates folder structure with user-specific permissions
func (um *boxUploadManager) createFolderStructureWithPermissions(ctx context.Context, folderPath, userEmail string) (*Folder, error) {
	// Extract username from folderPath to set user permissions on their folder
	pathParts := strings.Split(strings.Trim(folderPath, "/"), "/")
	if len(pathParts) == 0 {
		return um.createFolderStructure(ctx, folderPath)
	}
	
	// Create user permissions - grant user access to their own folder structure
	userPermissions := map[string]string{
		userEmail: RoleViewer, // User can view their recordings but not modify
	}
	
	return CreateFolderPathWithPermissions(um.client, folderPath, um.baseFolderID, userPermissions)
}

// setFilePermissions sets appropriate permissions on the uploaded file
func (um *boxUploadManager) setFilePermissions(ctx context.Context, fileID, videoOwner string) ([]string, error) {
	logging.Info("Setting permissions for file %s, owner: %s", fileID, videoOwner)
	
	var permissionIDs []string
	
	// Create collaboration to grant access to the video owner
	collaboration, err := um.client.CreateCollaboration(fileID, ItemTypeFile, videoOwner, RoleViewer)
	if err != nil {
		// Check if it's a conflict error (collaboration already exists)
		if boxErr, ok := err.(*BoxError); ok && boxErr.Code == ErrorCodeItemNameTaken {
			logging.Debug("Collaboration already exists for user %s on file %s", videoOwner, fileID)
			
			// List existing collaborations to get the ID
			collaborations, listErr := um.client.ListCollaborations(fileID, ItemTypeFile)
			if listErr != nil {
				return nil, fmt.Errorf("failed to list existing collaborations: %w", listErr)
			}
			
			// Find the collaboration for this user
			for _, collab := range collaborations.Entries {
				if collab.AccessibleBy != nil && collab.AccessibleBy.Login == videoOwner {
					permissionIDs = append(permissionIDs, collab.ID)
					break
				}
			}
			
			if len(permissionIDs) == 0 {
				return nil, fmt.Errorf("could not find existing collaboration for user %s", videoOwner)
			}
		} else {
			return nil, fmt.Errorf("failed to create collaboration: %w", err)
		}
	} else {
		// Successfully created new collaboration
		permissionIDs = append(permissionIDs, collaboration.ID)
		
		logging.LogUserAction("box_permission_granted", videoOwner, map[string]interface{}{
			"file_id":          fileID,
			"collaboration_id": collaboration.ID,
			"role":            collaboration.Role,
			"status":          collaboration.Status,
		})
	}
	
	logging.Info("Successfully set permissions for file %s, granted access to %s (permissions: %v)", 
		fileID, videoOwner, permissionIDs)
	
	return permissionIDs, nil
}

// Helper functions

// extractUsernameFromEmail extracts username portion from email address
func extractUsernameFromEmail(email string) string {
	if email == "" {
		return ""
	}
	
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	
	return parts[0]
}

// createDateBasedFolderPath creates a date-based folder path for the given username and date
func createDateBasedFolderPath(username string, date time.Time) string {
	utcDate := date.UTC()
	return fmt.Sprintf("%s/%04d/%02d/%02d", 
		username, 
		utcDate.Year(), 
		utcDate.Month(), 
		utcDate.Day())
}

// UploadWithResume uploads a file with support for resuming interrupted uploads
func (um *boxUploadManager) UploadWithResume(ctx context.Context, localPath, videoOwner, downloadID string, statusTracker download.StatusTracker) (*UploadResult, error) {
	// Check if upload already exists
	boxInfo, err := statusTracker.GetBoxUploadStatus(downloadID)
	if err == nil && boxInfo != nil {
		// Check if upload was completed successfully
		if boxInfo.Uploaded && boxInfo.FileID != "" {
			// Validate the existing upload
			valid, err := um.ValidateUploadedFile(ctx, boxInfo.FileID, 0) // Size will be checked by ValidateUploadedFile
			if err == nil && valid {
				// Upload already exists and is valid
				return &UploadResult{
					Success:        true,
					FileID:         boxInfo.FileID,
					FolderID:       boxInfo.FolderID,
					FileName:       filepath.Base(localPath),
					UploadDate:     boxInfo.UploadDate,
					PermissionsSet: boxInfo.PermissionsSet,
					PermissionIDs:  boxInfo.PermissionIDs,
					Duration:       0, // No upload time since it was already done
				}, nil
			}
			
			logging.Warn("Existing upload validation failed for %s, will re-upload", downloadID)
		}
		
		// Check if we should retry failed uploads
		if !download.ShouldRetryBoxUpload(download.DownloadEntry{Box: boxInfo}, um.maxRetries) {
			return nil, fmt.Errorf("upload for %s exceeded max retries", downloadID)
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
		if result.PermissionsSet {
			statusTracker.MarkBoxPermissionsSet(downloadID, result.PermissionIDs)
		}
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