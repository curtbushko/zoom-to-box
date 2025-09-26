package zoom

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

// TestListUserRecordings tests the ListUserRecordings API method
func TestListUserRecordings(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		params         ListRecordingsParams
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedCount  int
		validateQuery  func(t *testing.T, query url.Values)
	}{
		{
			name:   "successful request with default params",
			userID: "test@example.com",
			params: ListRecordingsParams{},
			serverResponse: `{
				"from": "2024-01-01",
				"to": "2024-01-31",
				"page_count": 1,
				"page_size": 30,
				"total_records": 2,
				"meetings": [
					{
						"uuid": "4444AAAiAAAAAiAiAiiAii==",
						"id": 123456789,
						"account_id": "account123",
						"host_id": "host123",
						"topic": "Test Meeting 1",
						"type": 2,
						"start_time": "2024-01-15T10:00:00Z",
						"duration": 3600,
						"total_size": 1048576,
						"recording_count": 1,
						"recording_files": [
							{
								"id": "rec123",
								"meeting_id": "4444AAAiAAAAAiAiAiiAii==",
								"recording_start": "2024-01-15T10:00:00Z",
								"recording_end": "2024-01-15T11:00:00Z",
								"file_type": "MP4",
								"file_size": 1048576,
								"download_url": "https://zoom.us/download/rec123",
								"status": "completed"
							}
						]
					},
					{
						"uuid": "5555BBBiAAAAAiAiAiiAii==",
						"id": 987654321,
						"account_id": "account123",
						"host_id": "host123",
						"topic": "Test Meeting 2",
						"type": 1,
						"start_time": "2024-01-16T14:00:00Z",
						"duration": 1800,
						"total_size": 524288,
						"recording_count": 1,
						"recording_files": []
					}
				]
			}`,
			serverStatus:  200,
			expectedError: false,
			expectedCount: 2,
			validateQuery: func(t *testing.T, query url.Values) {
				// Should have default page_size
				if query.Get("page_size") != "30" {
					t.Errorf("Expected default page_size=30, got %s", query.Get("page_size"))
				}
			},
		},
		{
			name:   "request with custom parameters",
			userID: "user@company.com",
			params: ListRecordingsParams{
				From:         parseTime(t, "2024-01-01"),
				To:           parseTime(t, "2024-01-31"),
				PageSize:     50,
				NextPageToken: "token123",
			},
			serverResponse: `{
				"from": "2024-01-01",
				"to": "2024-01-31",
				"page_count": 1,
				"page_size": 50,
				"total_records": 0,
				"meetings": []
			}`,
			serverStatus:  200,
			expectedError: false,
			expectedCount: 0,
			validateQuery: func(t *testing.T, query url.Values) {
				if query.Get("from") != "2024-01-01" {
					t.Errorf("Expected from=2024-01-01, got %s", query.Get("from"))
				}
				if query.Get("to") != "2024-01-31" {
					t.Errorf("Expected to=2024-01-31, got %s", query.Get("to"))
				}
				if query.Get("page_size") != "50" {
					t.Errorf("Expected page_size=50, got %s", query.Get("page_size"))
				}
				if query.Get("next_page_token") != "token123" {
					t.Errorf("Expected next_page_token=token123, got %s", query.Get("next_page_token"))
				}
			},
		},
		{
			name:   "user not found error",
			userID: "nonexistent@example.com",
			params: ListRecordingsParams{},
			serverResponse: `{
				"code": 1001,
				"message": "User does not exist: nonexistent@example.com"
			}`,
			serverStatus:  404,
			expectedError: true,
		},
		{
			name:   "invalid date range error",
			userID: "test@example.com",
			params: ListRecordingsParams{
				From: nil, // invalid date will be handled by server
				To:   parseTime(t, "2024-01-31"),
			},
			serverResponse: `{
				"code": 300,
				"message": "Invalid date format"
			}`,
			serverStatus:  400,
			expectedError: true,
		},
		{
			name:   "large dataset with pagination",
			userID: "active@company.com",
			params: ListRecordingsParams{PageSize: 100},
			serverResponse: `{
				"from": "2024-01-01",
				"to": "2024-01-31",
				"page_count": 5,
				"page_size": 100,
				"total_records": 500,
				"next_page_token": "next_page_token_456",
				"meetings": []
			}`,
			serverStatus:  200,
			expectedError: false,
			expectedCount: 0,
			validateQuery: func(t *testing.T, query url.Values) {
				if query.Get("page_size") != "100" {
					t.Errorf("Expected page_size=100, got %s", query.Get("page_size"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle OAuth token request
				if r.URL.Path == "/oauth/token" && r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					w.Write([]byte(`{
						"access_token": "test_token_123",
						"token_type": "Bearer",
						"expires_in": 3600,
						"scope": "recording:read user:read"
					}`))
					return
				}

				// Handle API request
				expectedPath := fmt.Sprintf("/users/%s/recordings", url.PathEscape(tt.userID))
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Verify method
				if r.Method != "GET" {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				// Verify Authorization header
				authHeader := r.Header.Get("Authorization")
				if !strings.HasPrefix(authHeader, "Bearer ") {
					t.Errorf("Expected Bearer token in Authorization header, got %s", authHeader)
				}

				// Validate query parameters if provided
				if tt.validateQuery != nil {
					tt.validateQuery(t, r.URL.Query())
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			// Create client
			client := createTestClient(t, server.URL)

			// Make request
			ctx := context.Background()
			response, err := client.ListUserRecordings(ctx, tt.userID, tt.params)

			// Verify expectations
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(response.Meetings) != tt.expectedCount {
				t.Errorf("Expected %d meetings, got %d", tt.expectedCount, len(response.Meetings))
			}

			// Verify response structure for successful cases
			if !tt.expectedError && response != nil {
				if response.From == "" {
					t.Error("Expected From field to be populated")
				}
				if response.To == "" {
					t.Error("Expected To field to be populated")
				}
				if response.PageSize <= 0 {
					t.Error("Expected PageSize to be positive")
				}
			}
		})
	}
}

// TestGetMeetingRecordings tests the GetMeetingRecordings API method
func TestGetMeetingRecordings(t *testing.T) {
	tests := []struct {
		name           string
		meetingID      string
		serverResponse string
		serverStatus   int
		expectedError  bool
		hasFiles       bool
	}{
		{
			name:      "successful request with recording files",
			meetingID: "4444AAAiAAAAAiAiAiiAii==",
			serverResponse: `{
				"uuid": "4444AAAiAAAAAiAiAiiAii==",
				"id": 123456789,
				"account_id": "account123",
				"host_id": "host123",
				"topic": "Test Meeting Recording",
				"type": 2,
				"start_time": "2024-01-15T10:00:00Z",
				"duration": 3600,
				"total_size": 2097152,
				"recording_count": 2,
				"recording_files": [
					{
						"id": "rec123",
						"meeting_id": "4444AAAiAAAAAiAiAiiAii==",
						"recording_start": "2024-01-15T10:00:00Z",
						"recording_end": "2024-01-15T11:00:00Z",
						"file_type": "MP4",
						"file_size": 1048576,
						"download_url": "https://zoom.us/download/rec123",
						"status": "completed"
					},
					{
						"id": "rec124",
						"meeting_id": "4444AAAiAAAAAiAiAiiAii==",
						"recording_start": "2024-01-15T10:00:00Z",
						"recording_end": "2024-01-15T11:00:00Z",
						"file_type": "CHAT",
						"file_size": 1048576,
						"download_url": "https://zoom.us/download/rec124",
						"status": "completed"
					}
				]
			}`,
			serverStatus:  200,
			expectedError: false,
			hasFiles:      true,
		},
		{
			name:      "meeting with special characters in UUID",
			meetingID: "/ajXp112QmuoKj4854875==",
			serverResponse: `{
				"uuid": "/ajXp112QmuoKj4854875==",
				"id": 987654321,
				"account_id": "account123",
				"host_id": "host123",
				"topic": "Meeting with Special UUID",
				"type": 1,
				"start_time": "2024-01-16T14:00:00Z",
				"duration": 1800,
				"total_size": 524288,
				"recording_count": 0,
				"recording_files": []
			}`,
			serverStatus:  200,
			expectedError: false,
			hasFiles:      false,
		},
		{
			name:      "meeting not found",
			meetingID: "nonexistent_meeting_id",
			serverResponse: `{
				"code": 3001,
				"message": "Meeting does not exist: nonexistent_meeting_id"
			}`,
			serverStatus:  404,
			expectedError: true,
		},
		{
			name:      "meeting without recordings",
			meetingID: "no_recordings_meeting",
			serverResponse: `{
				"code": 1012,
				"message": "No recording found for this meeting."
			}`,
			serverStatus:  404,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle OAuth token request
				if r.URL.Path == "/oauth/token" && r.Method == "POST" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					w.Write([]byte(`{
						"access_token": "test_token_123",
						"token_type": "Bearer",
						"expires_in": 3600,
						"scope": "recording:read user:read"
					}`))
					return
				}

				// Handle API request - Verify URL path (server receives decoded path)
				expectedPath := fmt.Sprintf("/meetings/%s/recordings", tt.meetingID)
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Verify method
				if r.Method != "GET" {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			// Create client
			client := createTestClient(t, server.URL)

			// Make request
			ctx := context.Background()
			recording, err := client.GetMeetingRecordings(ctx, tt.meetingID)

			// Verify expectations
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if recording == nil {
				t.Fatal("Expected recording to be non-nil")
			}

			if recording.UUID != tt.meetingID {
				t.Errorf("Expected UUID %s, got %s", tt.meetingID, recording.UUID)
			}

			if tt.hasFiles {
				if len(recording.RecordingFiles) == 0 {
					t.Error("Expected recording files, but got none")
				}
			}
		})
	}
}

// TestDownloadRecordingFile tests the DownloadRecordingFile method
func TestDownloadRecordingFile(t *testing.T) {
	tests := []struct {
		name           string
		downloadURL    string
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedData   string
	}{
		{
			name:           "successful download",
			downloadURL:    "/download/test_file.mp4",
			serverResponse: "fake video file content",
			serverStatus:   200,
			expectedError:  false,
			expectedData:   "fake video file content",
		},
		{
			name:        "download with redirect",
			downloadURL: "/download/redirect_file.mp4",
			// Will be handled by redirect test server
			serverStatus:  200,
			expectedError: false,
			expectedData:  "redirected file content",
		},
		{
			name:        "file not found",
			downloadURL: "/download/nonexistent.mp4",
			serverResponse: `{
				"code": 5000,
				"message": "File not found"
			}`,
			serverStatus:  404,
			expectedError: true,
		},
		{
			name:        "unauthorized download",
			downloadURL: "/download/unauthorized.mp4",
			serverResponse: `{
				"code": 124,
				"message": "Invalid access token"
			}`,
			serverStatus:  401,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock servers
			var server *httptest.Server
			
			if tt.name == "download with redirect" {
				// Create final destination server
				finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "video/mp4")
					w.WriteHeader(200)
					w.Write([]byte("redirected file content"))
				}))
				defer finalServer.Close()

				// Create redirect server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Handle OAuth token request
					if r.URL.Path == "/oauth/token" && r.Method == "POST" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(200)
						w.Write([]byte(`{
							"access_token": "test_token_123",
							"token_type": "Bearer",
							"expires_in": 3600,
							"scope": "recording:read user:read"
						}`))
						return
					}

					if strings.Contains(r.URL.Path, "redirect_file.mp4") {
						http.Redirect(w, r, finalServer.URL+"/final_file.mp4", http.StatusFound)
						return
					}
					w.WriteHeader(404)
				}))
			} else {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Handle OAuth token request
					if r.URL.Path == "/oauth/token" && r.Method == "POST" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(200)
						w.Write([]byte(`{
							"access_token": "test_token_123",
							"token_type": "Bearer",
							"expires_in": 3600,
							"scope": "recording:read user:read"
						}`))
						return
					}

					// Handle download request
					if tt.serverStatus >= 400 {
						w.Header().Set("Content-Type", "application/json")
					} else {
						w.Header().Set("Content-Type", "video/mp4")
					}
					w.WriteHeader(tt.serverStatus)
					w.Write([]byte(tt.serverResponse))
				}))
			}
			defer server.Close()

			// Create client
			client := createTestClient(t, server.URL)

			// Create buffer for download
			var buf bytes.Buffer

			// Make request
			ctx := context.Background()
			fullURL := server.URL + tt.downloadURL
			err := client.DownloadRecordingFile(ctx, fullURL, &buf)

			// Verify expectations
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if buf.String() != tt.expectedData {
				t.Errorf("Expected data %q, got %q", tt.expectedData, buf.String())
			}
		})
	}
}

// TestPaginationHandling tests pagination with multiple pages
func TestPaginationHandling(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle OAuth token request
		if r.URL.Path == "/oauth/token" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{
				"access_token": "test_token_123",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "recording:read user:read"
			}`))
			return
		}

		pageCount++
		
		nextToken := r.URL.Query().Get("next_page_token")
		
		var response string
		switch pageCount {
		case 1:
			// First page
			response = `{
				"from": "2024-01-01",
				"to": "2024-01-31", 
				"page_count": 3,
				"page_size": 2,
				"total_records": 5,
				"next_page_token": "page_2_token",
				"meetings": [
					{"uuid": "meeting1", "id": 1, "account_id": "acc1", "host_id": "host1", "topic": "Meeting 1", "type": 1, "start_time": "2024-01-01T10:00:00Z", "duration": 60, "total_size": 1024, "recording_count": 0, "recording_files": []},
					{"uuid": "meeting2", "id": 2, "account_id": "acc1", "host_id": "host1", "topic": "Meeting 2", "type": 1, "start_time": "2024-01-02T10:00:00Z", "duration": 60, "total_size": 1024, "recording_count": 0, "recording_files": []}
				]
			}`
		case 2:
			// Second page
			if nextToken != "page_2_token" {
				t.Errorf("Expected next_page_token 'page_2_token', got %s", nextToken)
			}
			response = `{
				"from": "2024-01-01",
				"to": "2024-01-31",
				"page_count": 3,
				"page_size": 2, 
				"total_records": 5,
				"next_page_token": "page_3_token",
				"meetings": [
					{"uuid": "meeting3", "id": 3, "account_id": "acc1", "host_id": "host1", "topic": "Meeting 3", "type": 1, "start_time": "2024-01-03T10:00:00Z", "duration": 60, "total_size": 1024, "recording_count": 0, "recording_files": []},
					{"uuid": "meeting4", "id": 4, "account_id": "acc1", "host_id": "host1", "topic": "Meeting 4", "type": 1, "start_time": "2024-01-04T10:00:00Z", "duration": 60, "total_size": 1024, "recording_count": 0, "recording_files": []}
				]
			}`
		case 3:
			// Third page (final)
			if nextToken != "page_3_token" {
				t.Errorf("Expected next_page_token 'page_3_token', got %s", nextToken)
			}
			response = `{
				"from": "2024-01-01",
				"to": "2024-01-31",
				"page_count": 3,
				"page_size": 2,
				"total_records": 5,
				"meetings": [
					{"uuid": "meeting5", "id": 5, "account_id": "acc1", "host_id": "host1", "topic": "Meeting 5", "type": 1, "start_time": "2024-01-05T10:00:00Z", "duration": 60, "total_size": 1024, "recording_count": 0, "recording_files": []}
				]
			}`
		default:
			t.Errorf("Unexpected request count: %d", pageCount)
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	client := createTestClient(t, server.URL)
	ctx := context.Background()

	// Test pagination workflow
	allMeetings := []Recording{}
	
	// First page
	params := ListRecordingsParams{PageSize: 2}
	response, err := client.ListUserRecordings(ctx, "test@example.com", params)
	if err != nil {
		t.Fatalf("First page request failed: %v", err)
	}
	
	allMeetings = append(allMeetings, response.Meetings...)
	
	// Continue pagination while there are more pages
	for response.NextPageToken != "" {
		params.NextPageToken = response.NextPageToken
		response, err = client.ListUserRecordings(ctx, "test@example.com", params)
		if err != nil {
			t.Fatalf("Pagination request failed: %v", err)
		}
		allMeetings = append(allMeetings, response.Meetings...)
	}

	// Verify we got all meetings
	if len(allMeetings) != 5 {
		t.Errorf("Expected 5 total meetings, got %d", len(allMeetings))
	}

	// Verify server was called 3 times
	if pageCount != 3 {
		t.Errorf("Expected 3 server calls, got %d", pageCount)
	}

	// Verify meeting order
	expectedIDs := []int64{1, 2, 3, 4, 5}
	for i, meeting := range allMeetings {
		if meeting.ID != expectedIDs[i] {
			t.Errorf("Expected meeting ID %d at index %d, got %d", expectedIDs[i], i, meeting.ID)
		}
	}
}

// Helper function to create a test client
func createTestClient(t *testing.T, baseURL string) CloudRecordingClient {
	cfg := config.ZoomConfig{
		AccountID:    "test_account",
		ClientID:     "test_client", 
		ClientSecret: "test_secret",
		BaseURL:      baseURL,
	}

	auth := NewServerToServerAuth(cfg)
	
	// Create HTTP client with retry logic
	downloadConfig := config.DownloadConfig{
		TimeoutSeconds: 10,
		RetryAttempts:  2,
	}
	httpConfig := HTTPClientConfigFromDownloadConfig(downloadConfig)
	retryClient := NewRetryHTTPClient(httpConfig)
	authenticatedClient := NewAuthenticatedRetryClient(retryClient, auth)

	client := NewZoomClient(authenticatedClient, baseURL)
	return client
}

// TestQueryParameterEncoding tests special character handling in URLs
func TestQueryParameterEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle OAuth token request
		if r.URL.Path == "/oauth/token" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{
				"access_token": "test_token_123",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "recording:read user:read"
			}`))
			return
		}

		// Verify query parameters are properly encoded
		query := r.URL.Query()
		
		expectedParams := map[string]string{
			"from": "2024-01-01",
			"to":   "2024-01-31",
		}
		
		for key, expected := range expectedParams {
			if actual := query.Get(key); actual != expected {
				t.Errorf("Expected %s=%s, got %s=%s", key, expected, key, actual)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"meetings": []}`))
	}))
	defer server.Close()

	client := createTestClient(t, server.URL)
	ctx := context.Background()

	// Test with special characters in user ID
	userID := "user+test@company.com"
	params := ListRecordingsParams{
		From: parseTime(t, "2024-01-01"),
		To:   parseTime(t, "2024-01-31"),
	}

	_, err := client.ListUserRecordings(ctx, userID, params)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}

// parseTime helper function to parse date strings into *time.Time
func parseTime(t *testing.T, dateStr string) *time.Time {
	t.Helper()
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t.Fatalf("Failed to parse date %s: %v", dateStr, err)
	}
	return &date
}