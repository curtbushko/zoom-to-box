package users

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestActiveUserManager tests the complete active user list management functionality
func TestActiveUserManager(t *testing.T) {
	tests := []struct {
		name           string
		fileContent    string
		expectedUsers  []string
		expectedError  bool
		caseSensitive  bool
	}{
		{
			name: "valid user list with mixed content",
			fileContent: `john.doe@company.com
jane.smith@company.com
admin@company.com

# This is a comment
user@example.org
# Another comment

test.user@domain.co.uk`,
			expectedUsers: []string{
				"john.doe@company.com",
				"jane.smith@company.com", 
				"admin@company.com",
				"user@example.org",
				"test.user@domain.co.uk",
			},
			expectedError: false,
			caseSensitive: false,
		},
		{
			name: "empty file",
			fileContent: ``,
			expectedUsers: []string{},
			expectedError: false,
			caseSensitive: false,
		},
		{
			name: "only comments and empty lines",
			fileContent: `
# This is a comment
# Another comment


   # Indented comment
`,
			expectedUsers: []string{},
			expectedError: false,
			caseSensitive: false,
		},
		{
			name: "case sensitivity test",
			fileContent: `John.Doe@Company.com
JANE.SMITH@COMPANY.COM
admin@company.com`,
			expectedUsers: []string{
				"john.doe@company.com",
				"jane.smith@company.com", 
				"admin@company.com",
			},
			expectedError: false,
			caseSensitive: false,
		},
		{
			name: "case sensitive mode",
			fileContent: `John.Doe@Company.com
JANE.SMITH@COMPANY.COM
admin@company.com`,
			expectedUsers: []string{
				"John.Doe@Company.com",
				"JANE.SMITH@COMPANY.COM", 
				"admin@company.com",
			},
			expectedError: false,
			caseSensitive: true,
		},
		{
			name: "invalid email formats",
			fileContent: `valid@example.com
invalid-email
@missing-domain.com
missing-at-sign.com
valid.user@domain.org`,
			expectedUsers: []string{
				"valid@example.com",
				"valid.user@domain.org",
			},
			expectedError: false,
			caseSensitive: false,
		},
		{
			name: "duplicate emails",
			fileContent: `user@example.com
admin@company.com
user@example.com
admin@company.com
USER@EXAMPLE.COM`,
			expectedUsers: []string{
				"user@example.com",
				"admin@company.com",
			},
			expectedError: false,
			caseSensitive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tempDir := t.TempDir()
			userListFile := filepath.Join(tempDir, "active_users.txt")
			
			err := os.WriteFile(userListFile, []byte(tt.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Create manager
			config := ActiveUserConfig{
				FilePath:      userListFile,
				CaseSensitive: tt.caseSensitive,
				WatchFile:     false, // Disable file watching for basic tests
			}
			
			manager, err := NewActiveUserManager(config)
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			defer manager.Close()

			// Verify loaded users
			users := manager.GetActiveUsers()
			if len(users) != len(tt.expectedUsers) {
				t.Errorf("Expected %d users, got %d", len(tt.expectedUsers), len(users))
			}

			// Check each expected user
			for _, expectedUser := range tt.expectedUsers {
				if !manager.IsUserActive(expectedUser) {
					t.Errorf("Expected user %s to be active", expectedUser)
				}
			}

			// Verify user list matches expected (order may vary due to map iteration)
			expectedMap := make(map[string]bool)
			for _, user := range tt.expectedUsers {
				expectedMap[user] = true
			}
			
			actualMap := make(map[string]bool)  
			for _, user := range users {
				actualMap[user] = true
			}

			for expectedUser := range expectedMap {
				if !actualMap[expectedUser] {
					t.Errorf("Expected user %s not found in actual users", expectedUser)
				}
			}

			for actualUser := range actualMap {
				if !expectedMap[actualUser] {
					t.Errorf("Unexpected user %s found in actual users", actualUser)
				}
			}
		})
	}
}

// TestEmailValidation tests email validation functionality
func TestEmailValidation(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{"valid simple email", "user@example.com", true},
		{"valid complex email", "john.doe+test@company-name.co.uk", true},
		{"valid with numbers", "user123@example123.com", true},
		{"valid with hyphens", "first-last@sub-domain.example.com", true},
		{"valid with underscores", "first_last@example_domain.com", true},
		{"empty string", "", false},
		{"missing @ symbol", "userexample.com", false},
		{"missing domain", "user@", false},
		{"missing username", "@example.com", false},
		{"multiple @ symbols", "user@@example.com", false},
		{"invalid domain", "user@.com", false},
		{"spaces in email", "user name@example.com", false},
		{"leading/trailing spaces", " user@example.com ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidEmail(tt.email)
			if result != tt.expected {
				t.Errorf("Expected %t for email '%s', got %t", tt.expected, tt.email, result)
			}
		})
	}
}

// TestUserListFileWatching tests real-time file watching functionality
func TestUserListFileWatching(t *testing.T) {
	// Create temporary file
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")
	
	initialContent := `user1@example.com
user2@example.com`
	
	err := os.WriteFile(userListFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create manager with file watching enabled
	config := ActiveUserConfig{
		FilePath:      userListFile,
		CaseSensitive: false,
		WatchFile:     true,
	}
	
	manager, err := NewActiveUserManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Verify initial users
	if !manager.IsUserActive("user1@example.com") {
		t.Error("Expected user1@example.com to be active initially")
	}
	if !manager.IsUserActive("user2@example.com") {
		t.Error("Expected user2@example.com to be active initially")
	}
	if manager.IsUserActive("user3@example.com") {
		t.Error("Expected user3@example.com to be inactive initially")
	}

	// Update file content
	updatedContent := `user2@example.com
user3@example.com
user4@example.com`
	
	err = os.WriteFile(userListFile, []byte(updatedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to update test file: %v", err)
	}

	// Wait for file watcher to detect changes
	time.Sleep(100 * time.Millisecond)

	// Verify updated users
	if manager.IsUserActive("user1@example.com") {
		t.Error("Expected user1@example.com to be inactive after update")
	}
	if !manager.IsUserActive("user2@example.com") {
		t.Error("Expected user2@example.com to be active after update")
	}
	if !manager.IsUserActive("user3@example.com") {
		t.Error("Expected user3@example.com to be active after update")
	}
	if !manager.IsUserActive("user4@example.com") {
		t.Error("Expected user4@example.com to be active after update")
	}
}

// TestMalformedFileHandling tests handling of malformed and problematic files
func TestMalformedFileHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     bool
		fileContent   string
		fileMode      os.FileMode
		expectedError bool
		expectedUsers int
	}{
		{
			name:          "non-existent file",
			setupFile:     false,
			expectedError: true,
			expectedUsers: 0,
		},
		{
			name:          "unreadable file",
			setupFile:     true,
			fileContent:   "user@example.com",
			fileMode:      0000, // No read permissions
			expectedError: true,
			expectedUsers: 0,
		},
		{
			name:        "very long lines",
			setupFile:   true,
			fileContent: strings.Repeat("a", 10000) + "@example.com\nuser@example.com",
			fileMode:    0644,
			expectedError: false,
			expectedUsers: 1, // Only valid email should be processed
		},
		{
			name:        "binary content",
			setupFile:   true,
			fileContent: "\x00\x01\x02\x03user@example.com\nvalid@example.com",
			fileMode:    0644,
			expectedError: false,
			expectedUsers: 1, // Only valid email should be processed
		},
		{
			name:        "unicode content",
			setupFile:   true,
			fileContent: "用户@example.com\nuser@example.com\nтест@example.com",
			fileMode:    0644,
			expectedError: false,
			expectedUsers: 1, // Only ASCII email should be valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			userListFile := filepath.Join(tempDir, "active_users.txt")
			
			if tt.setupFile {
				err := os.WriteFile(userListFile, []byte(tt.fileContent), tt.fileMode)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			config := ActiveUserConfig{
				FilePath:      userListFile,
				CaseSensitive: false,
				WatchFile:     false,
			}
			
			manager, err := NewActiveUserManager(config)
			
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			defer manager.Close()

			users := manager.GetActiveUsers()
			if len(users) != tt.expectedUsers {
				t.Errorf("Expected %d users, got %d", tt.expectedUsers, len(users))
			}
		})
	}
}

// TestConcurrentAccess tests thread-safe concurrent access
func TestConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")
	
	fileContent := `user1@example.com
user2@example.com
user3@example.com
user4@example.com
user5@example.com`
	
	err := os.WriteFile(userListFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := ActiveUserConfig{
		FilePath:      userListFile,
		CaseSensitive: false,
		WatchFile:     false,
	}
	
	manager, err := NewActiveUserManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Start multiple goroutines to test concurrent access
	done := make(chan bool, 10)
	
	// Readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				manager.IsUserActive("user1@example.com")
				manager.GetActiveUsers()
				time.Sleep(time.Microsecond)
			}
			done <- true
		}()
	}

	// File updaters (simulating file watching updates)
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				newContent := fileContent + "\nnewuser" + string(rune(id)) + "@example.com"
				os.WriteFile(userListFile, []byte(newContent), 0644)
				time.Sleep(10 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify manager is still functional
	if !manager.IsUserActive("user1@example.com") {
		t.Error("Expected user1@example.com to be active after concurrent access")
	}
}

// TestActiveUserStats tests user statistics functionality
func TestActiveUserStats(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")
	
	fileContent := `user1@example.com
user2@company.org
admin@company.org
# Comment line
test@example.net

invalid-email
user3@company.org`
	
	err := os.WriteFile(userListFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := ActiveUserConfig{
		FilePath:      userListFile,
		CaseSensitive: false,
		WatchFile:     false,
	}
	
	manager, err := NewActiveUserManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Test statistics
	stats := manager.GetStats()
	
	expectedTotal := 5 // 5 valid emails
	if stats.TotalUsers != expectedTotal {
		t.Errorf("Expected %d total users, got %d", expectedTotal, stats.TotalUsers)
	}

	if stats.LastUpdated.IsZero() {
		t.Error("Expected LastUpdated to be set")
	}

	// Test domain statistics
	domainCounts := make(map[string]int)
	for _, user := range manager.GetActiveUsers() {
		parts := strings.Split(user, "@")
		if len(parts) == 2 {
			domainCounts[parts[1]]++
		}
	}

	expectedDomains := map[string]int{
		"example.com": 1,
		"company.org": 3,
		"example.net": 1,
	}

	for domain, expectedCount := range expectedDomains {
		if domainCounts[domain] != expectedCount {
			t.Errorf("Expected %d users for domain %s, got %d", expectedCount, domain, domainCounts[domain])
		}
	}
}

// TestDisabledUserFiltering tests behavior when user filtering is disabled
func TestDisabledUserFiltering(t *testing.T) {
	// When no file path is provided, all users should be considered active
	config := ActiveUserConfig{
		FilePath:      "", // Empty path disables filtering
		CaseSensitive: false,
		WatchFile:     false,
	}
	
	manager, err := NewActiveUserManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// When filtering is disabled, any email should be considered active
	testEmails := []string{
		"any@example.com",
		"random@domain.org",
		"test@company.com",
	}

	for _, email := range testEmails {
		if !manager.IsUserActive(email) {
			t.Errorf("Expected %s to be active when filtering is disabled", email)
		}
	}

	// GetActiveUsers should return empty slice when filtering is disabled
	users := manager.GetActiveUsers()
	if len(users) != 0 {
		t.Errorf("Expected empty user list when filtering is disabled, got %d users", len(users))
	}
}