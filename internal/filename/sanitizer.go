// Package filename provides filename sanitization functionality for Zoom recording files
package filename

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/curtbushko/zoom-to-box/internal/zoom"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// FileSanitizer handles filename sanitization for Zoom recordings
type FileSanitizer interface {
	// SanitizeTopic converts a meeting topic to a filesystem-safe lowercase string with dashes
	SanitizeTopic(topic string) string
	
	// FormatTime formats a time to HHMM format for filename timestamps
	FormatTime(t time.Time) string
	
	// GenerateFilename creates a complete filename from recording data and file type
	GenerateFilename(recording zoom.Recording, fileType string) string
	
	// GetFileExtension returns the appropriate file extension for a given file type
	GetFileExtension(fileType string) string
}

// FileSanitizerOptions contains configuration options for the file sanitizer
type FileSanitizerOptions struct {
	// MaxTopicLength sets the maximum length for sanitized topic (default: 100)
	MaxTopicLength int
	
	// DefaultTopic is used when the topic is empty or only contains invalid characters (default: "untitled")
	DefaultTopic string
}

// fileSanitizer is the concrete implementation of FileSanitizer
type fileSanitizer struct {
	maxTopicLength int
	defaultTopic   string
	
	// Compiled regex for performance
	invalidCharsRegex    *regexp.Regexp
	multipleSpacesRegex  *regexp.Regexp
	nonAlphaNumRegex     *regexp.Regexp
}

// NewFileSanitizer creates a new FileSanitizer with the given options
func NewFileSanitizer(options FileSanitizerOptions) FileSanitizer {
	maxLength := options.MaxTopicLength
	if maxLength <= 0 {
		maxLength = 100 // Default max length
	}
	
	defaultTopic := options.DefaultTopic
	if defaultTopic == "" {
		defaultTopic = "untitled"
	}
	
	return &fileSanitizer{
		maxTopicLength:       maxLength,
		defaultTopic:        defaultTopic,
		invalidCharsRegex:   regexp.MustCompile(`[<>:"/\\|?*]`),
		multipleSpacesRegex: regexp.MustCompile(`\s+`),
		nonAlphaNumRegex:    regexp.MustCompile(`[^a-zA-Z0-9\s]`),
	}
}

// SanitizeTopic converts a meeting topic to a filesystem-safe lowercase string with dashes
func (fs *fileSanitizer) SanitizeTopic(topic string) string {
	if topic == "" {
		return fs.defaultTopic
	}
	
	// Normalize unicode characters and remove diacritics
	normalized := fs.normalizeUnicode(topic)
	
	// Remove invalid filesystem characters but preserve spaces for word separation
	cleaned := fs.invalidCharsRegex.ReplaceAllString(normalized, " ")
	
	// Handle special characters more carefully
	// Replace punctuation with spaces but keep alphanumeric characters intact
	var result strings.Builder
	for _, r := range cleaned {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune(' ')
		}
	}
	
	// Replace multiple spaces with single space
	singleSpaced := fs.multipleSpacesRegex.ReplaceAllString(result.String(), " ")
	
	// Trim whitespace
	trimmed := strings.TrimSpace(singleSpaced)
	
	// Convert to lowercase
	lowercased := strings.ToLower(trimmed)
	
	// Replace spaces with dashes
	dashed := strings.ReplaceAll(lowercased, " ", "-")
	
	// Replace underscores with dashes for consistency
	dashed = strings.ReplaceAll(dashed, "_", "-")
	
	// Remove multiple consecutive dashes
	dashRegex := regexp.MustCompile(`-+`)
	dashed = dashRegex.ReplaceAllString(dashed, "-")
	
	// Remove leading/trailing dashes
	dashed = strings.Trim(dashed, "-")
	
	// If result is empty after cleaning, use default
	if dashed == "" {
		return fs.defaultTopic
	}
	
	// Truncate to max length, ensuring we don't cut in the middle of a word boundary
	if len(dashed) > fs.maxTopicLength {
		truncated := dashed[:fs.maxTopicLength]
		// Find the last dash to avoid cutting in middle of word
		lastDash := strings.LastIndex(truncated, "-")
		if lastDash > fs.maxTopicLength*2/3 { // Only use last dash if it's reasonably close to end
			dashed = truncated[:lastDash]
		} else {
			dashed = truncated
		}
		// Remove trailing dash
		dashed = strings.TrimRight(dashed, "-")
	}
	
	return dashed
}

// normalizeUnicode removes diacritics and converts unicode to ASCII equivalents
func (fs *fileSanitizer) normalizeUnicode(s string) string {
	// Create a transformer that removes diacritics
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	
	// Transform the string
	result, _, _ := transform.String(t, s)
	
	// Remove emojis and other non-printable unicode characters
	var cleaned strings.Builder
	for _, r := range result {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsPunct(r)) {
			cleaned.WriteRune(r)
		} else if unicode.IsSpace(r) {
			cleaned.WriteRune(' ')
		}
	}
	
	return cleaned.String()
}

// FormatTime formats a time to HHMM format for filename timestamps
// Always formats in the original timezone to preserve meeting context
func (fs *fileSanitizer) FormatTime(t time.Time) string {
	return t.Format("1504") // Go's reference time format for HHMM
}

// GenerateFilename creates a complete filename from recording data and file type
func (fs *fileSanitizer) GenerateFilename(recording zoom.Recording, fileType string) string {
	sanitizedTopic := fs.SanitizeTopic(recording.Topic)
	timeComponent := fs.FormatTime(recording.StartTime)
	extension := fs.GetFileExtension(fileType)
	
	return sanitizedTopic + "-" + timeComponent + extension
}

// GetFileExtension returns the appropriate file extension for a given file type
func (fs *fileSanitizer) GetFileExtension(fileType string) string {
	// Convert to lowercase for comparison
	fileTypeLower := strings.ToLower(fileType)
	
	switch fileTypeLower {
	case "mp4":
		return ".mp4"
	case "m4a":
		return ".m4a"
	case "json":
		return ".json"
	case "transcript":
		return ".txt"
	case "chat":
		return ".txt"
	case "cc":
		return ".vtt"
	case "csv":
		return ".csv"
	default:
		return ".bin" // Unknown file types
	}
}