package users

import (
	"fmt"
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

// TestActiveUsersFileWithUploadTracking tests the enhanced 3-column file format
func TestActiveUsersFileWithUploadTracking(t *testing.T) {
	tests := []struct {
		name                string
		fileContent         string
		expectedEntries     []UserEntry
		expectedIncomplete  int
	}{
		{
			name: "3-column format with mixed completion status",
			fileContent: `john.doe@zoom.com,john.doe@box.com,false
jane.smith@zoom.com,jane.smith@box.com,true
admin@zoom.com,admin@box.com,false`,
			expectedEntries: []UserEntry{
				{ZoomEmail: "john.doe@zoom.com", BoxEmail: "john.doe@box.com", UploadComplete: false, LineNumber: 1},
				{ZoomEmail: "jane.smith@zoom.com", BoxEmail: "jane.smith@box.com", UploadComplete: true, LineNumber: 2},
				{ZoomEmail: "admin@zoom.com", BoxEmail: "admin@box.com", UploadComplete: false, LineNumber: 3},
			},
			expectedIncomplete: 2,
		},
		{
			name: "2-column backward compatibility (defaults to incomplete)",
			fileContent: `user1@zoom.com,user1@box.com
user2@zoom.com,user2@box.com`,
			expectedEntries: []UserEntry{
				{ZoomEmail: "user1@zoom.com", BoxEmail: "user1@box.com", UploadComplete: false, LineNumber: 1},
				{ZoomEmail: "user2@zoom.com", BoxEmail: "user2@box.com", UploadComplete: false, LineNumber: 2},
			},
			expectedIncomplete: 2,
		},
		{
			name: "1-column backward compatibility",
			fileContent: `user@example.com
admin@company.org`,
			expectedEntries: []UserEntry{
				{ZoomEmail: "user@example.com", BoxEmail: "user@example.com", UploadComplete: false, LineNumber: 1},
				{ZoomEmail: "admin@company.org", BoxEmail: "admin@company.org", UploadComplete: false, LineNumber: 2},
			},
			expectedIncomplete: 2,
		},
		{
			name: "boolean parsing variations (true/false, yes/no, 1/0)",
			fileContent: `user1@example.com,user1@box.com,true
user2@example.com,user2@box.com,false
user3@example.com,user3@box.com,yes
user4@example.com,user4@box.com,no
user5@example.com,user5@box.com,1
user6@example.com,user6@box.com,0`,
			expectedEntries: []UserEntry{
				{ZoomEmail: "user1@example.com", BoxEmail: "user1@box.com", UploadComplete: true, LineNumber: 1},
				{ZoomEmail: "user2@example.com", BoxEmail: "user2@box.com", UploadComplete: false, LineNumber: 2},
				{ZoomEmail: "user3@example.com", BoxEmail: "user3@box.com", UploadComplete: true, LineNumber: 3},
				{ZoomEmail: "user4@example.com", BoxEmail: "user4@box.com", UploadComplete: false, LineNumber: 4},
				{ZoomEmail: "user5@example.com", BoxEmail: "user5@box.com", UploadComplete: true, LineNumber: 5},
				{ZoomEmail: "user6@example.com", BoxEmail: "user6@box.com", UploadComplete: false, LineNumber: 6},
			},
			expectedIncomplete: 3,
		},
		{
			name: "mixed format with comments and empty lines",
			fileContent: `# Header comment
john@zoom.com,john@box.com,false

# Another comment
jane@zoom.com,,true
admin@example.com

# Final comment`,
			expectedEntries: []UserEntry{
				{ZoomEmail: "john@zoom.com", BoxEmail: "john@box.com", UploadComplete: false, LineNumber: 2},
				{ZoomEmail: "jane@zoom.com", BoxEmail: "jane@zoom.com", UploadComplete: true, LineNumber: 5},
				{ZoomEmail: "admin@example.com", BoxEmail: "admin@example.com", UploadComplete: false, LineNumber: 6},
			},
			expectedIncomplete: 2,
		},
		{
			name: "case-insensitive boolean parsing",
			fileContent: `user1@example.com,user1@box.com,TRUE
user2@example.com,user2@box.com,False
user3@example.com,user3@box.com,YES
user4@example.com,user4@box.com,No`,
			expectedEntries: []UserEntry{
				{ZoomEmail: "user1@example.com", BoxEmail: "user1@box.com", UploadComplete: true, LineNumber: 1},
				{ZoomEmail: "user2@example.com", BoxEmail: "user2@box.com", UploadComplete: false, LineNumber: 2},
				{ZoomEmail: "user3@example.com", BoxEmail: "user3@box.com", UploadComplete: true, LineNumber: 3},
				{ZoomEmail: "user4@example.com", BoxEmail: "user4@box.com", UploadComplete: false, LineNumber: 4},
			},
			expectedIncomplete: 2,
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

			// Load file
			usersFile, err := LoadActiveUsersFile(userListFile)
			if err != nil {
				t.Fatalf("Failed to load users file: %v", err)
			}

			// Verify number of entries
			if len(usersFile.Entries) != len(tt.expectedEntries) {
				t.Errorf("Expected %d entries, got %d", len(tt.expectedEntries), len(usersFile.Entries))
			}

			// Verify each entry
			for i, expected := range tt.expectedEntries {
				if i >= len(usersFile.Entries) {
					break
				}
				actual := usersFile.Entries[i]

				if actual.ZoomEmail != expected.ZoomEmail {
					t.Errorf("Entry %d: expected ZoomEmail %s, got %s", i, expected.ZoomEmail, actual.ZoomEmail)
				}
				if actual.BoxEmail != expected.BoxEmail {
					t.Errorf("Entry %d: expected BoxEmail %s, got %s", i, expected.BoxEmail, actual.BoxEmail)
				}
				if actual.UploadComplete != expected.UploadComplete {
					t.Errorf("Entry %d: expected UploadComplete %t, got %t", i, expected.UploadComplete, actual.UploadComplete)
				}
				if actual.LineNumber != expected.LineNumber {
					t.Errorf("Entry %d: expected LineNumber %d, got %d", i, expected.LineNumber, actual.LineNumber)
				}
			}

			// Verify incomplete users count
			incompleteUsers := usersFile.GetIncompleteUsers()
			if len(incompleteUsers) != tt.expectedIncomplete {
				t.Errorf("Expected %d incomplete users, got %d", tt.expectedIncomplete, len(incompleteUsers))
			}
		})
	}
}

// TestAtomicFileUpdate tests atomic file updates with concurrent access
func TestAtomicFileUpdate(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")

	initialContent := `user1@zoom.com,user1@box.com,false
user2@zoom.com,user2@box.com,false
user3@zoom.com,user3@box.com,false`

	err := os.WriteFile(userListFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Load file
	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	// Mark user as complete
	err = usersFile.MarkUserComplete("user2@zoom.com")
	if err != nil {
		t.Fatalf("Failed to mark user complete: %v", err)
	}

	// Reload file and verify update persisted
	usersFile2, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to reload users file: %v", err)
	}

	// Verify user2 is marked complete
	found := false
	for _, entry := range usersFile2.Entries {
		if entry.ZoomEmail == "user2@zoom.com" {
			found = true
			if !entry.UploadComplete {
				t.Errorf("Expected user2@zoom.com to be complete after update")
			}
		}
	}

	if !found {
		t.Error("Expected to find user2@zoom.com in reloaded file")
	}

	// Verify other users unchanged
	for _, entry := range usersFile2.Entries {
		if entry.ZoomEmail != "user2@zoom.com" {
			if entry.UploadComplete {
				t.Errorf("Expected %s to remain incomplete", entry.ZoomEmail)
			}
		}
	}
}

// TestUpdateUserStatus tests updating user upload status
func TestUpdateUserStatus(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")

	initialContent := `user1@zoom.com,user1@box.com,false
user2@zoom.com,user2@box.com,true`

	err := os.WriteFile(userListFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	// Test setting status to true
	err = usersFile.UpdateUserStatus("user1@zoom.com", true)
	if err != nil {
		t.Fatalf("Failed to update user status: %v", err)
	}

	// Test setting status to false
	err = usersFile.UpdateUserStatus("user2@zoom.com", false)
	if err != nil {
		t.Fatalf("Failed to update user status: %v", err)
	}

	// Reload and verify
	usersFile2, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to reload users file: %v", err)
	}

	for _, entry := range usersFile2.Entries {
		if entry.ZoomEmail == "user1@zoom.com" {
			if !entry.UploadComplete {
				t.Error("Expected user1@zoom.com to be complete")
			}
		}
		if entry.ZoomEmail == "user2@zoom.com" {
			if entry.UploadComplete {
				t.Error("Expected user2@zoom.com to be incomplete")
			}
		}
	}
}

// TestGetIncompleteUsers tests filtering incomplete users
func TestGetIncompleteUsersFiltering(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")

	fileContent := `user1@zoom.com,user1@box.com,false
user2@zoom.com,user2@box.com,true
user3@zoom.com,user3@box.com,false
user4@zoom.com,user4@box.com,true
user5@zoom.com,user5@box.com,false`

	err := os.WriteFile(userListFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	incompleteUsers := usersFile.GetIncompleteUsers()

	// Should have 3 incomplete users: user1, user3, user5
	if len(incompleteUsers) != 3 {
		t.Errorf("Expected 3 incomplete users, got %d", len(incompleteUsers))
	}

	// Verify correct users are incomplete
	expectedIncomplete := map[string]bool{
		"user1@zoom.com": true,
		"user3@zoom.com": true,
		"user5@zoom.com": true,
	}

	for _, entry := range incompleteUsers {
		if !expectedIncomplete[entry.ZoomEmail] {
			t.Errorf("Unexpected incomplete user: %s", entry.ZoomEmail)
		}
		if entry.UploadComplete {
			t.Errorf("User %s should be incomplete", entry.ZoomEmail)
		}
	}
}

// TestFileUpdatePreservesFormatting tests that updates preserve comments and formatting
func TestFileUpdatePreservesFormatting(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")

	initialContent := `# Header comment
user1@zoom.com,user1@box.com,false
# Middle comment
user2@zoom.com,user2@box.com,false

# Final comment`

	err := os.WriteFile(userListFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	// Update user status
	err = usersFile.MarkUserComplete("user1@zoom.com")
	if err != nil {
		t.Fatalf("Failed to mark user complete: %v", err)
	}

	// Read file content
	content, err := os.ReadFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	contentStr := string(content)

	// Verify comments are preserved
	if !strings.Contains(contentStr, "# Header comment") {
		t.Error("Header comment was not preserved")
	}
	if !strings.Contains(contentStr, "# Middle comment") {
		t.Error("Middle comment was not preserved")
	}
	if !strings.Contains(contentStr, "# Final comment") {
		t.Error("Final comment was not preserved")
	}

	// Verify user1 is marked complete
	if !strings.Contains(contentStr, "user1@zoom.com,user1@box.com,true") {
		t.Error("User1 was not updated to complete status")
	}
}

// TestConcurrentFileUpdates tests concurrent updates to user status
func TestConcurrentFileUpdates(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")

	// Create file with 10 users
	content := ""
	for i := 1; i <= 10; i++ {
		content += fmt.Sprintf("user%d@zoom.com,user%d@box.com,false\n", i, i)
	}

	err := os.WriteFile(userListFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	// Perform concurrent updates
	done := make(chan bool, 10)
	errors := make(chan error, 10)

	for i := 1; i <= 10; i++ {
		go func(userNum int) {
			email := fmt.Sprintf("user%d@zoom.com", userNum)
			err := usersFile.MarkUserComplete(email)
			if err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Wait for all updates to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent update error: %v", err)
	}

	// Reload and verify all users are complete
	usersFile2, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to reload users file: %v", err)
	}

	incompleteUsers := usersFile2.GetIncompleteUsers()
	if len(incompleteUsers) != 0 {
		t.Errorf("Expected 0 incomplete users after concurrent updates, got %d", len(incompleteUsers))
	}
}

// TestInvalidUserUpdate tests error handling for updating non-existent users
func TestInvalidUserUpdate(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")

	fileContent := `user1@zoom.com,user1@box.com,false`

	err := os.WriteFile(userListFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	// Try to update non-existent user
	err = usersFile.MarkUserComplete("nonexistent@zoom.com")
	if err == nil {
		t.Error("Expected error when updating non-existent user")
	}
}

// TestEdgeCasesFileLoading tests edge cases in file loading
func TestEdgeCasesFileLoading(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		expectedCount int
		description   string
	}{
		{
			name:          "empty lines between entries",
			fileContent:   "user1@zoom.com\n\n\nuser2@zoom.com\n\n",
			expectedCount: 2,
			description:   "Should skip multiple empty lines",
		},
		{
			name:          "whitespace only lines",
			fileContent:   "user1@zoom.com\n   \n\t\nuser2@zoom.com",
			expectedCount: 2,
			description:   "Should skip whitespace-only lines",
		},
		{
			name:          "trailing comma with no value",
			fileContent:   "user1@zoom.com,\nuser2@zoom.com,,",
			expectedCount: 2,
			description:   "Should handle trailing commas",
		},
		{
			name:          "extra columns ignored",
			fileContent:   "user1@zoom.com,user1@box.com,false,extra,data",
			expectedCount: 0,
			description:   "Should reject lines with more than 3 columns",
		},
		{
			name:          "mixed valid and invalid emails",
			fileContent:   "valid@example.com\ninvalid@\n@invalid.com\nvalid2@example.com",
			expectedCount: 2,
			description:   "Should skip invalid emails but process valid ones",
		},
		{
			name:          "unicode in emails",
			fileContent:   "user@例え.com\nvalid@example.com",
			expectedCount: 1,
			description:   "Should reject non-ASCII emails",
		},
		{
			name:          "very long email",
			fileContent:   strings.Repeat("a", 310) + "@example.com\nvalid@example.com",
			expectedCount: 1,
			description:   "Should reject emails over 320 characters",
		},
		{
			name:          "email exactly 320 chars",
			fileContent:   strings.Repeat("a", 308) + "@example.com\nvalid@example.com",
			expectedCount: 2,
			description:   "Should accept emails at exactly 320 characters",
		},
		{
			name:          "duplicate users",
			fileContent:   "user@example.com,user@box.com,false\nuser@example.com,user@box.com,true",
			expectedCount: 2,
			description:   "Should allow duplicate users (last one may override)",
		},
		{
			name:          "spaces around email",
			fileContent:   "  user@example.com  ,  user@box.com  ,  true  ",
			expectedCount: 1,
			description:   "Should trim whitespace around values",
		},
		{
			name:          "invalid boolean values",
			fileContent:   "user1@example.com,user1@box.com,maybe\nuser2@example.com,user2@box.com,invalid",
			expectedCount: 2,
			description:   "Invalid boolean values should default to false",
		},
		{
			name:          "empty file",
			fileContent:   "",
			expectedCount: 0,
			description:   "Empty file should result in no entries",
		},
		{
			name:          "only comments",
			fileContent:   "# Comment 1\n# Comment 2\n# Comment 3",
			expectedCount: 0,
			description:   "File with only comments should have no entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			userListFile := filepath.Join(tempDir, "active_users.txt")

			err := os.WriteFile(userListFile, []byte(tt.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			usersFile, err := LoadActiveUsersFile(userListFile)
			if err != nil {
				t.Fatalf("Failed to load users file: %v", err)
			}

			if len(usersFile.Entries) != tt.expectedCount {
				t.Errorf("%s: expected %d entries, got %d", tt.description, tt.expectedCount, len(usersFile.Entries))
			}
		})
	}
}

// TestEdgeCasesUpdateOperations tests edge cases in update operations
func TestEdgeCasesUpdateOperations(t *testing.T) {
	t.Run("update same user multiple times", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := "user@example.com,user@box.com,false"

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		// Update multiple times
		for i := 0; i < 5; i++ {
			err = usersFile.UpdateUserStatus("user@example.com", i%2 == 0)
			if err != nil {
				t.Fatalf("Failed to update user status (iteration %d): %v", i, err)
			}
		}

		// Reload and verify final state
		usersFile2, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to reload users file: %v", err)
		}

		if len(usersFile2.Entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(usersFile2.Entries))
		}

		// i=4: 4%2==0 is true, so final state should be true
		if !usersFile2.Entries[0].UploadComplete {
			t.Error("Expected final state to be true (4 % 2 == 0)")
		}
	})

	t.Run("update with empty email", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := "user@example.com,user@box.com,false"

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		err = usersFile.UpdateUserStatus("", true)
		if err == nil {
			t.Error("Expected error when updating with empty email")
		}
	})

	t.Run("update read-only directory", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := "user@example.com,user@box.com,false"

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		// Make directory read-only (prevents temp file creation)
		err = os.Chmod(tempDir, 0555)
		if err != nil {
			t.Fatalf("Failed to change directory permissions: %v", err)
		}
		defer os.Chmod(tempDir, 0755) // Restore permissions for cleanup

		err = usersFile.MarkUserComplete("user@example.com")
		if err == nil {
			t.Error("Expected error when directory is read-only")
		}
	})

	t.Run("update when original file deleted", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := "user@example.com,user@box.com,false"

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		// Delete original file
		err = os.Remove(userListFile)
		if err != nil {
			t.Fatalf("Failed to remove file: %v", err)
		}

		err = usersFile.MarkUserComplete("user@example.com")
		if err == nil {
			t.Error("Expected error when original file is deleted")
		}
	})
}

// TestEdgeCasesParseBool tests edge cases in boolean parsing
func TestEdgeCasesParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Positive cases
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"1", true},
		{" true ", true},
		{"\ttrue\t", true},

		// Negative cases
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"no", false},
		{"No", false},
		{"NO", false},
		{"0", false},
		{" false ", false},

		// Invalid values default to false
		{"maybe", false},
		{"2", false},
		{"-1", false},
		{"t", false},
		{"f", false},
		{"y", false},
		{"n", false},
		{"", false},
		{"   ", false},
		{"truee", false},
		{"falsee", false},
		{"yes!", false},
		{"no!", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("parseBool('%s')", tt.input), func(t *testing.T) {
			result := parseBool(tt.input)
			if result != tt.expected {
				t.Errorf("parseBool(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestEdgeCasesGetIncompleteUsers tests edge cases for GetIncompleteUsers
func TestEdgeCasesGetIncompleteUsers(t *testing.T) {
	t.Run("all users complete", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := `user1@example.com,user1@box.com,true
user2@example.com,user2@box.com,true
user3@example.com,user3@box.com,true`

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		incomplete := usersFile.GetIncompleteUsers()
		if len(incomplete) != 0 {
			t.Errorf("Expected 0 incomplete users, got %d", len(incomplete))
		}
	})

	t.Run("all users incomplete", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := `user1@example.com,user1@box.com,false
user2@example.com,user2@box.com,false
user3@example.com,user3@box.com,false`

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		incomplete := usersFile.GetIncompleteUsers()
		if len(incomplete) != 3 {
			t.Errorf("Expected 3 incomplete users, got %d", len(incomplete))
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tempDir := t.TempDir()
		userListFile := filepath.Join(tempDir, "active_users.txt")
		fileContent := ""

		err := os.WriteFile(userListFile, []byte(fileContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		usersFile, err := LoadActiveUsersFile(userListFile)
		if err != nil {
			t.Fatalf("Failed to load users file: %v", err)
		}

		incomplete := usersFile.GetIncompleteUsers()
		if len(incomplete) != 0 {
			t.Errorf("Expected 0 incomplete users from empty file, got %d", len(incomplete))
		}
	})
}

// TestEdgeCasesFileNotFound tests negative case when file doesn't exist
func TestEdgeCasesFileNotFound(t *testing.T) {
	nonExistentFile := "/tmp/nonexistent_users_file_12345.txt"

	_, err := LoadActiveUsersFile(nonExistentFile)
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}
}

// TestEdgeCasesMalformedLines tests various malformed line formats
func TestEdgeCasesMalformedLines(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		expectedCount int
	}{
		{
			name:          "comma at start",
			fileContent:   ",user@example.com,false",
			expectedCount: 0,
		},
		{
			name:          "multiple commas in a row",
			fileContent:   "user1@example.com,,,false",
			expectedCount: 0,
		},
		{
			name:          "special characters in email",
			fileContent:   "user!@#$@example.com",
			expectedCount: 0,
		},
		{
			name:          "email with spaces",
			fileContent:   "user name@example.com",
			expectedCount: 0,
		},
		{
			name:          "incomplete email domain",
			fileContent:   "user@example",
			expectedCount: 0,
		},
		{
			name:          "mixed case boolean with typo",
			fileContent:   "user@example.com,user@box.com,Tru",
			expectedCount: 1, // Should parse but default to false
		},
		{
			name:          "tab separated instead of comma",
			fileContent:   "user@example.com\tuser@box.com\tfalse",
			expectedCount: 0, // Should fail as we expect commas
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			userListFile := filepath.Join(tempDir, "active_users.txt")

			err := os.WriteFile(userListFile, []byte(tt.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			usersFile, err := LoadActiveUsersFile(userListFile)
			if err != nil {
				t.Fatalf("Failed to load users file: %v", err)
			}

			if len(usersFile.Entries) != tt.expectedCount {
				t.Errorf("Expected %d entries, got %d", tt.expectedCount, len(usersFile.Entries))
			}
		})
	}
}

// TestEdgeCasesLineNumberTracking tests that line numbers are correctly tracked
func TestEdgeCasesLineNumberTracking(t *testing.T) {
	tempDir := t.TempDir()
	userListFile := filepath.Join(tempDir, "active_users.txt")
	fileContent := `# Line 1: Comment
user1@example.com,user1@box.com,false

# Line 4: Another comment
user2@example.com,user2@box.com,true
# Line 6: Comment
user3@example.com,user3@box.com,false`

	err := os.WriteFile(userListFile, []byte(fileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	usersFile, err := LoadActiveUsersFile(userListFile)
	if err != nil {
		t.Fatalf("Failed to load users file: %v", err)
	}

	// Verify line numbers are correct
	expectedLineNumbers := map[string]int{
		"user1@example.com": 2,
		"user2@example.com": 5,
		"user3@example.com": 7,
	}

	for _, entry := range usersFile.Entries {
		expectedLine, exists := expectedLineNumbers[entry.ZoomEmail]
		if !exists {
			t.Errorf("Unexpected user: %s", entry.ZoomEmail)
			continue
		}

		if entry.LineNumber != expectedLine {
			t.Errorf("User %s: expected line number %d, got %d",
				entry.ZoomEmail, expectedLine, entry.LineNumber)
		}
	}
}