package tracking

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UploadEntry represents a single upload record
type UploadEntry struct {
	ZoomUser       string
	FileName       string
	RecordingSize  int64
	UploadDate     time.Time
	ProcessingTime time.Duration
}

// CSVTracker defines the interface for tracking uploads to CSV files
type CSVTracker interface {
	// TrackUpload records an upload entry to the CSV file
	TrackUpload(entry UploadEntry) error
}

// GlobalCSVTracker manages the global all-uploads.csv file
type GlobalCSVTracker struct {
	filePath string
	mu       sync.Mutex
}

// UserCSVTracker manages per-user uploads.csv files
type UserCSVTracker struct {
	filePath string
	zoomUser string
	mu       sync.Mutex
}

// NewGlobalCSVTracker creates a new global CSV tracker
// Creates the CSV file with headers if it doesn't exist
func NewGlobalCSVTracker(filePath string) (*GlobalCSVTracker, error) {
	tracker := &GlobalCSVTracker{
		filePath: filePath,
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		// Create directory if needed
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		// Create file with header
		if err := tracker.writeHeader(); err != nil {
			return nil, fmt.Errorf("failed to write header: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to check file: %w", err)
	}

	return tracker, nil
}

// NewUserCSVTracker creates a new user-specific CSV tracker
// Creates the CSV file with headers if it doesn't exist
func NewUserCSVTracker(userDir string, zoomUser string) (*UserCSVTracker, error) {
	filePath := filepath.Join(userDir, "uploads.csv")

	tracker := &UserCSVTracker{
		filePath: filePath,
		zoomUser: zoomUser,
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		// Ensure user directory exists
		if err := os.MkdirAll(userDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create user directory: %w", err)
		}

		// Create file with header
		if err := tracker.writeHeader(); err != nil {
			return nil, fmt.Errorf("failed to write header: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to check file: %w", err)
	}

	return tracker, nil
}

// TrackUpload records an upload entry to the global CSV file
func (t *GlobalCSVTracker) TrackUpload(entry UploadEntry) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.appendEntry(entry)
}

// TrackUpload records an upload entry to the user CSV file
func (t *UserCSVTracker) TrackUpload(entry UploadEntry) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.appendEntry(entry)
}

// writeHeader writes the CSV header to the global tracker file
func (t *GlobalCSVTracker) writeHeader() error {
	file, err := os.Create(t.filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"user", "file_name", "recording_size", "upload_date", "processing_time_seconds"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	return writer.Error()
}

// writeHeader writes the CSV header to the user tracker file
func (t *UserCSVTracker) writeHeader() error {
	file, err := os.Create(t.filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"user", "file_name", "recording_size", "upload_date", "processing_time_seconds"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	return writer.Error()
}

// appendEntry appends an upload entry to the global tracker CSV file
func (t *GlobalCSVTracker) appendEntry(entry UploadEntry) error {
	file, err := os.OpenFile(t.filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for append: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	record := []string{
		entry.ZoomUser,
		entry.FileName,
		fmt.Sprintf("%d", entry.RecordingSize),
		entry.UploadDate.Format(time.RFC3339),
		fmt.Sprintf("%d", int64(entry.ProcessingTime.Seconds())),
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	return writer.Error()
}

// appendEntry appends an upload entry to the user tracker CSV file
func (t *UserCSVTracker) appendEntry(entry UploadEntry) error {
	file, err := os.OpenFile(t.filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for append: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	record := []string{
		entry.ZoomUser,
		entry.FileName,
		fmt.Sprintf("%d", entry.RecordingSize),
		entry.UploadDate.Format(time.RFC3339),
		fmt.Sprintf("%d", int64(entry.ProcessingTime.Seconds())),
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	return writer.Error()
}
