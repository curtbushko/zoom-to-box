package tracking

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewGlobalCSVTracker(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "all-uploads.csv")

	tracker, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed: %v", err)
	}

	if tracker == nil {
		t.Fatal("Expected tracker to be non-nil")
	}

	// Verify CSV file was created with header
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	expected := "user,file_name,recording_size,upload_date\n"
	if string(data) != expected {
		t.Errorf("Expected header %q, got %q", expected, string(data))
	}
}

func TestGlobalCSVTracker_TrackUpload(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "all-uploads.csv")

	tracker, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed: %v", err)
	}

	// Track an upload
	uploadTime := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)
	entry := UploadEntry{
		ZoomUser:      "john.doe@company.com",
		FileName:      "team-standup-meeting-1500.mp4",
		RecordingSize: 1048576,
		UploadDate:    uploadTime,
	}

	err = tracker.TrackUpload(entry)
	if err != nil {
		t.Fatalf("TrackUpload failed: %v", err)
	}

	// Verify the entry was written
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	expectedContent := "user,file_name,recording_size,upload_date\njohn.doe@company.com,team-standup-meeting-1500.mp4,1048576,2024-01-15T15:00:00Z\n"
	if string(data) != expectedContent {
		t.Errorf("Expected content:\n%s\nGot:\n%s", expectedContent, string(data))
	}
}

func TestGlobalCSVTracker_MultipleUploads(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "all-uploads.csv")

	tracker, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed: %v", err)
	}

	// Track multiple uploads
	uploads := []UploadEntry{
		{
			ZoomUser:      "john.doe@company.com",
			FileName:      "meeting-1.mp4",
			RecordingSize: 1048576,
			UploadDate:    time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC),
		},
		{
			ZoomUser:      "jane.smith@company.com",
			FileName:      "meeting-2.mp4",
			RecordingSize: 2097152,
			UploadDate:    time.Date(2024, 1, 15, 14, 20, 0, 0, time.UTC),
		},
	}

	for _, upload := range uploads {
		if err := tracker.TrackUpload(upload); err != nil {
			t.Fatalf("TrackUpload failed: %v", err)
		}
	}

	// Verify both entries were written
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	lines := string(data)
	expectedLines := []string{
		"user,file_name,recording_size,upload_date",
		"john.doe@company.com,meeting-1.mp4,1048576,2024-01-15T15:00:00Z",
		"jane.smith@company.com,meeting-2.mp4,2097152,2024-01-15T14:20:00Z",
	}

	for _, expected := range expectedLines {
		if !contains(lines, expected) {
			t.Errorf("Expected to find %q in CSV file", expected)
		}
	}
}

func TestNewUserCSVTracker(t *testing.T) {
	tempDir := t.TempDir()
	userDir := filepath.Join(tempDir, "john.doe")
	err := os.MkdirAll(userDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create user directory: %v", err)
	}

	tracker, err := NewUserCSVTracker(userDir, "john.doe@company.com")
	if err != nil {
		t.Fatalf("NewUserCSVTracker failed: %v", err)
	}

	if tracker == nil {
		t.Fatal("Expected tracker to be non-nil")
	}

	// Verify CSV file was created with header
	csvPath := filepath.Join(userDir, "uploads.csv")
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	expected := "user,file_name,recording_size,upload_date\n"
	if string(data) != expected {
		t.Errorf("Expected header %q, got %q", expected, string(data))
	}
}

func TestUserCSVTracker_TrackUpload(t *testing.T) {
	tempDir := t.TempDir()
	userDir := filepath.Join(tempDir, "john.doe")
	err := os.MkdirAll(userDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create user directory: %v", err)
	}

	tracker, err := NewUserCSVTracker(userDir, "john.doe@company.com")
	if err != nil {
		t.Fatalf("NewUserCSVTracker failed: %v", err)
	}

	// Track an upload
	uploadTime := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)
	entry := UploadEntry{
		ZoomUser:      "john.doe@company.com",
		FileName:      "team-standup-meeting-1500.mp4",
		RecordingSize: 1048576,
		UploadDate:    uploadTime,
	}

	err = tracker.TrackUpload(entry)
	if err != nil {
		t.Fatalf("TrackUpload failed: %v", err)
	}

	// Verify the entry was written
	csvPath := filepath.Join(userDir, "uploads.csv")
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	expectedContent := "user,file_name,recording_size,upload_date\njohn.doe@company.com,team-standup-meeting-1500.mp4,1048576,2024-01-15T15:00:00Z\n"
	if string(data) != expectedContent {
		t.Errorf("Expected content:\n%s\nGot:\n%s", expectedContent, string(data))
	}
}

func TestCSVTracker_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "all-uploads.csv")

	tracker, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed: %v", err)
	}

	// Perform concurrent writes
	done := make(chan bool, 10)
	uploadTime := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			entry := UploadEntry{
				ZoomUser:      "john.doe@company.com",
				FileName:      "meeting-" + string(rune('0'+idx)) + ".mp4",
				RecordingSize: int64(1048576 * (idx + 1)),
				UploadDate:    uploadTime,
			}
			if err := tracker.TrackUpload(entry); err != nil {
				t.Errorf("Concurrent TrackUpload failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify file is not corrupted and has all entries
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	lines := countLines(string(data))
	expectedLines := 11 // 1 header + 10 data lines
	if lines != expectedLines {
		t.Errorf("Expected %d lines, got %d", expectedLines, lines)
	}
}

func TestCSVTracker_ExistingFile(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "all-uploads.csv")

	// Create initial tracker and add entry
	tracker1, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed: %v", err)
	}

	entry1 := UploadEntry{
		ZoomUser:      "john.doe@company.com",
		FileName:      "meeting-1.mp4",
		RecordingSize: 1048576,
		UploadDate:    time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC),
	}

	if err := tracker1.TrackUpload(entry1); err != nil {
		t.Fatalf("TrackUpload failed: %v", err)
	}

	// Create new tracker with same file (should append, not overwrite)
	tracker2, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed on existing file: %v", err)
	}

	entry2 := UploadEntry{
		ZoomUser:      "jane.smith@company.com",
		FileName:      "meeting-2.mp4",
		RecordingSize: 2097152,
		UploadDate:    time.Date(2024, 1, 15, 14, 20, 0, 0, time.UTC),
	}

	if err := tracker2.TrackUpload(entry2); err != nil {
		t.Fatalf("TrackUpload failed: %v", err)
	}

	// Verify both entries exist
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	lines := string(data)
	if !contains(lines, "john.doe@company.com,meeting-1.mp4") {
		t.Error("First entry missing after reopening file")
	}
	if !contains(lines, "jane.smith@company.com,meeting-2.mp4") {
		t.Error("Second entry missing")
	}

	// Should have exactly 3 lines (1 header + 2 data)
	lineCount := countLines(lines)
	if lineCount != 3 {
		t.Errorf("Expected 3 lines, got %d", lineCount)
	}
}

func TestCSVTracker_InvalidPath(t *testing.T) {
	// Test with invalid path (directory doesn't exist)
	_, err := NewGlobalCSVTracker("/nonexistent/directory/file.csv")
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

func TestCSVTracker_EmptyEntry(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := filepath.Join(tempDir, "all-uploads.csv")

	tracker, err := NewGlobalCSVTracker(csvPath)
	if err != nil {
		t.Fatalf("NewGlobalCSVTracker failed: %v", err)
	}

	// Track entry with empty fields (should still work)
	entry := UploadEntry{
		ZoomUser:      "",
		FileName:      "",
		RecordingSize: 0,
		UploadDate:    time.Time{},
	}

	err = tracker.TrackUpload(entry)
	if err != nil {
		t.Fatalf("TrackUpload with empty fields failed: %v", err)
	}

	// Verify the entry was written
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	if !contains(string(data), ",,0,") {
		t.Error("Expected empty fields to be written to CSV")
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
		}
	}
	return count
}
