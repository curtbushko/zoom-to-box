// Package download provides tests for email mapping functionality
package download

import (
	"testing"
)

func TestCreateDownloadEntryWithEmailMapping(t *testing.T) {
	req := DownloadRequest{
		Destination: "/test/path/file.mp4",
		FileSize:    1024,
		Metadata:    map[string]interface{}{"test": "data"},
	}
	
	zoomEmail := "zoom@example.com"
	boxEmail := "box@example.com"
	
	entry := CreateDownloadEntryWithEmailMapping(req, StatusPending, zoomEmail, boxEmail)
	
	// Verify basic fields are set
	if entry.Status != StatusPending {
		t.Errorf("Expected status %v, got %v", StatusPending, entry.Status)
	}
	
	if entry.FilePath != req.Destination {
		t.Errorf("Expected file path %q, got %q", req.Destination, entry.FilePath)
	}
	
	if entry.FileSize != req.FileSize {
		t.Errorf("Expected file size %d, got %d", req.FileSize, entry.FileSize)
	}
	
	// Verify email mapping fields
	if entry.VideoOwner != zoomEmail {
		t.Errorf("Expected VideoOwner %q, got %q", zoomEmail, entry.VideoOwner)
	}
	
	if entry.BoxUser != boxEmail {
		t.Errorf("Expected BoxUser %q, got %q", boxEmail, entry.BoxUser)
	}
	
	// Verify timestamps are set
	if entry.LastAttempt.IsZero() {
		t.Error("Expected LastAttempt to be set")
	}
	
	if entry.StartTime.IsZero() {
		t.Error("Expected StartTime to be set")
	}
}

func TestGetBoxEmailForEntry(t *testing.T) {
	tests := []struct {
		name        string
		videoOwner  string
		boxUser     string
		expected    string
	}{
		{
			name:       "both emails set - use BoxUser",
			videoOwner: "zoom@example.com",
			boxUser:    "box@example.com",
			expected:   "box@example.com",
		},
		{
			name:       "only VideoOwner set - fallback to VideoOwner",
			videoOwner: "zoom@example.com",
			boxUser:    "",
			expected:   "zoom@example.com",
		},
		{
			name:       "both empty - return empty",
			videoOwner: "",
			boxUser:    "",
			expected:   "",
		},
		{
			name:       "same email for both",
			videoOwner: "user@example.com",
			boxUser:    "user@example.com",
			expected:   "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := DownloadEntry{
				VideoOwner: tt.videoOwner,
				BoxUser:    tt.boxUser,
			}
			
			result := GetBoxEmailForEntry(entry)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetZoomEmailForEntry(t *testing.T) {
	tests := []struct {
		name       string
		videoOwner string
		expected   string
	}{
		{
			name:       "video owner set",
			videoOwner: "zoom@example.com",
			expected:   "zoom@example.com",
		},
		{
			name:       "video owner empty",
			videoOwner: "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := DownloadEntry{
				VideoOwner: tt.videoOwner,
			}
			
			result := GetZoomEmailForEntry(entry)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}