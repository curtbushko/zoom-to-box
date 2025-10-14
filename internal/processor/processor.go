// Package processor provides user-level processing orchestration for zoom-to-box
package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/box"
	"github.com/curtbushko/zoom-to-box/internal/directory"
	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/email"
	"github.com/curtbushko/zoom-to-box/internal/filename"
	"github.com/curtbushko/zoom-to-box/internal/logging"
	"github.com/curtbushko/zoom-to-box/internal/tracking"
	"github.com/curtbushko/zoom-to-box/internal/users"
	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

// UserProcessor defines the interface for processing users
type UserProcessor interface {
	// ProcessUser downloads and uploads recordings for a single user
	ProcessUser(ctx context.Context, zoomEmail, boxEmail string) (*ProcessorResult, error)

	// ProcessAllUsers processes all incomplete users from the active users file
	ProcessAllUsers(ctx context.Context, usersFile *users.ActiveUsersFile) (*ProcessorSummary, error)
}

// ProcessorConfig holds configuration for the user processor
type ProcessorConfig struct {
	BaseDownloadDir   string
	BoxEnabled        bool
	DeleteAfterUpload bool
	ContinueOnError   bool
	MetaOnly          bool
	Limit             int
	DryRun            bool
	Verbose           bool
}

// ProcessorResult represents the result of processing a single user
type ProcessorResult struct {
	ZoomEmail       string
	BoxEmail        string
	DownloadedCount int
	UploadedCount   int
	SkippedCount    int
	ErrorCount      int
	DeletedCount    int
	Errors          []error
	Duration        time.Duration
}

// ProcessorSummary represents the summary of processing multiple users
type ProcessorSummary struct {
	TotalUsers       int
	ProcessedUsers   int
	FailedUsers      int
	TotalDownloads   int
	TotalUploads     int
	TotalSkipped     int
	TotalErrors      int
	TotalDeleted     int
	Duration         time.Duration
	UserResults      []*ProcessorResult
}

// ZoomClientInterface defines the methods we need from ZoomClient
type ZoomClientInterface interface {
	GetAllUserRecordings(ctx context.Context, userID string, params zoom.ListRecordingsParams) ([]*zoom.Recording, error)
	GetOAuthAccessToken(ctx context.Context) (string, error)
}

// userProcessorImpl implements the UserProcessor interface
type userProcessorImpl struct {
	zoomClient        ZoomClientInterface
	downloadManager   download.DownloadManager
	dirManager        directory.DirectoryManager
	filenameSanitizer filename.FileSanitizer
	boxUploadManager  box.UploadManager
	config            ProcessorConfig
}

// NewUserProcessor creates a new user processor
func NewUserProcessor(
	zoomClient ZoomClientInterface,
	downloadManager download.DownloadManager,
	dirManager directory.DirectoryManager,
	filenameSanitizer filename.FileSanitizer,
	boxUploadManager box.UploadManager,
	config ProcessorConfig,
) UserProcessor {
	return &userProcessorImpl{
		zoomClient:        zoomClient,
		downloadManager:   downloadManager,
		dirManager:        dirManager,
		filenameSanitizer: filenameSanitizer,
		boxUploadManager:  boxUploadManager,
		config:            config,
	}
}

// ProcessUser downloads and uploads recordings for a single user
func (p *userProcessorImpl) ProcessUser(ctx context.Context, zoomEmail, boxEmail string) (*ProcessorResult, error) {
	startTime := time.Now()

	result := &ProcessorResult{
		ZoomEmail: zoomEmail,
		BoxEmail:  boxEmail,
		Errors:    make([]error, 0),
	}

	logger := logging.GetDefaultLogger()
	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Processing user: %s (Box email: %s)", zoomEmail, boxEmail))
	}

	// Get recordings for this user FIRST before any setup
	params := zoom.ListRecordingsParams{
		From:     getFromDate(),
		To:       getToDate(),
		PageSize: 300,
	}

	recordings, err := p.zoomClient.GetAllUserRecordings(ctx, zoomEmail, params)
	if err != nil {
		err = fmt.Errorf("failed to get recordings for user %s: %w", zoomEmail, err)
		result.Errors = append(result.Errors, err)
		result.ErrorCount++
		result.Duration = time.Since(startTime)

		if logger != nil {
			logger.ErrorWithContext(ctx, err.Error())
		}

		if !p.config.ContinueOnError {
			return result, err
		}
		return result, nil // Continue with empty result
	}

	// Always log the recordings count and API parameters used
	if logger != nil {
		fromStr := "nil (all time)"
		if params.From != nil {
			fromStr = params.From.Format("2006-01-02")
		}
		toStr := "nil (all time)"
		if params.To != nil {
			toStr = params.To.Format("2006-01-02")
		}
		logger.InfoWithContext(ctx, fmt.Sprintf("Zoom API returned %d recordings for user %s (from: %s, to: %s, page_size: %d)",
			len(recordings), zoomEmail, fromStr, toStr, params.PageSize))
	}

	// If user has no recordings, skip them (mark as complete, don't create any directories/files)
	if len(recordings) == 0 {
		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("User %s has no recordings, skipping", zoomEmail))
		}
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// If Box is enabled, verify access to the zoom folder BEFORE downloading anything
	if p.config.BoxEnabled && p.boxUploadManager != nil {
		boxClient := p.boxUploadManager.GetBoxClient()
		_, err := boxClient.FindZoomFolderByOwner(boxEmail)
		if err != nil {
			// Cannot access zoom folder - mark this user as failed so they remain in active_users with upload_complete=false
			boxErr := fmt.Errorf("cannot access zoom folder for user %s (Box email: %s): %w", zoomEmail, boxEmail, err)
			result.Errors = append(result.Errors, boxErr)
			result.ErrorCount++
			result.Duration = time.Since(startTime)

			if logger != nil {
				logger.WarnWithContext(ctx, boxErr.Error())
			}

			if !p.config.ContinueOnError {
				return result, boxErr
			}
			return result, nil
		}

		// User has recordings AND we can access their Box zoom folder - initialize CSV tracker
		username := email.ExtractUsername(boxEmail)
		if username != "" {
			userDir := filepath.Join(p.config.BaseDownloadDir, username)
			userCSVTracker, err := tracking.NewUserCSVTracker(userDir, zoomEmail)
			if err != nil {
				if logger != nil {
					logger.WarnWithContext(ctx, fmt.Sprintf("Failed to create user CSV tracker for %s: %v", zoomEmail, err))
				}
			} else {
				p.boxUploadManager.SetUserCSVTracker(userCSVTracker)
				if logger != nil {
					logger.InfoWithContext(ctx, fmt.Sprintf("Initialized user CSV tracker for %s at %s/uploads.csv", zoomEmail, userDir))
				}
			}
		}
	}

	// Process each recording
	processedCount := 0
	for _, recording := range recordings {
		// Check limit
		if p.config.Limit > 0 && processedCount >= p.config.Limit {
			if logger != nil {
				logger.InfoWithContext(ctx, fmt.Sprintf("Reached limit of %d recordings for user %s", p.config.Limit, zoomEmail))
			}
			break
		}

		// Process recording files
		for _, recordingFile := range recording.RecordingFiles {
			// Check limit again
			if p.config.Limit > 0 && processedCount >= p.config.Limit {
				break
			}

			// Skip if no download URL
			if recordingFile.DownloadURL == "" {
				continue
			}

			// Skip non-MP4 files unless we want all
			if recordingFile.FileType != "MP4" && !p.config.MetaOnly {
				continue
			}

			// Process this recording file
			fileResult := p.processRecordingFile(ctx, zoomEmail, boxEmail, recording, recordingFile)

			// Update counters
			if fileResult.Downloaded {
				result.DownloadedCount++
			}
			if fileResult.Uploaded {
				result.UploadedCount++
			}
			if fileResult.Skipped {
				result.SkippedCount++
			}
			if fileResult.Deleted {
				result.DeletedCount++
			}
			if fileResult.Error != nil {
				result.ErrorCount++
				result.Errors = append(result.Errors, fileResult.Error)

				// Stop processing this user if not continuing on error
				if !p.config.ContinueOnError {
					result.Duration = time.Since(startTime)
					return result, fileResult.Error
				}
			}

			processedCount++
		}
	}

	result.Duration = time.Since(startTime)

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Completed processing user %s: %d downloaded, %d uploaded, %d skipped, %d deleted, %d errors in %v",
			zoomEmail, result.DownloadedCount, result.UploadedCount, result.SkippedCount, result.DeletedCount, result.ErrorCount, result.Duration))
	}

	// Upload the user's uploads.csv to their Box zoom folder if Box is enabled and uploads occurred
	if p.config.BoxEnabled && p.boxUploadManager != nil && result.UploadedCount > 0 {
		if err := p.uploadUserCSVToBox(ctx, zoomEmail, boxEmail); err != nil {
			if logger != nil {
				logger.WarnWithContext(ctx, fmt.Sprintf("Failed to upload uploads.csv to Box for user %s: %v", zoomEmail, err))
			}
			// Don't fail the entire user processing if CSV upload fails
		}
	}

	return result, nil
}

// recordingFileResult represents the result of processing a single recording file
type recordingFileResult struct {
	Downloaded bool
	Uploaded   bool
	Skipped    bool
	Deleted    bool
	Error      error
}

// processRecordingFile processes a single recording file (download, upload, delete)
func (p *userProcessorImpl) processRecordingFile(ctx context.Context, zoomEmail, boxEmail string, recording *zoom.Recording, recordingFile zoom.RecordingFile) *recordingFileResult {
	result := &recordingFileResult{}
	logger := logging.GetDefaultLogger()

	// Extract username from Box email for directory structure
	username := email.ExtractUsername(boxEmail)
	if username == "" {
		result.Error = fmt.Errorf("invalid box email format: %s", boxEmail)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result
	}

	// Create directory path
	meetingTime := recording.StartTime
	dirPath := filepath.Join(p.config.BaseDownloadDir, username,
		fmt.Sprintf("%04d", meetingTime.Year()),
		fmt.Sprintf("%02d", int(meetingTime.Month())),
		fmt.Sprintf("%02d", meetingTime.Day()))

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create directory %s: %w", dirPath, err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result
	}

	// Generate filename
	meetingFileName := p.filenameSanitizer.SanitizeTopic(recording.Topic)
	timeStr := p.filenameSanitizer.FormatTime(meetingTime)
	filename := fmt.Sprintf("%s-%s.%s", meetingFileName, timeStr, strings.ToLower(recordingFile.FileType))
	filePath := filepath.Join(dirPath, filename)

	// Check if file already exists locally
	if _, err := os.Stat(filePath); err == nil {
		if p.config.Verbose && logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Skipped (already exists locally): %s", filename))
		}
		result.Skipped = true
		return result
	}

	// Skip if meta-only mode and this is not a metadata file
	if p.config.MetaOnly && recordingFile.FileType == "MP4" {
		if p.config.Verbose && logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Skipped (meta-only mode): %s", filename))
		}
		result.Skipped = true
		return result
	}

	// Skip download if dry run
	if p.config.DryRun {
		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Would download: %s", filename))
		}
		result.Downloaded = true
		return result
	}

	// Start timing the total process (download + upload)
	processingStartTime := time.Now()

	// Prepare download URL and headers with access token if available
	downloadURL := recordingFile.DownloadURL
	headers := make(map[string]string)

	// Add download access token as Authorization Bearer header (not query parameter)
	// This prevents file size limitations that occur when using query parameter tokens
	// Use download_access_token if available, otherwise fall back to OAuth token
	if recording.DownloadAccessToken != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", recording.DownloadAccessToken)
	} else {
		// Fall back to OAuth access token if download_access_token is not available
		// This happens when "View the recording content" permission is not enabled
		oauthToken, err := p.zoomClient.GetOAuthAccessToken(ctx)
		if err != nil {
			result.Error = fmt.Errorf("failed to get access token for download: %w", err)
			return result
		}
		headers["Authorization"] = oauthToken
	}

	// Download the file
	downloadReq := download.DownloadRequest{
		ID:          fmt.Sprintf("%s-%s", recording.UUID, recordingFile.ID),
		URL:         downloadURL,
		Destination: filePath,
		FileSize:    recordingFile.FileSize,
		Headers:     headers,
		Metadata: map[string]interface{}{
			"user_email":    zoomEmail,
			"meeting_id":    recording.UUID,
			"meeting_topic": recording.Topic,
			"file_type":     recordingFile.FileType,
			"filename":      filename,
		},
	}

	downloadResult, err := p.downloadManager.Download(ctx, downloadReq, nil)
	if err != nil {
		result.Error = fmt.Errorf("download failed for %s: %w", filename, err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result
	}

	result.Downloaded = true
	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Downloaded: %s (%d bytes)", filename, downloadResult.BytesDownloaded))
	}

	// Upload to Box if enabled
	if p.config.BoxEnabled && p.boxUploadManager != nil {
		// Upload the main file WITHOUT tracking yet (we'll track after we know the total time)
		uploadResult, uploadErr := p.uploadToBoxWithoutTracking(ctx, filePath, zoomEmail, boxEmail, recordingFile.FileType, meetingTime)

		// Calculate processing time AFTER the main file upload completes
		// This captures only the download + upload time for the main recording file (excluding metadata operations)
		processingTime := time.Since(processingStartTime)

		if uploadErr != nil {
			result.Error = uploadErr
			// Don't delete file if upload failed
			return result
		}

		if uploadResult.Skipped {
			result.Skipped = true
		} else {
			result.Uploaded = true
		}

		// Now track the upload with the accurate processing time
		p.boxUploadManager.TrackUploadWithTime(zoomEmail, filename, recordingFile.FileSize, time.Now(), processingTime)

		// Save and upload metadata file AFTER tracking the main file (for MP4 files only)
		if recordingFile.FileType == "MP4" {
			metadataFilename := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json"
			metadataPath := filepath.Join(dirPath, metadataFilename)

			// Save metadata file if it doesn't exist
			if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
				if err := saveRecordingMetadata(ctx, recording, &recordingFile, metadataPath); err != nil {
					if logger != nil {
						logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to save metadata %s: %v", metadataFilename, err))
					}
					// Don't fail the entire operation if metadata save fails
				}
			}
		}

		// Upload metadata file to Box if this is an MP4 file
		if recordingFile.FileType == "MP4" {
			metadataFilename := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json"
			metadataPath := filepath.Join(dirPath, metadataFilename)

			// Check if metadata file exists before uploading
			if _, err := os.Stat(metadataPath); err == nil {
				// Get file size for metadata
				metadataFileInfo, _ := os.Stat(metadataPath)
				metadataFileSize := int64(0)
				if metadataFileInfo != nil {
					metadataFileSize = metadataFileInfo.Size()
				}

				// Use zero processing time for metadata files since they're not part of the main recording
				metadataUploadResult, metadataUploadErr := p.uploadToBox(ctx, metadataPath, boxEmail, "JSON", meetingTime, 0, zoomEmail, metadataFilename, metadataFileSize)
				if metadataUploadErr != nil {
					if logger != nil {
						logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to upload metadata to Box: %s - %v", metadataFilename, metadataUploadErr))
					}
					// Don't fail the entire operation if metadata upload fails
				} else if metadataUploadResult.Uploaded || metadataUploadResult.Skipped {
					if metadataUploadResult.Uploaded && logger != nil {
						logger.InfoWithContext(ctx, fmt.Sprintf("Uploaded metadata to Box: %s", metadataFilename))
					}
					// Delete metadata file after successful upload or if already in Box (if configured)
					if p.config.DeleteAfterUpload {
						if err := os.Remove(metadataPath); err != nil {
							if logger != nil {
								logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to delete metadata after upload: %s - %v", metadataPath, err))
							}
						} else if logger != nil {
							logger.InfoWithContext(ctx, fmt.Sprintf("Deleted local metadata after upload: %s", metadataFilename))
						}
					}
				}
			}
		}

		// Delete local file after successful upload or if it was skipped (already in Box)
		if p.config.DeleteAfterUpload && (uploadResult.Uploaded || uploadResult.Skipped) {
			if err := os.Remove(filePath); err != nil {
				if logger != nil {
					logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to delete file after upload: %s - %v", filePath, err))
				}
			} else {
				result.Deleted = true
				if logger != nil {
					logger.InfoWithContext(ctx, fmt.Sprintf("Deleted local file after upload: %s", filename))
				}
			}
		}
	}

	return result
}

// uploadResult represents the result of a Box upload
type uploadResult struct {
	Uploaded bool
	Skipped  bool
	Error    error
}

// uploadToBoxWithoutTracking uploads a file to Box without tracking (tracking done by caller)
func (p *userProcessorImpl) uploadToBoxWithoutTracking(ctx context.Context, localPath, zoomEmail, boxEmail, fileType string, recordingTime time.Time) (*uploadResult, error) {
	logger := logging.GetDefaultLogger()
	result := &uploadResult{}

	// Get Box client from upload manager
	boxClient := p.boxUploadManager.GetBoxClient()

	// Find the user's zoom folder in Box using their email
	zoomFolder, err := boxClient.FindZoomFolderByOwner(boxEmail)
	if err != nil {
		result.Error = fmt.Errorf("failed to find zoom folder for user %s: %w", boxEmail, err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result, result.Error
	}

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Found zoom folder for %s: %s", boxEmail, zoomFolder.ID))
	}

	// Set the upload manager's base folder to the user's zoom folder
	p.boxUploadManager.SetBaseFolderID(zoomFolder.ID)

	// Use recording time (from Zoom metadata) to create folder structure
	folderPath := fmt.Sprintf("%04d/%02d/%02d",
		recordingTime.Year(),
		int(recordingTime.Month()),
		recordingTime.Day())

	// Create/get the folder structure using the user's zoom folder as parent
	folder, err := box.CreateFolderPath(boxClient, folderPath, zoomFolder.ID)
	if err != nil {
		result.Error = fmt.Errorf("failed to create Box folder structure: %w", err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result, result.Error
	}

	baseFileName := filepath.Base(localPath)

	// Check if file already exists in Box (check-before-upload)
	existingFile, err := boxClient.FindFileByName(folder.ID, baseFileName)
	if err == nil && existingFile != nil {
		// File already exists in Box - skip upload (tracking done by caller)
		result.Skipped = true
		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Skipped Box upload (file already exists): %s", baseFileName))
		}
		return result, nil
	}

	// File doesn't exist - proceed with upload (without tracking - tracking done by caller)
	uploadResult, err := p.boxUploadManager.UploadFileWithEmailMapping(ctx, localPath, zoomEmail, boxEmail, fmt.Sprintf("upload-%s", baseFileName), nil)
	if err != nil {
		result.Error = fmt.Errorf("Box upload failed for %s: %w", baseFileName, err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result, result.Error
	}

	result.Uploaded = true
	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Uploaded to Box: %s (file ID: %s)", baseFileName, uploadResult.FileID))
	}

	return result, nil
}

// uploadToBox uploads a file to Box with check-before-upload logic (kept for metadata uploads)
// Uses the recording time (from Zoom metadata) to determine the Box folder structure
func (p *userProcessorImpl) uploadToBox(ctx context.Context, localPath, boxEmail, fileType string, recordingTime time.Time, processingTime time.Duration, zoomEmail, fileName string, fileSize int64) (*uploadResult, error) {
	logger := logging.GetDefaultLogger()
	result := &uploadResult{}

	// Get Box client from upload manager
	boxClient := p.boxUploadManager.GetBoxClient()

	// Find the user's zoom folder in Box using their email
	zoomFolder, err := boxClient.FindZoomFolderByOwner(boxEmail)
	if err != nil {
		result.Error = fmt.Errorf("failed to find zoom folder for user %s: %w", boxEmail, err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result, result.Error
	}

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Found zoom folder for %s: %s", boxEmail, zoomFolder.ID))
	}

	// Set the upload manager's base folder to the user's zoom folder
	// This ensures files are uploaded to: zoomFolder/<year>/<month>/<day>/
	p.boxUploadManager.SetBaseFolderID(zoomFolder.ID)

	// Use recording time (from Zoom metadata) to create folder structure
	// Create folder path: <year>/<month>/<day> (within user's zoom folder)
	folderPath := fmt.Sprintf("%04d/%02d/%02d",
		recordingTime.Year(),
		int(recordingTime.Month()),
		recordingTime.Day())

	// Create/get the folder structure using the user's zoom folder as parent
	folder, err := box.CreateFolderPath(boxClient, folderPath, zoomFolder.ID)
	if err != nil {
		result.Error = fmt.Errorf("failed to create Box folder structure: %w", err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result, result.Error
	}

	baseFileName := filepath.Base(localPath)

	// Check if file already exists in Box (check-before-upload)
	existingFile, err := boxClient.FindFileByName(folder.ID, baseFileName)
	if err == nil && existingFile != nil {
		// File already exists in Box - skip upload but still track it with processing time
		result.Skipped = true
		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Skipped Box upload (file already exists): %s", baseFileName))
		}

		// Track the skipped upload with processing time
		p.boxUploadManager.TrackUploadWithTime(zoomEmail, fileName, fileSize, time.Now(), processingTime)

		return result, nil
	}

	// File doesn't exist - proceed with upload
	// The upload manager will use the baseFolderID (zoomFolder.ID) we set above
	uploadResult, err := p.boxUploadManager.UploadFileWithEmailMappingWithTime(ctx, localPath, zoomEmail, boxEmail, fmt.Sprintf("upload-%s", baseFileName), nil, processingTime, zoomEmail, fileSize)
	if err != nil {
		result.Error = fmt.Errorf("Box upload failed for %s: %w", baseFileName, err)
		if logger != nil {
			logger.ErrorWithContext(ctx, result.Error.Error())
		}
		return result, result.Error
	}

	result.Uploaded = true
	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Uploaded to Box: %s (file ID: %s)", baseFileName, uploadResult.FileID))
	}

	return result, nil
}

// ProcessAllUsers processes all incomplete users from the active users file
func (p *userProcessorImpl) ProcessAllUsers(ctx context.Context, usersFile *users.ActiveUsersFile) (*ProcessorSummary, error) {
	startTime := time.Now()
	logger := logging.GetDefaultLogger()

	summary := &ProcessorSummary{
		UserResults: make([]*ProcessorResult, 0),
	}

	// Get incomplete users
	incompleteUsers := usersFile.GetIncompleteUsers()
	summary.TotalUsers = len(incompleteUsers)

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Processing %d incomplete users", summary.TotalUsers))
	}

	// Process each user serially
	for _, userEntry := range incompleteUsers {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		default:
		}

		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Processing user: %s → %s", userEntry.ZoomEmail, userEntry.BoxEmail))
		}

		// Process the user
		userResult, err := p.ProcessUser(ctx, userEntry.ZoomEmail, userEntry.BoxEmail)
		summary.UserResults = append(summary.UserResults, userResult)

		// Update summary counters
		summary.TotalDownloads += userResult.DownloadedCount
		summary.TotalUploads += userResult.UploadedCount
		summary.TotalSkipped += userResult.SkippedCount
		summary.TotalErrors += userResult.ErrorCount
		summary.TotalDeleted += userResult.DeletedCount

		if err != nil || userResult.ErrorCount > 0 {
			summary.FailedUsers++

			// Stop processing if not continuing on error
			if !p.config.ContinueOnError {
				summary.Duration = time.Since(startTime)
				return summary, fmt.Errorf("user processing failed for %s: %w", userEntry.ZoomEmail, err)
			}

			// Mark upload_complete as false (user had errors)
			if markErr := usersFile.UpdateUserStatus(userEntry.ZoomEmail, false); markErr != nil {
				if logger != nil {
					logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to update user status for %s: %v", userEntry.ZoomEmail, markErr))
				}
			}
		} else {
			summary.ProcessedUsers++

			// Mark user as complete
			if err := usersFile.MarkUserComplete(userEntry.ZoomEmail); err != nil {
				if logger != nil {
					logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to mark user complete %s: %v", userEntry.ZoomEmail, err))
				}
			} else {
				if logger != nil {
					logger.InfoWithContext(ctx, fmt.Sprintf("Marked user complete: %s", userEntry.ZoomEmail))
				}
			}
		}
	}

	summary.Duration = time.Since(startTime)

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Completed processing all users: %d processed, %d failed, %d total downloads, %d total uploads, %d total deleted in %v",
			summary.ProcessedUsers, summary.FailedUsers, summary.TotalDownloads, summary.TotalUploads, summary.TotalDeleted, summary.Duration))
	}

	return summary, nil
}

// uploadUserCSVToBox uploads the user's uploads.csv file to their Box zoom folder
func (p *userProcessorImpl) uploadUserCSVToBox(ctx context.Context, zoomEmail, boxEmail string) error {
	logger := logging.GetDefaultLogger()

	// Extract username from Box email
	username := email.ExtractUsername(boxEmail)
	if username == "" {
		return fmt.Errorf("invalid box email format: %s", boxEmail)
	}

	// Construct path to the uploads.csv file
	userDir := filepath.Join(p.config.BaseDownloadDir, username)
	csvFilePath := filepath.Join(userDir, "uploads.csv")

	// Check if the CSV file exists
	if _, err := os.Stat(csvFilePath); os.IsNotExist(err) {
		// CSV file doesn't exist, nothing to upload
		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("No uploads.csv found for user %s, skipping upload to Box", zoomEmail))
		}
		return nil
	}

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Uploading uploads.csv to Box for user %s", zoomEmail))
	}

	// Get the base folder ID (should be the user's zoom folder)
	baseFolderID := p.boxUploadManager.GetBaseFolderID()
	if baseFolderID == "" || baseFolderID == box.RootFolderID {
		return fmt.Errorf("base folder ID not set for Box uploads")
	}

	// Upload the CSV file to the zoom folder root (not in date subfolders)
	boxClient := p.boxUploadManager.GetBoxClient()
	if boxClient == nil {
		return fmt.Errorf("box client not available")
	}

	// Upload the file
	file, err := boxClient.UploadFileWithProgress(csvFilePath, baseFolderID, "uploads.csv", nil)
	if err != nil {
		return fmt.Errorf("failed to upload uploads.csv: %w", err)
	}

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Successfully uploaded uploads.csv to Box for user %s (file ID: %s)", zoomEmail, file.ID))
	}

	return nil
}

// Helper functions

// saveRecordingMetadata saves the recording metadata as a JSON file
// This includes both the meeting/recording details and the specific file information
func saveRecordingMetadata(ctx context.Context, recording *zoom.Recording, recordingFile *zoom.RecordingFile, metadataPath string) error {
	logger := logging.GetDefaultLogger()

	// Create metadata structure that combines recording and file details
	metadata := map[string]interface{}{
		"meeting": map[string]interface{}{
			"uuid":       recording.UUID,
			"id":         recording.ID,
			"account_id": recording.AccountID,
			"host_id":    recording.HostID,
			"topic":      recording.Topic,
			"type":       recording.Type,
			"start_time": recording.StartTime,
			"duration":   recording.Duration,
			"total_size": recording.TotalSize,
		},
		"recording_file": map[string]interface{}{
			"id":              recordingFile.ID,
			"meeting_id":      recordingFile.MeetingID,
			"recording_start": recordingFile.RecordingStart,
			"recording_end":   recordingFile.RecordingEnd,
			"file_type":       recordingFile.FileType,
			"file_extension":  recordingFile.FileExtension,
			"file_size":       recordingFile.FileSize,
			"download_url":    recordingFile.DownloadURL,
			"play_url":        recordingFile.PlayURL,
			"status":          recordingFile.Status,
			"recording_type":  recordingFile.RecordingType,
		},
	}

	// Marshal to JSON with pretty printing
	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recording metadata: %w", err)
	}

	// Write the JSON data to file
	if err := os.WriteFile(metadataPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file %s: %w", metadataPath, err)
	}

	if logger != nil {
		logger.InfoWithContext(ctx, fmt.Sprintf("Saved metadata: %s", filepath.Base(metadataPath)))
	}

	return nil
}

// getFromDate returns the start date for fetching recordings (2020-06-30)
func getFromDate() *time.Time {
	from := time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC)
	return &from
}

// getToDate returns the end date for fetching recordings (today)
func getToDate() *time.Time {
	to := time.Now()
	return &to
}
