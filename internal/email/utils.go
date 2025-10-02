// Package email provides utilities for email address handling
package email

import (
	"regexp"
	"strings"
)

// Email validation regex - same as directory and users packages
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9._-]+\.[a-zA-Z]{2,}$`)

// ExtractUsername extracts the username portion from an email address
// Returns empty string if the email is invalid or malformed
func ExtractUsername(email string) string {
	if email == "" {
		return ""
	}
	
	// Validate email format first
	if !IsValidEmail(email) {
		return ""
	}
	
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	
	return parts[0]
}

// IsValidEmail performs basic email validation
func IsValidEmail(email string) bool {
	// Check for empty email first
	if email == "" {
		return false
	}
	
	// Check if email has leading/trailing spaces (invalid)
	if strings.TrimSpace(email) != email {
		return false
	}
	
	// Check for reasonable length limit (RFC 5321 suggests 320 chars max)
	if len(email) > 320 {
		return false
	}
	
	// Check for basic format using regex
	return emailRegex.MatchString(email)
}

// SanitizeForDirectory extracts the username portion from an email address
// and sanitizes it for use as a directory name. This is an alias for ExtractUsername
// for backwards compatibility.
func SanitizeForDirectory(email string) string {
	return ExtractUsername(email)
}