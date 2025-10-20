// Package box provides Box API client functionality for zoom-to-box
package box

import (
	"fmt"
	"time"
)

// BoxClient defines the interface for Box API operations
type BoxClient interface {
	// Authentication
	RefreshToken() error
	IsAuthenticated() bool

	// User operations
	GetCurrentUser() (*User, error)
	GetUserByEmail(email string) (*User, error)

	// Folder operations
	CreateFolder(name string, parentID string) (*Folder, error)
	CreateFolderAsUser(name string, parentID string, userID string) (*Folder, error)
	GetFolder(folderID string) (*Folder, error)
	ListFolderItems(folderID string) (*FolderItems, error)
	ListFolderItemsAsUser(folderID string, userID string) (*FolderItems, error)
	FindZoomFolder() (string, error)
	FindFolderByName(parentID string, name string) (*Folder, error)
	FindZoomFolderByOwner(ownerEmail string) (*Folder, error)

	// File operations
	UploadFile(filePath string, parentFolderID string, fileName string) (*File, error)
	UploadFileWithProgress(filePath string, parentFolderID string, fileName string, progressCallback ProgressCallback) (*File, error)
	UploadFileAsUser(filePath string, parentFolderID string, fileName string, userID string, progressCallback ProgressCallback) (*File, error)
	GetFile(fileID string) (*File, error)
	DeleteFile(fileID string) error
	FindFileByName(folderID string, name string) (*File, error)

	// Chunked upload operations (for files >= 20MB)
	CreateUploadSession(fileName string, folderID string, fileSize int64) (*UploadSession, error)
	UploadPart(sessionID string, part []byte, offset int64, totalSize int64) (*UploadPart, error)
	CommitUploadSession(sessionID string, parts []UploadPartInfo, attributes map[string]interface{}) (*File, error)
	AbortUploadSession(sessionID string) error
}

// ProgressCallback is called during file upload to report progress
type ProgressCallback func(bytesUploaded int64, totalBytes int64)

// OAuth2Credentials represents Box OAuth 2.0 credentials
type OAuth2Credentials struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	EnterpriseID string    `json:"enterprise_id,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired returns true if the access token is expired or will expire soon
func (c *OAuth2Credentials) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return true
	}
	// Consider token expired if it expires within 5 minutes
	return time.Now().Add(5 * time.Minute).After(c.ExpiresAt)
}

// Folder represents a Box folder
type Folder struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Size              int64     `json:"size"`
	PathCollection    *Path     `json:"path_collection,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	ModifiedAt        time.Time `json:"modified_at"`
	CreatedBy         *User     `json:"created_by,omitempty"`
	ModifiedBy        *User     `json:"modified_by,omitempty"`
	OwnedBy           *User     `json:"owned_by,omitempty"`
	Parent            *Folder   `json:"parent,omitempty"`
	ItemStatus        string    `json:"item_status"`
	ItemCollection    *Items    `json:"item_collection,omitempty"`
	HasCollaborations bool      `json:"has_collaborations"`
	CanDownload       bool      `json:"can_download"`
	CanUpload         bool      `json:"can_upload"`
	CanRename         bool      `json:"can_rename"`
	CanDelete         bool      `json:"can_delete"`
	CanShare          bool      `json:"can_share"`
	CanSetShareAccess bool      `json:"can_set_share_access"`
}

// File represents a Box file
type File struct {
	ID                 string    `json:"id"`
	Type               string    `json:"type"`
	Name               string   `json:"name"`
	Description        string    `json:"description"`
	Size               int64     `json:"size"`
	PathCollection     *Path     `json:"path_collection,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	ModifiedAt         time.Time `json:"modified_at"`
	TrashedAt          *time.Time `json:"trashed_at,omitempty"`
	PurgedAt           *time.Time `json:"purged_at,omitempty"`
	ContentCreatedAt   time.Time `json:"content_created_at"`
	ContentModifiedAt  time.Time `json:"content_modified_at"`
	CreatedBy          *User     `json:"created_by,omitempty"`
	ModifiedBy         *User     `json:"modified_by,omitempty"`
	OwnedBy            *User     `json:"owned_by,omitempty"`
	Parent             *Folder   `json:"parent,omitempty"`
	ItemStatus         string    `json:"item_status"`
	VersionNumber      string    `json:"version_number"`
	CommentCount       int       `json:"comment_count"`
	Extension          string    `json:"extension"`
	IsPackage          bool      `json:"is_package"`
	HasCollaborations  bool      `json:"has_collaborations"`
	CanDownload        bool      `json:"can_download"`
	CanPreview         bool      `json:"can_preview"`
	CanUpload          bool      `json:"can_upload"`
	CanComment         bool      `json:"can_comment"`
	CanRename          bool      `json:"can_rename"`
	CanDelete          bool      `json:"can_delete"`
	CanShare           bool      `json:"can_share"`
	CanSetShareAccess  bool      `json:"can_set_share_access"`
	SHA1               string    `json:"sha1"`
	FileVersion        *FileVersion `json:"file_version,omitempty"`
}

// FileVersion represents a Box file version
type FileVersion struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
	ModifiedBy *User    `json:"modified_by,omitempty"`
	SHA1      string    `json:"sha1"`
}

// User represents a Box user
type User struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Login  string `json:"login"`
	Avatar string `json:"avatar_url,omitempty"`
}

// Path represents a folder path collection
type Path struct {
	TotalCount int       `json:"total_count"`
	Entries    []*Folder `json:"entries"`
}

// Items represents a collection of items (files and folders)
type Items struct {
	TotalCount int    `json:"total_count"`
	Entries    []Item `json:"entries"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	Order      []struct {
		By        string `json:"by"`
		Direction string `json:"direction"`
	} `json:"order"`
}

// Item represents either a file or folder
type Item struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Size       int64  `json:"size,omitempty"`
	Etag       string `json:"etag"`
	SequenceID string `json:"sequence_id"`
	OwnedBy    *User  `json:"owned_by,omitempty"`
}

// FolderItems represents the response when listing folder contents
type FolderItems struct {
	TotalCount int    `json:"total_count"`
	Entries    []Item `json:"entries"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
}

// CreateFolderRequest represents the request to create a folder
type CreateFolderRequest struct {
	Name   string       `json:"name"`
	Parent *FolderParent `json:"parent"`
}

// FolderParent represents a parent folder reference
type FolderParent struct {
	ID string `json:"id"`
}

// UploadFileRequest represents the metadata for file upload
type UploadFileRequest struct {
	Name   string       `json:"name"`
	Parent *FolderParent `json:"parent"`
}

// UploadSession represents a chunked upload session
type UploadSession struct {
	ID                string                  `json:"id"`
	Type              string                  `json:"type"`
	SessionExpiresAt  time.Time               `json:"session_expires_at"`
	PartSize          int64                   `json:"part_size"`
	TotalParts        int                     `json:"total_parts"`
	NumPartsProcessed int                     `json:"num_parts_processed"`
	SessionEndpoints  *UploadSessionEndpoints `json:"session_endpoints,omitempty"`
}

// UploadSessionEndpoints contains URLs for upload operations
type UploadSessionEndpoints struct {
	UploadPart  string `json:"upload_part,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Abort       string `json:"abort,omitempty"`
	ListParts   string `json:"list_parts,omitempty"`
	Status      string `json:"status,omitempty"`
	LogEvent    string `json:"log_event,omitempty"`
}

// CreateUploadSessionRequest represents the request to create an upload session
type CreateUploadSessionRequest struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	FolderID string `json:"folder_id"`
}

// UploadPart represents information about an uploaded part
type UploadPart struct {
	Part   *UploadPartInfo `json:"part"`
	Offset int64           `json:"offset,omitempty"`
	Size   int64           `json:"size,omitempty"`
}

// UploadPartInfo contains digest and offset for a completed upload part
type UploadPartInfo struct {
	PartID string `json:"part_id,omitempty"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA1   string `json:"sha1,omitempty"`
}

// CommitUploadSessionRequest represents the request to commit an upload session
type CommitUploadSessionRequest struct {
	Parts      []UploadPartInfo       `json:"parts"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// TokenResponse represents Box OAuth token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ErrorResponse represents Box API error response
type ErrorResponse struct {
	Type        string `json:"type"`
	Status      int    `json:"status"`
	Code        string `json:"code"`
	ContextInfo struct {
		Conflicts []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"conflicts,omitempty"`
	} `json:"context_info,omitempty"`
	HelpURL     string `json:"help_url"`
	Message     string `json:"message"`
	RequestID   string `json:"request_id"`
}

// Error implements the error interface for ErrorResponse
func (e *ErrorResponse) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Box API error: %s (status: %d)", e.Code, e.Status)
}

// BoxError represents Box-specific errors
type BoxError struct {
	StatusCode int
	Message    string
	Code       string
	RequestID  string
	Retryable  bool
}

// Error implements the error interface for BoxError
func (e *BoxError) Error() string {
	return fmt.Sprintf("Box API error: %s (status: %d, code: %s)", e.Message, e.StatusCode, e.Code)
}

// IsRetryable returns true if the error is retryable
func (e *BoxError) IsRetryable() bool {
	return e.Retryable
}

// Common Box API constants
const (
	// API endpoints
	BoxAPIBaseURL    = "https://api.box.com/2.0"
	BoxUploadBaseURL = "https://upload.box.com/api/2.0"
	BoxAuthURL       = "https://account.box.com/api/oauth2/authorize"
	BoxTokenURL      = "https://api.box.com/oauth2/token"

	// Folder IDs
	RootFolderID = "0"

	// Item types
	ItemTypeFile   = "file"
	ItemTypeFolder = "folder"

	// Upload limits
	MinChunkedUploadSize = 20 * 1024 * 1024 // 20MB minimum for chunked uploads
	DefaultChunkSize     = 8 * 1024 * 1024  // 8MB default chunk size

	// OAuth scopes
	ScopeBaseExplorer = "base_explorer"
	ScopeBaseUpload   = "base_upload"
	ScopeBaseWrite    = "base_write"
	ScopeBasePreview  = "base_preview"

	// Error codes
	ErrorCodeItemNotFound      = "not_found"
	ErrorCodeItemNameTaken     = "item_name_taken"
	ErrorCodeItemNameInvalid   = "item_name_invalid"
	ErrorCodeInsufficientScope = "insufficient_scope"
	ErrorCodeInvalidGrant      = "invalid_grant"
	ErrorCodeUnauthorized      = "unauthorized"
	ErrorCodeRateLimitExceeded = "rate_limit_exceeded"
)