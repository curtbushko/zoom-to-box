package box

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/logging"
)

type boxClient struct {
	httpClient AuthenticatedHTTPClient
}

func NewBoxClient(auth Authenticator, httpClient *http.Client) BoxClient {
	authClient := NewAuthenticatedHTTPClient(auth, httpClient)
	return &boxClient{
		httpClient: authClient,
	}
}

func (c *boxClient) RefreshToken() error {
	return fmt.Errorf("token refresh not implemented via client interface")
}

func (c *boxClient) IsAuthenticated() bool {
	return true
}

func (c *boxClient) GetCurrentUser() (*User, error) {
	url := fmt.Sprintf("%s/users/me", BoxAPIBaseURL)
	resp, err := c.httpClient.Get(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeUnauthorized,
			Message:    "unauthorized - invalid or expired access token",
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get current user, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &user, nil
}

func (c *boxClient) GetUserByEmail(email string) (*User, error) {
	if email == "" {
		return nil, fmt.Errorf("email cannot be empty")
	}

	// Box API requires filtering users by login (email)
	// URL encode the email and search for all user types
	// The filter_term parameter matches the beginning of the login string
	// Valid user_type values: all, managed, external
	escapedEmail := url.QueryEscape(email)
	apiURL := fmt.Sprintf("%s/users?filter_term=%s&user_type=all", BoxAPIBaseURL, escapedEmail)

	resp, err := c.httpClient.Get(context.Background(), apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeUnauthorized,
			Message:    "unauthorized - invalid or expired access token",
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user by email, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		TotalCount int     `json:"total_count"`
		Entries    []*User `json:"entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	if len(response.Entries) == 0 {
		return nil, &BoxError{
			StatusCode: http.StatusNotFound,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("user with email '%s' not found", email),
			Retryable:  false,
		}
	}

	// Find exact match by login (case-insensitive comparison)
	emailLower := strings.ToLower(email)
	for _, user := range response.Entries {
		if strings.ToLower(user.Login) == emailLower {
			return user, nil
		}
	}

	// If no exact match found, return error instead of first result
	// to avoid returning wrong user
	return nil, &BoxError{
		StatusCode: http.StatusNotFound,
		Code:       ErrorCodeItemNotFound,
		Message:    fmt.Sprintf("user with exact email '%s' not found (found %d partial matches)", email, len(response.Entries)),
		Retryable:  false,
	}
}

func (c *boxClient) CreateFolder(name string, parentID string) (*Folder, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}
	if parentID == "" {
		parentID = RootFolderID
	}

	request := CreateFolderRequest{
		Name: name,
		Parent: &FolderParent{
			ID: parentID,
		},
	}

	url := fmt.Sprintf("%s/folders", BoxAPIBaseURL)
	resp, err := c.httpClient.PostJSON(context.Background(), url, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for both success and error cases
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusConflict {
		// Try to extract folder ID from conflict response
		// Box API returns the conflicting item in context_info.conflicts
		var errorResp ErrorResponse
		if json.Unmarshal(bodyBytes, &errorResp) == nil &&
			len(errorResp.ContextInfo.Conflicts) > 0 {
			// Return the existing folder info
			conflict := errorResp.ContextInfo.Conflicts[0]
			return &Folder{
				ID:   conflict.ID,
				Type: conflict.Type,
				Name: conflict.Name,
			}, nil
		}

		// If we couldn't extract from conflict response, return error
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNameTaken,
			Message:    fmt.Sprintf("folder '%s' already exists in parent folder", name),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create folder, status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var folder Folder
	if err := json.Unmarshal(bodyBytes, &folder); err != nil {
		return nil, fmt.Errorf("failed to decode folder response: %w", err)
	}

	return &folder, nil
}

func (c *boxClient) CreateFolderAsUser(name string, parentID string, userID string) (*Folder, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}
	if parentID == "" {
		parentID = RootFolderID
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	request := CreateFolderRequest{
		Name: name,
		Parent: &FolderParent{
			ID: parentID,
		},
	}

	url := fmt.Sprintf("%s/folders", BoxAPIBaseURL)
	resp, err := c.httpClient.PostJSONAsUser(context.Background(), url, request, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder as user: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for both success and error cases
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusConflict {
		// Try to extract folder ID from conflict response
		// Box API returns the conflicting item in context_info.conflicts
		var errorResp ErrorResponse
		if json.Unmarshal(bodyBytes, &errorResp) == nil &&
			len(errorResp.ContextInfo.Conflicts) > 0 {
			// Return the existing folder info
			conflict := errorResp.ContextInfo.Conflicts[0]
			return &Folder{
				ID:   conflict.ID,
				Type: conflict.Type,
				Name: conflict.Name,
			}, nil
		}

		// If we couldn't extract from conflict response, return error
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNameTaken,
			Message:    fmt.Sprintf("folder '%s' already exists in parent folder", name),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create folder as user, status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var folder Folder
	if err := json.Unmarshal(bodyBytes, &folder); err != nil {
		return nil, fmt.Errorf("failed to decode folder response: %w", err)
	}

	return &folder, nil
}

func (c *boxClient) GetFolder(folderID string) (*Folder, error) {
	if folderID == "" {
		return nil, fmt.Errorf("folder ID cannot be empty")
	}

	url := fmt.Sprintf("%s/folders/%s", BoxAPIBaseURL, folderID)
	resp, err := c.httpClient.Get(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("folder with ID '%s' not found", folderID),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get folder, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var folder Folder
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
		return nil, fmt.Errorf("failed to decode folder response: %w", err)
	}

	return &folder, nil
}

func (c *boxClient) ListFolderItems(folderID string) (*FolderItems, error) {
	if folderID == "" {
		folderID = RootFolderID
	}

	url := fmt.Sprintf("%s/folders/%s/items", BoxAPIBaseURL, folderID)
	resp, err := c.httpClient.Get(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to list folder items: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("folder with ID '%s' not found", folderID),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list folder items, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var items FolderItems
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode folder items response: %w", err)
	}

	return &items, nil
}

func (c *boxClient) ListFolderItemsAsUser(folderID string, userID string) (*FolderItems, error) {
	if folderID == "" {
		folderID = RootFolderID
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	url := fmt.Sprintf("%s/folders/%s/items", BoxAPIBaseURL, folderID)
	resp, err := c.httpClient.GetAsUser(context.Background(), url, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list folder items as user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("folder with ID '%s' not found", folderID),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list folder items as user, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var items FolderItems
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode folder items response: %w", err)
	}

	return &items, nil
}

// FindZoomFolder finds the "zoom" folder in the root directory
// This matches the behavior of the box-upload.sh script
func (c *boxClient) FindZoomFolder() (string, error) {
	url := fmt.Sprintf("%s/folders/0/items?fields=id,name,type&limit=1000", BoxAPIBaseURL)
	resp, err := c.httpClient.Get(context.Background(), url)
	if err != nil {
		return "", fmt.Errorf("failed to list root folder items: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to list root folder items, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var items FolderItems
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return "", fmt.Errorf("failed to decode folder items response: %w", err)
	}

	// Search for the zoom folder
	for _, item := range items.Entries {
		if item.Type == ItemTypeFolder && item.Name == "zoom" {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("zoom folder not found in root directory")
}

// FindFolderByName searches for a folder by name within a parent folder
// Returns the full folder information if found, or a BoxError if not found
func (c *boxClient) FindFolderByName(parentID string, name string) (*Folder, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}

	if parentID == "" {
		parentID = RootFolderID
	}

	// List items in the parent folder
	items, err := c.ListFolderItems(parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list folder items: %w", err)
	}

	// Search for the folder by name
	for _, item := range items.Entries {
		if item.Type == ItemTypeFolder && item.Name == name {
			// Get full folder information
			return c.GetFolder(item.ID)
		}
	}

	// Folder not found
	return nil, &BoxError{
		StatusCode: http.StatusNotFound,
		Code:       ErrorCodeItemNotFound,
		Message:    fmt.Sprintf("folder '%s' not found in parent folder", name),
		Retryable:  false,
	}
}

// FindFileByName searches for a file by name within a folder
// Returns the full file information if found, or a BoxError if not found
func (c *boxClient) FindFileByName(folderID string, name string) (*File, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("file name cannot be empty")
	}

	if folderID == "" {
		folderID = RootFolderID
	}

	// List items in the folder
	items, err := c.ListFolderItems(folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to list folder items: %w", err)
	}

	// Search for the file by name
	for _, item := range items.Entries {
		if item.Type == ItemTypeFile && item.Name == name {
			// Get full file information
			return c.GetFile(item.ID)
		}
	}

	// File not found
	return nil, &BoxError{
		StatusCode: http.StatusNotFound,
		Code:       ErrorCodeItemNotFound,
		Message:    fmt.Sprintf("file '%s' not found in folder", name),
		Retryable:  false,
	}
}

// FindZoomFolderByOwner finds the "zoom" folder owned by a specific user
// Searches the root directory for zoom folders and matches by owner email
// Returns the full folder information if found, or a BoxError if not found
// Supports pagination to handle cases where there are more than 1000 items in root
func (c *boxClient) FindZoomFolderByOwner(ownerEmail string) (*Folder, error) {
	if strings.TrimSpace(ownerEmail) == "" {
		return nil, fmt.Errorf("owner email cannot be empty")
	}

	ownerEmailLower := strings.ToLower(ownerEmail)
	offset := 0
	limit := 1000

	logging.Info("Searching for zoom folder for owner: %s", ownerEmail)

	// Paginate through all items in the root folder
	for {
		// List root folder items with owned_by field
		apiURL := fmt.Sprintf("%s/folders/0/items?fields=id,name,type,owned_by&limit=%d&offset=%d", BoxAPIBaseURL, limit, offset)

		logging.Debug("Fetching Box root folder items - offset: %d, limit: %d", offset, limit)

		resp, err := c.httpClient.Get(context.Background(), apiURL)
		if err != nil {
			return nil, fmt.Errorf("failed to list root folder items: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list root folder items, status: %d, body: %s", resp.StatusCode, string(body))
		}

		var items FolderItems
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			return nil, fmt.Errorf("failed to decode folder items response: %w", err)
		}

		logging.Debug("Retrieved %d items from Box root folder (offset: %d)", len(items.Entries), offset)

		// Search for zoom folder owned by the specified user (case-insensitive)
		for _, item := range items.Entries {
			if item.Type == ItemTypeFolder && item.Name == "zoom" {
				// Check if owner matches
				if item.OwnedBy != nil && strings.ToLower(item.OwnedBy.Login) == ownerEmailLower {
					// Get full folder information
					folder, err := c.GetFolder(item.ID)
					if err != nil {
						return nil, err
					}

					logging.Info("Found zoom folder for %s - folder ID: %s", ownerEmail, folder.ID)
					return folder, nil
				}
			}
		}

		// Check if there are more items to fetch
		if len(items.Entries) < limit {
			// No more items to fetch
			logging.Debug("Reached end of Box root folder items (total pages checked: %d)", (offset/limit)+1)
			break
		}

		// Move to next page
		offset += limit
		logging.Debug("Moving to next page of Box root folder items")
	}

	// Zoom folder not found for this owner
	return nil, &BoxError{
		StatusCode: http.StatusNotFound,
		Code:       ErrorCodeItemNotFound,
		Message:    fmt.Sprintf("zoom folder not found for owner '%s'", ownerEmail),
		Retryable:  false,
	}
}

func (c *boxClient) UploadFile(filePath string, parentFolderID string, fileName string) (*File, error) {
	return c.UploadFileWithProgress(filePath, parentFolderID, fileName, nil)
}

func (c *boxClient) UploadFileWithProgress(filePath string, parentFolderID string, fileName string, progressCallback ProgressCallback) (*File, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if parentFolderID == "" {
		parentFolderID = RootFolderID
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	// Check file size to determine upload method
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Use chunked upload for files >= 20MB
	if fileInfo.Size() >= MinChunkedUploadSize {
		return c.UploadLargeFile(filePath, parentFolderID, fileName, progressCallback)
	}

	// Use regular upload for smaller files
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err = file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	attributes := UploadFileRequest{
		Name: fileName,
		Parent: &FolderParent{
			ID: parentFolderID,
		},
	}

	attributesJSON, err := json.Marshal(attributes)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal file attributes: %w", err)
	}

	if err := writer.WriteField("attributes", string(attributesJSON)); err != nil {
		return nil, fmt.Errorf("failed to write attributes field: %w", err)
	}

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	totalBytes := fileInfo.Size()
	var bytesWritten int64

	if progressCallback != nil {
		progressCallback(0, totalBytes)
	}

	buffer := make([]byte, 32*1024)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if _, writeErr := part.Write(buffer[:n]); writeErr != nil {
				return nil, fmt.Errorf("failed to write file data: %w", writeErr)
			}
			bytesWritten += int64(n)
			if progressCallback != nil {
				progressCallback(bytesWritten, totalBytes)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s/files/content", BoxUploadBaseURL)
	resp, err := c.httpClient.Post(context.Background(), url, writer.FormDataContentType(), &body)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNameTaken,
			Message:    fmt.Sprintf("file '%s' already exists in folder", fileName),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to upload file, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var uploadResponse struct {
		TotalCount int     `json:"total_count"`
		Entries    []*File `json:"entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&uploadResponse); err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}

	if len(uploadResponse.Entries) == 0 {
		return nil, fmt.Errorf("no file entries in upload response")
	}

	return uploadResponse.Entries[0], nil
}

func (c *boxClient) UploadFileAsUser(filePath string, parentFolderID string, fileName string, userID string, progressCallback ProgressCallback) (*File, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if parentFolderID == "" {
		parentFolderID = RootFolderID
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	attributes := UploadFileRequest{
		Name: fileName,
		Parent: &FolderParent{
			ID: parentFolderID,
		},
	}

	attributesJSON, err := json.Marshal(attributes)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal file attributes: %w", err)
	}

	if err := writer.WriteField("attributes", string(attributesJSON)); err != nil {
		return nil, fmt.Errorf("failed to write attributes field: %w", err)
	}

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	totalBytes := fileInfo.Size()
	var bytesWritten int64

	if progressCallback != nil {
		progressCallback(0, totalBytes)
	}

	buffer := make([]byte, 32*1024)
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if _, writeErr := part.Write(buffer[:n]); writeErr != nil {
				return nil, fmt.Errorf("failed to write file data: %w", writeErr)
			}
			bytesWritten += int64(n)
			if progressCallback != nil {
				progressCallback(bytesWritten, totalBytes)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s/files/content", BoxUploadBaseURL)
	resp, err := c.httpClient.PostAsUser(context.Background(), url, writer.FormDataContentType(), &body, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file as user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNameTaken,
			Message:    fmt.Sprintf("file '%s' already exists in folder", fileName),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to upload file as user, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var uploadResponse struct {
		TotalCount int     `json:"total_count"`
		Entries    []*File `json:"entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&uploadResponse); err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}

	if len(uploadResponse.Entries) == 0 {
		return nil, fmt.Errorf("no file entries in upload response")
	}

	return uploadResponse.Entries[0], nil
}

func (c *boxClient) GetFile(fileID string) (*File, error) {
	if fileID == "" {
		return nil, fmt.Errorf("file ID cannot be empty")
	}

	url := fmt.Sprintf("%s/files/%s", BoxAPIBaseURL, fileID)
	resp, err := c.httpClient.Get(context.Background(), url)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("file with ID '%s' not found", fileID),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get file, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var file File
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, fmt.Errorf("failed to decode file response: %w", err)
	}

	return &file, nil
}

func (c *boxClient) DeleteFile(fileID string) error {
	if fileID == "" {
		return fmt.Errorf("file ID cannot be empty")
	}

	url := fmt.Sprintf("%s/files/%s", BoxAPIBaseURL, fileID)
	req, err := http.NewRequestWithContext(context.Background(), "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("file with ID '%s' not found", fileID),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete file, status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

func CreateFolderPath(client BoxClient, folderPath string, parentID string) (*Folder, error) {
	if folderPath == "" || folderPath == "/" {
		if parentID == "" {
			parentID = RootFolderID
		}
		return client.GetFolder(parentID)
	}

	if parentID == "" {
		parentID = RootFolderID
	}

	parts := strings.Split(strings.Trim(folderPath, "/"), "/")
	currentParentID := parentID
	var lastFolder *Folder

	for _, part := range parts {
		if part == "" {
			continue
		}

		items, err := client.ListFolderItems(currentParentID)
		if err != nil {
			return nil, fmt.Errorf("failed to list items in folder %s: %w", currentParentID, err)
		}

		var found *Item
		for _, item := range items.Entries {
			if item.Type == ItemTypeFolder && item.Name == part {
				found = &item
				break
			}
		}

		if found != nil {
			currentParentID = found.ID
			// Get full folder info for found folder
			lastFolder, err = client.GetFolder(found.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to get folder %s: %w", found.ID, err)
			}
		} else {
			folder, err := client.CreateFolder(part, currentParentID)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder '%s' in parent %s: %w", part, currentParentID, err)
			}
			currentParentID = folder.ID
			lastFolder = folder
		}
	}

	return lastFolder, nil
}

// CreateFolderPathAsUser creates a folder path as a specific user using As-User header
func CreateFolderPathAsUser(client BoxClient, folderPath string, parentID string, userID string) (*Folder, error) {
	if folderPath == "" || folderPath == "/" {
		if parentID == "" {
			parentID = RootFolderID
		}
		return client.GetFolder(parentID)
	}

	if parentID == "" {
		parentID = RootFolderID
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	parts := strings.Split(strings.Trim(folderPath, "/"), "/")
	currentParentID := parentID

	for _, part := range parts {
		if part == "" {
			continue
		}

		items, err := client.ListFolderItemsAsUser(currentParentID, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to list items in folder %s as user: %w", currentParentID, err)
		}

		var found *Item
		for _, item := range items.Entries {
			if item.Type == ItemTypeFolder && item.Name == part {
				found = &item
				break
			}
		}

		if found != nil {
			currentParentID = found.ID
		} else {
			folder, err := client.CreateFolderAsUser(part, currentParentID, userID)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder '%s' in parent %s as user: %w", part, currentParentID, err)
			}
			currentParentID = folder.ID
		}
	}

	return client.GetFolder(currentParentID)
}

func ValidateFileName(fileName string) error {
	if strings.TrimSpace(fileName) == "" {
		return fmt.Errorf("file name cannot be empty")
	}

	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		if strings.Contains(fileName, char) {
			return fmt.Errorf("file name contains invalid character: %s", char)
		}
	}

	if len(fileName) > 255 {
		return fmt.Errorf("file name too long (max 255 characters)")
	}

	return nil
}

// FindFolderByPath searches for a folder by its path within a parent folder
func FindFolderByPath(client BoxClient, folderPath string, parentID string) (*Folder, error) {
	if folderPath == "" || folderPath == "/" {
		if parentID == "" {
			parentID = RootFolderID
		}
		return client.GetFolder(parentID)
	}

	if parentID == "" {
		parentID = RootFolderID
	}

	parts := strings.Split(strings.Trim(folderPath, "/"), "/")
	currentParentID := parentID

	for _, part := range parts {
		if part == "" {
			continue
		}

		items, err := client.ListFolderItems(currentParentID)
		if err != nil {
			return nil, fmt.Errorf("failed to list items in folder %s: %w", currentParentID, err)
		}

		var found *Item
		for _, item := range items.Entries {
			if item.Type == ItemTypeFolder && item.Name == part {
				found = &item
				break
			}
		}

		if found == nil {
			return nil, &BoxError{
				StatusCode: 404,
				Code:       ErrorCodeItemNotFound,
				Message:    fmt.Sprintf("folder '%s' not found in path '%s'", part, folderPath),
				Retryable:  false,
			}
		}

		currentParentID = found.ID
	}

	return client.GetFolder(currentParentID)
}

// ValidateFolderStructure validates that the expected folder structure exists and is accessible
func ValidateFolderStructure(client BoxClient, folderPath string, parentID string) error {
	_, err := FindFolderByPath(client, folderPath, parentID)
	if err != nil {
		return fmt.Errorf("folder structure validation failed: %w", err)
	}
	return nil
}

// CreateUploadSession creates a new chunked upload session
func (c *boxClient) CreateUploadSession(fileName string, folderID string, fileSize int64) (*UploadSession, error) {
	if strings.TrimSpace(fileName) == "" {
		return nil, fmt.Errorf("file name cannot be empty")
	}
	if folderID == "" {
		folderID = RootFolderID
	}
	if fileSize < MinChunkedUploadSize {
		return nil, fmt.Errorf("file size %d is less than minimum chunked upload size %d", fileSize, MinChunkedUploadSize)
	}

	request := CreateUploadSessionRequest{
		FileName: fileName,
		FolderID: folderID,
		FileSize: fileSize,
	}

	url := fmt.Sprintf("%s/files/upload_sessions", BoxUploadBaseURL)
	resp, err := c.httpClient.PostJSON(context.Background(), url, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNotFound,
			Message:    fmt.Sprintf("folder with ID '%s' not found", folderID),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create upload session, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var session UploadSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode upload session response: %w", err)
	}

	return &session, nil
}

// UploadPart uploads a single part of a chunked upload with retry logic
func (c *boxClient) UploadPart(sessionID string, part []byte, offset int64, totalSize int64) (*UploadPart, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if len(part) == 0 {
		return nil, fmt.Errorf("part data cannot be empty")
	}

	// Calculate SHA1 digest for data integrity validation
	h := sha1.New()
	h.Write(part)
	sha1Hash := h.Sum(nil)
	digest := "sha=" + base64.StdEncoding.EncodeToString(sha1Hash)

	// Calculate content range
	partSize := int64(len(part))
	rangeEnd := offset + partSize - 1
	contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, totalSize)

	// Use retry logic for transient failures
	maxRetries := 3
	var lastErr error
	var uploadPart *UploadPart

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Create request for each attempt (can't reuse request body)
		url := fmt.Sprintf("%s/files/upload_sessions/%s", BoxUploadBaseURL, sessionID)
		req, err := http.NewRequestWithContext(context.Background(), "PUT", url, bytes.NewReader(part))
		if err != nil {
			return nil, fmt.Errorf("failed to create upload part request: %w", err)
		}

		// Set headers with SHA1 digest for data integrity
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Range", contentRange)
		req.Header.Set("Digest", digest)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			// Check if error is retryable (network/timeout errors)
			if isRetryableError(err) && attempt < maxRetries-1 {
				// Exponential backoff: 500ms, 1s, 2s
				backoff := 500 * (1 << attempt) * time.Millisecond
				time.Sleep(backoff)
				continue
			}
			return nil, fmt.Errorf("failed to upload part after %d attempts: %w", attempt+1, err)
		}
		defer resp.Body.Close()

		// Check for retryable HTTP status codes
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("failed to upload part, status: %d, body: %s", resp.StatusCode, string(body))

			// Retry on 5xx server errors and 429 rate limit
			if (resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests) && attempt < maxRetries-1 {
				backoff := 500 * (1 << attempt) * time.Millisecond
				if resp.StatusCode == http.StatusTooManyRequests {
					// Longer backoff for rate limits
					backoff = 5 * time.Second
				}
				time.Sleep(backoff)
				continue
			}
			return nil, lastErr
		}

		// Success - decode response
		if err := json.NewDecoder(resp.Body).Decode(&uploadPart); err != nil {
			return nil, fmt.Errorf("failed to decode upload part response: %w", err)
		}

		return uploadPart, nil
	}

	return nil, fmt.Errorf("failed to upload part after %d attempts: %w", maxRetries, lastErr)
}

// isRetryableError checks if an error is transient and should be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context errors (don't retry if context was canceled/timed out)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Check for network errors
	errStr := err.Error()
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"no such host",
		"network",
		"timeout",
		"temporary failure",
		"TLS handshake",
		"EOF",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// validateUploadedParts validates that uploaded parts are complete and sequential
func validateUploadedParts(parts []UploadPartInfo, totalSize int64) error {
	if len(parts) == 0 {
		return fmt.Errorf("no parts uploaded")
	}

	// Sort parts by offset to ensure sequential validation
	// Note: In practice, parts should already be in order from upload loop
	var expectedOffset int64 = 0
	var totalBytes int64 = 0

	for i, part := range parts {
		// Check for gaps or overlaps
		if part.Offset != expectedOffset {
			if part.Offset < expectedOffset {
				return fmt.Errorf("part %d overlaps: offset %d, expected %d", i, part.Offset, expectedOffset)
			}
			return fmt.Errorf("gap detected before part %d: offset %d, expected %d", i, part.Offset, expectedOffset)
		}

		// Validate part size
		if part.Size <= 0 {
			return fmt.Errorf("part %d has invalid size: %d", i, part.Size)
		}

		// Validate SHA1 is present
		if part.SHA1 == "" {
			return fmt.Errorf("part %d missing SHA1 hash", i)
		}

		expectedOffset += part.Size
		totalBytes += part.Size
	}

	// Verify total size matches
	if totalBytes != totalSize {
		return fmt.Errorf("total uploaded size %d does not match expected size %d", totalBytes, totalSize)
	}

	return nil
}

// CommitUploadSession commits a chunked upload session
func (c *boxClient) CommitUploadSession(sessionID string, parts []UploadPartInfo, attributes map[string]interface{}, digest string) (*File, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("parts list cannot be empty")
	}
	if digest == "" {
		return nil, fmt.Errorf("digest cannot be empty")
	}

	request := CommitUploadSessionRequest{
		Parts:      parts,
		Attributes: attributes,
	}

	url := fmt.Sprintf("%s/files/upload_sessions/%s/commit", BoxUploadBaseURL, sessionID)

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal commit request: %w", err)
	}

	// Create HTTP request to add custom Digest header
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create commit request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Digest", digest)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to commit upload session: %w", err)
	}
	defer resp.Body.Close()

	// Box may return 202 Accepted if processing is still ongoing
	if resp.StatusCode == http.StatusAccepted {
		// Check Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		return nil, fmt.Errorf("upload session commit still processing, retry after: %s seconds", retryAfter)
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to commit upload session, status: %d, body: %s", resp.StatusCode, string(body))
	}

	// Response contains entries array like regular upload
	var uploadResponse struct {
		TotalCount int     `json:"total_count"`
		Entries    []*File `json:"entries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&uploadResponse); err != nil {
		return nil, fmt.Errorf("failed to decode commit response: %w", err)
	}

	if len(uploadResponse.Entries) == 0 {
		return nil, fmt.Errorf("no file entries in commit response")
	}

	return uploadResponse.Entries[0], nil
}

// AbortUploadSession aborts a chunked upload session
func (c *boxClient) AbortUploadSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	url := fmt.Sprintf("%s/files/upload_sessions/%s", BoxUploadBaseURL, sessionID)
	req, err := http.NewRequestWithContext(context.Background(), "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create abort request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to abort upload session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to abort upload session, status: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// calculateFileSHA1 computes the SHA-1 hash of an entire file
// Returns the hash in the format "sha=<base64-encoded-hash>" as required by Box API
func calculateFileSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	h := sha1.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("failed to calculate SHA-1: %w", err)
	}

	sha1Hash := h.Sum(nil)
	digest := "sha=" + base64.StdEncoding.EncodeToString(sha1Hash)
	return digest, nil
}

// UploadLargeFile uploads a file using chunked upload API
// This is a helper function that orchestrates the entire chunked upload process
func (c *boxClient) UploadLargeFile(filePath string, parentFolderID string, fileName string, progressCallback ProgressCallback) (*File, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if parentFolderID == "" {
		parentFolderID = RootFolderID
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	// Open file and get size
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	totalSize := fileInfo.Size()

	// Calculate SHA-1 digest of entire file for commit
	fileSHA1, err := calculateFileSHA1(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate file digest: %w", err)
	}

	// Create upload session
	session, err := c.CreateUploadSession(fileName, parentFolderID, totalSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}

	// Track uploaded parts for commit
	var uploadedParts []UploadPartInfo
	var offset int64 = 0
	partSize := session.PartSize
	if partSize == 0 {
		partSize = DefaultChunkSize
	}

	// Upload parts
	buffer := make([]byte, partSize)
	for offset < totalSize {
		n, readErr := file.Read(buffer)
		if n > 0 {
			// Upload this part - make a copy to avoid buffer reuse issues
			part := make([]byte, n)
			copy(part, buffer[:n])

			uploadPart, err := c.UploadPart(session.ID, part, offset, totalSize)
			if err != nil {
				// Abort session on error
				_ = c.AbortUploadSession(session.ID)
				return nil, fmt.Errorf("failed to upload part at offset %d: %w", offset, err)
			}

			// Track the uploaded part - always calculate SHA1 for validation
			h := sha1.New()
			h.Write(part)
			sha1Hash := base64.StdEncoding.EncodeToString(h.Sum(nil))

			partInfo := UploadPartInfo{
				Offset: offset,
				Size:   int64(n),
				SHA1:   sha1Hash,
			}

			// Use Box-returned part info if available, otherwise use our calculated values
			if uploadPart.Part != nil {
				partInfo = *uploadPart.Part
			}

			uploadedParts = append(uploadedParts, partInfo)

			offset += int64(n)

			// Report progress
			if progressCallback != nil {
				progressCallback(offset, totalSize)
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = c.AbortUploadSession(session.ID)
			return nil, fmt.Errorf("failed to read file: %w", readErr)
		}
	}

	// Validate uploaded parts before committing
	if err := validateUploadedParts(uploadedParts, totalSize); err != nil {
		_ = c.AbortUploadSession(session.ID)
		return nil, fmt.Errorf("upload validation failed: %w", err)
	}

	// Prepare file attributes for commit
	// Note: "name" is not allowed in attributes - it was already set during CreateUploadSession
	attributes := map[string]interface{}{}

	// Commit the upload session with file metadata and digest
	uploadedFile, err := c.CommitUploadSession(session.ID, uploadedParts, attributes, fileSHA1)
	if err != nil {
		// Don't abort on commit error - the session might still be processing
		return nil, fmt.Errorf("failed to commit upload session: %w", err)
	}

	// Final progress callback
	if progressCallback != nil {
		progressCallback(totalSize, totalSize)
	}

	return uploadedFile, nil
}