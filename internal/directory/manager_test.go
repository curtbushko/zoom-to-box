package directory

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/users"
)

// TestDirectoryManager tests the complete directory structure generation functionality
func TestDirectoryManager(t *testing.T) {
	tests := []struct {
		name             string
		baseDir          string
		userEmail        string
		meetingDate      time.Time
		expectedPath     string
		expectedError    bool
		createDirs       bool
		activeUserConfig users.ActiveUserConfig
		userListContent  string
	}{
		{
			name:         "valid directory creation with UTC time",
			baseDir:      "downloads",
			userEmail:    "john.doe@company.com",
			meetingDate:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectedPath: "downloads/john.doe/2024/01/15",
			expectedError: false,
			createDirs:   true,
			activeUserConfig: users.ActiveUserConfig{
				FilePath: "",
			},
		},
		{
			name:         "directory creation with different user",
			baseDir:      "downloads",
			userEmail:    "jane.smith@company.com",
			meetingDate:  time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC),
			expectedPath: "downloads/jane.smith/2024/12/31",
			expectedError: false,
			createDirs:   true,
			activeUserConfig: users.ActiveUserConfig{
				FilePath: "",
			},
		},
		{
			name:         "directory creation with timezone conversion",
			baseDir:      "recordings",
			userEmail:    "admin@company.com",
			meetingDate:  time.Date(2024, 6, 15, 14, 30, 0, 0, time.FixedZone("EST", -5*3600)),
			expectedPath: "recordings/admin/2024/06/15",
			expectedError: false,
			createDirs:   true,
			activeUserConfig: users.ActiveUserConfig{
				FilePath: "",
			},
		},
		{
			name:         "user email with special characters",
			baseDir:      "downloads",
			userEmail:    "test.user+tag@sub-domain.example.co.uk",
			meetingDate:  time.Date(2024, 3, 1, 8, 0, 0, 0, time.UTC),
			expectedPath: "downloads/test.user+tag/2024/03/01",
			expectedError: false,
			createDirs:   true,
			activeUserConfig: users.ActiveUserConfig{
				FilePath: "",
			},
		},
		{
			name:         "empty user email",
			baseDir:      "downloads",
			userEmail:    "",
			meetingDate:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectedPath: "",
			expectedError: true,
			createDirs:   false,
			activeUserConfig: users.ActiveUserConfig{
				FilePath: "",
			},
		},
		{
			name:         "invalid user email format",
			baseDir:      "downloads",
			userEmail:    "invalid-email",
			meetingDate:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectedPath: "",
			expectedError: true,
			createDirs:   false,
			activeUserConfig: users.ActiveUserConfig{
				FilePath: "",
			},
		},
		{
			name:         "user not in active list",
			baseDir:      "downloads",
			userEmail:    "inactive.user@company.com",
			meetingDate:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectedPath: "",
			expectedError: true,
			createDirs:   false,
			userListContent: `active.user@company.com
john.doe@company.com
admin@company.com`,
			activeUserConfig: users.ActiveUserConfig{
				FilePath:      "", // Will be set during test
				CaseSensitive: false,
				WatchFile:     false,
			},
		},
		{
			name:         "user in active list",
			baseDir:      "downloads",
			userEmail:    "john.doe@company.com",
			meetingDate:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectedPath: "downloads/john.doe/2024/01/15",
			expectedError: false,
			createDirs:   true,
			userListContent: `active.user@company.com
john.doe@company.com
admin@company.com`,
			activeUserConfig: users.ActiveUserConfig{
				FilePath:      "", // Will be set during test
				CaseSensitive: false,
				WatchFile:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tempDir := t.TempDir()
			baseDir := filepath.Join(tempDir, tt.baseDir)
			
			// Setup active user manager
			var activeUserManager users.ActiveUserManager
			var err error
			
			if tt.userListContent != "" {
				// Create temporary active users file
				userListFile := filepath.Join(tempDir, "active_users.txt")
				err := os.WriteFile(userListFile, []byte(tt.userListContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create active users file: %v", err)
				}
				
				tt.activeUserConfig.FilePath = userListFile
				activeUserManager, err = users.NewActiveUserManager(tt.activeUserConfig)
				if err != nil {
					t.Fatalf("Failed to create active user manager: %v", err)
				}
				defer activeUserManager.Close()
			} else {
				activeUserManager, err = users.NewActiveUserManager(tt.activeUserConfig)
				if err != nil {
					t.Fatalf("Failed to create active user manager: %v", err)
				}
				defer activeUserManager.Close()
			}

			// Create directory manager
			config := DirectoryConfig{
				BaseDirectory: baseDir,
				CreateDirs:    tt.createDirs,
			}
			
			manager := NewDirectoryManager(config, activeUserManager)

			// Test directory generation
			result, err := manager.GenerateDirectory(tt.userEmail, tt.meetingDate)
			
			// Verify expectations
			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify generated path
			expectedFullPath := filepath.Join(tempDir, tt.expectedPath)
			if result.FullPath != expectedFullPath {
				t.Errorf("Expected path %s, got %s", expectedFullPath, result.FullPath)
			}

			// Verify directory structure
			if tt.createDirs {
				if _, err := os.Stat(result.FullPath); os.IsNotExist(err) {
					t.Errorf("Expected directory to be created: %s", result.FullPath)
				}
			}

			// Verify directory components
			expectedUser := sanitizeEmailForDirectory(tt.userEmail)
			if result.UserDirectory != expectedUser {
				t.Errorf("Expected user directory %s, got %s", expectedUser, result.UserDirectory)
			}

			expectedYear := tt.meetingDate.UTC().Format("2006")
			if result.Year != expectedYear {
				t.Errorf("Expected year %s, got %s", expectedYear, result.Year)
			}

			expectedMonth := tt.meetingDate.UTC().Format("01")
			if result.Month != expectedMonth {
				t.Errorf("Expected month %s, got %s", expectedMonth, result.Month)
			}

			expectedDay := tt.meetingDate.UTC().Format("02")
			if result.Day != expectedDay {
				t.Errorf("Expected day %s, got %s", expectedDay, result.Day)
			}
		})
	}
}

// TestEmailSanitization tests email address sanitization for directory names
func TestEmailSanitization(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{"simple email", "john.doe@company.com", "john.doe"},
		{"email with plus", "user+tag@example.org", "user+tag"},
		{"complex email", "test.user+project@sub-domain.company.co.uk", "test.user+project"},
		{"email with hyphens", "first-last@example-company.com", "first-last"},
		{"email with numbers", "user123@example456.com", "user123"},
		{"email with underscores", "first_last@example_domain.com", "first_last"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeEmailForDirectory(tt.email)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestTimezoneHandling tests timezone conversion for directory paths
func TestTimezoneHandling(t *testing.T) {
	tests := []struct {
		name          string
		inputTime     time.Time
		expectedYear  string
		expectedMonth string
		expectedDay   string
	}{
		{
			name:          "UTC time",
			inputTime:     time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
			expectedYear:  "2024",
			expectedMonth: "06",
			expectedDay:   "15",
		},
		{
			name:          "EST timezone",
			inputTime:     time.Date(2024, 6, 15, 22, 30, 0, 0, time.FixedZone("EST", -5*3600)),
			expectedYear:  "2024",
			expectedMonth: "06",
			expectedDay:   "16", // Should be next day in UTC
		},
		{
			name:          "PST timezone crossing year boundary",
			inputTime:     time.Date(2024, 1, 1, 2, 30, 0, 0, time.FixedZone("PST", -8*3600)),
			expectedYear:  "2024",
			expectedMonth: "01",
			expectedDay:   "01", // Should be same day in UTC (2:30 PST = 10:30 UTC)
		},
		{
			name:          "JST timezone",
			inputTime:     time.Date(2024, 6, 15, 2, 30, 0, 0, time.FixedZone("JST", 9*3600)),
			expectedYear:  "2024",
			expectedMonth: "06",
			expectedDay:   "14", // Should be previous day in UTC (2:30 JST = 17:30 UTC previous day)
		},
	}

	// Create temporary directory for test
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "downloads")
	
	// Create directory manager without active user filtering
	activeUserManager, err := users.NewActiveUserManager(users.ActiveUserConfig{FilePath: ""})
	if err != nil {
		t.Fatalf("Failed to create active user manager: %v", err)
	}
	defer activeUserManager.Close()

	config := DirectoryConfig{
		BaseDirectory: baseDir,
		CreateDirs:    false, // Don't create dirs for this test
	}
	manager := NewDirectoryManager(config, activeUserManager)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.GenerateDirectory("test@example.com", tt.inputTime)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Year != tt.expectedYear {
				t.Errorf("Expected year %s, got %s", tt.expectedYear, result.Year)
			}
			if result.Month != tt.expectedMonth {
				t.Errorf("Expected month %s, got %s", tt.expectedMonth, result.Month)
			}
			if result.Day != tt.expectedDay {
				t.Errorf("Expected day %s, got %s", tt.expectedDay, result.Day)
			}
		})
	}
}

// TestDirectoryCreation tests actual directory creation
func TestDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "downloads")
	
	// Create directory manager without active user filtering
	activeUserManager, err := users.NewActiveUserManager(users.ActiveUserConfig{FilePath: ""})
	if err != nil {
		t.Fatalf("Failed to create active user manager: %v", err)
	}
	defer activeUserManager.Close()

	config := DirectoryConfig{
		BaseDirectory: baseDir,
		CreateDirs:    true,
	}
	manager := NewDirectoryManager(config, activeUserManager)

	// Test directory creation
	testTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	result, err := manager.GenerateDirectory("test.user@company.com", testTime)
	if err != nil {
		t.Fatalf("Failed to generate directory: %v", err)
	}

	// Verify directory exists
	if _, err := os.Stat(result.FullPath); os.IsNotExist(err) {
		t.Errorf("Directory was not created: %s", result.FullPath)
	}

	// Verify directory structure
	expectedPath := filepath.Join(baseDir, "test.user", "2024", "06", "15")
	if result.FullPath != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, result.FullPath)
	}

	// Test creating same directory again (should not error)
	result2, err := manager.GenerateDirectory("test.user@company.com", testTime)
	if err != nil {
		t.Errorf("Failed to handle existing directory: %v", err)
	}
	if result2.FullPath != result.FullPath {
		t.Errorf("Paths should be identical for same input")
	}
}

// TestActiveUserIntegration tests integration with active user list
func TestActiveUserIntegration(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "downloads")
	
	// Create active users file
	userListContent := `john.doe@company.com
jane.smith@company.com
admin@company.com
# This is a comment
test.user@example.org`
	
	userListFile := filepath.Join(tempDir, "active_users.txt")
	err := os.WriteFile(userListFile, []byte(userListContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create active users file: %v", err)
	}

	// Create active user manager with file
	activeUserConfig := users.ActiveUserConfig{
		FilePath:      userListFile,
		CaseSensitive: false,
		WatchFile:     false,
	}
	activeUserManager, err := users.NewActiveUserManager(activeUserConfig)
	if err != nil {
		t.Fatalf("Failed to create active user manager: %v", err)
	}
	defer activeUserManager.Close()

	config := DirectoryConfig{
		BaseDirectory: baseDir,
		CreateDirs:    true,
	}
	manager := NewDirectoryManager(config, activeUserManager)

	testTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)

	// Test active user
	result, err := manager.GenerateDirectory("john.doe@company.com", testTime)
	if err != nil {
		t.Errorf("Expected active user to succeed: %v", err)
	}
	if result == nil {
		t.Error("Expected result for active user")
	}

	// Test inactive user
	_, err = manager.GenerateDirectory("inactive.user@company.com", testTime)
	if err == nil {
		t.Error("Expected error for inactive user")
	}

	// Test case insensitive matching
	result, err = manager.GenerateDirectory("JOHN.DOE@COMPANY.COM", testTime)
	if err != nil {
		t.Errorf("Expected case insensitive match to succeed: %v", err)
	}
	if result == nil {
		t.Error("Expected result for case insensitive match")
	}
}

// TestDirectoryManagerStats tests statistics functionality
func TestDirectoryManagerStats(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "downloads")
	
	// Create directory manager without active user filtering
	activeUserManager, err := users.NewActiveUserManager(users.ActiveUserConfig{FilePath: ""})
	if err != nil {
		t.Fatalf("Failed to create active user manager: %v", err)
	}
	defer activeUserManager.Close()

	config := DirectoryConfig{
		BaseDirectory: baseDir,
		CreateDirs:    true,
	}
	manager := NewDirectoryManager(config, activeUserManager)

	// Generate some directories
	testTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	_, err = manager.GenerateDirectory("user1@company.com", testTime)
	if err != nil {
		t.Fatalf("Failed to generate directory: %v", err)
	}

	_, err = manager.GenerateDirectory("user2@company.com", testTime)
	if err != nil {
		t.Fatalf("Failed to generate directory: %v", err)
	}

	// Get stats
	stats := manager.GetStats()
	if stats.DirectoriesCreated != 2 {
		t.Errorf("Expected 2 directories created, got %d", stats.DirectoriesCreated)
	}

	if stats.BaseDirectory != baseDir {
		t.Errorf("Expected base directory %s, got %s", baseDir, stats.BaseDirectory)
	}

	if stats.LastCreated.IsZero() {
		t.Error("Expected LastCreated to be set")
	}
}

// TestInvalidDirectoryPaths tests handling of invalid directory paths
func TestInvalidDirectoryPaths(t *testing.T) {
	tests := []struct {
		name           string
		baseDirectory  string
		expectedError  bool
	}{
		{"valid directory", "/tmp/test", false},
		{"relative directory", "downloads", false},
		{"empty directory", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create directory manager without active user filtering
			activeUserManager, err := users.NewActiveUserManager(users.ActiveUserConfig{FilePath: ""})
			if err != nil {
				t.Fatalf("Failed to create active user manager: %v", err)
			}
			defer activeUserManager.Close()

			config := DirectoryConfig{
				BaseDirectory: tt.baseDirectory,
				CreateDirs:    false,
			}
			manager := NewDirectoryManager(config, activeUserManager)

			testTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
			_, err = manager.GenerateDirectory("test@example.com", testTime)
			
			if tt.expectedError && err == nil {
				t.Error("Expected error but got none")
			} else if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}