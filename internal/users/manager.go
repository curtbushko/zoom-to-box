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

// UserEntry represents a user with upload tracking information
type UserEntry struct {
	ZoomEmail      string // Zoom account email
	BoxEmail       string // Box account email (may differ from Zoom email)
	UploadComplete bool   // Whether uploads for this user are complete
	LineNumber     int    // Original line number in file for updates
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
	users       map[string]bool              // Set of active users (by Zoom email)
	userList    []string                     // Ordered list of Zoom emails for GetActiveUsers
	mappings    map[string]*UserEmailMapping // Map from Zoom email to full mapping
	allMappings []UserEmailMapping           // Ordered list of all mappings
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

	return m.users[checkEmail]
}

// GetActiveUsers returns a copy of the active user list (Zoom emails)
func (m *activeUserManagerImpl) GetActiveUsers() []string {
	// If no file path is configured, return empty list
	if m.config.FilePath == "" {
		return []string{}
	}

	
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

	
	// Return a copy to prevent external modification
	result := make([]UserEmailMapping, len(m.allMappings))
	copy(result, m.allMappings)
	return result
}

// GetStats returns statistics about the active user list
func (m *activeUserManagerImpl) GetStats() UserStats {
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
	m.users = newUsers
	m.userList = newUserList
	m.mappings = newMappings
	m.allMappings = newAllMappings
	m.stats.TotalUsers = len(newUserList)
	m.stats.LastUpdated = time.Now()
	m.stats.FileSize = fileInfo.Size()

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

// ActiveUsersFile represents a file containing users with upload tracking
type ActiveUsersFile struct {
	FilePath string
	Entries  []UserEntry
	mu       sync.RWMutex
}

// LoadActiveUsersFile loads an active users file with upload tracking support
func LoadActiveUsersFile(filePath string) (*ActiveUsersFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open users file: %w", err)
	}
	defer file.Close()

	usersFile := &ActiveUsersFile{
		FilePath: filePath,
		Entries:  make([]UserEntry, 0),
	}

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry, err := parseUserEntry(line, lineNumber)
		if err != nil {
			// Skip malformed lines
			continue
		}

		usersFile.Entries = append(usersFile.Entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading users file: %w", err)
	}

	return usersFile, nil
}

// parseUserEntry parses a line from the users file into a UserEntry
func parseUserEntry(line string, lineNumber int) (UserEntry, error) {
	parts := strings.Split(line, ",")

	var zoomEmail, boxEmail string
	var uploadComplete bool

	switch len(parts) {
	case 1:
		// 1-column format: zoom_email (box_email defaults to zoom_email, upload_complete defaults to false)
		zoomEmail = strings.TrimSpace(parts[0])
		if !isValidEmail(zoomEmail) {
			return UserEntry{}, fmt.Errorf("invalid email: %s", zoomEmail)
		}
		boxEmail = zoomEmail
		uploadComplete = false

	case 2:
		// 2-column format: zoom_email,box_email (upload_complete defaults to false)
		zoomEmail = strings.TrimSpace(parts[0])
		boxEmail = strings.TrimSpace(parts[1])

		// If box_email is empty, use zoom_email
		if boxEmail == "" {
			boxEmail = zoomEmail
		}

		if !isValidEmail(zoomEmail) || !isValidEmail(boxEmail) {
			return UserEntry{}, fmt.Errorf("invalid email")
		}
		uploadComplete = false

	case 3:
		// 3-column format: zoom_email,box_email,upload_complete
		zoomEmail = strings.TrimSpace(parts[0])
		boxEmail = strings.TrimSpace(parts[1])
		uploadCompleteStr := strings.TrimSpace(parts[2])

		// If box_email is empty, use zoom_email
		if boxEmail == "" {
			boxEmail = zoomEmail
		}

		if !isValidEmail(zoomEmail) || !isValidEmail(boxEmail) {
			return UserEntry{}, fmt.Errorf("invalid email")
		}

		// Parse boolean value (supports true/false, yes/no, 1/0)
		uploadComplete = parseBool(uploadCompleteStr)

	default:
		return UserEntry{}, fmt.Errorf("invalid format: expected 1-3 columns")
	}

	return UserEntry{
		ZoomEmail:      zoomEmail,
		BoxEmail:       boxEmail,
		UploadComplete: uploadComplete,
		LineNumber:     lineNumber,
	}, nil
}

// parseBool parses a boolean value from string (case-insensitive)
// Supports: true/false, yes/no, 1/0
func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		return false // Default to false for unknown values
	}
}

// GetIncompleteUsers returns a list of users with incomplete uploads
func (f *ActiveUsersFile) GetIncompleteUsers() []UserEntry {
	f.mu.RLock()
	defer f.mu.RUnlock()

	incomplete := make([]UserEntry, 0)
	for _, entry := range f.Entries {
		if !entry.UploadComplete {
			incomplete = append(incomplete, entry)
		}
	}
	return incomplete
}

// UpdateUserStatus updates the upload completion status for a user
func (f *ActiveUsersFile) UpdateUserStatus(zoomEmail string, complete bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Find the user entry
	found := false
	for i := range f.Entries {
		if f.Entries[i].ZoomEmail == zoomEmail {
			f.Entries[i].UploadComplete = complete
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("user not found: %s", zoomEmail)
	}

	// Write updates to file atomically
	return f.writeToFileAtomic()
}

// MarkUserComplete marks a user's uploads as complete
func (f *ActiveUsersFile) MarkUserComplete(zoomEmail string) error {
	return f.UpdateUserStatus(zoomEmail, true)
}

// writeToFileAtomic writes the file content atomically using temp file + rename
func (f *ActiveUsersFile) writeToFileAtomic() error {
	// Create temporary file
	tempFile := f.FilePath + ".tmp"
	file, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Read original file to preserve comments and empty lines
	originalLines, err := readFileLines(f.FilePath)
	if err != nil {
		file.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to read original file: %w", err)
	}

	// Create a map of line numbers to updated entries
	updates := make(map[int]UserEntry)
	for _, entry := range f.Entries {
		updates[entry.LineNumber] = entry
	}

	// Write file with preserved comments and updated entries
	writer := bufio.NewWriter(file)
	lineNumber := 0

	for _, line := range originalLines {
		lineNumber++

		// Check if this line should be updated
		if entry, exists := updates[lineNumber]; exists {
			// Write updated entry
			_, err := writer.WriteString(fmt.Sprintf("%s,%s,%t\n",
				entry.ZoomEmail, entry.BoxEmail, entry.UploadComplete))
			if err != nil {
				file.Close()
				os.Remove(tempFile)
				return fmt.Errorf("failed to write entry: %w", err)
			}
		} else {
			// Preserve original line (comment or empty line)
			_, err := writer.WriteString(line + "\n")
			if err != nil {
				file.Close()
				os.Remove(tempFile)
				return fmt.Errorf("failed to write line: %w", err)
			}
		}
	}

	// Flush and close
	if err := writer.Flush(); err != nil {
		file.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, f.FilePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// readFileLines reads all lines from a file
func readFileLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}