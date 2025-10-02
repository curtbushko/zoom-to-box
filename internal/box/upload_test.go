package box

import (
	"context"
	"fmt"
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
	files                  map[string]*File
	folders                map[string]*Folder
	collaborations         map[string]*Collaboration
	folderItems            map[string][]Item
	folderCollaborations   []*Collaboration
	deletedCollaborations  []string
	uploadError            error
	folderError            error
	permissionError        error
	collaborationExists    bool
}

func newMockBoxClient() *mockBoxClient {
	return &mockBoxClient{
		files:                 make(map[string]*File),
		folders:               make(map[string]*Folder),
		collaborations:        make(map[string]*Collaboration),
		folderItems:           make(map[string][]Item),
		folderCollaborations:  make([]*Collaboration, 0),
		deletedCollaborations: make([]string, 0),
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

func (m *mockBoxClient) CreateCollaboration(itemID, itemType, userEmail, role string) (*Collaboration, error) {
	if m.permissionError != nil {
		return nil, m.permissionError
	}
	
	if m.collaborationExists {
		return nil, &BoxError{
			StatusCode: 409,
			Code:       ErrorCodeItemNameTaken,
			Message:    "collaboration already exists",
		}
	}
	
	collabID := fmt.Sprintf("collab_%s_%s", itemID, userEmail)
	collaboration := &Collaboration{
		ID:   collabID,
		Type: "collaboration",
		Role: role,
		AccessibleBy: &User{
			Login: userEmail,
			Type:  "user",
		},
		Status: StatusAccepted,
	}
	
	// Store collaboration with key that includes both item and user
	m.collaborations[itemID+"_"+userEmail] = collaboration
	return collaboration, nil
}

func (m *mockBoxClient) ListCollaborations(itemID, itemType string) (*CollaborationsResponse, error) {
	// Use folderCollaborations if set (for specific test scenarios)
	if len(m.folderCollaborations) > 0 {
		var entries []Collaboration
		for _, collab := range m.folderCollaborations {
			entries = append(entries, *collab)
		}
		return &CollaborationsResponse{
			TotalCount: len(entries),
			Entries:    entries,
		}, nil
	}
	
	// Otherwise, filter collaborations for this item
	var entries []Collaboration
	for key, collab := range m.collaborations {
		if strings.HasPrefix(key, itemID+"_") {
			entries = append(entries, *collab)
		}
	}
	
	return &CollaborationsResponse{
		TotalCount: len(entries),
		Entries:    entries,
	}, nil
}

func (m *mockBoxClient) DeleteCollaboration(collaborationID string) error {
	// Mark collaboration as deleted for testing
	m.deletedCollaborations = append(m.deletedCollaborations, collaborationID)
	return nil
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

func (m *mockStatusTracker) MarkBoxPermissionsSet(downloadID string, permissionIDs []string) error {
	entry := m.entries[downloadID]
	if entry.Box == nil {
		return fmt.Errorf("no box upload info")
	}
	entry.Box.PermissionsSet = true
	entry.Box.PermissionIDs = permissionIDs
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
	
	expectedPhases := []UploadPhase{PhaseCreatingFolders, PhaseUploadingFile, PhaseSettingPermissions, PhaseCompleted}
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
			Uploaded:       true,
			FileID:         "existing-file-id",
			FolderID:       "existing-folder-id",
			UploadDate:     time.Now(),
			PermissionsSet: true,
			PermissionIDs:  []string{"perm-1"},
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

// Tests for Feature 4.4 - Enhanced Folder Management with Permissions

func TestCreateFolderStructureWithPermissions(t *testing.T) {
	client := newMockBoxClient()
	manager := NewUploadManager(client).(*boxUploadManager)
	
	ctx := context.Background()
	folderPath := "john.doe/2024/01/15"
	userEmail := "john.doe@company.com"
	
	folder, err := manager.createFolderStructureWithPermissions(ctx, folderPath, userEmail)
	
	if err != nil {
		t.Fatalf("Expected successful folder creation, got error: %v", err)
	}
	
	if folder == nil {
		t.Fatal("Expected folder but got nil")
	}
	
	// Verify that the user folder has permissions set
	expectedUserFolderID := "folder_0_john.doe"
	if collaboration, exists := client.collaborations[expectedUserFolderID+"_"+userEmail]; exists {
		if collaboration.AccessibleBy == nil || collaboration.AccessibleBy.Login != userEmail {
			t.Errorf("Expected collaboration for user %s", userEmail)
		}
		if collaboration.Role != RoleViewer {
			t.Errorf("Expected viewer role, got %s", collaboration.Role)
		}
	}
}

func TestUploadFileWithEnhancedFolderPermissions(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	client := newMockBoxClient()
	manager := NewUploadManager(client)
	
	ctx := context.Background()
	videoOwner := "john.doe@company.com"
	downloadID := "test-download-permissions"
	
	result, err := manager.UploadFile(ctx, testFile, videoOwner, downloadID)
	
	if err != nil {
		t.Fatalf("Expected successful upload, got error: %v", err)
	}
	
	if !result.Success {
		t.Error("Expected upload to be successful")
	}
	
	// Verify that permissions were set on both the file and user folder
	expectedUserFolderID := "folder_0_john.doe"
	
	// Check folder permissions
	if collaboration, exists := client.collaborations[expectedUserFolderID+"_"+videoOwner]; exists {
		if collaboration.AccessibleBy == nil || collaboration.AccessibleBy.Login != videoOwner {
			t.Errorf("Expected folder collaboration for user %s", videoOwner)
		}
	}
	
	// Check file permissions
	if collaboration, exists := client.collaborations[result.FileID+"_"+videoOwner]; exists {
		if collaboration.AccessibleBy == nil || collaboration.AccessibleBy.Login != videoOwner {
			t.Errorf("Expected file collaboration for user %s", videoOwner)
		}
		if collaboration.Role != RoleViewer {
			t.Errorf("Expected viewer role for file, got %s", collaboration.Role)
		}
	}
	
	if !result.PermissionsSet {
		t.Error("Expected permissions to be set")
	}
	
	if len(result.PermissionIDs) == 0 {
		t.Error("Expected permission IDs to be populated")
	}
}

func TestFolderPermissionManagement(t *testing.T) {
	client := newMockBoxClient()
	
	// Test setting permissions on a folder
	folderID := "test-folder-123"
	client.folders[folderID] = &Folder{
		ID:   folderID,
		Name: "test-folder",
	}
	
	userPermissions := map[string]string{
		"user1@company.com": RoleViewer,
		"user2@company.com": RoleEditor,
	}
	
	err := SetFolderPermissions(client, folderID, userPermissions)
	if err != nil {
		t.Fatalf("Expected successful permission setting, got error: %v", err)
	}
	
	// Verify permissions were created
	for userEmail, expectedRole := range userPermissions {
		collabKey := folderID + "_" + userEmail
		if collab, exists := client.collaborations[collabKey]; exists {
			if collab.Role != expectedRole {
				t.Errorf("Expected role %s for user %s, got %s", expectedRole, userEmail, collab.Role)
			}
		} else {
			t.Errorf("Expected collaboration for user %s", userEmail)
		}
	}
}


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

func (m *mockBoxClient) isCollaborationDeleted(collaborationID string) bool {
	return contains(m.deletedCollaborations, collaborationID)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}