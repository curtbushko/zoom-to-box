package zoom

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRecordingFileMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected RecordingFile
	}{
		{
			name: "complete recording file",
			jsonData: `{
				"deleted_time": "2021-03-18T05:41:36Z",
				"download_url": "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"file_path": "/9090876528/path01/demo.mp4",
				"file_size": 7220,
				"file_type": "MP4",
				"file_extension": "MP4",
				"id": "72576a1f-4e66-4a77-87c4-f13f9808bd76",
				"meeting_id": "L0AGOEPVR9m5WSOOs/d+FQ==",
				"play_url": "https://example.com/rec/play/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"recording_end": "2021-03-18T05:41:36Z",
				"recording_start": "2021-03-18T05:41:36Z",
				"recording_type": "shared_screen_with_speaker_view",
				"status": "completed"
			}`,
			expected: RecordingFile{
				ID:             "72576a1f-4e66-4a77-87c4-f13f9808bd76",
				MeetingID:      "L0AGOEPVR9m5WSOOs/d+FQ==",
				RecordingStart: time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
				RecordingEnd:   time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
				FileType:       "MP4",
				FileExtension:  "MP4",
				FileSize:       7220,
				DownloadURL:    "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				PlayURL:        "https://example.com/rec/play/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				Status:         "completed",
				FilePath:       "/9090876528/path01/demo.mp4",
				RecordingType:  "shared_screen_with_speaker_view",
				DeletedTime:    &[]time.Time{time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC)}[0],
			},
		},
		{
			name: "minimal recording file",
			jsonData: `{
				"id": "72576a1f-4e66-4a77-87c4-f13f9808bd76",
				"meeting_id": "L0AGOEPVR9m5WSOOs/d+FQ==",
				"recording_start": "2021-03-18T05:41:36Z",
				"recording_end": "2021-03-18T05:41:36Z",
				"file_type": "MP4",
				"file_size": 7220,
				"download_url": "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"status": "completed"
			}`,
			expected: RecordingFile{
				ID:             "72576a1f-4e66-4a77-87c4-f13f9808bd76",
				MeetingID:      "L0AGOEPVR9m5WSOOs/d+FQ==",
				RecordingStart: time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
				RecordingEnd:   time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
				FileType:       "MP4",
				FileSize:       7220,
				DownloadURL:    "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				Status:         "completed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Unmarshal
			var recordingFile RecordingFile
			err := json.Unmarshal([]byte(tt.jsonData), &recordingFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Compare fields
			if recordingFile.ID != tt.expected.ID {
				t.Errorf("Expected ID %s, got %s", tt.expected.ID, recordingFile.ID)
			}
			if recordingFile.MeetingID != tt.expected.MeetingID {
				t.Errorf("Expected MeetingID %s, got %s", tt.expected.MeetingID, recordingFile.MeetingID)
			}
			if !recordingFile.RecordingStart.Equal(tt.expected.RecordingStart) {
				t.Errorf("Expected RecordingStart %v, got %v", tt.expected.RecordingStart, recordingFile.RecordingStart)
			}
			if !recordingFile.RecordingEnd.Equal(tt.expected.RecordingEnd) {
				t.Errorf("Expected RecordingEnd %v, got %v", tt.expected.RecordingEnd, recordingFile.RecordingEnd)
			}
			if recordingFile.FileType != tt.expected.FileType {
				t.Errorf("Expected FileType %s, got %s", tt.expected.FileType, recordingFile.FileType)
			}
			if recordingFile.FileSize != tt.expected.FileSize {
				t.Errorf("Expected FileSize %d, got %d", tt.expected.FileSize, recordingFile.FileSize)
			}

			// Test Marshal roundtrip
			jsonBytes, err := json.Marshal(recordingFile)
			if err != nil {
				t.Fatalf("Failed to marshal to JSON: %v", err)
			}

			var roundtripRecordingFile RecordingFile
			err = json.Unmarshal(jsonBytes, &roundtripRecordingFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal roundtrip JSON: %v", err)
			}

			if roundtripRecordingFile.ID != recordingFile.ID {
				t.Errorf("Roundtrip failed for ID: expected %s, got %s", recordingFile.ID, roundtripRecordingFile.ID)
			}
		})
	}
}

func TestRecordingMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected Recording
	}{
		{
			name: "complete recording",
			jsonData: `{
				"account_id": "Cx3wERazSgup7ZWRHQM8-w",
				"duration": 20,
				"host_id": "_0ctZtY0REqWalTmwvrdIw", 
				"id": 6840331990,
				"recording_count": 22,
				"start_time": "2021-03-18T05:41:36Z",
				"topic": "My Personal Meeting",
				"total_size": 22,
				"type": 1,
				"uuid": "BOKXuumlTAGXuqwr3bLyuQ==",
				"recording_play_passcode": "yNYIS408EJygs7rE5vVsJwXIz4-VW7MH",
				"auto_delete": true,
				"auto_delete_date": "2028-07-12",
				"recording_files": [
					{
						"id": "72576a1f-4e66-4a77-87c4-f13f9808bd76",
						"meeting_id": "L0AGOEPVR9m5WSOOs/d+FQ==",
						"recording_start": "2021-03-18T05:41:36Z",
						"recording_end": "2021-03-18T05:41:36Z",
						"file_type": "MP4",
						"file_size": 7220,
						"download_url": "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
						"status": "completed"
					}
				]
			}`,
			expected: Recording{
				UUID:                   "BOKXuumlTAGXuqwr3bLyuQ==",
				ID:                     6840331990,
				AccountID:              "Cx3wERazSgup7ZWRHQM8-w",
				HostID:                 "_0ctZtY0REqWalTmwvrdIw",
				Topic:                  "My Personal Meeting",
				Type:                   1,
				StartTime:              time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
				Duration:               20,
				TotalSize:              22,
				RecordingCount:         22,
				RecordingPlayPasscode:  "yNYIS408EJygs7rE5vVsJwXIz4-VW7MH",
				AutoDelete:             true,
				AutoDeleteDate:         "2028-07-12",
				RecordingFiles: []RecordingFile{
					{
						ID:             "72576a1f-4e66-4a77-87c4-f13f9808bd76",
						MeetingID:      "L0AGOEPVR9m5WSOOs/d+FQ==",
						RecordingStart: time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
						RecordingEnd:   time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
						FileType:       "MP4",
						FileSize:       7220,
						DownloadURL:    "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
						Status:         "completed",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Unmarshal
			var recording Recording
			err := json.Unmarshal([]byte(tt.jsonData), &recording)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Compare main fields
			if recording.UUID != tt.expected.UUID {
				t.Errorf("Expected UUID %s, got %s", tt.expected.UUID, recording.UUID)
			}
			if recording.ID != tt.expected.ID {
				t.Errorf("Expected ID %d, got %d", tt.expected.ID, recording.ID)
			}
			if recording.Topic != tt.expected.Topic {
				t.Errorf("Expected Topic %s, got %s", tt.expected.Topic, recording.Topic)
			}
			if !recording.StartTime.Equal(tt.expected.StartTime) {
				t.Errorf("Expected StartTime %v, got %v", tt.expected.StartTime, recording.StartTime)
			}
			if len(recording.RecordingFiles) != len(tt.expected.RecordingFiles) {
				t.Errorf("Expected %d recording files, got %d", len(tt.expected.RecordingFiles), len(recording.RecordingFiles))
			}

			// Test Marshal roundtrip
			jsonBytes, err := json.Marshal(recording)
			if err != nil {
				t.Fatalf("Failed to marshal to JSON: %v", err)
			}

			var roundtripRecording Recording
			err = json.Unmarshal(jsonBytes, &roundtripRecording)
			if err != nil {
				t.Fatalf("Failed to unmarshal roundtrip JSON: %v", err)
			}

			if roundtripRecording.UUID != recording.UUID {
				t.Errorf("Roundtrip failed for UUID: expected %s, got %s", recording.UUID, roundtripRecording.UUID)
			}
		})
	}
}

func TestListRecordingsResponseMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected ListRecordingsResponse
	}{
		{
			name: "complete list response",
			jsonData: `{
				"from": "2022-01-01",
				"to": "2022-04-01", 
				"next_page_token": "Tva2CuIdTgsv8wAnhyAdU3m06Y2HuLQtlh3",
				"page_count": 1,
				"page_size": 30,
				"total_records": 1,
				"meetings": [
					{
						"account_id": "Cx3wERazSgup7ZWRHQM8-w",
						"duration": 20,
						"host_id": "_0ctZtY0REqWalTmwvrdIw", 
						"id": 6840331990,
						"recording_count": 1,
						"start_time": "2021-03-18T05:41:36Z",
						"topic": "My Personal Meeting",
						"total_size": 22,
						"type": 1,
						"uuid": "BOKXuumlTAGXuqwr3bLyuQ==",
						"recording_files": []
					}
				]
			}`,
			expected: ListRecordingsResponse{
				From:          "2022-01-01",
				To:            "2022-04-01",
				PageCount:     1,
				PageSize:      30,
				TotalRecords:  1,
				NextPageToken: "Tva2CuIdTgsv8wAnhyAdU3m06Y2HuLQtlh3",
				Meetings: []Recording{
					{
						UUID:           "BOKXuumlTAGXuqwr3bLyuQ==",
						ID:             6840331990,
						AccountID:      "Cx3wERazSgup7ZWRHQM8-w",
						HostID:         "_0ctZtY0REqWalTmwvrdIw",
						Topic:          "My Personal Meeting",
						Type:           1,
						StartTime:      time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
						Duration:       20,
						TotalSize:      22,
						RecordingCount: 1,
						RecordingFiles: []RecordingFile{},
					},
				},
			},
		},
		{
			name: "empty meetings list",
			jsonData: `{
				"from": "2022-01-01",
				"to": "2022-04-01",
				"page_count": 0,
				"page_size": 30,
				"total_records": 0,
				"meetings": []
			}`,
			expected: ListRecordingsResponse{
				From:         "2022-01-01",
				To:           "2022-04-01",
				PageCount:    0,
				PageSize:     30,
				TotalRecords: 0,
				Meetings:     []Recording{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Unmarshal
			var response ListRecordingsResponse
			err := json.Unmarshal([]byte(tt.jsonData), &response)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Compare fields
			if response.From != tt.expected.From {
				t.Errorf("Expected From %s, got %s", tt.expected.From, response.From)
			}
			if response.To != tt.expected.To {
				t.Errorf("Expected To %s, got %s", tt.expected.To, response.To)
			}
			if response.TotalRecords != tt.expected.TotalRecords {
				t.Errorf("Expected TotalRecords %d, got %d", tt.expected.TotalRecords, response.TotalRecords)
			}
			if len(response.Meetings) != len(tt.expected.Meetings) {
				t.Errorf("Expected %d meetings, got %d", len(tt.expected.Meetings), len(response.Meetings))
			}

			// Test Marshal roundtrip
			jsonBytes, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("Failed to marshal to JSON: %v", err)
			}

			var roundtripResponse ListRecordingsResponse
			err = json.Unmarshal(jsonBytes, &roundtripResponse)
			if err != nil {
				t.Fatalf("Failed to unmarshal roundtrip JSON: %v", err)
			}

			if roundtripResponse.From != response.From {
				t.Errorf("Roundtrip failed for From: expected %s, got %s", response.From, roundtripResponse.From)
			}
		})
	}
}

func TestParticipantAudioFileMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected ParticipantAudioFile
	}{
		{
			name: "complete participant audio file",
			jsonData: `{
				"download_url": "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"file_name": "test.m4a",
				"file_path": "/9090876528/path01/demo.m4a",
				"file_size": 65536,
				"file_type": "M4A",
				"id": "a2f19f96-9294-4f51-8134-6f0eea108eb2",
				"play_url": "https://example.com/rec/play/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"recording_end": "2021-06-30T22:14:57Z",
				"recording_start": "2021-06-30T22:12:35Z"
			}`,
			expected: ParticipantAudioFile{
				ID:             "a2f19f96-9294-4f51-8134-6f0eea108eb2",
				FileName:       "test.m4a",
				FilePath:       "/9090876528/path01/demo.m4a",
				FileSize:       65536,
				FileType:       "M4A",
				DownloadURL:    "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				PlayURL:        "https://example.com/rec/play/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				RecordingStart: time.Date(2021, 6, 30, 22, 12, 35, 0, time.UTC),
				RecordingEnd:   time.Date(2021, 6, 30, 22, 14, 57, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Unmarshal
			var audioFile ParticipantAudioFile
			err := json.Unmarshal([]byte(tt.jsonData), &audioFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Compare fields
			if audioFile.ID != tt.expected.ID {
				t.Errorf("Expected ID %s, got %s", tt.expected.ID, audioFile.ID)
			}
			if audioFile.FileName != tt.expected.FileName {
				t.Errorf("Expected FileName %s, got %s", tt.expected.FileName, audioFile.FileName)
			}

			// Test Marshal roundtrip
			jsonBytes, err := json.Marshal(audioFile)
			if err != nil {
				t.Fatalf("Failed to marshal to JSON: %v", err)
			}

			var roundtripAudioFile ParticipantAudioFile
			err = json.Unmarshal(jsonBytes, &roundtripAudioFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal roundtrip JSON: %v", err)
			}

			if roundtripAudioFile.ID != audioFile.ID {
				t.Errorf("Roundtrip failed for ID: expected %s, got %s", audioFile.ID, roundtripAudioFile.ID)
			}
		})
	}
}

// TestDateTimeParsing tests various date/time formats that Zoom API might return
func TestDateTimeParsing(t *testing.T) {
	tests := []struct {
		name         string
		jsonData     string
		expectedTime time.Time
		shouldError  bool
	}{
		{
			name:         "standard RFC3339 format",
			jsonData:     `{"recording_start": "2021-03-18T05:41:36Z"}`,
			expectedTime: time.Date(2021, 3, 18, 5, 41, 36, 0, time.UTC),
			shouldError:  false,
		},
		{
			name:         "RFC3339 with milliseconds",
			jsonData:     `{"recording_start": "2021-03-18T05:41:36.123Z"}`,
			expectedTime: time.Date(2021, 3, 18, 5, 41, 36, 123000000, time.UTC),
			shouldError:  false,
		},
		{
			name:         "RFC3339 with timezone offset",
			jsonData:     `{"recording_start": "2021-03-18T05:41:36-07:00"}`,
			expectedTime: time.Date(2021, 3, 18, 12, 41, 36, 0, time.UTC), // Convert to UTC
			shouldError:  false,
		},
		{
			name:        "invalid date format",
			jsonData:    `{"recording_start": "invalid-date"}`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var testStruct struct {
				RecordingStart time.Time `json:"recording_start"`
			}
			
			err := json.Unmarshal([]byte(tt.jsonData), &testStruct)
			
			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for invalid date format, but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			
			if !testStruct.RecordingStart.Equal(tt.expectedTime) {
				t.Errorf("Expected time %v, got %v", tt.expectedTime, testStruct.RecordingStart)
			}
		})
	}
}

// TestRecordingFileTypes tests various file types that might be returned by the API
func TestRecordingFileTypes(t *testing.T) {
	fileTypes := []string{"MP4", "M4A", "TRANSCRIPT", "CHAT", "CC", "CSV", "SUMMARY", "TIMELINE"}
	
	for _, fileType := range fileTypes {
		t.Run("file_type_"+fileType, func(t *testing.T) {
			jsonData := `{
				"id": "test-id",
				"meeting_id": "test-meeting",
				"recording_start": "2021-03-18T05:41:36Z",
				"recording_end": "2021-03-18T05:41:36Z",
				"file_type": "` + fileType + `",
				"file_size": 1024,
				"download_url": "https://example.com/download",
				"status": "completed"
			}`
			
			var recordingFile RecordingFile
			err := json.Unmarshal([]byte(jsonData), &recordingFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}
			
			if recordingFile.FileType != fileType {
				t.Errorf("Expected FileType %s, got %s", fileType, recordingFile.FileType)
			}
		})
	}
}

// TestMeetingTypes tests various meeting types that might be returned by the API  
func TestMeetingTypes(t *testing.T) {
	meetingTypes := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 100}
	
	for _, meetingType := range meetingTypes {
		t.Run(fmt.Sprintf("meeting_type_%d", meetingType), func(t *testing.T) {
			jsonData := `{
				"uuid": "test-uuid",
				"id": 123456789,
				"account_id": "test-account",
				"host_id": "test-host",
				"topic": "Test Meeting",
				"type": ` + fmt.Sprintf("%d", meetingType) + `,
				"start_time": "2021-03-18T05:41:36Z",
				"duration": 60,
				"total_size": 1024,
				"recording_count": 1,
				"recording_files": []
			}`
			
			var recording Recording
			err := json.Unmarshal([]byte(jsonData), &recording)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}
			
			if recording.Type != meetingType {
				t.Errorf("Expected Type %d, got %d", meetingType, recording.Type)
			}
		})
	}
}

// TestLargeFileSize tests handling of large file sizes
func TestLargeFileSize(t *testing.T) {
	tests := []struct {
		name     string
		fileSize int64
	}{
		{"small file", 1024},
		{"medium file", 1024 * 1024},        // 1MB
		{"large file", 1024 * 1024 * 1024},  // 1GB
		{"very large file", 5 * 1024 * 1024 * 1024}, // 5GB
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData := fmt.Sprintf(`{
				"id": "test-id",
				"meeting_id": "test-meeting",
				"recording_start": "2021-03-18T05:41:36Z",
				"recording_end": "2021-03-18T05:41:36Z",
				"file_type": "MP4",
				"file_size": %d,
				"download_url": "https://example.com/download",
				"status": "completed"
			}`, tt.fileSize)
			
			var recordingFile RecordingFile
			err := json.Unmarshal([]byte(jsonData), &recordingFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}
			
			if recordingFile.FileSize != tt.fileSize {
				t.Errorf("Expected FileSize %d, got %d", tt.fileSize, recordingFile.FileSize)
			}
		})
	}
}

// TestHandleNullAndEmptyFields tests proper handling of null and empty values
func TestHandleNullAndEmptyFields(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
	}{
		{
			name: "null optional fields",
			jsonData: `{
				"id": "72576a1f-4e66-4a77-87c4-f13f9808bd76",
				"meeting_id": "L0AGOEPVR9m5WSOOs/d+FQ==",
				"recording_start": "2021-03-18T05:41:36Z",
				"recording_end": "2021-03-18T05:41:36Z",
				"file_type": "MP4",
				"file_size": 7220,
				"download_url": "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"status": "completed",
				"deleted_time": null,
				"file_path": null,
				"play_url": null
			}`,
		},
		{
			name: "empty string fields",
			jsonData: `{
				"id": "72576a1f-4e66-4a77-87c4-f13f9808bd76",
				"meeting_id": "L0AGOEPVR9m5WSOOs/d+FQ==",
				"recording_start": "2021-03-18T05:41:36Z",
				"recording_end": "2021-03-18T05:41:36Z",
				"file_type": "MP4",
				"file_size": 7220,
				"download_url": "https://example.com/rec/download/Qg75t7xZBtEbAkjdlgbfdngBBBB",
				"status": "completed",
				"file_path": "",
				"play_url": ""
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recordingFile RecordingFile
			err := json.Unmarshal([]byte(tt.jsonData), &recordingFile)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Should handle null/empty fields without error
			if recordingFile.ID == "" {
				t.Errorf("ID should not be empty")
			}
		})
	}
}