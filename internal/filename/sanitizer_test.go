// Package filename provides filename sanitization functionality for Zoom recording files
package filename

import (
	"strings"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

func TestSanitizeTopic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic conversion",
			input:    "Weekly Team Meeting",
			expected: "weekly-team-meeting",
		},
		{
			name:     "multiple spaces",
			input:    "Q4   Planning   Session",
			expected: "q4-planning-session",
		},
		{
			name:     "special characters",
			input:    "Q4 Planning: Budget & Goals",
			expected: "q4-planning-budget-goals",
		},
		{
			name:     "parentheses and slashes",
			input:    "Test/Meeting (Final)",
			expected: "test-meeting-final",
		},
		{
			name:     "unicode characters",
			input:    "Café Meeting 🎉",
			expected: "cafe-meeting",
		},
		{
			name:     "dots and commas",
			input:    "Project Review, Phase 1.0",
			expected: "project-review-phase-1-0",
		},
		{
			name:     "question marks and exclamation",
			input:    "What's Next? Action Items!",
			expected: "what-s-next-action-items",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  Meeting Topic  ",
			expected: "meeting-topic",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "untitled",
		},
		{
			name:     "only special characters",
			input:    "!@#$%^&*()",
			expected: "untitled",
		},
		{
			name:     "very long title",
			input:    "This is a very long meeting title that should be truncated to a reasonable length for filesystem compatibility",
			expected: "this-is-a-very-long-meeting-title-that-should-be-truncated-to-a-reasonable-length-for-filesystem",
		},
		{
			name:     "underscores",
			input:    "Dev_Team_Standup",
			expected: "dev-team-standup",
		},
		{
			name:     "numbers only",
			input:    "123456",
			expected: "123456",
		},
	}

	sanitizer := NewFileSanitizer(FileSanitizerOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.SanitizeTopic(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeTopic(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "morning time",
			time:     time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
			expected: "1030",
		},
		{
			name:     "afternoon time",
			time:     time.Date(2024, 1, 15, 14, 15, 0, 0, time.UTC),
			expected: "1415",
		},
		{
			name:     "evening time",
			time:     time.Date(2024, 1, 15, 21, 45, 30, 0, time.UTC),
			expected: "2145",
		},
		{
			name:     "midnight",
			time:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			expected: "0000",
		},
		{
			name:     "single digit minute",
			time:     time.Date(2024, 1, 15, 9, 5, 0, 0, time.UTC),
			expected: "0905",
		},
	}

	sanitizer := NewFileSanitizer(FileSanitizerOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.FormatTime(tt.time)
			if result != tt.expected {
				t.Errorf("FormatTime(%v) = %q, want %q", tt.time, result, tt.expected)
			}
		})
	}
}

func TestGenerateFilename(t *testing.T) {
	tests := []struct {
		name      string
		recording zoom.Recording
		fileType  string
		expected  string
	}{
		{
			name: "basic MP4 file",
			recording: zoom.Recording{
				Topic:     "Weekly Team Meeting",
				StartTime: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			},
			fileType: "MP4",
			expected: "weekly-team-meeting-1030.mp4",
		},
		{
			name: "JSON metadata file",
			recording: zoom.Recording{
				Topic:     "Q4 Planning: Budget & Goals",
				StartTime: time.Date(2024, 1, 15, 14, 15, 0, 0, time.UTC),
			},
			fileType: "JSON",
			expected: "q4-planning-budget-goals-1415.json",
		},
		{
			name: "transcript file",
			recording: zoom.Recording{
				Topic:     "Test/Meeting (Final)",
				StartTime: time.Date(2024, 1, 15, 9, 45, 0, 0, time.UTC),
			},
			fileType: "TRANSCRIPT",
			expected: "test-meeting-final-0945.txt",
		},
		{
			name: "chat file", 
			recording: zoom.Recording{
				Topic:     "Daily Standup",
				StartTime: time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC),
			},
			fileType: "CHAT",
			expected: "daily-standup-1600.txt",
		},
		{
			name: "M4A audio file",
			recording: zoom.Recording{
				Topic:     "Audio Only Meeting",
				StartTime: time.Date(2024, 1, 15, 11, 30, 0, 0, time.UTC),
			},
			fileType: "M4A",
			expected: "audio-only-meeting-1130.m4a",
		},
		{
			name: "empty topic fallback",
			recording: zoom.Recording{
				Topic:     "",
				StartTime: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			},
			fileType: "MP4",
			expected: "untitled-1200.mp4",
		},
	}

	sanitizer := NewFileSanitizer(FileSanitizerOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.GenerateFilename(tt.recording, tt.fileType)
			if result != tt.expected {
				t.Errorf("GenerateFilename() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		expected string
	}{
		{"MP4 video", "MP4", ".mp4"},
		{"M4A audio", "M4A", ".m4a"},
		{"JSON metadata", "JSON", ".json"},
		{"TRANSCRIPT text", "TRANSCRIPT", ".txt"},
		{"CHAT text", "CHAT", ".txt"},
		{"CC captions", "CC", ".vtt"},
		{"CSV data", "CSV", ".csv"},
		{"unknown type", "UNKNOWN", ".bin"},
		{"lowercase mp4", "mp4", ".mp4"},
		{"mixed case", "Mp4", ".mp4"},
	}

	sanitizer := NewFileSanitizer(FileSanitizerOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.GetFileExtension(tt.fileType)
			if result != tt.expected {
				t.Errorf("GetFileExtension(%q) = %q, want %q", tt.fileType, result, tt.expected)
			}
		})
	}
}

func TestFileSanitizerOptions(t *testing.T) {
	t.Run("custom max length", func(t *testing.T) {
		sanitizer := NewFileSanitizer(FileSanitizerOptions{
			MaxTopicLength: 20,
		})

		longTopic := "This is a very long meeting title"
		result := sanitizer.SanitizeTopic(longTopic)
		
		// Should be truncated to 20 characters or less, ideally at word boundary
		if len(result) > 20 {
			t.Errorf("SanitizeTopic with MaxTopicLength=20: result length %d > 20, got %q", len(result), result)
		}
		
		// Should start with expected prefix
		expectedPrefix := "this-is-a-very-long"
		if !strings.HasPrefix(result, expectedPrefix) {
			t.Errorf("SanitizeTopic with MaxTopicLength=20: got %q, expected to start with %q", result, expectedPrefix)
		}
	})

	t.Run("custom default topic", func(t *testing.T) {
		sanitizer := NewFileSanitizer(FileSanitizerOptions{
			DefaultTopic: "no-topic",
		})

		result := sanitizer.SanitizeTopic("")
		expected := "no-topic"
		if result != expected {
			t.Errorf("SanitizeTopic with DefaultTopic: got %q, want %q", result, expected)
		}
	})
}

func TestTimezoneHandling(t *testing.T) {
	// Test that time formatting preserves the original timezone context
	easternTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("EST", -5*3600))
	utcTime := easternTime.UTC()
	
	sanitizer := NewFileSanitizer(FileSanitizerOptions{})
	
	// The times should format to their respective local times
	easternResult := sanitizer.FormatTime(easternTime)
	utcResult := sanitizer.FormatTime(utcTime)
	
	// Eastern time should be 10:30 (1030), UTC should be 15:30 (1530)
	expectedEastern := "1030"
	expectedUTC := "1530"
	
	if easternResult != expectedEastern {
		t.Errorf("Eastern time formatting: got %q, want %q", easternResult, expectedEastern)
	}
	
	if utcResult != expectedUTC {
		t.Errorf("UTC time formatting: got %q, want %q", utcResult, expectedUTC)
	}
}

func TestUnicodeHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "emoji removal",
			input:    "Meeting 🎉 with emojis 👍",
			expected: "meeting-with-emojis",
		},
		{
			name:     "accented characters",
			input:    "Café Résumé Naïve",
			expected: "cafe-resume-naive",
		},
		{
			name:     "mixed unicode",
			input:    "日本語 English 中文",
			expected: "english",
		},
		{
			name:     "currency symbols",
			input:    "Budget $100 €50 ¥1000",
			expected: "budget-100-50-1000",
		},
	}

	sanitizer := NewFileSanitizer(FileSanitizerOptions{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.SanitizeTopic(tt.input)
			if result != tt.expected {
				t.Errorf("Unicode handling for %q: got %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}