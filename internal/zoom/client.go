// Package zoom provides API client for Zoom Cloud Recording endpoints
package zoom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CloudRecordingClient defines the interface for Zoom Cloud Recording API operations
type CloudRecordingClient interface {
	ListUserRecordings(ctx context.Context, userID string, params ListRecordingsParams) (*ListRecordingsResponse, error)
	GetMeetingRecordings(ctx context.Context, meetingID string) (*Recording, error)
	DownloadRecordingFile(ctx context.Context, downloadURL string, writer io.Writer) error
}

// ListRecordingsParams holds parameters for listing recordings
type ListRecordingsParams struct {
	From         *time.Time // Start date for the date range
	To           *time.Time // End date for the date range
	PageSize     int        // Number of records per page (default: 30, max: 300)
	NextPageToken string    // Next page token for pagination
	MC           bool       // Query meeting cloud recordings only
	Trash        bool       // Query recordings from trash
	TrashType    string     // Type of trash recordings to query ("meeting_recordings", "recording_file", or "all")
}

// ZoomClient implements the CloudRecordingClient interface
type ZoomClient struct {
	httpClient *AuthenticatedRetryClient
	baseURL    string
}

// NewZoomClient creates a new Zoom API client
func NewZoomClient(httpClient *AuthenticatedRetryClient, baseURL string) *ZoomClient {
	// Remove trailing slash from baseURL
	baseURL = strings.TrimSuffix(baseURL, "/")
	
	return &ZoomClient{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// ListUserRecordings retrieves cloud recordings for a user
func (c *ZoomClient) ListUserRecordings(ctx context.Context, userID string, params ListRecordingsParams) (*ListRecordingsResponse, error) {
	// Build URL
	endpoint := fmt.Sprintf("%s/users/%s/recordings", c.baseURL, url.PathEscape(userID))
	
	// Build query parameters
	queryParams := url.Values{}
	
	if params.From != nil {
		queryParams.Set("from", params.From.Format("2006-01-02"))
	}
	if params.To != nil {
		queryParams.Set("to", params.To.Format("2006-01-02"))
	}
	pageSize := params.PageSize
	if pageSize == 0 {
		pageSize = 30 // Default page size
	}
	queryParams.Set("page_size", strconv.Itoa(pageSize))
	if params.NextPageToken != "" {
		queryParams.Set("next_page_token", params.NextPageToken)
	}
	if params.MC {
		queryParams.Set("mc", "true")
	}
	if params.Trash {
		queryParams.Set("trash", "true")
	}
	if params.TrashType != "" {
		queryParams.Set("trash_type", params.TrashType)
	}
	
	// Add query parameters to URL
	if len(queryParams) > 0 {
		endpoint += "?" + queryParams.Encode()
	}
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Parse response
	var result ListRecordingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &result, nil
}

// GetMeetingRecordings retrieves recordings for a specific meeting
func (c *ZoomClient) GetMeetingRecordings(ctx context.Context, meetingID string) (*Recording, error) {
	// Build URL - URL encode the meeting ID to handle UUIDs and special characters
	// Use QueryEscape to properly encode special characters including forward slashes
	endpoint := fmt.Sprintf("%s/meetings/%s/recordings", c.baseURL, url.QueryEscape(meetingID))
	
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Parse response
	var result Recording
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &result, nil
}

// DownloadRecordingFile downloads a recording file from the provided download URL
func (c *ZoomClient) DownloadRecordingFile(ctx context.Context, downloadURL string, writer io.Writer) error {
	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	
	// Execute request - the authenticated client will handle redirects automatically
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Check for successful response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, resp.Status)
	}
	
	// Stream the file content to the writer
	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}
	
	return nil
}

// GetAllUserRecordings retrieves all recordings for a user using pagination
func (c *ZoomClient) GetAllUserRecordings(ctx context.Context, userID string, params ListRecordingsParams) ([]*Recording, error) {
	var allRecordings []*Recording
	nextPageToken := params.NextPageToken
	
	for {
		// Update params with current page token
		currentParams := params
		currentParams.NextPageToken = nextPageToken
		
		// Get page of recordings
		response, err := c.ListUserRecordings(ctx, userID, currentParams)
		if err != nil {
			return nil, fmt.Errorf("failed to list recordings (page token: %s): %w", nextPageToken, err)
		}
		
		// Add recordings to result
		for _, meeting := range response.Meetings {
			allRecordings = append(allRecordings, &meeting)
		}
		
		// Check if there are more pages
		if response.NextPageToken == "" {
			break
		}
		nextPageToken = response.NextPageToken
	}
	
	return allRecordings, nil
}