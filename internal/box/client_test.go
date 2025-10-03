package box

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockAuthenticatedHTTPClient struct {
	responses map[string][]*http.Response
	requests  []*http.Request
	callCounts map[string]int
}

func newMockAuthenticatedHTTPClient() *mockAuthenticatedHTTPClient {
	return &mockAuthenticatedHTTPClient{
		responses: make(map[string][]*http.Response),
		requests:  make([]*http.Request, 0),
		callCounts: make(map[string]int),
	}
}

func (m *mockAuthenticatedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	key := fmt.Sprintf("%s %s", req.Method, req.URL.String())
	
	if responses, exists := m.responses[key]; exists {
		callCount := m.callCounts[key]
		if callCount < len(responses) {
			m.callCounts[key]++
			return responses[callCount], nil
		}
		// Return the last response if we've exhausted the list
		return responses[len(responses)-1], nil
	}
	
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{"message": "not found"}`)),
	}, nil
}

func (m *mockAuthenticatedHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	return m.Do(req)
}

func (m *mockAuthenticatedHTTPClient) GetAsUser(ctx context.Context, url string, userID string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if userID != "" {
		req.Header.Set("As-User", userID)
	}
	return m.Do(req)
}

func (m *mockAuthenticatedHTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
	req.Header.Set("Content-Type", contentType)
	return m.Do(req)
}

func (m *mockAuthenticatedHTTPClient) PostAsUser(ctx context.Context, url string, contentType string, body io.Reader, userID string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
	req.Header.Set("Content-Type", contentType)
	if userID != "" {
		req.Header.Set("As-User", userID)
	}
	return m.Do(req)
}

func (m *mockAuthenticatedHTTPClient) PostJSON(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	jsonData, _ := json.Marshal(payload)
	return m.Post(ctx, url, "application/json", bytes.NewReader(jsonData))
}

func (m *mockAuthenticatedHTTPClient) PostJSONAsUser(ctx context.Context, url string, payload interface{}, userID string) (*http.Response, error) {
	jsonData, _ := json.Marshal(payload)
	return m.PostAsUser(ctx, url, "application/json", bytes.NewReader(jsonData), userID)
}

func (m *mockAuthenticatedHTTPClient) setResponse(method, url string, statusCode int, responseBody string) {
	key := fmt.Sprintf("%s %s", method, url)
	resp := &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}
	m.responses[key] = append(m.responses[key], resp)
}

func TestNewBoxClient(t *testing.T) {
	mockAuth := &mockAuthenticator{}
	client := NewBoxClient(mockAuth, nil)
	
	if client == nil {
		t.Error("Expected non-nil client")
	}
}

func TestBoxClient_CreateFolder(t *testing.T) {
	tests := []struct {
		name           string
		folderName     string
		parentID       string
		statusCode     int
		responseBody   string
		expectedError  string
		expectedFolder *Folder
	}{
		{
			name:       "successful folder creation",
			folderName: "test-folder",
			parentID:   "123",
			statusCode: http.StatusCreated,
			responseBody: `{
				"id": "456",
				"type": "folder",
				"name": "test-folder",
				"description": "",
				"size": 0
			}`,
			expectedFolder: &Folder{
				ID:   "456",
				Type: "folder",
				Name: "test-folder",
			},
		},
		{
			name:          "empty folder name",
			folderName:    "",
			parentID:      "123",
			expectedError: "folder name cannot be empty",
		},
		{
			name:          "folder already exists",
			folderName:    "existing-folder",
			parentID:      "123",
			statusCode:    http.StatusConflict,
			responseBody:  `{"message": "Item with the same name already exists"}`,
			expectedError: "folder 'existing-folder' already exists in parent folder",
		},
		{
			name:          "server error",
			folderName:    "test-folder",
			parentID:      "123",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"message": "Internal server error"}`,
			expectedError: "failed to create folder, status: 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			if tt.statusCode > 0 {
				mockClient.setResponse("POST", BoxAPIBaseURL+"/folders", tt.statusCode, tt.responseBody)
			}
			
			client := &boxClient{httpClient: mockClient}
			
			folder, err := client.CreateFolder(tt.folderName, tt.parentID)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if folder == nil {
				t.Error("Expected non-nil folder")
				return
			}
			
			if folder.ID != tt.expectedFolder.ID {
				t.Errorf("Expected folder ID %q, got %q", tt.expectedFolder.ID, folder.ID)
			}
			
			if folder.Name != tt.expectedFolder.Name {
				t.Errorf("Expected folder name %q, got %q", tt.expectedFolder.Name, folder.Name)
			}
		})
	}
}

func TestBoxClient_GetFolder(t *testing.T) {
	tests := []struct {
		name           string
		folderID       string
		statusCode     int
		responseBody   string
		expectedError  string
		expectedFolder *Folder
	}{
		{
			name:     "successful folder retrieval",
			folderID: "123",
			statusCode: http.StatusOK,
			responseBody: `{
				"id": "123",
				"type": "folder",
				"name": "Documents",
				"description": "My documents folder"
			}`,
			expectedFolder: &Folder{
				ID:          "123",
				Type:        "folder",
				Name:        "Documents",
				Description: "My documents folder",
			},
		},
		{
			name:          "empty folder ID",
			folderID:      "",
			expectedError: "folder ID cannot be empty",
		},
		{
			name:          "folder not found",
			folderID:      "999",
			statusCode:    http.StatusNotFound,
			responseBody:  `{"message": "Not Found"}`,
			expectedError: "folder with ID '999' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			if tt.statusCode > 0 {
				url := fmt.Sprintf("%s/folders/%s", BoxAPIBaseURL, tt.folderID)
				mockClient.setResponse("GET", url, tt.statusCode, tt.responseBody)
			}
			
			client := &boxClient{httpClient: mockClient}
			
			folder, err := client.GetFolder(tt.folderID)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if folder.ID != tt.expectedFolder.ID {
				t.Errorf("Expected folder ID %q, got %q", tt.expectedFolder.ID, folder.ID)
			}
		})
	}
}

func TestBoxClient_ListFolderItems(t *testing.T) {
	tests := []struct {
		name           string
		folderID       string
		statusCode     int
		responseBody   string
		expectedError  string
		expectedCount  int
	}{
		{
			name:     "successful folder listing",
			folderID: "123",
			statusCode: http.StatusOK,
			responseBody: `{
				"total_count": 2,
				"entries": [
					{"id": "1", "type": "file", "name": "document.pdf"},
					{"id": "2", "type": "folder", "name": "subfolder"}
				]
			}`,
			expectedCount: 2,
		},
		{
			name:          "folder not found",
			folderID:      "999",
			statusCode:    http.StatusNotFound,
			responseBody:  `{"message": "Not Found"}`,
			expectedError: "folder with ID '999' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			folderID := tt.folderID
			if folderID == "" {
				folderID = RootFolderID
			}
			
			if tt.statusCode > 0 {
				url := fmt.Sprintf("%s/folders/%s/items", BoxAPIBaseURL, folderID)
				mockClient.setResponse("GET", url, tt.statusCode, tt.responseBody)
			}
			
			client := &boxClient{httpClient: mockClient}
			
			items, err := client.ListFolderItems(tt.folderID)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if len(items.Entries) != tt.expectedCount {
				t.Errorf("Expected %d items, got %d", tt.expectedCount, len(items.Entries))
			}
		})
	}
}

func TestBoxClient_UploadFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, Box!"
	
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name           string
		filePath       string
		parentFolderID string
		fileName       string
		statusCode     int
		responseBody   string
		expectedError  string
	}{
		{
			name:           "successful file upload",
			filePath:       testFile,
			parentFolderID: "123",
			fileName:       "test.txt",
			statusCode:     http.StatusCreated,
			responseBody: `{
				"total_count": 1,
				"entries": [{
					"id": "456",
					"type": "file",
					"name": "test.txt",
					"size": 11
				}]
			}`,
		},
		{
			name:          "empty file path",
			filePath:      "",
			expectedError: "file path cannot be empty",
		},
		{
			name:          "file already exists",
			filePath:      testFile,
			fileName:      "test.txt",
			statusCode:    http.StatusConflict,
			responseBody:  `{"message": "Item with the same name already exists"}`,
			expectedError: "file 'test.txt' already exists in folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			if tt.statusCode > 0 {
				mockClient.setResponse("POST", BoxUploadBaseURL+"/files/content", tt.statusCode, tt.responseBody)
			}
			
			client := &boxClient{httpClient: mockClient}
			
			file, err := client.UploadFile(tt.filePath, tt.parentFolderID, tt.fileName)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if file == nil {
				t.Error("Expected non-nil file")
			}
		})
	}
}

func TestBoxClient_UploadFileWithProgress(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, Box! This is a test file for progress tracking."
	
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	mockClient := newMockAuthenticatedHTTPClient()
	mockClient.setResponse("POST", BoxUploadBaseURL+"/files/content", http.StatusCreated, `{
		"total_count": 1,
		"entries": [{
			"id": "456",
			"type": "file",
			"name": "test.txt",
			"size": 53
		}]
	}`)
	
	client := &boxClient{httpClient: mockClient}
	
	var progressUpdates []struct {
		uploaded int64
		total    int64
	}
	
	progressCallback := func(bytesUploaded int64, totalBytes int64) {
		progressUpdates = append(progressUpdates, struct {
			uploaded int64
			total    int64
		}{bytesUploaded, totalBytes})
	}
	
	file, err := client.UploadFileWithProgress(testFile, "123", "test.txt", progressCallback)
	
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}
	
	if file == nil {
		t.Error("Expected non-nil file")
		return
	}
	
	if len(progressUpdates) == 0 {
		t.Error("Expected progress updates, got none")
	}
	
	firstUpdate := progressUpdates[0]
	if firstUpdate.uploaded != 0 {
		t.Errorf("Expected first progress update to have 0 uploaded bytes, got %d", firstUpdate.uploaded)
	}
	
	if firstUpdate.total != int64(len(testContent)) {
		t.Errorf("Expected total bytes to be %d, got %d", len(testContent), firstUpdate.total)
	}
}

func TestBoxClient_GetFile(t *testing.T) {
	tests := []struct {
		name          string
		fileID        string
		statusCode    int
		responseBody  string
		expectedError string
	}{
		{
			name:   "successful file retrieval",
			fileID: "123",
			statusCode: http.StatusOK,
			responseBody: `{
				"id": "123",
				"type": "file",
				"name": "document.pdf",
				"size": 1024
			}`,
		},
		{
			name:          "empty file ID",
			fileID:        "",
			expectedError: "file ID cannot be empty",
		},
		{
			name:          "file not found",
			fileID:        "999",
			statusCode:    http.StatusNotFound,
			responseBody:  `{"message": "Not Found"}`,
			expectedError: "file with ID '999' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			if tt.statusCode > 0 {
				url := fmt.Sprintf("%s/files/%s", BoxAPIBaseURL, tt.fileID)
				mockClient.setResponse("GET", url, tt.statusCode, tt.responseBody)
			}
			
			client := &boxClient{httpClient: mockClient}
			
			file, err := client.GetFile(tt.fileID)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if file == nil {
				t.Error("Expected non-nil file")
			}
		})
	}
}

func TestBoxClient_DeleteFile(t *testing.T) {
	tests := []struct {
		name          string
		fileID        string
		statusCode    int
		responseBody  string
		expectedError string
	}{
		{
			name:         "successful file deletion",
			fileID:       "123",
			statusCode:   http.StatusNoContent,
			responseBody: "",
		},
		{
			name:          "empty file ID",
			fileID:        "",
			expectedError: "file ID cannot be empty",
		},
		{
			name:          "file not found",
			fileID:        "999",
			statusCode:    http.StatusNotFound,
			responseBody:  `{"message": "Not Found"}`,
			expectedError: "file with ID '999' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			if tt.statusCode > 0 {
				url := fmt.Sprintf("%s/files/%s", BoxAPIBaseURL, tt.fileID)
				mockClient.setResponse("DELETE", url, tt.statusCode, tt.responseBody)
			}
			
			client := &boxClient{httpClient: mockClient}
			
			err := client.DeleteFile(tt.fileID)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestCreateFolderPath(t *testing.T) {
	tests := []struct {
		name        string
		folderPath  string
		parentID    string
		expectError bool
	}{
		{
			name:       "empty path returns root",
			folderPath: "",
			parentID:   "",
		},
		{
			name:       "root path returns root",
			folderPath: "/",
			parentID:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAuthenticatedHTTPClient()
			
			// Mock response for getting root folder
			mockClient.setResponse("GET", BoxAPIBaseURL+"/folders/0", http.StatusOK, `{
				"id": "0",
				"type": "folder",
				"name": "All Files",
				"description": ""
			}`)
			
			client := &boxClient{httpClient: mockClient}
			
			folder, err := CreateFolderPath(client, tt.folderPath, tt.parentID)
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if folder == nil {
				t.Error("Expected non-nil folder")
			}
		})
	}
}

func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name          string
		fileName      string
		expectedError string
	}{
		{
			name:     "valid file name",
			fileName: "document.pdf",
		},
		{
			name:          "empty file name",
			fileName:      "",
			expectedError: "file name cannot be empty",
		},
		{
			name:          "file name with slash",
			fileName:      "folder/file.txt",
			expectedError: "file name contains invalid character: /",
		},
		{
			name:          "file name with colon",
			fileName:      "file:name.txt",
			expectedError: "file name contains invalid character: :",
		},
		{
			name:          "file name too long",
			fileName:      strings.Repeat("a", 256),
			expectedError: "file name too long (max 255 characters)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileName(tt.fileName)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

type mockAuthenticator struct {
	credentials *OAuth2Credentials
}

func (m *mockAuthenticator) RefreshToken(ctx context.Context) error {
	return nil
}

func (m *mockAuthenticator) GetAccessToken() string {
	if m.credentials == nil {
		return "mock-token"
	}
	return m.credentials.AccessToken
}

func (m *mockAuthenticator) IsAuthenticated() bool {
	return true
}

func (m *mockAuthenticator) GetCredentials() *OAuth2Credentials {
	if m.credentials == nil {
		return &OAuth2Credentials{
			AccessToken:  "mock-token",
			RefreshToken: "mock-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
		}
	}
	return m.credentials
}

func (m *mockAuthenticator) UpdateCredentials(creds *OAuth2Credentials) error {
	m.credentials = creds
	return nil
}