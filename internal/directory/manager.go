// Package directory provides directory structure management for zoom-to-box
package directory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/users"
)

// DirectoryManager defines the interface for directory structure operations
type DirectoryManager interface {
	GenerateDirectory(userEmail string, meetingDate time.Time) (*DirectoryResult, error)
	GetStats() DirectoryStats
}

// DirectoryConfig holds configuration for the directory manager
type DirectoryConfig struct {
	BaseDirectory string // Base directory path for all downloads
	CreateDirs    bool   // Whether to create directories if they don't exist
}

// DirectoryResult represents the result of directory generation
type DirectoryResult struct {
	FullPath      string // Complete path to the directory
	UserDirectory string // Sanitized user directory name (email prefix)
	Year          string // Year component (YYYY)
	Month         string // Month component (MM)
	Day           string // Day component (DD)
	BasePath      string // Base directory path
	RelativePath  string // Relative path from base directory
}

// DirectoryStats provides statistics about directory operations
type DirectoryStats struct {
	DirectoriesCreated int       // Number of directories created
	BaseDirectory      string    // Base directory path
	LastCreated        time.Time // When the last directory was created
}

// directoryManagerImpl implements the DirectoryManager interface
type directoryManagerImpl struct {
	config            DirectoryConfig
	activeUserManager users.ActiveUserManager
	stats             DirectoryStats
	mutex             sync.RWMutex
}

// Email validation regex - same as users package
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9._-]+\.[a-zA-Z]{2,}$`)

// NewDirectoryManager creates a new directory manager with the given configuration
func NewDirectoryManager(config DirectoryConfig, activeUserManager users.ActiveUserManager) DirectoryManager {
	return &directoryManagerImpl{
		config:            config,
		activeUserManager: activeUserManager,
		stats: DirectoryStats{
			DirectoriesCreated: 0,
			BaseDirectory:      config.BaseDirectory,
		},
		mutex: sync.RWMutex{},
	}
}

// GenerateDirectory generates a directory structure based on user email and meeting date
func (dm *directoryManagerImpl) GenerateDirectory(userEmail string, meetingDate time.Time) (*DirectoryResult, error) {
	// Validate user email
	if userEmail == "" {
		return nil, fmt.Errorf("user email cannot be empty")
	}

	if !isValidEmail(userEmail) {
		return nil, fmt.Errorf("invalid email format: %s", userEmail)
	}

	// Check if user is active (if active user filtering is enabled)
	if !dm.activeUserManager.IsUserActive(userEmail) {
		return nil, fmt.Errorf("user not in active users list: %s", userEmail)
	}

	// Validate base directory
	if dm.config.BaseDirectory == "" {
		return nil, fmt.Errorf("base directory cannot be empty")
	}

	// Sanitize user email for directory name
	userDir := sanitizeEmailForDirectory(userEmail)
	
	// Convert meeting date to UTC for consistent directory structure
	utcDate := meetingDate.UTC()
	
	// Generate date components
	year := utcDate.Format("2006")
	month := utcDate.Format("01")
	day := utcDate.Format("02")
	
	// Build directory path: <base>/<user>/<year>/<month>/<day>
	relativePath := filepath.Join(userDir, year, month, day)
	fullPath := filepath.Join(dm.config.BaseDirectory, relativePath)
	
	// Create directory if requested
	if dm.config.CreateDirs {
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", fullPath, err)
		}
		
		// Update stats
		dm.mutex.Lock()
		dm.stats.DirectoriesCreated++
		dm.stats.LastCreated = time.Now()
		dm.mutex.Unlock()
	}
	
	return &DirectoryResult{
		FullPath:      fullPath,
		UserDirectory: userDir,
		Year:          year,
		Month:         month,
		Day:           day,
		BasePath:      dm.config.BaseDirectory,
		RelativePath:  relativePath,
	}, nil
}

// GetStats returns statistics about directory operations
func (dm *directoryManagerImpl) GetStats() DirectoryStats {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	return dm.stats
}

// sanitizeEmailForDirectory extracts the username portion from an email address
// and sanitizes it for use as a directory name
func sanitizeEmailForDirectory(email string) string {
	// Split email at @ symbol and take the first part
	parts := strings.Split(email, "@")
	if len(parts) < 2 {
		return email // Return as-is if no @ found (shouldn't happen with validation)
	}
	
	username := parts[0]
	
	// The username part should already be valid for directory names
	// Email validation ensures it contains only: a-zA-Z0-9._%+-
	// These are all valid filesystem characters on most systems
	
	return username
}

// isValidEmail performs basic email validation
func isValidEmail(email string) bool {
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