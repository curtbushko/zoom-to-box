# zoom-to-box Implementation Plan

This plan provides a comprehensive roadmap for implementing the Zoom cloud recording downloader CLI tool. Each feature includes specific implementation details, test scenarios, and verification steps.

**Note: Mark features as complete with ✅ when fully implemented, tested, and verified.**

## Phase 1: Project Foundation & API Client

### Feature 1.1: Go Module Setup and Dependencies ✅ COMPLETED
- [x] Initialize Go module structure with proper versioning
- [x] Add required dependencies: cobra, http clients, YAML parsing, logging libraries
- [x] Set up project directory structure following Go conventions
**Tests:**
- [x] Verify module initialization and dependency resolution
- [x] Test import paths and module versioning
- [x] Validate project structure conventions

**Verification Command:** `go mod verify && go build .`

### Feature 1.2: Zoom API Data Structures ✅ COMPLETED
- [x] Define Go structs based on zoom-openapi.json Cloud Recording schemas
- [x] Implement proper JSON marshaling/unmarshaling tags
- [x] Handle optional fields and different data types correctly
**Core Structures:**
```go
type Recording struct {
    UUID         string          `json:"uuid"`
    ID           int64           `json:"id"`
    AccountID    string          `json:"account_id"`
    HostID       string          `json:"host_id"`
    Topic        string          `json:"topic"`
    Type         int             `json:"type"`
    StartTime    time.Time       `json:"start_time"`
    Duration     int             `json:"duration"`
    TotalSize    int64           `json:"total_size"`
    RecordingFiles []RecordingFile `json:"recording_files"`
}

type RecordingFile struct {
    ID           string    `json:"id"`
    MeetingID    string    `json:"meeting_id"`
    RecordingStart time.Time `json:"recording_start"`
    RecordingEnd time.Time   `json:"recording_end"`
    FileType     string    `json:"file_type"`
    FileSize     int64     `json:"file_size"`
    DownloadURL  string    `json:"download_url"`
    Status       string    `json:"status"`
}

type ListRecordingsResponse struct {
    From          string      `json:"from"`
    To            string      `json:"to"`
    PageCount     int         `json:"page_count"`
    PageSize      int         `json:"page_size"`
    TotalRecords  int         `json:"total_records"`
    NextPageToken string      `json:"next_page_token"`
    Meetings      []Recording `json:"meetings"`
}
```

**Tests:**
- [x] JSON marshaling/unmarshaling roundtrip tests
- [x] Validate struct tags match OpenAPI specification
- [x] Test handling of null/empty fields
- [x] Verify date/time parsing from different formats

**Mock Data:**
```json
{
  "from": "2024-01-01T00:00:00Z",
  "to": "2024-01-31T23:59:59Z",
  "page_count": 1,
  "page_size": 30,
  "total_records": 2,
  "meetings": [
    {
      "uuid": "4444AAAiAAAAAiAiAiiAii==",
      "id": 123456789,
      "account_id": "account123",
      "host_id": "host123",
      "topic": "Test Meeting Recording",
      "type": 2,
      "start_time": "2024-01-15T10:00:00Z",
      "duration": 3600,
      "total_size": 1048576,
      "recording_files": [
        {
          "id": "rec123",
          "meeting_id": "4444AAAiAAAAAiAiAiiAii==",
          "recording_start": "2024-01-15T10:00:00Z",
          "recording_end": "2024-01-15T11:00:00Z",
          "file_type": "MP4",
          "file_size": 1048576,
          "download_url": "https://api.zoom.us/v2/accounts/account123/recordings/rec123/download",
          "status": "completed"
        }
      ]
    }
  ]
}
```

**Implementation Summary:**
- ✅ Created `/internal/zoom/models.go` with complete data structures
- ✅ Created comprehensive test suite in `/internal/zoom/models_test.go`
- ✅ All structs match OpenAPI specification exactly
- ✅ Supports all file types (MP4, M4A, TRANSCRIPT, CHAT, CC, etc.)
- ✅ Handles multiple date/time formats (RFC3339, milliseconds, timezones)
- ✅ Comprehensive edge case testing (large files, null fields, various meeting types)
- ✅ 100% test coverage for JSON marshaling/unmarshaling
- ✅ Build verified: `make build && make test && make vet` all pass

**Verification Commands:**
```bash
go test ./internal/zoom -v    # Run data structure tests
make build                    # Build complete application
make test                     # Run all tests
make vet                      # Run static analysis
```

### Feature 1.3: YAML Configuration Management ✅ COMPLETED
- [x] Implement YAML configuration file parser
- [x] Define configuration structure with validation
- [x] Support both Zoom and Box settings
- [x] Handle configuration file loading and error reporting
**Configuration File Structure (config.yaml):**
```yaml
zoom:
  account_id: "your_zoom_account_id"
  client_id: "your_zoom_client_id"
  client_secret: "your_zoom_client_secret"
  base_url: "https://api.zoom.us/v2"
  # Server-to-Server OAuth - no access_token or refresh_token needed

box:
  enabled: true
  credentials_file: "/path/to/box_credentials.json"
  folder_id: "your_box_folder_id"

download:
  output_dir: "./downloads"
  concurrent_limit: 3
  retry_attempts: 3
  timeout_seconds: 300

logging:
  level: "info"  # debug, info, warn, error
  file: "./zoom-downloader.log"
  console: true
  json_format: false

active_users:
  file: "./active_users.txt"
  check_enabled: true
```

**Tests:**
- [x] Test YAML configuration parsing and validation
- [x] Test configuration file loading from different paths
- [x] Verify default value handling
- [x] Test configuration error scenarios

**Implementation Summary:**
- ✅ Created `/internal/config/config.go` with complete configuration management
- ✅ Created comprehensive test suite in `/internal/config/config_test.go`
- ✅ Supports all configuration sections (Zoom, Box, Download, Logging, ActiveUsers)
- ✅ Full YAML parsing with gopkg.in/yaml.v3
- ✅ Environment variable overrides for sensitive values
- ✅ Comprehensive validation with clear error messages
- ✅ Default values for all optional settings
- ✅ Helper methods (TimeoutDuration) for convenient value access
- ✅ Build verified: All tests pass, builds successfully
- ✅ Example configuration file: `config.example.yaml`

**Verification Commands:**
```bash
go test ./internal/config -v  # Run configuration tests
make build                    # Build complete application
make test                     # Run all tests
make vet                      # Run static analysis
```

### Feature 1.4: Server-to-Server OAuth Authentication Client ✅ COMPLETED
- [x] Implement Server-to-Server OAuth authentication using account credentials
- [x] Handle JWT token generation and Bearer token authentication
- [x] Support account-level access for all users and recordings
- [x] Load authentication settings from YAML configuration

**Zoom Server-to-Server OAuth App:**
- **Authentication Method**: Server-to-Server OAuth (not regular OAuth)
- **Access Level**: Account-level access to all users and recordings
- **Token Type**: JWT-based authentication generating Bearer tokens
- **No User Consent Required**: Direct API access using account credentials

**Required Server-to-Server OAuth Scopes:**
- `recording:read` - Access to view and download cloud recordings across the account
- `user:read` - Access to read user information for user ID resolution
- `meeting:read` - Access to read meeting information and metadata

**Authentication Flow:**
1. Generate JWT token using Account ID, Client ID, and Client Secret
2. Exchange JWT for Bearer access token via `/oauth/token` endpoint
3. Use Bearer token for API requests with automatic refresh

**Server-to-Server vs Regular OAuth:**
- **Server-to-Server**: Account-level access, no user interaction required
- **Regular OAuth**: User-level access, requires user consent flow
- **Recommended**: Server-to-Server for bulk recording downloads

**Tests:**
- [x] Mock Server-to-Server OAuth token generation
- [x] Test JWT creation and Bearer token exchange
- [x] Verify proper header construction with Bearer tokens
- [x] Test error handling for invalid credentials
- [x] Test account-level scope validation
- [x] Verify automatic token refresh mechanisms

**Implementation Summary:**
- ✅ Created `/internal/zoom/auth.go` with complete Server-to-Server OAuth implementation
- ✅ Created comprehensive test suite in `/internal/zoom/auth_test.go`
- ✅ JWT token generation using HMAC-SHA256 signing with proper claims
- ✅ Automatic Bearer token exchange via `/oauth/token` endpoint
- ✅ Token caching with automatic refresh (5-minute expiry buffer)
- ✅ Thread-safe token management with read/write mutexes
- ✅ Interface-driven design (Authenticator interface) for testability
- ✅ AuthenticatedClient wrapper for automatic header injection
- ✅ Comprehensive error handling with typed AuthError
- ✅ Scope validation for required permissions
- ✅ Integration with configuration system
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **JWT Generation**: Complete Server-to-Server OAuth flow with proper claims
- **Token Caching**: Automatic refresh with thread-safe concurrent access
- **Error Handling**: Detailed error types for troubleshooting
- **Scope Validation**: Ensures tokens have required permissions
- **Interface Design**: Easy mocking and testing
- **Bearer Authentication**: Automatic header injection for API requests

**Verification Commands:**
```bash
go test ./internal/zoom -v -run "Test.*Auth"  # Run authentication tests
make build                                    # Build complete application
make test                                     # Run all tests
make vet                                      # Run static analysis
```

### Feature 1.5: HTTP Client with Retry Logic ✅ COMPLETED
- [x] Create configurable HTTP client with timeout settings
- [x] Implement exponential backoff for rate limiting
- [x] Handle Zoom API-specific error responses
- [x] Support download URL redirection
**Tests:**
- [x] Test retry logic with simulated failures
- [x] Verify timeout handling
- [x] Test rate limit response handling (HTTP 429)
- [x] Validate redirect following for download URLs

**Implementation Summary:**
- ✅ Created `/internal/zoom/httpclient.go` with complete HTTP retry client
- ✅ Created comprehensive test suite in `/internal/zoom/httpclient_test.go`
- ✅ Configurable timeouts integrated with DownloadConfig
- ✅ Exponential backoff with jitter (500ms-5s range)
- ✅ Smart retry logic for 429, 5xx status codes
- ✅ Zoom API-specific error parsing and handling
- ✅ Retry-After header support (seconds and HTTP date formats)
- ✅ Automatic redirect following with configurable limits
- ✅ Thread-safe client with proper resource management
- ✅ Integration with existing configuration system
- ✅ Helper methods: GetWithRetry, PostWithRetry, CheckConnectivity
- ✅ All quality gates passed: 6 test functions, 20+ scenarios

**Key Features:**
- **Smart Retries**: Only retries appropriate errors (network, 5xx, 429)
- **Exponential Backoff**: 2^attempt with ±25% jitter to avoid thundering herd
- **Rate Limit Handling**: Respects Retry-After headers from Zoom API
- **Redirect Support**: Follows download URL redirections automatically
- **Error Handling**: Typed errors (ZoomAPIError, HTTPError) with detailed context
- **Configuration**: Integrates with YAML config timeout and retry settings
- **Resource Management**: Proper connection handling and cleanup

**Verification Commands:**
```bash
go test ./internal/zoom -v -run "HTTP|Backoff|Rate|Redirect|Timeout"  # HTTP client tests
make build                                                             # Build application
make test                                                              # Run all tests
make vet                                                               # Static analysis
```

### Feature 1.6: Cloud Recording API Client ✅ COMPLETED
- [x] Implement `ListUserRecordings()` method for `/users/{userId}/recordings`
- [x] Implement `GetMeetingRecordings()` method for `/meetings/{meetingId}/recordings`
- [x] Handle pagination with next_page_token
- [x] Support date range filtering and query parameters
**Key Methods:**
```go
type ZoomClient struct {
    httpClient *AuthenticatedRetryClient
    baseURL    string
}

// CloudRecordingClient interface for testability
type CloudRecordingClient interface {
    ListUserRecordings(ctx context.Context, userID string, params ListRecordingsParams) (*ListRecordingsResponse, error)
    GetMeetingRecordings(ctx context.Context, meetingID string) (*Recording, error)
    DownloadRecordingFile(ctx context.Context, downloadURL string, writer io.Writer) error
}

func (c *ZoomClient) ListUserRecordings(ctx context.Context, userID string, params ListRecordingsParams) (*ListRecordingsResponse, error)
func (c *ZoomClient) GetMeetingRecordings(ctx context.Context, meetingID string) (*Recording, error)
func (c *ZoomClient) DownloadRecordingFile(ctx context.Context, downloadURL string, writer io.Writer) error
func (c *ZoomClient) GetAllUserRecordings(ctx context.Context, userID string, params ListRecordingsParams) ([]*Recording, error)
```

**Tests:**
- [x] Mock API endpoints with test server
- [x] Test pagination handling with multiple pages
- [x] Verify query parameter encoding
- [x] Test meeting UUID encoding for special characters
- [x] Mock different response scenarios (empty, error, large datasets)

**Implementation Summary:**
- ✅ Created `/internal/zoom/client.go` with complete Cloud Recording API client
- ✅ Created comprehensive test suite in `/internal/zoom/client_test.go`
- ✅ Interface-driven design with CloudRecordingClient interface for testability
- ✅ ListUserRecordings with full parameter support (dates, pagination, filters)
- ✅ GetMeetingRecordings with proper UUID encoding for special characters
- ✅ DownloadRecordingFile with redirect support via HTTP client
- ✅ GetAllUserRecordings utility method for automatic pagination
- ✅ Default page size handling (30 records per page)
- ✅ URL encoding for special characters in user IDs and meeting UUIDs
- ✅ Integration with AuthenticatedRetryClient for automatic OAuth and retry logic
- ✅ Comprehensive test coverage with mock OAuth server handling
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Interface Design**: CloudRecordingClient interface enables easy mocking for tests
- **Parameter Support**: Full support for date ranges, page sizes, tokens, and filters
- **URL Encoding**: Proper encoding of special characters in user IDs and meeting UUIDs
- **Pagination**: Automatic pagination handling with GetAllUserRecordings helper
- **Error Handling**: Proper error propagation from HTTP and authentication layers
- **Integration**: Seamless integration with retry logic and OAuth authentication
- **Testability**: Comprehensive test suite with mock servers and OAuth handling

**Verification Commands:**
```bash
go test ./internal/zoom -v                         # Run all tests including client
go test ./internal/zoom/client_test.go -v          # Run client tests specifically
make build                                         # Build application
make test                                          # Run all tests
make vet                                           # Static analysis
```

## Phase 2: Core Download Engine

### Feature 2.1: Download Manager with Resume Support ✅ COMPLETED
- [x] Support partial downloads using HTTP Range headers
- [x] Track download progress and allow resumption
- [x] Handle network interruptions gracefully
- [x] Implement concurrent download limits
**Tests:**
- [x] Test resume functionality after interruption
- [x] Verify Range header usage for partial downloads
- [x] Test concurrent download limiting
- [x] Mock network failure scenarios

**Implementation Summary:**
- ✅ Created `/internal/download/manager.go` with complete download manager implementation
- ✅ Created comprehensive test suite in `/internal/download/manager_test.go`
- ✅ Interface-driven design with DownloadManager interface for testability
- ✅ HTTP Range header support for resuming partial downloads
- ✅ Progress tracking with callbacks for real-time updates
- ✅ Concurrent download limiting using semaphore pattern
- ✅ Network interruption handling with retry logic and exponential backoff
- ✅ Configurable chunk sizes, retry attempts, and timeout settings
- ✅ Thread-safe operation with proper mutex usage
- ✅ Download state management with status tracking
- ✅ File system operations with directory creation and file handling
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Resume Support**: Automatic detection of partial downloads and Range header usage
- **Progress Tracking**: Real-time progress callbacks with speed and ETA calculations
- **Concurrent Limiting**: Configurable semaphore-based concurrent download limits
- **Retry Logic**: Exponential backoff with configurable retry attempts and delays
- **Error Handling**: Comprehensive error types and graceful failure handling
- **State Management**: Download state tracking (queued, downloading, completed, failed)
- **Cancellation**: Context-based cancellation support for individual downloads
- **Thread Safety**: Safe for concurrent access with proper synchronization

**Verification Commands:**
```bash
go test ./internal/download -v                     # Run download manager tests
make build                                         # Build application
make test                                          # Run all tests
make vet                                           # Static analysis
```

### Feature 2.2: Active User List Management ✅ COMPLETED
- [x] Implement active user list file reader
- [x] Support user filtering based on email addresses
- [x] Handle user list file updates and reloading
- [x] Provide user existence validation for downloads
**Active User List File Format (active_users.txt):**
```
john.doe@company.com
jane.smith@company.com
admin@company.com
# Lines starting with # are comments
# Empty lines are ignored
user@example.org
```

**Integration Points:**
- Check user eligibility before creating directory structures
- Skip recordings for users not in active list when enabled
- Log when recordings are skipped due to inactive users
- Support real-time updates to user list during execution

**Implementation Summary:**
- ✅ Created `/internal/users/manager.go` with complete ActiveUserManager implementation
- ✅ Created comprehensive test suite in `/internal/users/manager_test.go`
- ✅ Interface-driven design with ActiveUserManager interface for testability
- ✅ File parsing with email validation and comment/empty line filtering
- ✅ Real-time file watching using fsnotify for automatic updates
- ✅ Thread-safe operations with read/write mutexes for concurrent access
- ✅ Case-sensitive and case-insensitive email matching support
- ✅ Email validation with regex patterns and length limits (320 char max)
- ✅ Malformed file handling with graceful error recovery
- ✅ User statistics with file size, last updated, and user count tracking
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **File Format Support**: Comments (#), empty lines, email validation
- **Real-time Updates**: fsnotify-based file watching with 10ms debounce
- **Thread Safety**: Safe for concurrent access with proper synchronization
- **Email Validation**: RFC-compliant validation with reasonable limits
- **Configuration**: Case sensitivity, file watching, and file path options
- **Error Handling**: Graceful handling of malformed files and missing files
- **Statistics**: User count, file size, last updated tracking

**Tests:**
- [x] Test user list file parsing and validation
- [x] Test user existence checking functionality
- [x] Test handling of malformed user list files
- [x] Test dynamic user list updates during runtime

**Verification Commands:**
```bash
go test ./internal/users -v                       # Run active user manager tests
make build                                         # Build application
make test                                          # Run all tests
make vet                                           # Static analysis
```

### Feature 2.3: Directory Structure Generator ✅ COMPLETED
- [x] Create directory structure: `<user_account>/<year>/<month>/<day>`
- [x] Generate based on meeting start time or recording date
- [x] Handle timezone conversion appropriately
- [x] Support configurable base directory
- [x] Integrate with active user list checking
**Directory Structure Example:**
```
downloads/
├── john.doe/
│   └── 2024/
│       └── 01/
│           └── 15/
│               ├── team-standup-meeting-1000.mp4
│               ├── team-standup-meeting-1000.json
│               └── weekly-review-call-1430.mp4
```

**Implementation Summary:**
- ✅ Created `/internal/directory/manager.go` with complete DirectoryManager implementation
- ✅ Created comprehensive test suite in `/internal/directory/manager_test.go`
- ✅ Interface-driven design with DirectoryManager interface for testability
- ✅ Date-based directory structure: `<user>/<year>/<month>/<day>`
- ✅ Timezone conversion to UTC for consistent directory structure
- ✅ Email sanitization extracting username portion (@domain.com removal)
- ✅ Integration with ActiveUserManager for user filtering
- ✅ Directory creation with configurable base directory
- ✅ Thread-safe operations with statistics tracking
- ✅ Email validation and error handling
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Directory Structure**: Automatic creation of `<base>/<user>/<YYYY>/<MM>/<DD>` paths
- **Timezone Handling**: Converts all dates to UTC for consistent directory structure
- **Email Sanitization**: Extracts username from email (john.doe@company.com → john.doe)
- **Active User Integration**: Checks user eligibility before creating directories  
- **Statistics**: Tracks directories created, last creation time, and base directory
- **Thread Safety**: Safe for concurrent access with proper synchronization
- **Validation**: Email format validation and error handling for edge cases

**Tests:**
- [x] Test directory creation for various date formats
- [x] Verify timezone handling
- [x] Test invalid characters in user accounts
- [x] Test email address sanitization (removing @domain.com from usernames)
- [x] Validate nested directory creation
- [x] Test integration with active user list checking

**Verification Commands:**
```bash
go test ./internal/directory -v                   # Run directory manager tests
make build                                         # Build application
make test                                          # Run all tests
make vet                                           # Static analysis
```

### Feature 2.4: Filename Sanitization ✅ COMPLETED
- [x] Convert meeting topic to lowercase
- [x] Replace spaces with dashes
- [x] Remove or replace invalid filesystem characters
- [x] Handle Unicode characters appropriately
- [x] Sanitize email addresses by removing @domain.com portion for directory names
- [x] Add time component to ensure filename uniqueness
- [x] Add file extensions (.mp4, .json, etc.)
**Filename Format:** `<sanitized-topic>-<HHMM>.<extension>`

**Sanitization Rules:**
- `"Weekly Team Meeting"` (started 10:30) → `"weekly-team-meeting-1030"`
- `"Q4 Planning: Budget & Goals"` (started 14:15) → `"q4-planning-budget-goals-1415"`
- `"Test/Meeting (Final)"` (started 09:45) → `"test-meeting-final-0945"`

**Time Component:**
- Use meeting start time from the recording metadata
- Format as HHMM (24-hour format to the minute)
- Ensures uniqueness even for meetings with identical topics on the same day
- Consistent sorting when listing files chronologically

**Tests:**
- [x] Test various special characters and Unicode
- [x] Verify case conversion
- [x] Test extremely long topic names
- [x] Validate file extension handling
- [x] Test email address sanitization (john.doe@company.com → john.doe)
- [x] Test time component formatting and uniqueness (HHMM format)
- [x] Verify consistent filename generation for duplicate topics
- [x] Test timezone handling in time component

**Implementation Summary:**
- ✅ Created `/internal/filename/sanitizer.go` with complete FileSanitizer implementation
- ✅ Created comprehensive test suite in `/internal/filename/sanitizer_test.go`
- ✅ Interface-driven design with FileSanitizer interface for testability
- ✅ Unicode normalization using golang.org/x/text for diacritic removal
- ✅ Robust special character handling with proper word boundary preservation
- ✅ Configurable options (max length, default topic)
- ✅ Support for all file types (MP4, M4A, JSON, TRANSCRIPT, CHAT, CC, CSV, etc.)
- ✅ Time formatting in original timezone context for meeting accuracy
- ✅ Integration with existing DirectoryManager via DirectoryResult methods
- ✅ Comprehensive edge case testing (emojis, long titles, empty topics, etc.)
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Topic Sanitization**: Converts topics to filesystem-safe lowercase with dashes
- **Unicode Handling**: Normalizes accented characters and removes emojis
- **Time Component**: HHMM format preserving original timezone context
- **File Extensions**: Automatic extension mapping for all Zoom file types
- **Interface Design**: Easy integration and mocking for tests
- **Configuration**: Customizable max length and default topic options
- **Directory Integration**: Seamless integration with existing directory manager

**Verification Commands:**
```bash
go test ./internal/filename -v                    # Run filename sanitizer tests
go test ./internal/directory -v                   # Run directory integration tests
make build                                        # Build complete application
make test                                         # Run all tests
make vet                                          # Run static analysis
```

### Feature 2.5: Comprehensive Logging System ✅ COMPLETED
- [x] Implement structured logging with configurable levels
- [x] Support both file and console output with different formats
- [x] Log all major operations including API calls, downloads, and errors
- [x] Provide request/response logging for debugging
**Logging Features:**
- Multiple log levels: DEBUG, INFO, WARN, ERROR
- Configurable output formats: plain text and JSON
- File rotation with size limits
- Contextual logging with request IDs
- Performance metrics logging
- User action logging (downloads, skips, errors)

**Log Examples:**
```
2024-01-15T10:00:00Z [INFO] Starting zoom recording downloader
2024-01-15T10:00:01Z [INFO] Loading configuration from config.yaml
2024-01-15T10:00:02Z [INFO] Loading active users from active_users.txt (45 users)
2024-01-15T10:00:03Z [DEBUG] API Request: GET /users/john.doe@company.com/recordings
2024-01-15T10:00:04Z [INFO] Found 12 recordings for user john.doe@company.com
2024-01-15T10:00:05Z [WARN] User jane.smith@company.com not in active users list, skipping
2024-01-15T10:00:06Z [INFO] Downloading: team-standup-meeting-1000.mp4 (1.2MB)
2024-01-15T10:00:10Z [INFO] Download completed: team-standup-meeting-1000.mp4
2024-01-15T10:00:11Z [ERROR] Download failed: network-error.mp4 - connection timeout
```

**Tests:**
- [x] Test logging configuration and initialization
- [x] Verify log level filtering
- [x] Test file rotation and size limits
- [x] Test JSON and plain text formatting
- [x] Test contextual logging with request IDs

**Implementation Summary:**
- ✅ Created `/internal/logging/logger.go` with complete Logger implementation
- ✅ Created comprehensive test suite in `/internal/logging/logger_test.go`
- ✅ Interface-driven design with Logger interface for testability
- ✅ Structured logging with configurable levels (DEBUG, INFO, WARN, ERROR)
- ✅ Dual output support (console and file) with format selection
- ✅ JSON and plain text format support with automatic field flattening
- ✅ Contextual logging with request ID propagation via context.Context
- ✅ Specialized logging methods for user actions, performance metrics, and API calls
- ✅ Thread-safe concurrent logging with mutex protection
- ✅ Global logger with package-level convenience functions
- ✅ Request ID generation and context utilities
- ✅ File handle management with proper cleanup
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Structured Logging**: Configurable levels with JSON/text output formats
- **Contextual Logging**: Request ID tracking via context.Context throughout request lifecycle
- **Specialized Methods**: UserAction, Performance, APIRequest/Response logging with structured data
- **Thread Safety**: Safe for concurrent access with proper synchronization
- **Global Access**: Package-level convenience functions for easy integration
- **File Management**: Automatic file creation and proper cleanup
- **API Integration**: Built-in support for API request/response logging with header sanitization
- **Performance Tracking**: Duration, bytes processed, and success metrics

**Usage Examples:**
```go
// Initialize logging
err := logging.InitializeLogging(config.Logging)

// Basic logging
logging.Info("Starting application")
logging.Error("Failed to connect: %v", err)

// Contextual logging
ctx := logging.WithRequestID(context.Background(), "req-123")
logging.InfoWithContext(ctx, "Processing request")

// Specialized logging
logging.LogUserAction("download_start", "john.doe@company.com", map[string]interface{}{
    "file_name": "meeting.mp4",
    "file_size": 1048576,
})

logging.LogPerformance(logging.PerformanceMetrics{
    Operation: "download_file",
    Duration: time.Second * 5,
    BytesProcessed: 1048576,
    Success: true,
})
```

**Verification Commands:**
```bash
go test ./internal/logging -v                     # Run logging system tests
make build                                        # Build complete application
make test                                         # Run all tests
make vet                                          # Run static analysis
```

### Feature 2.6: Download Status File System ✅ COMPLETED
- [x] JSON-based status file tracking download states
- [x] Support resume, completed, failed, and pending states
- [x] Track file checksums for integrity verification
- [x] Handle concurrent access safely
**Status File Structure:**
```json
{
  "version": "1.0",
  "last_updated": "2024-01-15T14:30:00Z",
  "downloads": {
    "rec123": {
      "status": "completed",
      "file_path": "john.doe@company.com/2024/01/15/team-standup-meeting-1025.mp4",
      "file_size": 1048576,
      "downloaded_size": 1048576,
      "checksum": "sha256:abc123...",
      "last_attempt": "2024-01-15T14:25:00Z",
      "metadata_downloaded": true
    },
    "rec124": {
      "status": "pending",
      "file_path": "john.doe@company.com/2024/01/15/weekly-review-call-1420.mp4",
      "file_size": 2097152,
      "downloaded_size": 524288,
      "last_attempt": "2024-01-15T14:20:00Z"
    }
  }
}
```

**Tests:**
- [x] Test status file creation and updates
- [x] Verify concurrent access handling
- [x] Test recovery from corrupted status files
- [x] Validate checksum verification

**Implementation Summary:**
- ✅ Created `/internal/download/status.go` with complete StatusTracker implementation
- ✅ Created comprehensive test suite in `/internal/download/status_test.go`
- ✅ Interface-driven design with StatusTracker interface for testability
- ✅ JSON-based status file with atomic write operations (temp file + rename)
- ✅ Support for all download states: pending, downloading, completed, failed, paused
- ✅ SHA256 checksum calculation and verification for file integrity
- ✅ Thread-safe concurrent access with read/write mutexes
- ✅ Automatic recovery from corrupted status files
- ✅ Integration helpers for seamless DownloadManager integration
- ✅ Resume logic with intelligent state management
- ✅ Status filtering and querying capabilities
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Status Tracking**: Complete lifecycle tracking with pending, downloading, completed, failed, paused states
- **Resume Support**: Intelligent resume detection with stale download cleanup
- **Integrity Verification**: SHA256 checksum calculation and verification
- **Concurrent Safety**: Thread-safe operations with proper mutex synchronization
- **Atomic Updates**: Safe file operations using temporary files and atomic rename
- **Error Recovery**: Graceful handling of corrupted status files with automatic recovery
- **Integration Ready**: Helper functions for seamless DownloadManager integration
- **Query Capabilities**: Status filtering, incomplete downloads, and summary statistics

**Advanced Functionality:**
- **StatusTrackerWithManager**: Integrated wrapper for automatic status tracking during downloads
- **Progress Integration**: Automatic status updates from ProgressUpdate and DownloadResult
- **Metadata Management**: Rich metadata storage for tracking download context
- **Retry Tracking**: Automatic retry count tracking and error message storage
- **Timestamp Management**: Comprehensive timing data (start, last attempt, completed)
- **File Validation**: Integrity checks with size and checksum verification

**Usage Examples:**
```go
// Create status tracker
tracker, err := NewStatusTracker("downloads_status.json")

// Track download
entry := CreateDownloadEntry(downloadRequest, StatusPending)
tracker.UpdateDownloadStatus("rec123", entry)

// Check if should resume
if ShouldResumeDownload(entry) {
    offset := GetResumeOffset(entry)
    // Resume download from offset
}

// Verify integrity
if IsIntegrityValid(entry) && entry.Checksum != "" {
    valid, err := VerifyFileChecksum(entry.FilePath, entry.Checksum)
}

// Integrated tracking
trackerWithManager, err := NewStatusTrackerWithManager("status.json", downloadManager)
result, err := trackerWithManager.StartDownloadWithTracking(ctx, request, progressCallback)
```

**Verification Commands:**
```bash
go test ./internal/download -v                    # Run status system tests
make build                                        # Build complete application
make test                                         # Run all tests
make vet                                          # Run static analysis
```

### Feature 2.7: Retry Logic and Error Handling ✅ COMPLETED
- [x] Exponential backoff for transient failures
- [x] Configurable maximum retry attempts
- [x] Different strategies for different error types
- [x] Comprehensive error logging and reporting
**Tests:**
- [x] Test retry behavior with various HTTP error codes
- [x] Verify backoff timing accuracy
- [x] Test maximum retry limits
- [x] Mock different failure scenarios

**Implementation Summary:**
- ✅ Created `/internal/download/retry.go` with comprehensive retry logic implementation
- ✅ Created comprehensive test suite in `/internal/download/retry_test.go`
- ✅ Interface-driven design with RetryStrategy and RetryExecutor interfaces for testability
- ✅ Exponential backoff with configurable jitter to prevent thundering herd effects
- ✅ Error classification system distinguishing network, timeout, server, rate limit, auth, and client errors
- ✅ Circuit breaker pattern for fault tolerance with configurable failure thresholds
- ✅ Error-specific retry delays (network: 1s, timeout: 2s, server: 1s, rate limit: 60s)
- ✅ Configurable retry strategies with validation for all parameters
- ✅ Comprehensive metrics tracking for retry operations (attempts, duration, success rate)
- ✅ Context-aware cancellation support for graceful shutdown
- ✅ Thread-safe concurrent access with proper mutex protection
- ✅ Integration with existing zoom package HTTPError and ZoomAPIError types
- ✅ All quality gates passed: Tests, build, vet

**Key Features:**
- **Error Classification**: Automatic categorization of errors into retryable types
- **Exponential Backoff**: Configurable base delay, multiplier, and maximum delay
- **Jitter Support**: ±25% random jitter to prevent synchronized retry storms
- **Circuit Breaker**: Fail-fast behavior after threshold failures with recovery timeout
- **Error-Specific Delays**: Different retry delays for different error types
- **Comprehensive Metrics**: Track attempts, duration, and success patterns
- **Configuration Validation**: Ensures retry parameters are sensible
- **Context Support**: Respects cancellation and deadlines
- **Interface Design**: Easy integration and mocking for tests

**Retry Configuration Options:**
- `MaxAttempts`: Maximum number of retry attempts (default: 3)
- `BaseDelay`: Initial delay before first retry (default: 500ms)
- `MaxDelay`: Maximum delay cap (default: 30s)
- `Multiplier`: Exponential backoff multiplier (default: 2.0)
- `Jitter`: Enable random jitter (default: true, ±25%)
- `RetryableErrors`: Which error types to retry (network, timeout, server, rate limit)
- `CircuitBreaker`: Enable circuit breaker pattern (default: true)
- `FailureThreshold`: Circuit breaker failure threshold (default: 5)
- `RecoveryTimeout`: Circuit breaker recovery time (default: 30s)

**Error Types and Default Retry Delays:**
- **Network Errors**: 1 second (connection failures, DNS issues)
- **Timeout Errors**: 2 seconds (request timeouts, deadline exceeded)
- **Server Errors**: 1 second (HTTP 5xx responses)
- **Rate Limit Errors**: 60 seconds (HTTP 429 responses)
- **Auth Errors**: No retry (HTTP 401/403 responses)
- **Client Errors**: No retry (HTTP 4xx except 429)

**Usage Examples:**
```go
// Create retry strategy with custom config
config := RetryConfig{
    MaxAttempts: 5,
    BaseDelay: 1 * time.Second,
    MaxDelay: 30 * time.Second,
    Multiplier: 2.0,
    Jitter: true,
    RetryableErrors: []ErrorType{ErrorTypeNetwork, ErrorTypeServer},
}
strategy := NewRetryStrategy(config)
executor := NewRetryExecutor(strategy)

// Execute operation with retry logic
err := executor.Execute(ctx, func() error {
    // Your operation here
    return someHTTPCall()
})

// Get metrics
metrics := executor.GetMetrics()
fmt.Printf("Attempts: %d, Duration: %v\n", metrics.TotalAttempts, metrics.TotalDuration)
```

**Verification Commands:**
```bash
go test ./internal/download -v -run TestRetry          # Run retry logic tests
go test ./internal/download -v                        # Run all download tests including retry
go build .                                            # Build complete application
go vet ./...                                          # Run static analysis
```

## Phase 3: CLI Interface & Commands

### Feature 3.1: Cobra CLI Application Structure
- [ ] Main command structure with subcommands
- [ ] Global flags for common options
- [ ] Proper help text and usage examples
- [ ] Version information display
**CLI Structure:**
```bash
zoom-to-box [flags]
zoom-to-box help
zoom-to-box version
```

**Global Flags:**
- `--config` - Configuration file path
- `--output-dir` - Base download directory
- `--verbose` - Verbose logging
- `--dry-run` - Show what would be downloaded without downloading

**Tests:**
- [ ] Test command parsing and flag handling
- [ ] Verify help text generation
- [ ] Test version display
- [ ] Validate flag combination handling

### Feature 3.2: Meta-only and Limit Flags
- [ ] `--meta-only` flag downloads only JSON metadata files
- [ ] `--limit N` flag limits processing to N recordings
- [ ] Proper flag validation and error messages
- [ ] Integration with download manager
**Usage Examples:**
```bash
zoom-to-box --meta-only --limit 10
zoom-to-box --limit 50 --output-dir ./recordings
```

**Tests:**
- [ ] Test meta-only download behavior
- [ ] Verify limit functionality with various values
- [ ] Test flag validation (negative numbers, zero, etc.)
- [ ] Test combination of flags

### Feature 3.3: Configuration File Help System
- [ ] Display required configuration file structure in help output
- [ ] Show example YAML configuration with descriptions
- [ ] Detect missing configuration and provide helpful messages
- [ ] Support for different authentication methods
**Help Output Example:**
```
Configuration File Structure (config.yaml):
  zoom:
    account_id: "your_zoom_account_id"       # Zoom Account ID from Server-to-Server OAuth app
    client_id: "your_zoom_client_id"         # Client ID from Server-to-Server OAuth app
    client_secret: "your_zoom_client_secret" # Client Secret from Server-to-Server OAuth app
    # Required scopes: recording:read, user:read, meeting:read
    # Uses Server-to-Server OAuth (account-level access, no user tokens needed)

  box:
    enabled: true                            # Enable Box uploads
    credentials_file: "/path/to/box_creds.json" # Path to Box credentials JSON file
    folder_id: "your_box_folder_id"           # Target folder ID for uploads
    # Credential file contains: developer_token, OAuth tokens, or JWT config

  active_users:
    file: "./active_users.txt"               # Path to active users list file
    check_enabled: true                      # Enable active user filtering

  logging:
    level: "info"                            # Log level: debug, info, warn, error
    file: "./zoom-downloader.log"            # Log file path
```

**Tests:**
- [ ] Test help text generation
- [ ] Verify configuration file detection
- [ ] Test different authentication scenarios
- [ ] Validate error messages for missing configuration

### Feature 3.4: Progress Reporting and Enhanced Logging Integration
- [ ] Real-time download progress bars with file logging
- [ ] Integration with comprehensive logging system from Feature 2.5
- [ ] Summary statistics with detailed log entries
- [ ] Progress updates written to log files
**Progress Display with Logging:**
```
Downloading recordings...
[████████████████████████████████████████] 100% | 15/15 recordings
└─ team-standup-meeting-1000.mp4: 1.2MB/1.2MB [100%] 2.5MB/s

Summary:
- Total recordings: 15
- Downloaded: 14
- Failed: 1  
- Skipped (already exists): 3
- Skipped (inactive users): 2
- Total size: 125.4MB
- Time elapsed: 2m 15s

All operations logged to: ./zoom-downloader.log
```

**Enhanced Logging Integration:**
- Progress updates written to log file with timestamps
- User filtering actions logged with reasons
- Error details logged with full context
- Performance metrics logged for analysis

**Tests:**
- [ ] Test progress bar display and updates
- [ ] Verify integration with logging system
- [ ] Test summary generation with user filtering
- [ ] Test log file updates during progress reporting

## Phase 4: Box Integration

### Feature 4.1: Box API Client
- [ ] Box API v2 client implementation
- [ ] OAuth 2.0 authentication with automatic token refresh
- [ ] Folder creation and file upload capabilities
- [ ] Proper error handling and retry logic
**Tests:**
- [ ] Mock Box API endpoints
- [ ] Test OAuth 2.0 authentication and token refresh
- [ ] Verify folder creation and navigation with OAuth tokens
- [ ] Test file upload with progress tracking
- [ ] Test API client behavior with expired tokens

### Feature 4.2: Box OAuth 2.0 Authentication
- [ ] Implement OAuth 2.0 authentication flow for Box API access
- [ ] Support access token and refresh token management from credential files
- [ ] Handle Box API OAuth scopes for file access
- [ ] Secure credential file loading and validation
- [ ] Automatic token refresh when access tokens expire
- [ ] Integration with existing authentication system

**Required Box API OAuth 2.0 Scopes:**
- `base_upload` - Upload files and create folders
- `base_write` - Edit files and folders stored in Box
- `base_explorer` - Access and modify files and folders
- `base_preview` - View files and folders stored in Box (optional for metadata)

**Authentication Method:**
- **OAuth 2.0**: Standard OAuth 2.0 flow with access tokens and refresh tokens
- **Token Management**: Automatic refresh of expired access tokens using refresh tokens
- **Credential Storage**: Secure storage of OAuth tokens in credential files

**Box OAuth 2.0 Credential File (box_credentials.json):**
```json
{
  "client_id": "your_box_oauth_client_id",
  "client_secret": "your_box_oauth_client_secret",
  "access_token": "your_oauth_access_token",
  "refresh_token": "your_oauth_refresh_token",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "base_explorer base_upload base_write"
}
```

**OAuth 2.0 Flow:**
1. User authorizes application via Box OAuth consent screen
2. Application receives authorization code
3. Exchange authorization code for access_token and refresh_token
4. Store tokens in credential file
5. Use access_token for API requests
6. Automatically refresh access_token using refresh_token when expired

**OAuth 2.0 Scope Requirements:**
- **base_upload**: Required for uploading files and creating folders
- **base_write**: Required for modifying file and folder metadata
- **base_explorer**: Required for navigating folder structure
- OAuth access token must include all required scopes during authorization

**Configuration Integration:**
- Load Box settings from YAML configuration
- Support for credential file containing OAuth 2.0 tokens
- Configurable credential file path and folder IDs
- Automatic token refresh and credential file updates
- Validation of OAuth token expiration and scope requirements

**Tests:**
- [ ] Test OAuth 2.0 credential file loading and validation
- [ ] Verify OAuth 2.0 scope handling and validation
- [ ] Test access token expiration and automatic refresh
- [ ] Test authentication error scenarios (invalid tokens, expired tokens)
- [ ] Mock OAuth 2.0 token refresh flow
- [ ] Test credential file updates after token refresh
- [ ] Verify folder access permissions with OAuth tokens
- [ ] Test invalid credential file handling
- [ ] Test OAuth 2.0 authorization flow simulation

### Feature 4.3: Cloud Upload with Status Tracking and Permission Management
- [ ] Upload downloaded files to Box
- [ ] Maintain directory structure in Drive
- [ ] Set appropriate permissions on uploaded files and folders
- [ ] Track upload status and permission status in status file system
- [ ] Support resume for interrupted uploads
**Enhanced Status File with Permission Tracking:**
```json
{
  "rec123": {
    "status": "completed",
    "file_path": "john.doe@company.com/2024/01/15/team-standup-meeting-1500.mp4",
    "downloaded": true,
    "video_owner": "john.doe@company.com",
    "box": {
      "uploaded": true,
      "file_id": "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms",
      "folder_id": "1FolderABC123DefGhi456JklMno789",
      "upload_date": "2024-01-15T15:00:00Z",
      "permissions_set": true,
      "permission_ids": [
        "permission_id_service_account",
        "permission_id_video_owner"
      ]
    }
  }
}
```

**Tests:**
- [ ] Test upload progress tracking
- [ ] Verify status file integration with permission tracking
- [ ] Test upload resume functionality
- [ ] Test permission setting during upload
- [ ] Verify permission status tracking
- [ ] Mock upload failure scenarios
- [ ] Mock permission setting failure scenarios

### Feature 4.4: Drive Folder Management with Permission Control
- [ ] Create folder structure matching local directory layout
- [ ] Handle existing folder detection
- [ ] Support shared drives and personal drives
- [ ] Implement granular permission management for files and folders
- [ ] Set video owner as the only user with access to their recordings

**Permission Management Strategy:**
- **User-Specific Folders**: Each user's folder (e.g., `john.doe@company.com/`) is only accessible by that user
- **File-Level Permissions**: Individual video files are only accessible by the original meeting host
- **Service Account Access**: Service account maintains management access for uploads and organization
- **Inheritance Control**: Child files and folders inherit restricted permissions from parent folders

**Permission Implementation:**
```go
type DrivePermission struct {
    UserEmail    string `json:"emailAddress"`
    Role         string `json:"role"`         // "reader", "writer", "owner"
    Type         string `json:"type"`         // "user", "group", "domain", "anyone"
    SendNotification bool `json:"sendNotificationEmail"`
}

// Example: Grant access only to video owner
permission := DrivePermission{
    UserEmail: "john.doe@company.com",
    Role:      "reader",
    Type:      "user", 
    SendNotification: false,
}
```

**Permission Levels:**
- **Video Owner**: Reader access to their own videos and metadata
- **Service Account**: Owner access for management operations
- **No Public Access**: Files are private by default
- **No Organization Access**: Files are not shared at domain level

**Tests:**
- [ ] Test folder creation with restricted permissions
- [ ] Verify user-specific access control
- [ ] Test permission inheritance from parent folders
- [ ] Test service account permission management
- [ ] Verify file-level permission setting
- [ ] Test permission error handling and validation
- [ ] Mock different permission scenarios and edge cases

## Phase 5: Testing & Documentation

### Feature 5.1: Comprehensive Unit Testing
- [ ] Unit tests for all core functionality
- [ ] Mock implementations for external APIs
- [ ] Test coverage reporting and validation
- [ ] Integration with CI/CD pipeline
**Test Organization:**
```
internal/
├── zoom/
│   ├── client_test.go
│   ├── auth_test.go
│   └── models_test.go
├── download/
│   ├── manager_test.go
│   ├── status_test.go
│   └── filesystem_test.go
└── googledrive/
    ├── client_test.go
    └── upload_test.go
```

**Mock Servers:**
- HTTP test server for Zoom API endpoints
- Mock Box API responses
- Configurable failure scenarios
- Realistic response data

**Tests:**
- [ ] Achieve >90% code coverage
- [ ] Test all error paths and edge cases
- [ ] Verify mock server accuracy
- [ ] Performance benchmarks for large files

### Feature 5.2: Integration Testing
- [ ] End-to-end testing with real API interactions
- [ ] Docker-based test environment
- [ ] Test data cleanup and isolation
- [ ] Automated test execution
**Integration Test Scenarios:**
- Complete download workflow with real Zoom data
- Box upload integration
- Error recovery and retry scenarios
- Large file handling and performance

**Tests:**
- [ ] Test with real Zoom API (rate-limited)
- [ ] Verify Box integration
- [ ] Test network interruption recovery
- [ ] Performance testing with large datasets


### Feature 5.3: Documentation and Examples
- [ ] Comprehensive README with setup instructions
- [ ] API documentation with examples
- [ ] Configuration guide for different scenarios
- [ ] Troubleshooting guide for common issues
**Documentation Structure:**
```
docs/
├── README.md
├── setup.md
├── configuration.md
├── troubleshooting.md
└── examples/
    ├── basic-usage.md
    ├── box-setup.md
    └── advanced-configuration.md
```

**Examples:**
- Basic download workflow
- Box integration setup
- Advanced configuration options
- Troubleshooting common issues

## Implementation Verification

### Testing Strategy
- [ ] Unit Tests: Test individual components in isolation
- [ ] Integration Tests: Test component interactions
- [ ] End-to-End Tests: Test complete workflows
- [ ] Performance Tests: Validate scalability and performance
- [ ] Mock Tests: Use controlled test data for reliable results
- [ ] Positive Tests: Verify correct functionality with valid inputs and expected scenarios
- [ ] Negative Tests: Test error handling, invalid inputs, edge cases, and failure scenarios to ensure robust error detection and graceful failure handling

### Verification Commands
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...

# Run integration tests (requires API credentials)
go test -tags=integration ./...

# Build and verify
go build . && ./zoom-to-box --help
```

### Success Criteria
- [ ] All tests pass with >90% coverage
- [ ] CLI help displays required yaml auth file settings
- [ ] Downloads create proper directory structure
- [ ] Metadata files contain complete recording information
- [ ] Resume functionality works after interruption
- [ ] Box integration uploads files correctly
- [ ] Error handling provides helpful messages
- [ ] Performance meets requirements for large files

This plan provides a comprehensive roadmap with specific, testable features that can be implemented incrementally while maintaining quality and reliability throughout the development process.
