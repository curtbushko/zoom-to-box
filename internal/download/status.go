// Package download provides status tracking functionality for download operations
package download

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DownloadStatusType represents the status of a download
type DownloadStatusType string

const (
	StatusPending     DownloadStatusType = "pending"
	StatusDownloading DownloadStatusType = "downloading"
	StatusCompleted   DownloadStatusType = "completed"
	StatusFailed      DownloadStatusType = "failed"
	StatusPaused      DownloadStatusType = "paused"
)

// BoxUploadInfo represents Box upload information
type BoxUploadInfo struct {
	Uploaded        bool      `json:"uploaded"`
	FileID          string    `json:"file_id,omitempty"`
	FolderID        string    `json:"folder_id,omitempty"`
	UploadDate      time.Time `json:"upload_date,omitempty"`
	PermissionsSet  bool      `json:"permissions_set"`
	PermissionIDs   []string  `json:"permission_ids,omitempty"`
	UploadRetries   int       `json:"upload_retries"`
	UploadError     string    `json:"upload_error,omitempty"`
	LastUploadAttempt time.Time `json:"last_upload_attempt,omitempty"`
}

// DownloadEntry represents a single download entry in the status file
type DownloadEntry struct {
	Status             DownloadStatusType     `json:"status"`
	FilePath           string                 `json:"file_path"`
	FileSize           int64                  `json:"file_size"`
	DownloadedSize     int64                  `json:"downloaded_size"`
	Checksum           string                 `json:"checksum,omitempty"`
	LastAttempt        time.Time              `json:"last_attempt"`
	MetadataDownloaded bool                   `json:"metadata_downloaded"`
	RetryCount         int                    `json:"retry_count"`
	Error              string                 `json:"error,omitempty"`
	StartTime          time.Time              `json:"start_time,omitempty"`
	CompletedTime      time.Time              `json:"completed_time,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	VideoOwner         string                 `json:"video_owner,omitempty"`
	Box                *BoxUploadInfo         `json:"box,omitempty"`
}

// StatusFile represents the structure of the status file
type StatusFile struct {
	Version     string                    `json:"version"`
	LastUpdated time.Time                 `json:"last_updated"`
	Downloads   map[string]DownloadEntry  `json:"downloads"`
}

// StatusTracker defines the interface for download status tracking
type StatusTracker interface {
	// Basic operations
	UpdateDownloadStatus(downloadID string, entry DownloadEntry) error
	GetDownloadStatus(downloadID string) (DownloadEntry, bool)
	DeleteDownloadStatus(downloadID string) error
	
	// Query operations
	GetAllDownloads() map[string]DownloadEntry
	GetDownloadsByStatus(status DownloadStatusType) map[string]DownloadEntry
	GetIncompleteDownloads() map[string]DownloadEntry
	
	// Box upload status methods
	UpdateBoxUploadStatus(downloadID string, boxInfo BoxUploadInfo) error
	GetBoxUploadStatus(downloadID string) (*BoxUploadInfo, error)
	MarkBoxUploadStarted(downloadID, folderID string) error
	MarkBoxUploadCompleted(downloadID, fileID string) error
	MarkBoxUploadFailed(downloadID, errorMsg string) error
	MarkBoxPermissionsSet(downloadID string, permissionIDs []string) error
	GetPendingBoxUploads() map[string]DownloadEntry
	GetFailedBoxUploads() map[string]DownloadEntry
	
	// Utility operations
	SaveToFile() error
	LoadFromFile() error
	Close() error
}

// statusTrackerImpl implements the StatusTracker interface
type statusTrackerImpl struct {
	statusFile string
	data       StatusFile
	mutex      sync.RWMutex
}

// NewStatusTracker creates a new status tracker with the given status file path
func NewStatusTracker(statusFile string) (StatusTracker, error) {
	if statusFile == "" {
		return nil, fmt.Errorf("status file path cannot be empty")
	}
	
	tracker := &statusTrackerImpl{
		statusFile: statusFile,
		data: StatusFile{
			Version:     "1.0",
			LastUpdated: time.Now().UTC(),
			Downloads:   make(map[string]DownloadEntry),
		},
		mutex: sync.RWMutex{},
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(statusFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create status file directory: %w", err)
	}
	
	// Load existing file if it exists
	if err := tracker.LoadFromFile(); err != nil {
		// If file doesn't exist or is corrupted, create a new one
		if err := tracker.SaveToFile(); err != nil {
			return nil, fmt.Errorf("failed to create status file: %w", err)
		}
	}
	
	return tracker, nil
}

// UpdateDownloadStatus updates or creates a download status entry
func (st *statusTrackerImpl) UpdateDownloadStatus(downloadID string, entry DownloadEntry) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// GetDownloadStatus retrieves a download status entry
func (st *statusTrackerImpl) GetDownloadStatus(downloadID string) (DownloadEntry, bool) {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	entry, exists := st.data.Downloads[downloadID]
	return entry, exists
}

// DeleteDownloadStatus removes a download status entry
func (st *statusTrackerImpl) DeleteDownloadStatus(downloadID string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	delete(st.data.Downloads, downloadID)
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// GetAllDownloads returns all download entries
func (st *statusTrackerImpl) GetAllDownloads() map[string]DownloadEntry {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	// Return a copy to prevent external modification
	result := make(map[string]DownloadEntry, len(st.data.Downloads))
	for id, entry := range st.data.Downloads {
		result[id] = entry
	}
	
	return result
}

// GetDownloadsByStatus returns downloads filtered by status
func (st *statusTrackerImpl) GetDownloadsByStatus(status DownloadStatusType) map[string]DownloadEntry {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	result := make(map[string]DownloadEntry)
	for id, entry := range st.data.Downloads {
		if entry.Status == status {
			result[id] = entry
		}
	}
	
	return result
}

// GetIncompleteDownloads returns downloads that are not completed
func (st *statusTrackerImpl) GetIncompleteDownloads() map[string]DownloadEntry {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	result := make(map[string]DownloadEntry)
	for id, entry := range st.data.Downloads {
		if entry.Status != StatusCompleted {
			result[id] = entry
		}
	}
	
	return result
}

// SaveToFile saves the current status to file
func (st *statusTrackerImpl) SaveToFile() error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	return st.saveToFileUnsafe()
}

// saveToFileUnsafe saves without acquiring mutex (internal use)
func (st *statusTrackerImpl) saveToFileUnsafe() error {
	st.data.LastUpdated = time.Now().UTC()
	
	data, err := json.MarshalIndent(st.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status data: %w", err)
	}
	
	// Write to temporary file first, then rename for atomic operation
	tempFile := st.statusFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary status file: %w", err)
	}
	
	if err := os.Rename(tempFile, st.statusFile); err != nil {
		os.Remove(tempFile) // Clean up temporary file
		return fmt.Errorf("failed to rename status file: %w", err)
	}
	
	return nil
}

// LoadFromFile loads status from file
func (st *statusTrackerImpl) LoadFromFile() error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	// Check if file exists
	if _, err := os.Stat(st.statusFile); os.IsNotExist(err) {
		return fmt.Errorf("status file does not exist")
	}
	
	data, err := os.ReadFile(st.statusFile)
	if err != nil {
		return fmt.Errorf("failed to read status file: %w", err)
	}
	
	var statusData StatusFile
	if err := json.Unmarshal(data, &statusData); err != nil {
		// File is corrupted, return error but don't fail completely
		return fmt.Errorf("failed to parse status file (corrupted): %w", err)
	}
	
	// Validate and set defaults if needed
	if statusData.Version == "" {
		statusData.Version = "1.0"
	}
	
	if statusData.Downloads == nil {
		statusData.Downloads = make(map[string]DownloadEntry)
	}
	
	st.data = statusData
	return nil
}

// Close closes the status tracker
func (st *statusTrackerImpl) Close() error {
	// Final save before closing
	return st.SaveToFile()
}

// CalculateFileChecksum calculates SHA256 checksum of a file
func CalculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %w", err)
	}
	
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

// VerifyFileChecksum verifies that a file matches the expected checksum
func VerifyFileChecksum(filePath, expectedChecksum string) (bool, error) {
	actualChecksum, err := CalculateFileChecksum(filePath)
	if err != nil {
		return false, err
	}
	
	return actualChecksum == expectedChecksum, nil
}

// UpdateDownloadProgress is a convenience method to update download progress
func (st *statusTrackerImpl) UpdateDownloadProgress(downloadID string, bytesDownloaded int64, status DownloadStatusType) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	entry.DownloadedSize = bytesDownloaded
	entry.Status = status
	entry.LastAttempt = time.Now().UTC()
	
	// Set completion time if completed
	if status == StatusCompleted {
		entry.CompletedTime = time.Now().UTC()
	}
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// IncrementRetryCount increments the retry count for a download
func (st *statusTrackerImpl) IncrementRetryCount(downloadID string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	entry.RetryCount++
	entry.LastAttempt = time.Now().UTC()
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// SetDownloadError sets an error message for a download
func (st *statusTrackerImpl) SetDownloadError(downloadID string, errorMsg string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	entry.Error = errorMsg
	entry.Status = StatusFailed
	entry.LastAttempt = time.Now().UTC()
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// GetStatusSummary returns a summary of download statuses
func (st *statusTrackerImpl) GetStatusSummary() map[DownloadStatusType]int {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	summary := make(map[DownloadStatusType]int)
	
	for _, entry := range st.data.Downloads {
		summary[entry.Status]++
	}
	
	return summary
}

// Integration helpers for DownloadManager

// CreateDownloadEntry creates a new download entry from a DownloadRequest
func CreateDownloadEntry(req DownloadRequest, status DownloadStatusType) DownloadEntry {
	return DownloadEntry{
		Status:             status,
		FilePath:           req.Destination,
		FileSize:           req.FileSize,
		DownloadedSize:     0,
		LastAttempt:        time.Now().UTC(),
		MetadataDownloaded: false,
		RetryCount:         0,
		StartTime:          time.Now().UTC(),
		Metadata:           req.Metadata,
	}
}

// UpdateFromProgressUpdate updates a download entry from a ProgressUpdate
func UpdateEntryFromProgress(entry DownloadEntry, progress ProgressUpdate) DownloadEntry {
	entry.DownloadedSize = progress.BytesDownloaded
	entry.LastAttempt = progress.Timestamp
	
	// Map DownloadState to DownloadStatusType
	switch progress.State {
	case DownloadStateQueued:
		entry.Status = StatusPending
	case DownloadStateDownloading:
		entry.Status = StatusDownloading
	case DownloadStatePaused:
		entry.Status = StatusPaused
	case DownloadStateCompleted:
		entry.Status = StatusCompleted
		entry.CompletedTime = time.Now().UTC()
	case DownloadStateFailed:
		entry.Status = StatusFailed
		if progress.Error != nil {
			entry.Error = progress.Error.Error()
		}
	case DownloadStateCancelled:
		entry.Status = StatusFailed
		entry.Error = "cancelled"
	}
	
	return entry
}

// UpdateFromDownloadResult updates a download entry from a DownloadResult
func UpdateEntryFromResult(entry DownloadEntry, result DownloadResult) DownloadEntry {
	entry.DownloadedSize = result.BytesDownloaded
	entry.RetryCount = result.RetryCount
	entry.CompletedTime = result.Timestamp
	
	if result.Success {
		entry.Status = StatusCompleted
	} else {
		entry.Status = StatusFailed
		if result.Error != nil {
			entry.Error = result.Error.Error()
		}
	}
	
	// Merge metadata
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}
	
	for key, value := range result.Metadata {
		entry.Metadata[key] = value
	}
	
	// Add result-specific metadata
	entry.Metadata["duration_seconds"] = result.Duration.Seconds()
	entry.Metadata["average_speed"] = result.AverageSpeed
	entry.Metadata["resumed"] = result.Resumed
	
	return entry
}

// ShouldResumeDownload checks if a download should be resumed based on its status
func ShouldResumeDownload(entry DownloadEntry) bool {
	switch entry.Status {
	case StatusPending, StatusFailed, StatusPaused:
		return true
	case StatusDownloading:
		// Resume if it's been too long since last attempt (likely stale)
		return time.Since(entry.LastAttempt) > 5*time.Minute
	case StatusCompleted:
		return false
	default:
		return false
	}
}

// GetResumeOffset returns the byte offset to resume download from
func GetResumeOffset(entry DownloadEntry) int64 {
	if entry.Status == StatusCompleted {
		return entry.FileSize // Don't resume completed downloads
	}
	
	return entry.DownloadedSize
}

// IsIntegrityValid checks if a completed download has valid integrity
func IsIntegrityValid(entry DownloadEntry) bool {
	if entry.Status != StatusCompleted {
		return false
	}
	
	// Check size match
	if entry.FileSize > 0 && entry.DownloadedSize != entry.FileSize {
		return false
	}
	
	// If we have a checksum, file should exist for verification
	if entry.Checksum != "" {
		if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
			return false
		}
	}
	
	return true
}

// NeedsChecksumVerification checks if a download needs checksum verification
func NeedsChecksumVerification(entry DownloadEntry) bool {
	return entry.Status == StatusCompleted && entry.Checksum == ""
}

// StatusTrackerWithManager provides integration between StatusTracker and DownloadManager
type StatusTrackerWithManager struct {
	StatusTracker
	manager DownloadManager
}

// NewStatusTrackerWithManager creates a status tracker integrated with a download manager
func NewStatusTrackerWithManager(statusFile string, manager DownloadManager) (*StatusTrackerWithManager, error) {
	tracker, err := NewStatusTracker(statusFile)
	if err != nil {
		return nil, err
	}
	
	return &StatusTrackerWithManager{
		StatusTracker: tracker,
		manager:      manager,
	}, nil
}

// StartDownloadWithTracking starts a download and tracks its status
func (stm *StatusTrackerWithManager) StartDownloadWithTracking(ctx context.Context, req DownloadRequest, progressCallback ProgressCallback) (*DownloadResult, error) {
	// Create initial entry
	entry := CreateDownloadEntry(req, StatusPending)
	
	// Check if download already exists and should be resumed
	if existing, exists := stm.GetDownloadStatus(req.ID); exists {
		if !ShouldResumeDownload(existing) {
			return nil, fmt.Errorf("download %s already exists with status %s", req.ID, existing.Status)
		}
		entry = existing
		entry.Status = StatusDownloading
		entry.StartTime = time.Now().UTC()
	}
	
	// Update status to downloading
	if err := stm.UpdateDownloadStatus(req.ID, entry); err != nil {
		return nil, fmt.Errorf("failed to update download status: %w", err)
	}
	
	// Create wrapper progress callback that updates status
	wrappedCallback := func(progress ProgressUpdate) {
		entry = UpdateEntryFromProgress(entry, progress)
		stm.UpdateDownloadStatus(req.ID, entry)
		
		// Call original callback if provided
		if progressCallback != nil {
			progressCallback(progress)
		}
	}
	
	// Start download
	result, err := stm.manager.Download(ctx, req, wrappedCallback)
	
	// Update final status
	if result != nil {
		entry = UpdateEntryFromResult(entry, *result)
	} else if err != nil {
		entry.Status = StatusFailed
		entry.Error = err.Error()
	}
	
	stm.UpdateDownloadStatus(req.ID, entry)
	
	return result, err
}

// Box Upload Status Helper Functions

// UpdateBoxUploadStatus updates the Box upload information for a download entry
func (st *statusTrackerImpl) UpdateBoxUploadStatus(downloadID string, boxInfo BoxUploadInfo) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	entry.Box = &boxInfo
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// GetBoxUploadStatus returns the Box upload status for a download entry
func (st *statusTrackerImpl) GetBoxUploadStatus(downloadID string) (*BoxUploadInfo, error) {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return nil, fmt.Errorf("download %s not found", downloadID)
	}
	
	return entry.Box, nil
}

// MarkBoxUploadStarted marks that a Box upload has started for a download entry
func (st *statusTrackerImpl) MarkBoxUploadStarted(downloadID, folderID string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	if entry.Box == nil {
		entry.Box = &BoxUploadInfo{}
	}
	
	entry.Box.FolderID = folderID
	entry.Box.LastUploadAttempt = time.Now().UTC()
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// MarkBoxUploadCompleted marks that a Box upload has completed successfully
func (st *statusTrackerImpl) MarkBoxUploadCompleted(downloadID, fileID string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	if entry.Box == nil {
		entry.Box = &BoxUploadInfo{}
	}
	
	entry.Box.Uploaded = true
	entry.Box.FileID = fileID
	entry.Box.UploadDate = time.Now().UTC()
	entry.Box.UploadError = ""
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// MarkBoxUploadFailed marks that a Box upload has failed
func (st *statusTrackerImpl) MarkBoxUploadFailed(downloadID, errorMsg string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	if entry.Box == nil {
		entry.Box = &BoxUploadInfo{}
	}
	
	entry.Box.Uploaded = false
	entry.Box.UploadError = errorMsg
	entry.Box.UploadRetries++
	entry.Box.LastUploadAttempt = time.Now().UTC()
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// MarkBoxPermissionsSet marks that Box permissions have been set for uploaded file
func (st *statusTrackerImpl) MarkBoxPermissionsSet(downloadID string, permissionIDs []string) error {
	st.mutex.Lock()
	defer st.mutex.Unlock()
	
	entry, exists := st.data.Downloads[downloadID]
	if !exists {
		return fmt.Errorf("download %s not found", downloadID)
	}
	
	if entry.Box == nil {
		return fmt.Errorf("no Box upload info found for download %s", downloadID)
	}
	
	entry.Box.PermissionsSet = true
	entry.Box.PermissionIDs = permissionIDs
	
	st.data.Downloads[downloadID] = entry
	st.data.LastUpdated = time.Now().UTC()
	
	return st.saveToFileUnsafe()
}

// GetPendingBoxUploads returns downloads that are completed but not uploaded to Box
func (st *statusTrackerImpl) GetPendingBoxUploads() map[string]DownloadEntry {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	result := make(map[string]DownloadEntry)
	for id, entry := range st.data.Downloads {
		if entry.Status == StatusCompleted && (entry.Box == nil || !entry.Box.Uploaded) {
			result[id] = entry
		}
	}
	
	return result
}

// GetFailedBoxUploads returns downloads with failed Box uploads that can be retried
func (st *statusTrackerImpl) GetFailedBoxUploads() map[string]DownloadEntry {
	st.mutex.RLock()
	defer st.mutex.RUnlock()
	
	result := make(map[string]DownloadEntry)
	for id, entry := range st.data.Downloads {
		if entry.Box != nil && !entry.Box.Uploaded && entry.Box.UploadError != "" {
			result[id] = entry
		}
	}
	
	return result
}

// ShouldRetryBoxUpload checks if a failed Box upload should be retried
func ShouldRetryBoxUpload(entry DownloadEntry, maxRetries int) bool {
	if entry.Box == nil {
		return true // No upload attempted yet
	}
	
	if entry.Box.Uploaded {
		return false // Already uploaded successfully
	}
	
	if entry.Box.UploadRetries >= maxRetries {
		return false // Exceeded max retries
	}
	
	// Check if enough time has passed since last attempt (exponential backoff)
	if !entry.Box.LastUploadAttempt.IsZero() {
		minWait := time.Duration(entry.Box.UploadRetries*entry.Box.UploadRetries) * time.Minute
		if time.Since(entry.Box.LastUploadAttempt) < minWait {
			return false // Too soon to retry
		}
	}
	
	return true
}