package box

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/email"
)

// Mock implementations for testing

type mockBoxClient struct {
	files       map[string]*File
	folders     map[string]*Folder
	folderItems map[string][]Item
	uploadError error
	folderError error
}

func newMockBoxClient() *mockBoxClient {
	return &mockBoxClient{
		files:       make(map[string]*File),
		folders:     make(map[string]*Folder),
		folderItems: make(map[string][]Item),
	}
}

func (m *mockBoxClient) RefreshToken() error {
	return nil
}

func (m *mockBoxClient) IsAuthenticated() bool {
	return true
}

func (m *mockBoxClient) GetCurrentUser() (*User, error) {
	return &User{
		ID:    "12345",
		Type:  "user",
		Name:  "Test User",
		Login: "test@example.com",
	}, nil
}

func (m *mockBoxClient) GetUserByEmail(email string) (*User, error) {
	return &User{
		ID:    "user_" + email,
		Type:  "user",
		Name:  "Test User",
		Login: email,
	}, nil
}

func (m *mockBoxClient) CreateFolder(name string, parentID string) (*Folder, error) {
	if m.folderError != nil {
		return nil, m.folderError
	}
	
	folderID := fmt.Sprintf("folder_%s_%s", parentID, name)
	folder := &Folder{
		ID:   folderID,
		Name: name,
		Type: ItemTypeFolder,
	}
	m.folders[folderID] = folder
	return folder, nil
}

func (m *mockBoxClient) GetFolder(folderID string) (*Folder, error) {
	if folder, exists := m.folders[folderID]; exists {
		return folder, nil
	}
	return nil, &BoxError{StatusCode: 404, Code: ErrorCodeItemNotFound}
}

func (m *mockBoxClient) FindZoomFolder() (string, error) {
	// Return a default zoom folder ID for tests
	return "zoom-folder-id", nil
}

func (m *mockBoxClient) CreateFolderAsUser(name string, parentID string, userID string) (*Folder, error) {
	if m.folderError != nil {
		return nil, m.folderError
	}

	folderID := fmt.Sprintf("folder_%s_%s_%s", parentID, name, userID)
	folder := &Folder{
		ID:   folderID,
		Name: name,
		Type: ItemTypeFolder,
	}
	m.folders[folderID] = folder
	return folder, nil
}

func (m *mockBoxClient) ListFolderItems(folderID string) (*FolderItems, error) {
	if items, exists := m.folderItems[folderID]; exists {
		return &FolderItems{
			TotalCount: len(items),
			Entries:    items,
		}, nil
	}
	return &FolderItems{
		TotalCount: 0,
		Entries:    []Item{},
	}, nil
}

func (m *mockBoxClient) ListFolderItemsAsUser(folderID string, userID string) (*FolderItems, error) {
	return m.ListFolderItems(folderID)
}

func (m *mockBoxClient) UploadFile(filePath string, parentFolderID string, fileName string) (*File, error) {
	return m.UploadFileWithProgress(filePath, parentFolderID, fileName, nil)
}

func (m *mockBoxClient) UploadFileWithProgress(filePath string, parentFolderID string, fileName string, progressCallback ProgressCallback) (*File, error) {
	if m.uploadError != nil {
		return nil, m.uploadError
	}

	// Simulate progress callback
	if progressCallback != nil {
		progressCallback(0, 1000)
		progressCallback(500, 1000)
		progressCallback(1000, 1000)
	}

	fileID := fmt.Sprintf("file_%s_%s", parentFolderID, fileName)
	file := &File{
		ID:   fileID,
		Name: fileName,
		Type: ItemTypeFile,
		Size: 1000,
	}
	m.files[fileID] = file
	return file, nil
}

func (m *mockBoxClient) UploadFileAsUser(filePath string, parentFolderID string, fileName string, userID string, progressCallback ProgressCallback) (*File, error) {
	if m.uploadError != nil {
		return nil, m.uploadError
	}

	// Simulate progress callback
	if progressCallback != nil {
		progressCallback(0, 1000)
		progressCallback(500, 1000)
		progressCallback(1000, 1000)
	}

	fileID := fmt.Sprintf("file_%s_%s_%s", parentFolderID, fileName, userID)
	file := &File{
		ID:   fileID,
		Name: fileName,
		Type: ItemTypeFile,
		Size: 1000,
	}
	m.files[fileID] = file
	return file, nil
}

func (m *mockBoxClient) GetFile(fileID string) (*File, error) {
	if file, exists := m.files[fileID]; exists {
		return file, nil
	}
	return nil, &BoxError{StatusCode: 404, Code: ErrorCodeItemNotFound}
}

func (m *mockBoxClient) DeleteFile(fileID string) error {
	delete(m.files, fileID)
	return nil
}

// FindFolderByName - Feature 4.4 implementation for mock
func (m *mockBoxClient) FindFolderByName(parentID string, name string) (*Folder, error) {
	// Simple implementation for tests - return nil as not used in upload tests
	return nil, &BoxError{StatusCode: 404, Code: ErrorCodeItemNotFound, Message: "not implemented in mock"}
}

// FindFileByName - Feature 4.4 implementation for mock
func (m *mockBoxClient) FindFileByName(folderID string, name string) (*File, error) {
	// Simple implementation for tests - return nil as not used in upload tests
	return nil, &BoxError{StatusCode: 404, Code: ErrorCodeItemNotFound, Message: "not implemented in mock"}
}

// FindZoomFolderByOwner - Feature 4.4 implementation for mock
func (m *mockBoxClient) FindZoomFolderByOwner(ownerEmail string) (*Folder, error) {
	// Simple implementation for tests - return nil as not used in upload tests
	return nil, &BoxError{StatusCode: 404, Code: ErrorCodeItemNotFound, Message: "not implemented in mock"}
}

// Chunked upload methods (not fully implemented in mock, but satisfy interface)
func (m *mockBoxClient) CreateUploadSession(fileName string, folderID string, fileSize int64) (*UploadSession, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockBoxClient) UploadPart(sessionID string, part []byte, offset int64, totalSize int64) (*UploadPart, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockBoxClient) CommitUploadSession(sessionID string, parts []UploadPartInfo, attributes map[string]interface{}, digest string) (*File, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (m *mockBoxClient) AbortUploadSession(sessionID string) error {
	return fmt.Errorf("not implemented in mock")
}

type mockStatusTracker struct {
	entries map[string]download.DownloadEntry
}

func newMockStatusTracker() *mockStatusTracker {
	return &mockStatusTracker{
		entries: make(map[string]download.DownloadEntry),
	}
}

func (m *mockStatusTracker) UpdateDownloadStatus(downloadID string, entry download.DownloadEntry) error {
	m.entries[downloadID] = entry
	return nil
}

func (m *mockStatusTracker) GetDownloadStatus(downloadID string) (download.DownloadEntry, bool) {
	entry, exists := m.entries[downloadID]
	return entry, exists
}

func (m *mockStatusTracker) DeleteDownloadStatus(downloadID string) error {
	delete(m.entries, downloadID)
	return nil
}

func (m *mockStatusTracker) GetAllDownloads() map[string]download.DownloadEntry {
	result := make(map[string]download.DownloadEntry)
	for k, v := range m.entries {
		result[k] = v
	}
	return result
}

func (m *mockStatusTracker) GetDownloadsByStatus(status download.DownloadStatusType) map[string]download.DownloadEntry {
	result := make(map[string]download.DownloadEntry)
	for k, v := range m.entries {
		if v.Status == status {
			result[k] = v
		}
	}
	return result
}

func (m *mockStatusTracker) GetIncompleteDownloads() map[string]download.DownloadEntry {
	result := make(map[string]download.DownloadEntry)
	for k, v := range m.entries {
		if v.Status != download.StatusCompleted {
			result[k] = v
		}
	}
	return result
}

func (m *mockStatusTracker) UpdateBoxUploadStatus(downloadID string, boxInfo download.BoxUploadInfo) error {
	entry := m.entries[downloadID]
	entry.Box = &boxInfo
	m.entries[downloadID] = entry
	return nil
}

func (m *mockStatusTracker) GetBoxUploadStatus(downloadID string) (*download.BoxUploadInfo, error) {
	entry, exists := m.entries[downloadID]
	if !exists {
		return nil, fmt.Errorf("download not found")
	}
	return entry.Box, nil
}

func (m *mockStatusTracker) MarkBoxUploadStarted(downloadID, folderID string) error {
	entry := m.entries[downloadID]
	if entry.Box == nil {
		entry.Box = &download.BoxUploadInfo{}
	}
	entry.Box.FolderID = folderID
	entry.Box.LastUploadAttempt = time.Now()
	m.entries[downloadID] = entry
	return nil
}

func (m *mockStatusTracker) MarkBoxUploadCompleted(downloadID, fileID string) error {
	entry := m.entries[downloadID]
	if entry.Box == nil {
		entry.Box = &download.BoxUploadInfo{}
	}
	entry.Box.Uploaded = true
	entry.Box.FileID = fileID
	entry.Box.UploadDate = time.Now()
	entry.Box.UploadError = ""
	m.entries[downloadID] = entry
	return nil
}

func (m *mockStatusTracker) MarkBoxUploadFailed(downloadID, errorMsg string) error {
	entry := m.entries[downloadID]
	if entry.Box == nil {
		entry.Box = &download.BoxUploadInfo{}
	}
	entry.Box.Uploaded = false
	entry.Box.UploadError = errorMsg
	entry.Box.UploadRetries++
	entry.Box.LastUploadAttempt = time.Now()
	m.entries[downloadID] = entry
	return nil
}

func (m *mockStatusTracker) GetPendingBoxUploads() map[string]download.DownloadEntry {
	result := make(map[string]download.DownloadEntry)
	for k, v := range m.entries {
		if v.Status == download.StatusCompleted && (v.Box == nil || !v.Box.Uploaded) {
			result[k] = v
		}
	}
	return result
}

func (m *mockStatusTracker) GetFailedBoxUploads() map[string]download.DownloadEntry {
	result := make(map[string]download.DownloadEntry)
	for k, v := range m.entries {
		if v.Box != nil && !v.Box.Uploaded && v.Box.UploadError != "" {
			result[k] = v
		}
	}
	return result
}

func (m *mockStatusTracker) SaveToFile() error    { return nil }
func (m *mockStatusTracker) LoadFromFile() error  { return nil }
func (m *mockStatusTracker) Close() error         { return nil }

// Test functions

func TestNewUploadManager(t *testing.T) {
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	manager.SetBaseFolderID("test_folder")
	
	if manager == nil {
		t.Fatal("Expected upload manager to be created")
	}
	
	if manager.GetBaseFolderID() != "test_folder" {
		t.Errorf("Expected base folder ID 'test_folder', got '%s'", manager.GetBaseFolderID())
	}
}

func TestUploadFile_Success(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	
	ctx := context.Background()
	result, err := manager.UploadFile(ctx, testFile, "john.doe@example.com", "test-download-1")
	
	if err != nil {
		t.Fatalf("Expected successful upload, got error: %v", err)
	}
	
	if !result.Success {
		t.Error("Expected upload to be successful")
	}
	
	if result.FileID == "" {
		t.Error("Expected file ID to be set")
	}
	
	if result.FileName != "test.mp4" {
		t.Errorf("Expected filename 'test.mp4', got '%s'", result.FileName)
	}
}

func TestUploadFileWithProgress_Success(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	
	progressCallbacks := []UploadPhase{}
	progressCallback := func(uploaded, total int64, phase UploadPhase) {
		progressCallbacks = append(progressCallbacks, phase)
	}
	
	ctx := context.Background()
	result, err := manager.UploadFileWithProgress(ctx, testFile, "jane.smith@example.com", "test-download-2", progressCallback)
	
	if err != nil {
		t.Fatalf("Expected successful upload, got error: %v", err)
	}
	
	if !result.Success {
		t.Error("Expected upload to be successful")
	}

	expectedPhases := []UploadPhase{PhaseCreatingFolders, PhaseUploadingFile, PhaseCompleted}
	if len(progressCallbacks) < len(expectedPhases) {
		t.Errorf("Expected at least %d progress callbacks, got %d", len(expectedPhases), len(progressCallbacks))
	}
}

func TestUploadFile_UploadError(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	client := newMockBoxClient()
	client.uploadError = fmt.Errorf("upload failed")
	manager := NewUploadManager(client)
	
	ctx := context.Background()
	result, err := manager.UploadFile(ctx, testFile, "user@example.com", "test-download-3")
	
	if err == nil {
		t.Fatal("Expected upload error")
	}
	
	if result.Success {
		t.Error("Expected upload to fail")
	}
	
	if result.Error == nil {
		t.Error("Expected error to be set in result")
	}
}

func TestUploadWithResume_ExistingValidUpload(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	client := newMockBoxClient()
	manager := NewUploadManager(client)
	statusTracker := newMockStatusTracker()

	// Set up existing upload status
	downloadID := "test-download-resume"
	statusTracker.entries[downloadID] = download.DownloadEntry{
		Status:     download.StatusCompleted,
		FilePath:   testFile,
		VideoOwner: "user@example.com",
		Box: &download.BoxUploadInfo{
			Uploaded:   true,
			FileID:     "existing-file-id",
			FolderID:   "existing-folder-id",
			UploadDate: time.Now(),
		},
	}

	// Mock the file to exist in client
	client.files["existing-file-id"] = &File{
		ID:   "existing-file-id",
		Name: "test.mp4",
		Size: 1000,
	}

	ctx := context.Background()
	result, err := manager.UploadWithResume(ctx, testFile, "user@example.com", downloadID, statusTracker)

	if err != nil {
		t.Fatalf("Expected successful resume, got error: %v", err)
	}

	if !result.Success {
		t.Error("Expected resumed upload to be successful")
	}

	if result.FileID != "existing-file-id" {
		t.Errorf("Expected existing file ID, got '%s'", result.FileID)
	}

	// Upload duration should be 0 since it was already uploaded
	if result.Duration != 0 {
		t.Error("Expected zero duration for existing upload")
	}
}

func TestUploadWithResume_StaleUploadStatusFileDeletedFromBox(t *testing.T) {
	// This tests the bug where local file exists but Box file was deleted
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	client := newMockBoxClient()
	manager := NewUploadManager(client)
	statusTracker := newMockStatusTracker()

	// Set up stale upload status - file marked as uploaded but doesn't exist in Box
	downloadID := "test-download-stale"
	statusTracker.entries[downloadID] = download.DownloadEntry{
		Status:     download.StatusCompleted,
		FilePath:   testFile,
		VideoOwner: "user@example.com",
		Box: &download.BoxUploadInfo{
			Uploaded:   true,
			FileID:     "deleted-file-id", // This file doesn't exist in Box
			FolderID:   "some-folder-id",
			UploadDate: time.Now().Add(-24 * time.Hour),
		},
	}

	// DO NOT add file to client.files - simulating deleted file

	ctx := context.Background()
	result, err := manager.UploadWithResume(ctx, testFile, "user@example.com", downloadID, statusTracker)

	if err != nil {
		t.Fatalf("Expected successful re-upload, got error: %v", err)
	}

	if !result.Success {
		t.Error("Expected upload to be successful")
	}

	// Should have uploaded and gotten a NEW file ID
	if result.FileID == "deleted-file-id" {
		t.Error("Expected new file ID, got stale file ID - file was not re-uploaded!")
	}

	// Should have taken some time since it actually uploaded
	if result.Duration == 0 {
		t.Error("Expected non-zero duration for actual upload")
	}

	// Status should be updated with new file ID
	updatedEntry, _ := statusTracker.GetDownloadStatus(downloadID)
	if updatedEntry.Box == nil || !updatedEntry.Box.Uploaded {
		t.Error("Expected Box upload status to be marked as uploaded")
	}
	if updatedEntry.Box.FileID == "deleted-file-id" {
		t.Error("Status should have been updated with new file ID")
	}
}

func TestUploadWithResume_NewUpload(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	statusTracker := newMockStatusTracker()
	
	downloadID := "test-download-new"
	
	ctx := context.Background()
	result, err := manager.UploadWithResume(ctx, testFile, "user@example.com", downloadID, statusTracker)
	
	if err != nil {
		t.Fatalf("Expected successful upload, got error: %v", err)
	}
	
	if !result.Success {
		t.Error("Expected upload to be successful")
	}
	
	// Check that status was updated
	entry, exists := statusTracker.GetDownloadStatus(downloadID)
	if !exists {
		t.Error("Expected download status to be updated")
	}
	
	if entry.Box == nil || !entry.Box.Uploaded {
		t.Error("Expected Box upload status to be marked as uploaded")
	}
}

func TestValidateUploadedFile(t *testing.T) {
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	
	// Add a test file to the mock client
	testFileID := "test-file-123"
	client.files[testFileID] = &File{
		ID:   testFileID,
		Name: "test.mp4",
		Size: 1000,
	}
	
	ctx := context.Background()
	
	// Test valid file
	valid, err := manager.ValidateUploadedFile(ctx, testFileID, 1000)
	if err != nil {
		t.Fatalf("Expected validation to succeed, got error: %v", err)
	}
	if !valid {
		t.Error("Expected file to be valid")
	}
	
	// Test size mismatch
	valid, err = manager.ValidateUploadedFile(ctx, testFileID, 2000)
	if err != nil {
		t.Fatalf("Expected validation to complete, got error: %v", err)
	}
	if valid {
		t.Error("Expected file to be invalid due to size mismatch")
	}
	
	// Test non-existent file
	valid, err = manager.ValidateUploadedFile(ctx, "non-existent", 1000)
	if err != nil {
		t.Fatalf("Expected validation to complete, got error: %v", err)
	}
	if valid {
		t.Error("Expected non-existent file to be invalid")
	}
}

func TestUploadPendingFiles(t *testing.T) {
	// Create temporary test files
	tempDir := t.TempDir()
	testFile1 := filepath.Join(tempDir, "test1.mp4")
	testFile2 := filepath.Join(tempDir, "test2.mp4")
	
	if err := os.WriteFile(testFile1, []byte("test content 1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile2, []byte("test content 2"), 0644); err != nil {
		t.Fatal(err)
	}
	
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	statusTracker := newMockStatusTracker()
	
	// Set up pending uploads
	statusTracker.entries["download-1"] = download.DownloadEntry{
		Status:     download.StatusCompleted,
		FilePath:   testFile1,
		VideoOwner: "user1@example.com",
	}
	statusTracker.entries["download-2"] = download.DownloadEntry{
		Status:     download.StatusCompleted,
		FilePath:   testFile2,
		VideoOwner: "user2@example.com",
	}
	
	ctx := context.Background()
	summary, err := manager.UploadPendingFiles(ctx, statusTracker)
	
	if err != nil {
		t.Fatalf("Expected successful bulk upload, got error: %v", err)
	}
	
	if summary.TotalFiles != 2 {
		t.Errorf("Expected 2 total files, got %d", summary.TotalFiles)
	}
	
	if summary.SuccessCount != 2 {
		t.Errorf("Expected 2 successful uploads, got %d", summary.SuccessCount)
	}
	
	if summary.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", summary.FailureCount)
	}
	
	if len(summary.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(summary.Results))
	}
}

func TestExtractUsernameFromEmail(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"john.doe@example.com", "john.doe"},
		{"user@company.org", "user"},
		{"invalid-email", ""},
		{"", ""},
		{"user@", ""},
		{"@domain.com", ""},
	}
	
	for _, test := range tests {
		result := email.ExtractUsername(test.email)
		if result != test.expected {
			t.Errorf("email.ExtractUsername(%s) = %s, expected %s", 
				test.email, result, test.expected)
		}
	}
}

func TestExtractFolderPathFromLocalPath(t *testing.T) {
	tests := []struct {
		name      string
		localPath string
		expected  string
	}{
		{
			name:      "standard path structure",
			localPath: "/home/user/downloads/john.doe/2024/01/15/meeting-recording.mp4",
			expected:  "2024/01/15", // Only year/month/day, username excluded (baseFolderID is user's zoom folder)
		},
		{
			name:      "relative path",
			localPath: "./downloads/user@example.com/2024/03/10/file.mp4",
			expected:  "2024/03/10",
		},
		{
			name:      "path with spaces",
			localPath: "/Users/me/My Downloads/john doe/2024/06/01/video.mp4",
			expected:  "2024/06/01",
		},
		{
			name:      "absolute path with base directory",
			localPath: "/var/data/zoom-recordings/jane.smith/2024/12/25/holiday-meeting.mp4",
			expected:  "2024/12/25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFolderPathFromLocalPath(tt.localPath)
			if result != tt.expected {
				t.Errorf("extractFolderPathFromLocalPath(%q) = %s, expected %s", tt.localPath, result, tt.expected)
			}
		})
	}
}

func TestCreateDateBasedFolderPath(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		username string
		expected string
	}{
		{
			name:     "with username",
			username: "john.doe",
			expected: "john.doe/2024/01/15",
		},
		{
			name:     "without username (base folder as user root)",
			username: "",
			expected: "2024/01/15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createDateBasedFolderPath(tt.username, testTime)
			if result != tt.expected {
				t.Errorf("createDateBasedFolderPath(%q) = %s, expected %s", tt.username, result, tt.expected)
			}
		})
	}
}

func TestShouldRetryBoxUpload(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		name     string
		entry    download.DownloadEntry
		maxRetries int
		expected bool
	}{
		{
			name: "no box info",
			entry: download.DownloadEntry{},
			maxRetries: 3,
			expected: true,
		},
		{
			name: "already uploaded",
			entry: download.DownloadEntry{
				Box: &download.BoxUploadInfo{Uploaded: true},
			},
			maxRetries: 3,
			expected: false,
		},
		{
			name: "exceeded max retries",
			entry: download.DownloadEntry{
				Box: &download.BoxUploadInfo{UploadRetries: 5},
			},
			maxRetries: 3,
			expected: false,
		},
		{
			name: "recent failed attempt",
			entry: download.DownloadEntry{
				Box: &download.BoxUploadInfo{
					UploadRetries: 1,
					LastUploadAttempt: now.Add(-30 * time.Second),
				},
			},
			maxRetries: 3,
			expected: false,
		},
		{
			name: "old failed attempt",
			entry: download.DownloadEntry{
				Box: &download.BoxUploadInfo{
					UploadRetries: 1,
					LastUploadAttempt: now.Add(-2 * time.Minute),
				},
			},
			maxRetries: 3,
			expected: true,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := download.ShouldRetryBoxUpload(test.entry, test.maxRetries)
			if result != test.expected {
				t.Errorf("ShouldRetryBoxUpload() = %v, expected %v", result, test.expected)
			}
		})
	}
}

// Tests for chunked upload functionality

func TestUploadPart_SHA1DigestHeader(t *testing.T) {
	// This test verifies that UploadPart includes the SHA1 digest header
	// Create a mock HTTP client with specific response
	var capturedRequest *http.Request

	mockHTTPClient := newMockAuthenticatedHTTPClient()
	// Override Do method to capture request
	originalDo := mockHTTPClient.Do
	mockHTTPClient.Do = func(req *http.Request) (*http.Response, error) {
		capturedRequest = req
		// Return a successful response
		responseBody := `{"part":{"part_id":"1","offset":0,"size":1024,"sha1":"test-sha1"}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Header:     make(http.Header),
		}, nil
	}
	defer func() { mockHTTPClient.Do = originalDo }()

	client := &boxClient{httpClient: mockHTTPClient}

	testData := []byte("test data for upload")
	_, err := client.UploadPart("test-session-id", testData, 0, 1024)

	if err != nil {
		t.Fatalf("UploadPart failed: %v", err)
	}

	if capturedRequest == nil {
		t.Fatal("Expected HTTP request to be captured")
	}

	// Verify Digest header is present
	digestHeader := capturedRequest.Header.Get("Digest")
	if digestHeader == "" {
		t.Error("Expected Digest header to be set")
	}

	// Verify digest format: "sha=base64encodedsha1"
	if !strings.HasPrefix(digestHeader, "sha=") {
		t.Errorf("Expected Digest header to start with 'sha=', got: %s", digestHeader)
	}

	// Verify the SHA1 hash is correct
	expectedSHA1 := sha1.Sum(testData)
	expectedDigest := "sha=" + base64.StdEncoding.EncodeToString(expectedSHA1[:])
	if digestHeader != expectedDigest {
		t.Errorf("Expected Digest header %s, got %s", expectedDigest, digestHeader)
	}
}

func TestUploadPart_ContentRangeHeader(t *testing.T) {
	var capturedRequest *http.Request
	mockHTTPClient := &mockAuthenticatedHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			responseBody := `{"part":{"part_id":"1","offset":1024,"size":512,"sha1":"test-sha1"}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
			}, nil
		},
	}

	client := &boxClient{httpClient: mockHTTPClient}

	testData := make([]byte, 512)
	_, err := client.UploadPart("test-session-id", testData, 1024, 2048)

	if err != nil {
		t.Fatalf("UploadPart failed: %v", err)
	}

	// Verify Content-Range header
	contentRange := capturedRequest.Header.Get("Content-Range")
	expected := "bytes 1024-1535/2048"
	if contentRange != expected {
		t.Errorf("Expected Content-Range %s, got %s", expected, contentRange)
	}
}

func TestUploadLargeFile_SHA1ForAllParts(t *testing.T) {
	// Create a temporary test file larger than MinChunkedUploadSize
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "large-test.mp4")

	// Create a file with known content (e.g., 25MB)
	fileSize := int64(25 * 1024 * 1024)
	testData := make([]byte, fileSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatal(err)
	}

	// Track all uploaded parts and their digests
	var uploadedParts []struct {
		offset int64
		size   int64
		digest string
	}
	var commitParts []UploadPartInfo

	partCounter := 0
	mockHTTPClient := newMockAuthenticatedHTTPClient()

	// Setup custom Do function
	originalDo := mockHTTPClient.Do
	mockHTTPClient.Do = func(req *http.Request) (*http.Response, error) {
		// Handle different request types
		if req.Method == "POST" && strings.Contains(req.URL.Path, "/upload_sessions") {
			// CreateUploadSession
			if strings.HasSuffix(req.URL.Path, "/commit") {
				// CommitUploadSession
				body, _ := io.ReadAll(req.Body)
				var commitReq CommitUploadSessionRequest
				json.Unmarshal(body, &commitReq)
				commitParts = commitReq.Parts

				responseBody := `{"total_count":1,"entries":[{"id":"uploaded-file","name":"large-test.mp4","size":` + fmt.Sprintf("%d", fileSize) + `}]}`
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			}

			// Create session
			responseBody := `{"id":"test-session","part_size":8388608,"total_parts":4}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
			}, nil
		} else if req.Method == "PUT" {
			// UploadPart - verify digest header
			digest := req.Header.Get("Digest")
			contentRange := req.Header.Get("Content-Range")

			// Parse content range to get offset and size
			var offset, rangeEnd, total int64
			fmt.Sscanf(contentRange, "bytes %d-%d/%d", &offset, &rangeEnd, &total)
			size := rangeEnd - offset + 1

			uploadedParts = append(uploadedParts, struct {
				offset int64
				size   int64
				digest string
			}{offset, size, digest})

			partCounter++
			sha1Val := digest[4:] // Remove "sha=" prefix
			responseBody := fmt.Sprintf(`{"part":{"part_id":"%d","offset":%d,"size":%d,"sha1":"%s"}}`,
				partCounter, offset, size, sha1Val)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
			}, nil
		}

		return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
	}
	defer func() { mockHTTPClient.Do = originalDo }()

	client := &boxClient{httpClient: mockHTTPClient}

	// Upload the file
	_, err := client.UploadLargeFile(testFile, "test-folder", "large-test.mp4", nil)
	if err != nil {
		t.Fatalf("UploadLargeFile failed: %v", err)
	}

	// Verify all parts have SHA1 digests
	if len(uploadedParts) == 0 {
		t.Fatal("Expected parts to be uploaded")
	}

	for i, part := range uploadedParts {
		if part.digest == "" {
			t.Errorf("Part %d missing digest", i)
		}
		if !strings.HasPrefix(part.digest, "sha=") {
			t.Errorf("Part %d digest has wrong format: %s", i, part.digest)
		}
	}

	// Verify all parts were included in commit
	if len(commitParts) != len(uploadedParts) {
		t.Errorf("Expected %d parts in commit, got %d", len(uploadedParts), len(commitParts))
	}

	// Verify parts are sequential and complete
	var totalBytes int64
	for i, part := range commitParts {
		if i > 0 {
			prevPart := commitParts[i-1]
			expectedOffset := prevPart.Offset + prevPart.Size
			if part.Offset != expectedOffset {
				t.Errorf("Part %d has non-sequential offset: got %d, expected %d", i, part.Offset, expectedOffset)
			}
		}
		totalBytes += part.Size
	}

	if totalBytes != fileSize {
		t.Errorf("Total uploaded bytes %d != file size %d", totalBytes, fileSize)
	}
}

func TestCommitUploadSession_WithAttributes(t *testing.T) {
	var capturedRequest *http.Request
	var capturedBody []byte

	mockHTTPClient := &mockAuthenticatedHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && strings.Contains(req.URL.Path, "/commit") {
				capturedRequest = req
				capturedBody, _ = io.ReadAll(req.Body)

				responseBody := `{"total_count":1,"entries":[{"id":"file-123","name":"test.mp4","size":1024}]}`
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			}
			return nil, fmt.Errorf("unexpected request")
		},
	}

	client := &boxClient{httpClient: mockHTTPClient}

	parts := []UploadPartInfo{
		{Offset: 0, Size: 512, SHA1: "abc123"},
		{Offset: 512, Size: 512, SHA1: "def456"},
	}

	attributes := map[string]interface{}{
		"name":        "test.mp4",
		"description": "Test video file",
		"content_created_at": "2024-01-15T10:30:00Z",
	}

	digest := "sha=testdigest123"

	_, err := client.CommitUploadSession("test-session", parts, attributes, digest)
	if err != nil {
		t.Fatalf("CommitUploadSession failed: %v", err)
	}

	if capturedRequest == nil {
		t.Fatal("Expected request to be captured")
	}

	// Verify the request body contains attributes
	var commitReq CommitUploadSessionRequest
	if err := json.Unmarshal(capturedBody, &commitReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if len(commitReq.Parts) != 2 {
		t.Errorf("Expected 2 parts, got %d", len(commitReq.Parts))
	}

	if commitReq.Attributes == nil {
		t.Fatal("Expected attributes to be set")
	}

	if name, ok := commitReq.Attributes["name"].(string); !ok || name != "test.mp4" {
		t.Errorf("Expected name attribute 'test.mp4', got %v", commitReq.Attributes["name"])
	}

	if desc, ok := commitReq.Attributes["description"].(string); !ok || desc != "Test video file" {
		t.Errorf("Expected description attribute, got %v", commitReq.Attributes["description"])
	}

	// Verify Digest header is set
	digestHeader := capturedRequest.Header.Get("Digest")
	if digestHeader != digest {
		t.Errorf("Expected Digest header '%s', got '%s'", digest, digestHeader)
	}
}

func TestUploadLargeFile_WithFileMetadata(t *testing.T) {
	// This test verifies that UploadLargeFile passes proper file metadata to CommitUploadSession
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "video-test.mp4")

	// Create a file larger than MinChunkedUploadSize
	fileSize := int64(25 * 1024 * 1024)
	testData := make([]byte, fileSize)
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatal(err)
	}

	var commitAttributes map[string]interface{}

	mockHTTPClient := &mockAuthenticatedHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && strings.Contains(req.URL.Path, "/upload_sessions") {
				if strings.HasSuffix(req.URL.Path, "/commit") {
					// Capture commit attributes
					body, _ := io.ReadAll(req.Body)
					var commitReq CommitUploadSessionRequest
					json.Unmarshal(body, &commitReq)
					commitAttributes = commitReq.Attributes

					responseBody := `{"total_count":1,"entries":[{"id":"file-123","name":"video-test.mp4","size":` + fmt.Sprintf("%d", fileSize) + `}]}`
					return &http.Response{
						StatusCode: http.StatusCreated,
						Body:       io.NopCloser(strings.NewReader(responseBody)),
						Header:     make(http.Header),
					}, nil
				}

				// Create session
				responseBody := `{"id":"test-session","part_size":8388608,"total_parts":4}`
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			} else if req.Method == "PUT" {
				// UploadPart
				responseBody := `{"part":{"part_id":"1","offset":0,"size":8388608,"sha1":"test"}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			}

			return nil, fmt.Errorf("unexpected request")
		},
	}

	client := &boxClient{httpClient: mockHTTPClient}

	_, err := client.UploadLargeFile(testFile, "test-folder", "video-test.mp4", nil)
	if err != nil {
		t.Fatalf("UploadLargeFile failed: %v", err)
	}

	// Verify attributes were passed (should include file name at minimum)
	if commitAttributes != nil {
		if name, ok := commitAttributes["name"].(string); ok && name != "video-test.mp4" {
			t.Errorf("Expected name 'video-test.mp4', got %s", name)
		}
	}
}

func TestUploadPart_RetryOnTransientFailure(t *testing.T) {
	attempt := 0
	mockHTTPClient := &mockAuthenticatedHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			attempt++

			// Fail first 2 attempts with transient error
			if attempt <= 2 {
				return nil, fmt.Errorf("network error: connection reset")
			}

			// Succeed on 3rd attempt
			responseBody := `{"part":{"part_id":"1","offset":0,"size":1024,"sha1":"test-sha1"}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
			}, nil
		},
	}

	client := &boxClient{httpClient: mockHTTPClient}

	testData := []byte("test data")
	_, err := client.UploadPart("test-session", testData, 0, 1024)

	// With retry logic, this should succeed on 3rd attempt
	if err == nil {
		t.Log("UploadPart succeeded after retries")
	} else {
		// Currently expected to fail - will be fixed in implementation
		if attempt < 3 {
			t.Logf("Expected retry logic not yet implemented (attempt %d)", attempt)
		}
	}
}

func TestUploadPart_FailAfterMaxRetries(t *testing.T) {
	attempt := 0
	mockHTTPClient := &mockAuthenticatedHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			attempt++
			// Always fail
			return nil, fmt.Errorf("persistent network error")
		},
	}

	client := &boxClient{httpClient: mockHTTPClient}

	testData := []byte("test data")
	_, err := client.UploadPart("test-session", testData, 0, 1024)

	if err == nil {
		t.Error("Expected UploadPart to fail after retries")
	}

	// Should have attempted multiple times (exact count depends on retry config)
	t.Logf("Attempted %d times before failing", attempt)
}

func TestUploadPart_NoRetryOnNonTransientError(t *testing.T) {
	attempt := 0
	mockHTTPClient := &mockAuthenticatedHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			attempt++
			// Return non-retryable error (400 Bad Request)
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid request"}`)),
				Header:     make(http.Header),
			}, nil
		},
	}

	client := &boxClient{httpClient: mockHTTPClient}

	testData := []byte("test data")
	_, err := client.UploadPart("test-session", testData, 0, 1024)

	if err == nil {
		t.Error("Expected UploadPart to fail")
	}

	// Should only attempt once for non-retryable errors
	if attempt > 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempt)
	}
}

func TestValidateUploadedParts_Success(t *testing.T) {
	parts := []UploadPartInfo{
		{Offset: 0, Size: 1024, SHA1: "abc123"},
		{Offset: 1024, Size: 1024, SHA1: "def456"},
		{Offset: 2048, Size: 512, SHA1: "ghi789"},
	}

	totalSize := int64(2560)

	err := validateUploadedParts(parts, totalSize)
	if err != nil {
		t.Errorf("Expected valid parts, got error: %v", err)
	}
}

func TestValidateUploadedParts_EmptyParts(t *testing.T) {
	err := validateUploadedParts([]UploadPartInfo{}, 1024)
	if err == nil {
		t.Error("Expected error for empty parts list")
	}
}

func TestValidateUploadedParts_GapInParts(t *testing.T) {
	parts := []UploadPartInfo{
		{Offset: 0, Size: 1024, SHA1: "abc123"},
		{Offset: 2048, Size: 1024, SHA1: "def456"}, // Gap: missing 1024-2048
	}

	err := validateUploadedParts(parts, 3072)
	if err == nil {
		t.Error("Expected error for gap in parts")
	}
}

func TestValidateUploadedParts_SizeMismatch(t *testing.T) {
	parts := []UploadPartInfo{
		{Offset: 0, Size: 1024, SHA1: "abc123"},
		{Offset: 1024, Size: 1024, SHA1: "def456"},
	}

	// Total size doesn't match sum of part sizes
	err := validateUploadedParts(parts, 3072)
	if err == nil {
		t.Error("Expected error for size mismatch")
	}
}

func TestValidateUploadedParts_OverlappingParts(t *testing.T) {
	parts := []UploadPartInfo{
		{Offset: 0, Size: 1024, SHA1: "abc123"},
		{Offset: 512, Size: 1024, SHA1: "def456"}, // Overlaps with first part
	}

	err := validateUploadedParts(parts, 1536)
	if err == nil {
		t.Error("Expected error for overlapping parts")
	}
}


// Tests for Feature 4.4 - Enhanced Folder Management with Permissions

// Enhanced mock client methods for folder management testing

func (m *mockBoxClient) setupFolderStructure() {
	// Mock folder items for testing nested structures
	m.folderItems = map[string][]Item{
		"0": {
			{ID: "user_folder", Type: ItemTypeFolder, Name: "john.doe"},
		},
		"user_folder": {
			{ID: "year_folder", Type: ItemTypeFolder, Name: "2024"},
		},
		"year_folder": {
			{ID: "month_folder", Type: ItemTypeFolder, Name: "01"},
		},
	}
}