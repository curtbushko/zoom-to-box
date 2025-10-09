// Package directory provides directory structure management for zoom-to-box
package directory

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/email"
	"github.com/curtbushko/zoom-to-box/internal/filename"
	"github.com/curtbushko/zoom-to-box/internal/users"
	"github.com/curtbushko/zoom-to-box/internal/zoom"
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

// GenerateFilePath creates a complete file path with sanitized filename for a recording
func (dr *DirectoryResult) GenerateFilePath(recording zoom.Recording, fileType string, sanitizer filename.FileSanitizer) string {
	filename := sanitizer.GenerateFilename(recording, fileType)
	return filepath.Join(dr.FullPath, filename)
}

// GenerateRelativeFilePath creates a relative file path from the base directory
func (dr *DirectoryResult) GenerateRelativeFilePath(recording zoom.Recording, fileType string, sanitizer filename.FileSanitizer) string {
	filename := sanitizer.GenerateFilename(recording, fileType)
	return filepath.Join(dr.RelativePath, filename)
}

// GenerateFilename generates just the sanitized filename for a recording
func (dr *DirectoryResult) GenerateFilename(recording zoom.Recording, fileType string, sanitizer filename.FileSanitizer) string {
	return sanitizer.GenerateFilename(recording, fileType)
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
}


// NewDirectoryManager creates a new directory manager with the given configuration
func NewDirectoryManager(config DirectoryConfig, activeUserManager users.ActiveUserManager) DirectoryManager {
	return &directoryManagerImpl{
		config:            config,
		activeUserManager: activeUserManager,
		stats: DirectoryStats{
			DirectoriesCreated: 0,
			BaseDirectory:      config.BaseDirectory,
		},
	}
}

// GenerateDirectory generates a directory structure based on user email and meeting date
// userEmail should be the Zoom email, but the directory will be created using the Box email
func (dm *directoryManagerImpl) GenerateDirectory(userEmail string, meetingDate time.Time) (*DirectoryResult, error) {
	// Validate user email (this is the Zoom email)
	if userEmail == "" {
		return nil, fmt.Errorf("user email cannot be empty")
	}

	if !email.IsValidEmail(userEmail) {
		return nil, fmt.Errorf("invalid email format: %s", userEmail)
	}

	// Check if user is active (if active user filtering is enabled)
	if !dm.activeUserManager.IsUserActive(userEmail) {
		return nil, fmt.Errorf("user not in active users list: %s", userEmail)
	}

	// Get user mapping to find the Box email for directory creation
	var boxEmail string
	if mapping, exists := dm.activeUserManager.GetUserMapping(userEmail); exists {
		// Use Box email from mapping
		boxEmail = mapping.BoxEmail
	} else {
		// Fallback to Zoom email if no mapping exists
		boxEmail = userEmail
	}

	// Validate base directory
	if dm.config.BaseDirectory == "" {
		return nil, fmt.Errorf("base directory cannot be empty")
	}

	// Sanitize Box email for directory name (use Box email for folder structure)
	userDir := email.ExtractUsername(boxEmail)
	
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
		dm.stats.DirectoriesCreated++
		dm.stats.LastCreated = time.Now()
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
	return dm.stats
}

