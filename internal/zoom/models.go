// Package zoom defines data structures for Zoom Cloud Recording API
package zoom

import (
	"time"
)

// RecordingFile represents a single recording file within a meeting recording
type RecordingFile struct {
	ID             string     `json:"id"`
	MeetingID      string     `json:"meeting_id"`
	RecordingStart time.Time  `json:"recording_start"`
	RecordingEnd   time.Time  `json:"recording_end"`
	FileType       string     `json:"file_type"`
	FileExtension  string     `json:"file_extension,omitempty"`
	FileSize       int64      `json:"file_size"`
	DownloadURL    string     `json:"download_url"`
	PlayURL        string     `json:"play_url,omitempty"`
	Status         string     `json:"status"`
	FilePath       string     `json:"file_path,omitempty"`
	RecordingType  string     `json:"recording_type,omitempty"`
	DeletedTime    *time.Time `json:"deleted_time,omitempty"`
}

// ParticipantAudioFile represents an individual participant's audio recording
type ParticipantAudioFile struct {
	ID             string    `json:"id"`
	FileName       string    `json:"file_name,omitempty"`
	FilePath       string    `json:"file_path,omitempty"`
	FileSize       int64     `json:"file_size"`
	FileType       string    `json:"file_type"`
	DownloadURL    string    `json:"download_url"`
	PlayURL        string    `json:"play_url,omitempty"`
	RecordingStart time.Time `json:"recording_start"`
	RecordingEnd   time.Time `json:"recording_end"`
}

// Recording represents a meeting or webinar recording with all associated files
type Recording struct {
	UUID                     string                 `json:"uuid"`
	ID                       int64                  `json:"id"`
	AccountID                string                 `json:"account_id"`
	HostID                   string                 `json:"host_id"`
	Topic                    string                 `json:"topic"`
	Type                     int                    `json:"type"`
	StartTime                time.Time              `json:"start_time"`
	Duration                 int                    `json:"duration"`
	TotalSize                int64                  `json:"total_size"`
	RecordingCount           int                    `json:"recording_count"`
	RecordingPlayPasscode    string                 `json:"recording_play_passcode,omitempty"`
	AutoDelete               bool                   `json:"auto_delete,omitempty"`
	AutoDeleteDate           string                 `json:"auto_delete_date,omitempty"`
	RecordingFiles           []RecordingFile        `json:"recording_files"`
	ParticipantAudioFiles    []ParticipantAudioFile `json:"participant_audio_files,omitempty"`
}

// ListRecordingsResponse represents the response from the list recordings API endpoint
type ListRecordingsResponse struct {
	From          string      `json:"from"`
	To            string      `json:"to"`
	PageCount     int         `json:"page_count"`
	PageSize      int         `json:"page_size"`
	TotalRecords  int         `json:"total_records"`
	NextPageToken string      `json:"next_page_token,omitempty"`
	Meetings      []Recording `json:"meetings"`
}