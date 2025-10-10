# CSV Upload Tracking

This package provides CSV-based tracking for Zoom recording uploads.

## Features

- **Global Tracking**: Track all uploads across all users in `all-uploads.csv`
- **Per-User Tracking**: Track individual user uploads in `<user-dir>/uploads.csv`
- **Thread-Safe**: Concurrent writes are protected with mutexes
- **Resume Support**: Existing CSV files are appended to, not overwritten

## Usage

### Global CSV Tracker

```go
import "github.com/curtbushko/zoom-to-box/internal/tracking"

// Create global tracker (creates all-uploads.csv in base directory)
tracker, err := tracking.NewGlobalCSVTracker("/path/to/downloads/all-uploads.csv")
if err != nil {
    log.Fatal(err)
}

// Track an upload
entry := tracking.UploadEntry{
    ZoomUser:      "john.doe@company.com",
    FileName:      "team-standup-meeting-1500.mp4",
    RecordingSize: 1048576, // bytes
    UploadDate:    time.Now(),
}

err = tracker.TrackUpload(entry)
if err != nil {
    log.Printf("Failed to track upload: %v", err)
}
```

### Per-User CSV Tracker

```go
import "github.com/curtbushko/zoom-to-box/internal/tracking"

// Create user tracker (creates uploads.csv in user directory)
tracker, err := tracking.NewUserCSVTracker("/path/to/downloads/john.doe", "john.doe@company.com")
if err != nil {
    log.Fatal(err)
}

// Track an upload
entry := tracking.UploadEntry{
    ZoomUser:      "john.doe@company.com",
    FileName:      "team-standup-meeting-1500.mp4",
    RecordingSize: 1048576,
    UploadDate:    time.Now(),
}

err = tracker.TrackUpload(entry)
if err != nil {
    log.Printf("Failed to track upload: %v", err)
}
```

## CSV Format

Both global and per-user CSV files use the same format:

```csv
user,file_name,recording_size,upload_date
john.doe@company.com,team-standup-meeting-1500.mp4,1048576,2024-01-15T15:00:00Z
jane.smith@company.com,weekly-review-call-1420.mp4,2097152,2024-01-15T14:20:00Z
```

### Fields

- `user`: Email address of the Zoom user who owns the recording
- `file_name`: Name of the uploaded file (with extension)
- `recording_size`: Size of the recording in bytes
- `upload_date`: ISO 8601 timestamp (RFC3339 format) when the upload completed

## Integration Example

```go
// After successful Box upload in upload manager:
func (m *UploadManager) uploadFile(localPath, boxFolderID string, entry UploadEntry) error {
    // ... perform Box upload ...

    // Track in global CSV
    if m.globalTracker != nil {
        trackingEntry := tracking.UploadEntry{
            ZoomUser:      entry.VideoOwner,
            FileName:      filepath.Base(localPath),
            RecordingSize: entry.FileSize,
            UploadDate:    time.Now(),
        }
        m.globalTracker.TrackUpload(trackingEntry)
    }

    // Track in user CSV
    if m.userTracker != nil {
        m.userTracker.TrackUpload(trackingEntry)
    }

    return nil
}
```

## Thread Safety

All tracker methods are thread-safe and can be called concurrently:

```go
var wg sync.WaitGroup

for _, upload := range uploads {
    wg.Add(1)
    go func(u UploadEntry) {
        defer wg.Done()
        tracker.TrackUpload(u) // Safe to call concurrently
    }(upload)
}

wg.Wait()
```

## Error Handling

Errors are returned for:
- Invalid file paths
- Directory creation failures
- File write failures
- Permission issues

Always check returned errors:

```go
if err := tracker.TrackUpload(entry); err != nil {
    log.Printf("Warning: Failed to track upload for %s: %v", entry.FileName, err)
    // Continue processing - tracking is non-critical
}
```
