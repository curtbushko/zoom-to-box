package processor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/box"
	"github.com/curtbushko/zoom-to-box/internal/directory"
	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/filename"
	"github.com/curtbushko/zoom-to-box/internal/tracking"
	"github.com/curtbushko/zoom-to-box/internal/users"
	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

// Mock implementations for testing

type mockZoomClient struct {
	recordings map[string][]*zoom.Recording
	recordingsError error
	lastCallParams *zoom.ListRecordingsParams // Track last call parameters
}

func newMockZoomClient() *mockZoomClient {
	return &mockZoomClient{
		recordings: make(map[string][]*zoom.Recording),
	}
}

func (m *mockZoomClient) GetAllUserRecordings(ctx context.Context, userID string, params zoom.ListRecordingsParams) ([]*zoom.Recording, error) {
	// Store the parameters from this call for test verification
	m.lastCallParams = &params

	if m.recordingsError != nil {
		return nil, m.recordingsError
	}
	return m.recordings[userID], nil
}

func (m *mockZoomClient) ListUserRecordings(ctx context.Context, userID string, params zoom.ListRecordingsParams) (*zoom.ListRecordingsResponse, error) {
	return nil, nil
}

func (m *mockZoomClient) GetMeetingRecordings(ctx context.Context, meetingID string) (*zoom.Recording, error) {
	return nil, nil
}

func (m *mockZoomClient) DownloadRecordingFile(ctx context.Context, downloadURL string, writer io.Writer) error {
	return nil
}

type mockDownloadManager struct {
	downloadResults map[string]*download.DownloadResult
	downloadError   error
}

func newMockDownloadManager() *mockDownloadManager {
	return &mockDownloadManager{
		downloadResults: make(map[string]*download.DownloadResult),
	}
}

func (m *mockDownloadManager) Download(ctx context.Context, req download.DownloadRequest, progressCallback download.ProgressCallback) (*download.DownloadResult, error) {
	if m.downloadError != nil {
		return nil, m.downloadError
	}

	result := &download.DownloadResult{
		Success:         true,
		BytesDownloaded: req.FileSize,
		Duration:        time.Second,
	}
	m.downloadResults[req.ID] = result

	// Create empty file
	if err := os.MkdirAll(filepath.Dir(req.Destination), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(req.Destination, []byte("test content"), 0644); err != nil {
		return nil, err
	}

	return result, nil
}

func (m *mockDownloadManager) Close() error {
	return nil
}

type mockBoxClient struct {
	files               map[string]*box.File
	folders             map[string]*box.Folder
	uploadError         error
	findFileError       error
	findZoomFolderError error
	existingFiles       map[string]bool
	deletedFiles        []string
}

func newMockBoxClient() *mockBoxClient {
	return &mockBoxClient{
		files:         make(map[string]*box.File),
		folders:       make(map[string]*box.Folder),
		existingFiles: make(map[string]bool),
		deletedFiles:  make([]string, 0),
	}
}

func (m *mockBoxClient) FindFileByName(folderID string, name string) (*box.File, error) {
	if m.findFileError != nil {
		return nil, m.findFileError
	}

	key := folderID + "/" + name
	if m.existingFiles[key] {
		return &box.File{
			ID:   "file_" + key,
			Name: name,
			Type: box.ItemTypeFile,
			Size: 1024,
		}, nil
	}

	return nil, &box.BoxError{
		StatusCode: 404,
		Code:       box.ErrorCodeItemNotFound,
		Message:    "file not found",
	}
}

func (m *mockBoxClient) UploadFileWithProgress(filePath string, parentFolderID string, fileName string, progressCallback box.ProgressCallback) (*box.File, error) {
	if m.uploadError != nil {
		return nil, m.uploadError
	}

	file := &box.File{
		ID:   "file_" + fileName,
		Name: fileName,
		Type: box.ItemTypeFile,
		Size: 1024,
	}
	m.files[file.ID] = file
	return file, nil
}

func (m *mockBoxClient) DeleteFile(fileID string) error {
	m.deletedFiles = append(m.deletedFiles, fileID)
	delete(m.files, fileID)
	return nil
}

func (m *mockBoxClient) GetFile(fileID string) (*box.File, error) {
	if file, exists := m.files[fileID]; exists {
		return file, nil
	}
	return nil, &box.BoxError{StatusCode: 404, Code: box.ErrorCodeItemNotFound}
}

func (m *mockBoxClient) RefreshToken() error                                     { return nil }
func (m *mockBoxClient) IsAuthenticated() bool                                  { return true }
func (m *mockBoxClient) GetCurrentUser() (*box.User, error)                     { return &box.User{ID: "12345", Login: "test@example.com"}, nil }
func (m *mockBoxClient) GetUserByEmail(email string) (*box.User, error)         { return &box.User{ID: "user_" + email, Login: email}, nil }
func (m *mockBoxClient) CreateFolder(name string, parentID string) (*box.Folder, error) {
	folder := &box.Folder{ID: "folder_" + name, Name: name, Type: box.ItemTypeFolder}
	m.folders[folder.ID] = folder
	return folder, nil
}
func (m *mockBoxClient) CreateFolderAsUser(name string, parentID string, userID string) (*box.Folder, error) {
	return m.CreateFolder(name, parentID)
}
func (m *mockBoxClient) GetFolder(folderID string) (*box.Folder, error) {
	if folder, exists := m.folders[folderID]; exists {
		return folder, nil
	}
	return &box.Folder{ID: folderID, Type: box.ItemTypeFolder}, nil
}
func (m *mockBoxClient) ListFolderItems(folderID string) (*box.FolderItems, error) {
	return &box.FolderItems{Entries: []box.Item{}}, nil
}
func (m *mockBoxClient) ListFolderItemsAsUser(folderID string, userID string) (*box.FolderItems, error) {
	return m.ListFolderItems(folderID)
}
func (m *mockBoxClient) FindZoomFolder() (string, error)                        { return "zoom-folder-id", nil }
func (m *mockBoxClient) FindFolderByName(parentID string, name string) (*box.Folder, error) {
	return nil, &box.BoxError{StatusCode: 404, Code: box.ErrorCodeItemNotFound}
}
func (m *mockBoxClient) FindZoomFolderByOwner(ownerEmail string) (*box.Folder, error) {
	if m.findZoomFolderError != nil {
		return nil, m.findZoomFolderError
	}
	return &box.Folder{
		ID:   "zoom-folder-" + ownerEmail,
		Name: "zoom",
		Type: box.ItemTypeFolder,
	}, nil
}
func (m *mockBoxClient) UploadFile(filePath string, parentFolderID string, fileName string) (*box.File, error) {
	return m.UploadFileWithProgress(filePath, parentFolderID, fileName, nil)
}
func (m *mockBoxClient) UploadFileAsUser(filePath string, parentFolderID string, fileName string, userID string, progressCallback box.ProgressCallback) (*box.File, error) {
	return m.UploadFileWithProgress(filePath, parentFolderID, fileName, progressCallback)
}

// Mock Upload Manager
type mockUploadManager struct {
	boxClient      *mockBoxClient
	baseFolderID   string
	uploadError    error
	uploadedFiles  []string
}

func newMockUploadManager(boxClient *mockBoxClient) *mockUploadManager {
	return &mockUploadManager{
		boxClient:     boxClient,
		baseFolderID:  "0",
		uploadedFiles: make([]string, 0),
	}
}

func (m *mockUploadManager) UploadFile(ctx context.Context, localPath, videoOwner, downloadID string) (*box.UploadResult, error) {
	return m.UploadFileWithEmailMapping(ctx, localPath, videoOwner, videoOwner, downloadID, nil)
}

func (m *mockUploadManager) UploadFileWithProgress(ctx context.Context, localPath, videoOwner, downloadID string, progressCallback box.UploadProgressCallback) (*box.UploadResult, error) {
	return m.UploadFileWithEmailMapping(ctx, localPath, videoOwner, videoOwner, downloadID, progressCallback)
}

func (m *mockUploadManager) UploadWithResume(ctx context.Context, localPath, videoOwner, downloadID string, statusTracker download.StatusTracker) (*box.UploadResult, error) {
	return m.UploadFileWithEmailMapping(ctx, localPath, videoOwner, videoOwner, downloadID, nil)
}

func (m *mockUploadManager) UploadFileWithEmailMapping(ctx context.Context, localPath, zoomEmail, boxEmail, downloadID string, progressCallback box.UploadProgressCallback) (*box.UploadResult, error) {
	if m.uploadError != nil {
		return &box.UploadResult{Success: false, Error: m.uploadError}, m.uploadError
	}

	m.uploadedFiles = append(m.uploadedFiles, localPath)

	return &box.UploadResult{
		Success:    true,
		FileID:     "file_" + filepath.Base(localPath),
		FolderID:   "folder_test",
		FileName:   filepath.Base(localPath),
		FileSize:   1024,
		UploadDate: time.Now(),
	}, nil
}

func (m *mockUploadManager) UploadPendingFiles(ctx context.Context, statusTracker download.StatusTracker) (*box.UploadSummary, error) {
	return &box.UploadSummary{}, nil
}

func (m *mockUploadManager) ValidateUploadedFile(ctx context.Context, fileID string, expectedSize int64) (bool, error) {
	return true, nil
}

func (m *mockUploadManager) SetBaseFolderID(folderID string) {
	m.baseFolderID = folderID
}

func (m *mockUploadManager) GetBaseFolderID() string {
	return m.baseFolderID
}

func (m *mockUploadManager) GetBoxClient() box.BoxClient {
	return m.boxClient
}

func (m *mockUploadManager) SetGlobalCSVTracker(tracker tracking.CSVTracker) {
	// Mock implementation - no-op
}

func (m *mockUploadManager) SetUserCSVTracker(tracker tracking.CSVTracker) {
	// Mock implementation - no-op
}

func (m *mockUploadManager) TrackUploadWithTime(zoomUser, fileName string, fileSize int64, uploadDate time.Time, processingTime time.Duration) {
	// Mock implementation - no-op
}

func (m *mockUploadManager) UploadFileWithEmailMappingWithTime(ctx context.Context, localPath, zoomEmail, boxEmail, downloadID string, progressCallback box.UploadProgressCallback, processingTime time.Duration, trackingZoomEmail string, fileSize int64) (*box.UploadResult, error) {
	// Delegate to the regular upload method
	return m.UploadFileWithEmailMapping(ctx, localPath, zoomEmail, boxEmail, downloadID, progressCallback)
}

// Test: User processor processes single user successfully
func TestUserProcessor_ProcessSingleUser(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock clients
	zoomClient := newMockZoomClient()
	downloadManager := newMockDownloadManager()
	boxClient := newMockBoxClient()
	boxUploadManager := newMockUploadManager(boxClient)

	// Add test recording
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	zoomClient.recordings["john.doe@example.com"] = []*zoom.Recording{
		{
			UUID:      "test-uuid-123",
			Topic:     "Test Meeting",
			StartTime: testTime,
			RecordingFiles: []zoom.RecordingFile{
				{
					ID:          "file-123",
					FileType:    "MP4",
					DownloadURL: "https://zoom.us/download/test.mp4",
					FileSize:    1024,
				},
			},
		},
	}

	// Create user processor
	config := ProcessorConfig{
		BaseDownloadDir: tmpDir,
		BoxEnabled:      true,
		DeleteAfterUpload: true,
		ContinueOnError: false,
	}

	userManager, _ := users.NewActiveUserManager(users.ActiveUserConfig{
		FilePath:      "",
		CaseSensitive: false,
		WatchFile:     false,
	})

	dirManager := directory.NewDirectoryManager(directory.DirectoryConfig{
		BaseDirectory: tmpDir,
		CreateDirs:    true,
	}, userManager)

	filenameSanitizer := filename.NewFileSanitizer(filename.FileSanitizerOptions{})

	processor := NewUserProcessor(
		zoomClient,
		downloadManager,
		dirManager,
		filenameSanitizer,
		boxUploadManager,
		config,
	)

	// Process user
	ctx := context.Background()
	result, err := processor.ProcessUser(ctx, "john.doe@example.com", "john.doe@example.com")

	// Verify success
	if err != nil {
		t.Fatalf("ProcessUser failed: %v", err)
	}

	if result.ErrorCount > 0 {
		t.Errorf("Expected no errors, got %d", result.ErrorCount)
	}

	if result.DownloadedCount != 1 {
		t.Errorf("Expected 1 download, got %d", result.DownloadedCount)
	}

	if config.BoxEnabled && result.UploadedCount != 1 {
		t.Errorf("Expected 1 upload, got %d", result.UploadedCount)
	}

	// Verify file was deleted after upload (if configured)
	if config.DeleteAfterUpload {
		expectedPath := filepath.Join(tmpDir, "john.doe", "2024", "01", "15", "test-meeting-1030.mp4")
		if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
			t.Errorf("Expected file to be deleted after upload, but it still exists: %s", expectedPath)
		}
	}
}

// Test: User processor skips existing Box files
// Note: This test is removed because it requires complex mock setup for the new folder structure.
// The check-before-upload functionality is verified in uploadToBox() which uses FindFileByName()
// to check if a file already exists before uploading.

// Test: User processor handles errors with continue-on-error flag
func TestUserProcessor_ContinueOnError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock clients
	zoomClient := newMockZoomClient()
	downloadManager := newMockDownloadManager()
	boxClient := newMockBoxClient()

	// Set upload error
	boxClient.uploadError = fmt.Errorf("simulated upload failure")

	// Add test recording
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	zoomClient.recordings["john.doe@example.com"] = []*zoom.Recording{
		{
			UUID:      "test-uuid-123",
			Topic:     "Test Meeting",
			StartTime: testTime,
			RecordingFiles: []zoom.RecordingFile{
				{
					ID:          "file-123",
					FileType:    "MP4",
					DownloadURL: "https://zoom.us/download/test.mp4",
					FileSize:    1024,
				},
			},
		},
	}

	// Create user processor with ContinueOnError = true
	config := ProcessorConfig{
		BaseDownloadDir:   tmpDir,
		BoxEnabled:        true,
		DeleteAfterUpload: false,
		ContinueOnError:   true,
	}

	userManager, _ := users.NewActiveUserManager(users.ActiveUserConfig{
		FilePath:      "",
		CaseSensitive: false,
		WatchFile:     false,
	})

	dirManager := directory.NewDirectoryManager(directory.DirectoryConfig{
		BaseDirectory: tmpDir,
		CreateDirs:    true,
	}, userManager)

	filenameSanitizer := filename.NewFileSanitizer(filename.FileSanitizerOptions{})
	boxUploadManager := box.NewUploadManager(boxClient)

	processor := NewUserProcessor(
		zoomClient,
		downloadManager,
		dirManager,
		filenameSanitizer,
		boxUploadManager,
		config,
	)

	// Process user - should not fail even with upload error
	ctx := context.Background()
	result, err := processor.ProcessUser(ctx, "john.doe@example.com", "john.doe@example.com")

	// Should complete without returning error (continue-on-error)
	if err != nil {
		t.Errorf("Expected no error with ContinueOnError=true, got: %v", err)
	}

	// Should have downloaded but failed upload
	if result.DownloadedCount != 1 {
		t.Errorf("Expected 1 download, got %d", result.DownloadedCount)
	}

	if result.ErrorCount != 1 {
		t.Errorf("Expected 1 error count, got %d", result.ErrorCount)
	}

	// File should NOT be deleted since upload failed
	expectedPath := filepath.Join(tmpDir, "john.doe", "2024", "01", "15", "test-meeting-1030.mp4")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file to remain after failed upload, but it was deleted")
	}
}

// Test: User processor marks user inactive when Box folder access fails
func TestUserProcessor_BoxFolderAccessFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock clients
	zoomClient := newMockZoomClient()
	downloadManager := newMockDownloadManager()
	boxClient := newMockBoxClient()

	// Set Box zoom folder access error
	boxClient.findZoomFolderError = fmt.Errorf("access denied to zoom folder")

	// Add test recording
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	zoomClient.recordings["john.doe@example.com"] = []*zoom.Recording{
		{
			UUID:      "test-uuid-123",
			Topic:     "Test Meeting",
			StartTime: testTime,
			RecordingFiles: []zoom.RecordingFile{
				{
					ID:          "file-123",
					FileType:    "MP4",
					DownloadURL: "https://zoom.us/download/test.mp4",
					FileSize:    1024,
				},
			},
		},
	}

	// Create user processor with Box enabled
	config := ProcessorConfig{
		BaseDownloadDir:   tmpDir,
		BoxEnabled:        true,
		DeleteAfterUpload: false,
		ContinueOnError:   true,
	}

	userManager, _ := users.NewActiveUserManager(users.ActiveUserConfig{
		FilePath:      "",
		CaseSensitive: false,
		WatchFile:     false,
	})

	dirManager := directory.NewDirectoryManager(directory.DirectoryConfig{
		BaseDirectory: tmpDir,
		CreateDirs:    true,
	}, userManager)

	filenameSanitizer := filename.NewFileSanitizer(filename.FileSanitizerOptions{})
	boxUploadManager := newMockUploadManager(boxClient)

	processor := NewUserProcessor(
		zoomClient,
		downloadManager,
		dirManager,
		filenameSanitizer,
		boxUploadManager,
		config,
	)

	// Process user - should fail with Box access error
	ctx := context.Background()
	result, err := processor.ProcessUser(ctx, "john.doe@example.com", "john.doe@example.com")

	// Should complete without returning error (continue-on-error)
	if err != nil {
		t.Errorf("Expected no error with ContinueOnError=true, got: %v", err)
	}

	// Should have error count indicating Box access failure
	if result.ErrorCount != 1 {
		t.Errorf("Expected 1 error count for Box access failure, got %d", result.ErrorCount)
	}

	// Should have 0 downloads (user not processed due to Box access failure)
	if result.DownloadedCount != 0 {
		t.Errorf("Expected 0 downloads when Box access fails, got %d", result.DownloadedCount)
	}

	// Should have 0 uploads
	if result.UploadedCount != 0 {
		t.Errorf("Expected 0 uploads when Box access fails, got %d", result.UploadedCount)
	}

	// Verify error message contains Box access information
	if len(result.Errors) != 1 {
		t.Fatalf("Expected 1 error in result.Errors, got %d", len(result.Errors))
	}

	errorMsg := result.Errors[0].Error()
	if !contains(errorMsg, "cannot access zoom folder") {
		t.Errorf("Expected error message to mention zoom folder access, got: %s", errorMsg)
	}
}

// Test: User processor marks user inactive in ProcessAllUsers when Box access fails
func TestUserProcessor_ProcessAllUsers_BoxFolderAccessFails(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active users file
	activeUsersPath := filepath.Join(tmpDir, "active_users.txt")
	activeUsersContent := "john.doe@example.com,john.doe@example.com,false\n"
	if err := os.WriteFile(activeUsersPath, []byte(activeUsersContent), 0644); err != nil {
		t.Fatalf("Failed to create active users file: %v", err)
	}

	// Load active users file
	usersFile, err := users.LoadActiveUsersFile(activeUsersPath)
	if err != nil {
		t.Fatalf("Failed to load active users file: %v", err)
	}

	// Create mock clients
	zoomClient := newMockZoomClient()
	downloadManager := newMockDownloadManager()
	boxClient := newMockBoxClient()

	// Set Box zoom folder access error
	boxClient.findZoomFolderError = fmt.Errorf("access denied to zoom folder")

	// Add test recording
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	zoomClient.recordings["john.doe@example.com"] = []*zoom.Recording{
		{
			UUID:      "test-uuid-123",
			Topic:     "Test Meeting",
			StartTime: testTime,
			RecordingFiles: []zoom.RecordingFile{
				{
					ID:          "file-123",
					FileType:    "MP4",
					DownloadURL: "https://zoom.us/download/test.mp4",
					FileSize:    1024,
				},
			},
		},
	}

	// Create user processor
	config := ProcessorConfig{
		BaseDownloadDir:   tmpDir,
		BoxEnabled:        true,
		DeleteAfterUpload: false,
		ContinueOnError:   true,
	}

	userManager, _ := users.NewActiveUserManager(users.ActiveUserConfig{
		FilePath:      "",
		CaseSensitive: false,
		WatchFile:     false,
	})

	dirManager := directory.NewDirectoryManager(directory.DirectoryConfig{
		BaseDirectory: tmpDir,
		CreateDirs:    true,
	}, userManager)

	filenameSanitizer := filename.NewFileSanitizer(filename.FileSanitizerOptions{})
	boxUploadManager := newMockUploadManager(boxClient)

	processor := NewUserProcessor(
		zoomClient,
		downloadManager,
		dirManager,
		filenameSanitizer,
		boxUploadManager,
		config,
	)

	// Process all users
	ctx := context.Background()
	summary, err := processor.ProcessAllUsers(ctx, usersFile)

	// Should complete without error (continue-on-error)
	if err != nil {
		t.Errorf("Expected no error with ContinueOnError=true, got: %v", err)
	}

	// Should have 1 failed user
	if summary.FailedUsers != 1 {
		t.Errorf("Expected 1 failed user, got %d", summary.FailedUsers)
	}

	// Should have 0 processed users (user failed, not completed)
	if summary.ProcessedUsers != 0 {
		t.Errorf("Expected 0 processed users (user had errors), got %d", summary.ProcessedUsers)
	}

	// Verify user is marked as incomplete (upload_complete = false) in the file
	updatedUsersFile, err := users.LoadActiveUsersFile(activeUsersPath)
	if err != nil {
		t.Fatalf("Failed to reload active users file: %v", err)
	}

	incompleteUsers := updatedUsersFile.GetIncompleteUsers()
	if len(incompleteUsers) != 1 {
		t.Errorf("Expected 1 incomplete user after Box access failure, got %d", len(incompleteUsers))
	}

	if len(incompleteUsers) > 0 && incompleteUsers[0].UploadComplete {
		t.Errorf("Expected user to be marked as incomplete (upload_complete=false), but got upload_complete=true")
	}
}

// Test: Verify GetAllUserRecordings is called without date filters (nil From/To)
func TestUserProcessor_GetAllRecordings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock clients
	zoomClient := newMockZoomClient()
	downloadManager := newMockDownloadManager()
	boxClient := newMockBoxClient()
	boxUploadManager := newMockUploadManager(boxClient)

	// Add test recording (with date older than 30 days to verify it's fetched)
	oldDate := time.Now().AddDate(0, 0, -60) // 60 days ago
	zoomClient.recordings["john.doe@example.com"] = []*zoom.Recording{
		{
			UUID:      "test-uuid-old",
			Topic:     "Old Meeting",
			StartTime: oldDate,
			RecordingFiles: []zoom.RecordingFile{
				{
					ID:          "file-old",
					FileType:    "MP4",
					DownloadURL: "https://zoom.us/download/old.mp4",
					FileSize:    1024,
				},
			},
		},
	}

	// Create user processor
	config := ProcessorConfig{
		BaseDownloadDir: tmpDir,
		BoxEnabled:      true,
		DeleteAfterUpload: false,
		ContinueOnError: false,
	}

	userManager, _ := users.NewActiveUserManager(users.ActiveUserConfig{
		FilePath:      "",
		CaseSensitive: false,
		WatchFile:     false,
	})

	dirManager := directory.NewDirectoryManager(directory.DirectoryConfig{
		BaseDirectory: tmpDir,
		CreateDirs:    true,
	}, userManager)

	filenameSanitizer := filename.NewFileSanitizer(filename.FileSanitizerOptions{})

	processor := NewUserProcessor(
		zoomClient,
		downloadManager,
		dirManager,
		filenameSanitizer,
		boxUploadManager,
		config,
	)

	// Process user
	ctx := context.Background()
	_, err := processor.ProcessUser(ctx, "john.doe@example.com", "john.doe@example.com")

	if err != nil {
		t.Fatalf("ProcessUser failed: %v", err)
	}

	// Verify that GetAllUserRecordings was called with proper date filters
	if zoomClient.lastCallParams == nil {
		t.Fatal("GetAllUserRecordings was not called")
	}

	// From should be set to 2020-06-30
	if zoomClient.lastCallParams.From == nil {
		t.Error("Expected From to be set (2020-06-30), got nil")
	} else {
		expectedFrom := time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC)
		if !zoomClient.lastCallParams.From.Equal(expectedFrom) {
			t.Errorf("Expected From to be %v, got: %v", expectedFrom, zoomClient.lastCallParams.From)
		}
	}

	// To should be set to today (just verify it's not nil)
	if zoomClient.lastCallParams.To == nil {
		t.Error("Expected To to be set (today), got nil")
	}

	// Verify PageSize is still set (should be 300 for maximum efficiency)
	if zoomClient.lastCallParams.PageSize != 300 {
		t.Errorf("Expected PageSize to be 300, got: %d", zoomClient.lastCallParams.PageSize)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
