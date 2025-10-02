package email

import "testing"

func TestExtractUsername(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{"valid email", "john.doe@company.com", "john.doe"},
		{"email with plus", "user+tag@example.org", "user+tag"},
		{"email with numbers", "user123@example456.com", "user123"},
		{"email with hyphens", "first-last@example-company.com", "first-last"},
		{"email with underscores", "first_last@example_domain.com", "first_last"},
		{"empty email", "", ""},
		{"invalid email - no @", "invalid-email", ""},
		{"invalid email - no domain", "user@", ""},
		{"invalid email - no username", "@domain.com", ""},
		{"invalid email - multiple @", "user@@domain.com", ""},
		{"email with leading space", " user@domain.com", ""},
		{"email with trailing space", "user@domain.com ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractUsername(tt.email)
			if result != tt.expected {
				t.Errorf("ExtractUsername(%q) = %q, expected %q", tt.email, result, tt.expected)
			}
		})
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{"valid email", "john.doe@company.com", true},
		{"valid email with plus", "user+tag@example.org", true},
		{"valid email with numbers", "user123@example456.com", true},
		{"valid email with hyphens", "first-last@example-company.com", true},
		{"valid email with underscores", "first_last@example_domain.com", true},
		{"empty email", "", false},
		{"invalid email - no @", "invalid-email", false},
		{"invalid email - no domain", "user@", false},
		{"invalid email - no username", "@domain.com", false},
		{"invalid email - multiple @", "user@@domain.com", false},
		{"email with leading space", " user@domain.com", false},
		{"email with trailing space", "user@domain.com ", false},
		{"too long email", string(make([]byte, 325)) + "@domain.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidEmail(tt.email)
			if result != tt.expected {
				t.Errorf("IsValidEmail(%q) = %t, expected %t", tt.email, result, tt.expected)
			}
		})
	}
}

func TestSanitizeForDirectory(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{"valid email", "john.doe@company.com", "john.doe"},
		{"email with special chars", "user+tag@example.org", "user+tag"},
		{"invalid email", "invalid-email", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeForDirectory(tt.email)
			if result != tt.expected {
				t.Errorf("SanitizeForDirectory(%q) = %q, expected %q", tt.email, result, tt.expected)
			}
		})
	}
}