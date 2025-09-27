package box

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateFolderPathWithPermissions(t *testing.T) {
	tests := []struct {
		name            string
		folderPath      string
		parentID        string
		userPermissions map[string]string
		serverResponses map[string]string
		expectedError   bool
		expectedPath    string
	}{
		{
			name:       "create simple folder with permissions",
			folderPath: "john.doe",
			parentID:   "0",
			userPermissions: map[string]string{
				"john.doe@company.com": RoleViewer,
			},
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{"total_count": 0, "entries": []}`,
				"POST /2.0/folders": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe",
					"created_at": "2024-01-15T10:00:00Z",
					"modified_at": "2024-01-15T10:00:00Z"
				}`,
				"POST /2.0/collaborations": `{
					"id": "collab123",
					"type": "collaboration",
					"role": "viewer",
					"status": "accepted"
				}`,
				"GET /2.0/folders/123456": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe"
				}`,
			},
			expectedError: false,
			expectedPath:  "john.doe",
		},
		{
			name:       "create nested folder structure with permissions",
			folderPath: "john.doe/2024/01/15",
			parentID:   "0",
			userPermissions: map[string]string{
				"john.doe@company.com": RoleViewer,
			},
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{"total_count": 0, "entries": []}`,
				"POST /2.0/folders": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe",
					"created_at": "2024-01-15T10:00:00Z",
					"modified_at": "2024-01-15T10:00:00Z"
				}`,
				"GET /2.0/folders/123456/items": `{"total_count": 0, "entries": []}`,
				"POST /2.0/collaborations": `{
					"id": "collab123",
					"type": "collaboration",
					"role": "viewer",
					"status": "accepted"
				}`,
				"GET /2.0/folders/789012": `{
					"id": "789012",
					"type": "folder",
					"name": "15"
				}`,
			},
			expectedError: false,
		},
		{
			name:       "existing folder structure",
			folderPath: "john.doe",
			parentID:   "0",
			userPermissions: map[string]string{
				"john.doe@company.com": RoleViewer,
			},
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{
					"total_count": 1,
					"entries": [
						{
							"id": "123456",
							"type": "folder",
							"name": "john.doe"
						}
					]
				}`,
				"GET /2.0/folders/123456": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe"
				}`,
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			folder, err := CreateFolderPathWithPermissions(client, tt.folderPath, tt.parentID, tt.userPermissions)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if folder == nil {
				t.Errorf("expected folder but got nil")
				return
			}

			if folder.ID == "" {
				t.Errorf("expected folder ID but got empty string")
			}
		})
	}
}

func TestFindFolderByPath(t *testing.T) {
	tests := []struct {
		name            string
		folderPath      string
		parentID        string
		serverResponses map[string]string
		expectedError   bool
		expectedID      string
	}{
		{
			name:       "find existing folder",
			folderPath: "john.doe",
			parentID:   "0",
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{
					"total_count": 1,
					"entries": [
						{
							"id": "123456",
							"type": "folder",
							"name": "john.doe"
						}
					]
				}`,
				"GET /2.0/folders/123456": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe"
				}`,
			},
			expectedError: false,
			expectedID:    "123456",
		},
		{
			name:       "folder not found",
			folderPath: "nonexistent",
			parentID:   "0",
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{"total_count": 0, "entries": []}`,
			},
			expectedError: true,
		},
		{
			name:       "nested folder path",
			folderPath: "john.doe/2024/01",
			parentID:   "0",
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{
					"total_count": 1,
					"entries": [
						{
							"id": "123456",
							"type": "folder",
							"name": "john.doe"
						}
					]
				}`,
				"GET /2.0/folders/123456/items": `{
					"total_count": 1,
					"entries": [
						{
							"id": "789012",
							"type": "folder",
							"name": "2024"
						}
					]
				}`,
				"GET /2.0/folders/789012/items": `{
					"total_count": 1,
					"entries": [
						{
							"id": "345678",
							"type": "folder",
							"name": "01"
						}
					]
				}`,
				"GET /2.0/folders/345678": `{
					"id": "345678",
					"type": "folder",
					"name": "01"
				}`,
			},
			expectedError: false,
			expectedID:    "345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			folder, err := FindFolderByPath(client, tt.folderPath, tt.parentID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if folder == nil {
				t.Errorf("expected folder but got nil")
				return
			}

			if folder.ID != tt.expectedID {
				t.Errorf("expected folder ID %s but got %s", tt.expectedID, folder.ID)
			}
		})
	}
}

func TestEnsureUserFolderWithPermissions(t *testing.T) {
	tests := []struct {
		name            string
		username        string
		userEmail       string
		baseFolderID    string
		serverResponses map[string]string
		expectedError   bool
	}{
		{
			name:         "create user folder with permissions",
			username:     "john.doe",
			userEmail:    "john.doe@company.com",
			baseFolderID: "0",
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{"total_count": 0, "entries": []}`,
				"POST /2.0/folders": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe",
					"created_at": "2024-01-15T10:00:00Z",
					"modified_at": "2024-01-15T10:00:00Z"
				}`,
				"POST /2.0/collaborations": `{
					"id": "collab123",
					"type": "collaboration",
					"role": "viewer",
					"status": "accepted"
				}`,
				"GET /2.0/folders/123456": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe"
				}`,
			},
			expectedError: false,
		},
		{
			name:          "empty username",
			username:      "",
			userEmail:     "john.doe@company.com",
			baseFolderID:  "0",
			expectedError: true,
		},
		{
			name:          "empty user email",
			username:      "john.doe",
			userEmail:     "",
			baseFolderID:  "0",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			folder, err := EnsureUserFolderWithPermissions(client, tt.username, tt.userEmail, tt.baseFolderID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if folder == nil {
				t.Errorf("expected folder but got nil")
				return
			}

			if folder.ID == "" {
				t.Errorf("expected folder ID but got empty string")
			}
		})
	}
}

func TestSetFolderPermissions(t *testing.T) {
	tests := []struct {
		name            string
		folderID        string
		userPermissions map[string]string
		serverResponses map[string]string
		expectedError   bool
	}{
		{
			name:     "set permissions for single user",
			folderID: "123456",
			userPermissions: map[string]string{
				"john.doe@company.com": RoleViewer,
			},
			serverResponses: map[string]string{
				"POST /2.0/collaborations": `{
					"id": "collab123",
					"type": "collaboration",
					"role": "viewer",
					"status": "accepted"
				}`,
			},
			expectedError: false,
		},
		{
			name:     "set permissions for multiple users",
			folderID: "123456",
			userPermissions: map[string]string{
				"john.doe@company.com":  RoleViewer,
				"jane.smith@company.com": RoleEditor,
			},
			serverResponses: map[string]string{
				"POST /2.0/collaborations": `{
					"id": "collab123",
					"type": "collaboration",
					"role": "viewer",
					"status": "accepted"
				}`,
			},
			expectedError: false,
		},
		{
			name:            "empty permissions map",
			folderID:        "123456",
			userPermissions: nil,
			expectedError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			err := SetFolderPermissions(client, tt.folderID, tt.userPermissions)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetFolderPermissions(t *testing.T) {
	tests := []struct {
		name            string
		folderID        string
		serverResponses map[string]string
		expectedError   bool
		expectedCount   int
	}{
		{
			name:     "get existing permissions",
			folderID: "123456",
			serverResponses: map[string]string{
				"GET /2.0/folders/123456/collaborations": `{
					"total_count": 2,
					"entries": [
						{
							"id": "collab123",
							"type": "collaboration",
							"role": "viewer",
							"accessible_by": {
								"login": "john.doe@company.com"
							}
						},
						{
							"id": "collab456",
							"type": "collaboration",
							"role": "editor",
							"accessible_by": {
								"login": "jane.smith@company.com"
							}
						}
					]
				}`,
			},
			expectedError: false,
			expectedCount: 2,
		},
		{
			name:     "no permissions",
			folderID: "123456",
			serverResponses: map[string]string{
				"GET /2.0/folders/123456/collaborations": `{
					"total_count": 0,
					"entries": []
				}`,
			},
			expectedError: false,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			permissions, err := GetFolderPermissions(client, tt.folderID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if permissions == nil {
				t.Errorf("expected permissions but got nil")
				return
			}

			if len(permissions.Entries) != tt.expectedCount {
				t.Errorf("expected %d permissions but got %d", tt.expectedCount, len(permissions.Entries))
			}
		})
	}
}

func TestRemoveFolderPermissions(t *testing.T) {
	tests := []struct {
		name            string
		folderID        string
		userEmails      []string
		serverResponses map[string]string
		expectedError   bool
	}{
		{
			name:       "remove permissions for single user",
			folderID:   "123456",
			userEmails: []string{"john.doe@company.com"},
			serverResponses: map[string]string{
				"GET /2.0/folders/123456/collaborations": `{
					"total_count": 1,
					"entries": [
						{
							"id": "collab123",
							"type": "collaboration",
							"role": "viewer",
							"accessible_by": {
								"login": "john.doe@company.com"
							}
						}
					]
				}`,
				"DELETE /2.0/collaborations/collab123": ``,
			},
			expectedError: false,
		},
		{
			name:       "remove permissions for multiple users",
			folderID:   "123456",
			userEmails: []string{"john.doe@company.com", "jane.smith@company.com"},
			serverResponses: map[string]string{
				"GET /2.0/folders/123456/collaborations": `{
					"total_count": 2,
					"entries": [
						{
							"id": "collab123",
							"type": "collaboration",
							"role": "viewer",
							"accessible_by": {
								"login": "john.doe@company.com"
							}
						},
						{
							"id": "collab456",
							"type": "collaboration",
							"role": "editor",
							"accessible_by": {
								"login": "jane.smith@company.com"
							}
						}
					]
				}`,
				"DELETE /2.0/collaborations/collab123": ``,
				"DELETE /2.0/collaborations/collab456": ``,
			},
			expectedError: false,
		},
		{
			name:          "empty user emails",
			folderID:      "123456",
			userEmails:    []string{},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			err := RemoveFolderPermissions(client, tt.folderID, tt.userEmails)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateFolderStructure(t *testing.T) {
	tests := []struct {
		name            string
		folderPath      string
		parentID        string
		serverResponses map[string]string
		expectedError   bool
	}{
		{
			name:       "valid folder structure",
			folderPath: "john.doe",
			parentID:   "0",
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{
					"total_count": 1,
					"entries": [
						{
							"id": "123456",
							"type": "folder",
							"name": "john.doe"
						}
					]
				}`,
				"GET /2.0/folders/123456": `{
					"id": "123456",
					"type": "folder",
					"name": "john.doe"
				}`,
			},
			expectedError: false,
		},
		{
			name:       "invalid folder structure",
			folderPath: "nonexistent",
			parentID:   "0",
			serverResponses: map[string]string{
				"GET /2.0/folders/0/items": `{"total_count": 0, "entries": []}`,
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockBoxServer(tt.serverResponses)
			defer server.Close()

			client := createTestBoxClient(tt.serverResponses)

			err := ValidateFolderStructure(client, tt.folderPath, tt.parentID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Helper functions for testing

func createMockBoxServer(responses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"type": "error", "status": 401, "code": "unauthorized", "message": "Unauthorized"}`))
			return
		}
		
		path := r.Method + " " + r.URL.Path
		if response, exists := responses[path]; exists {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "DELETE" {
				w.WriteHeader(http.StatusNoContent)
			} else if strings.Contains(path, "POST") {
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"type": "error", "status": 404, "code": "not_found", "message": "Not Found"}`))
		}
	}))
}

func createTestBoxClient(responses map[string]string) BoxClient {
	// Create a mock client using the pattern from client_test.go
	mockClient := newMockAuthenticatedHTTPClient()
	for path, responseBody := range responses {
		// Parse "METHOD /path" format
		parts := strings.SplitN(path, " ", 2)
		if len(parts) != 2 {
			continue
		}
		method := parts[0]
		url := parts[1]
		
		// Convert relative paths to full URLs
		if !strings.HasPrefix(url, "http") {
			if strings.HasPrefix(url, "/2.0") {
				// URL already has /2.0, so use base domain only
				url = "https://api.box.com" + url
			} else {
				url = BoxAPIBaseURL + url
			}
		}
		
		statusCode := http.StatusOK
		if method == "POST" {
			statusCode = http.StatusCreated
		}
		
		// Debug print to see what URLs we're setting up
		// fmt.Printf("Setting up mock response: %s %s -> %d\n", method, url, statusCode)
		
		mockClient.setResponse(method, url, statusCode, responseBody)
	}
	return &boxClient{httpClient: mockClient}
}

// testAuthenticator is a simple authenticator for testing
type testAuthenticator struct {
	baseURL string
	creds   *OAuth2Credentials
}

func (a *testAuthenticator) GetAccessToken() string {
	return "test_access_token"
}

func (a *testAuthenticator) RefreshToken(ctx context.Context) error {
	return nil
}

func (a *testAuthenticator) IsAuthenticated() bool {
	return true
}

func (a *testAuthenticator) GetCredentials() *OAuth2Credentials {
	if a.creds == nil {
		a.creds = &OAuth2Credentials{
			AccessToken:  "test_access_token",
			RefreshToken: "test_refresh_token",
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(time.Hour),
		}
	}
	return a.creds
}

func (a *testAuthenticator) UpdateCredentials(creds *OAuth2Credentials) error {
	a.creds = creds
	return nil
}