package box

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type boxClient struct {
	httpClient AuthenticatedHTTPClient
	mutex      sync.RWMutex
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

	if resp.StatusCode == http.StatusConflict {
		return nil, &BoxError{
			StatusCode: resp.StatusCode,
			Code:       ErrorCodeItemNameTaken,
			Message:    fmt.Sprintf("folder '%s' already exists in parent folder", name),
			Retryable:  false,
		}
	}

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create folder, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var folder Folder
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
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
		} else {
			folder, err := client.CreateFolder(part, currentParentID)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder '%s' in parent %s: %w", part, currentParentID, err)
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