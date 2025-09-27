// Package users provides active user list management for zoom-to-box
package users

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// UserEmailMapping represents a mapping between Zoom and Box emails
type UserEmailMapping struct {
	ZoomEmail string
	BoxEmail  string
}

// ActiveUserManager defines the interface for active user list operations
type ActiveUserManager interface {
	IsUserActive(email string) bool
	GetActiveUsers() []string
	GetUserMapping(zoomEmail string) (*UserEmailMapping, bool)
	GetAllMappings() []UserEmailMapping
	GetStats() UserStats
	Reload() error
	Close() error
}

// ActiveUserConfig holds configuration for the active user manager
type ActiveUserConfig struct {
	FilePath      string // Path to the active users file (empty disables filtering)
	CaseSensitive bool   // Whether email comparison should be case sensitive
	WatchFile     bool   // Whether to watch file for changes
}

// UserStats provides statistics about the active user list
type UserStats struct {
	TotalUsers    int       // Total number of active users
	LastUpdated   time.Time // When the list was last updated
	FilePath      string    // Path to the user list file
	FileSize      int64     // Size of the user list file
	IsWatching    bool      // Whether file watching is enabled
}

// activeUserManagerImpl implements the ActiveUserManager interface
type activeUserManagerImpl struct {
	config      ActiveUserConfig
	users       map[string]bool                // Set of active users (by Zoom email)
	userList    []string                       // Ordered list of Zoom emails for GetActiveUsers
	mappings    map[string]*UserEmailMapping   // Map from Zoom email to full mapping
	allMappings []UserEmailMapping             // Ordered list of all mappings
	mutex       sync.RWMutex                   // Protects concurrent access
	watcher     *fsnotify.Watcher
	stopWatch   chan struct{}
	stats       UserStats
}

// Email validation regex (basic validation) - allows underscores in domain
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9._-]+\.[a-zA-Z]{2,}$`)

// NewActiveUserManager creates a new active user manager
func NewActiveUserManager(config ActiveUserConfig) (ActiveUserManager, error) {
	manager := &activeUserManagerImpl{
		config:      config,
		users:       make(map[string]bool),
		userList:    make([]string, 0),
		mappings:    make(map[string]*UserEmailMapping),
		allMappings: make([]UserEmailMapping, 0),
		stopWatch:   make(chan struct{}),
		stats: UserStats{
			FilePath:   config.FilePath,
			IsWatching: config.WatchFile,
		},
	}

	// If no file path is provided, disable filtering (all users are active)
	if config.FilePath == "" {
		return manager, nil
	}

	// Load initial user list
	if err := manager.loadUserList(); err != nil {
		return nil, fmt.Errorf("failed to load initial user list: %w", err)
	}

	// Set up file watching if enabled
	if config.WatchFile {
		if err := manager.setupFileWatcher(); err != nil {
			return nil, fmt.Errorf("failed to setup file watcher: %w", err)
		}
	}

	return manager, nil
}

// IsUserActive checks if a user email is in the active user list
func (m *activeUserManagerImpl) IsUserActive(email string) bool {
	// If no file path is configured, all users are considered active
	if m.config.FilePath == "" {
		return true
	}

	// Normalize email case if case-insensitive
	checkEmail := email
	if !m.config.CaseSensitive {
		checkEmail = strings.ToLower(email)
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.users[checkEmail]
}

// GetActiveUsers returns a copy of the active user list (Zoom emails)
func (m *activeUserManagerImpl) GetActiveUsers() []string {
	// If no file path is configured, return empty list
	if m.config.FilePath == "" {
		return []string{}
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Return a copy to prevent external modification
	result := make([]string, len(m.userList))
	copy(result, m.userList)
	return result
}

// GetUserMapping returns the email mapping for a given Zoom email
func (m *activeUserManagerImpl) GetUserMapping(zoomEmail string) (*UserEmailMapping, bool) {
	// If no file path is configured, return nil
	if m.config.FilePath == "" {
		return nil, false
	}

	// Normalize email case if case-insensitive
	checkEmail := zoomEmail
	if !m.config.CaseSensitive {
		checkEmail = strings.ToLower(zoomEmail)
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	mapping, exists := m.mappings[checkEmail]
	if !exists {
		return nil, false
	}
	
	// Return a copy to prevent external modification
	return &UserEmailMapping{
		ZoomEmail: mapping.ZoomEmail,
		BoxEmail:  mapping.BoxEmail,
	}, true
}

// GetAllMappings returns a copy of all email mappings
func (m *activeUserManagerImpl) GetAllMappings() []UserEmailMapping {
	// If no file path is configured, return empty list
	if m.config.FilePath == "" {
		return []UserEmailMapping{}
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Return a copy to prevent external modification
	result := make([]UserEmailMapping, len(m.allMappings))
	copy(result, m.allMappings)
	return result
}

// GetStats returns statistics about the active user list
func (m *activeUserManagerImpl) GetStats() UserStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.stats
}

// Reload reloads the user list from the file
func (m *activeUserManagerImpl) Reload() error {
	if m.config.FilePath == "" {
		return nil // No file to reload
	}
	return m.loadUserList()
}

// Close closes the manager and cleans up resources
func (m *activeUserManagerImpl) Close() error {
	// Stop file watcher if running
	if m.config.WatchFile && m.watcher != nil {
		close(m.stopWatch)
		return m.watcher.Close()
	}
	return nil
}

// loadUserList loads the user list from the configured file
func (m *activeUserManagerImpl) loadUserList() error {
	file, err := os.Open(m.config.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open user list file: %w", err)
	}
	defer file.Close()

	// Get file info for stats
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Prepare new user data
	newUsers := make(map[string]bool)
	newUserList := make([]string, 0)
	newMappings := make(map[string]*UserEmailMapping)
	newAllMappings := make([]UserEmailMapping, 0)
	
	// Read file line by line
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		var zoomEmail, boxEmail string
		
		// Check if line contains comma separation for email mapping
		if strings.Contains(line, ",") {
			parts := strings.Split(line, ",")
			if len(parts) != 2 {
				// Skip malformed lines
				continue
			}
			
			zoomEmail = strings.TrimSpace(parts[0])
			boxEmail = strings.TrimSpace(parts[1])
			
			// Validate both emails
			if !isValidEmail(zoomEmail) || !isValidEmail(boxEmail) {
				// Skip invalid email mappings
				continue
			}
		} else {
			// Single email - use same for both Zoom and Box
			if !isValidEmail(line) {
				// Skip invalid emails
				continue
			}
			zoomEmail = line
			boxEmail = line
		}
		
		// Normalize case if case-insensitive
		normalizedZoomEmail := zoomEmail
		if !m.config.CaseSensitive {
			normalizedZoomEmail = strings.ToLower(zoomEmail)
		}
		
		// Add to set (prevents duplicates)
		if !newUsers[normalizedZoomEmail] {
			newUsers[normalizedZoomEmail] = true
			newUserList = append(newUserList, normalizedZoomEmail)
			
			// Create mapping
			mapping := &UserEmailMapping{
				ZoomEmail: zoomEmail, // Keep original case for display
				BoxEmail:  boxEmail,  // Keep original case for Box operations
			}
			newMappings[normalizedZoomEmail] = mapping
			newAllMappings = append(newAllMappings, *mapping)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading user list file: %w", err)
	}

	// Update data structures atomically
	m.mutex.Lock()
	m.users = newUsers
	m.userList = newUserList
	m.mappings = newMappings
	m.allMappings = newAllMappings
	m.stats.TotalUsers = len(newUserList)
	m.stats.LastUpdated = time.Now()
	m.stats.FileSize = fileInfo.Size()
	m.mutex.Unlock()

	return nil
}

// setupFileWatcher sets up file system watching for the user list file
func (m *activeUserManagerImpl) setupFileWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	err = watcher.Add(m.config.FilePath)
	if err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch file: %w", err)
	}

	m.watcher = watcher

	// Start watching in a separate goroutine
	go m.watchFileChanges()

	return nil
}

// watchFileChanges handles file system events for the user list file
func (m *activeUserManagerImpl) watchFileChanges() {
	defer func() {
		if m.watcher != nil {
			m.watcher.Close()
		}
	}()

	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			
			// Handle file write/modify events
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				// Small delay to ensure file write is complete
				time.Sleep(10 * time.Millisecond)
				
				// Reload user list
				if err := m.loadUserList(); err != nil {
					// Could add logging here for reload failures
					continue
				}
			}
			
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			// Could add logging here for watcher errors
			_ = err
			
		case <-m.stopWatch:
			return
		}
	}
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