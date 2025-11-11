// Package zoom defines data structures for Zoom Cloud Recording API
package zoom

import (
	"encoding/json"
	"time"
)

// NullableTime is a time.Time that can handle empty strings and null values during JSON unmarshaling
type NullableTime struct {
	Time  time.Time
	Valid bool // Valid is true if Time is not null or empty
}

// UnmarshalJSON implements json.Unmarshaler to handle empty strings and null values
func (nt *NullableTime) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		nt.Valid = false
		return nil
	}

	// Handle empty string
	if string(data) == `""` {
		nt.Valid = false
		return nil
	}

	// Try to unmarshal as time.Time
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}

	nt.Time = t
	nt.Valid = true
	return nil
}

// MarshalJSON implements json.Marshaler
func (nt NullableTime) MarshalJSON() ([]byte, error) {
	if !nt.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(nt.Time)
}

// RecordingFile represents a single recording file within a meeting recording
type RecordingFile struct {
	ID             string       `json:"id"`
	MeetingID      string       `json:"meeting_id"`
	RecordingStart NullableTime `json:"recording_start"`
	RecordingEnd   NullableTime `json:"recording_end"`
	FileType       string       `json:"file_type"`
	FileExtension  string       `json:"file_extension,omitempty"`
	FileSize       int64        `json:"file_size"`
	DownloadURL    string       `json:"download_url"`
	PlayURL        string       `json:"play_url,omitempty"`
	Status         string       `json:"status"`
	FilePath       string       `json:"file_path,omitempty"`
	RecordingType  string       `json:"recording_type,omitempty"`
	DeletedTime    *time.Time   `json:"deleted_time,omitempty"`
}

// ParticipantAudioFile represents an individual participant's audio recording
type ParticipantAudioFile struct {
	ID             string       `json:"id"`
	FileName       string       `json:"file_name,omitempty"`
	FilePath       string       `json:"file_path,omitempty"`
	FileSize       int64        `json:"file_size"`
	FileType       string       `json:"file_type"`
	DownloadURL    string       `json:"download_url"`
	PlayURL        string       `json:"play_url,omitempty"`
	RecordingStart NullableTime `json:"recording_start"`
	RecordingEnd   NullableTime `json:"recording_end"`
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
	DownloadAccessToken      string                 `json:"download_access_token,omitempty"`
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