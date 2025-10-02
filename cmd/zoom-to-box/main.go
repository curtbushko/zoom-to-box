package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/curtbushko/zoom-to-box/internal/config"
	"github.com/curtbushko/zoom-to-box/internal/directory"
	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/email"
	"github.com/curtbushko/zoom-to-box/internal/filename"
	"github.com/curtbushko/zoom-to-box/internal/logging"
	"github.com/curtbushko/zoom-to-box/internal/progress"
	"github.com/curtbushko/zoom-to-box/internal/users"
	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

var (
	// Version information - will be set during build
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
	
	// Global flags
	configFile  string
	outputDir   string
	verbose     bool
	dryRun      bool
	metaOnly    bool
	limit       int
	noProgress  bool
	compactMode bool
	zoomUser    string
	boxUser     string
)

// SingleUserConfig holds configuration for single user mode
type SingleUserConfig struct {
	Enabled   bool
	ZoomEmail string
	BoxEmail  string
}

// buildRootCommand creates and configures the root command
func buildRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "zoom-to-box",
		Short: "A CLI tool to download Zoom cloud recordings",
		Long: `zoom-to-box is a CLI tool that connects to the Zoom API 
and downloads video files with metadata, supporting resume functionality 
and Box integration.

This tool helps you:
- Download Zoom cloud recordings with proper file organization
- Resume interrupted downloads automatically  
- Save metadata in JSON format alongside recordings
- Upload recordings to Box (optional)
- Manage downloads with configurable retry logic`,
		Run: func(cmd *cobra.Command, args []string) {
			// Check if configuration exists and provide helpful guidance
			configPath := "config.yaml"
			if configFile != "" {
				configPath = configFile
			}

			// Try to load configuration to provide helpful feedback
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				cmd.Printf("⚠️  Configuration Issue Detected\n\n")
				
				// Check if it's a file not found error (check the error string since the error is wrapped)
				if strings.Contains(err.Error(), "no such file or directory") || strings.Contains(err.Error(), "cannot find the file") || strings.Contains(err.Error(), "failed to read config file") {
					cmd.Printf("Configuration file '%s' not found.\n\n", configPath)
					cmd.Printf("To get started:\n")
					cmd.Printf("1. Run 'zoom-to-box config' to see configuration structure\n")
					cmd.Printf("2. Copy config.example.yaml to config.yaml\n")
					cmd.Printf("3. Edit config.yaml with your Zoom credentials\n")
					cmd.Printf("4. Run 'zoom-to-box' to start downloading\n\n")
				} else {
					cmd.Printf("Configuration error: %v\n\n", err)
					cmd.Printf("To fix this:\n")
					cmd.Printf("1. Run 'zoom-to-box config' to see the correct configuration structure\n")
					cmd.Printf("2. Check your config file for syntax errors or missing required fields\n")
					cmd.Printf("3. Ensure all required Zoom API credentials are provided\n\n")
				}

				// Check environment variables as an alternative
				hasEnvCreds := os.Getenv("ZOOM_ACCOUNT_ID") != "" && 
							  os.Getenv("ZOOM_CLIENT_ID") != "" && 
							  os.Getenv("ZOOM_CLIENT_SECRET") != ""
				
				if hasEnvCreds {
					cmd.Printf("✅ Zoom credentials found in environment variables.\n")
					cmd.Printf("You can run 'zoom-to-box' without a config file.\n\n")
				} else {
					cmd.Printf("💡 Alternative: Set environment variables instead of using a config file:\n")
					cmd.Printf("   export ZOOM_ACCOUNT_ID='your-account-id'\n")
					cmd.Printf("   export ZOOM_CLIENT_ID='your-client-id'\n")
					cmd.Printf("   export ZOOM_CLIENT_SECRET='your-client-secret'\n\n")
				}

				cmd.Printf("For detailed help: zoom-to-box config\n")
				cmd.Printf("For general usage: zoom-to-box --help\n")
				return
			}

			// Configuration loaded successfully - now run the download operation
			ctx := context.Background()
			if err := runDownloadWithProgress(ctx, cmd, cfg); err != nil {
				cmd.Printf("❌ Download failed: %v\n", err)
				os.Exit(1)
			}
		},
	}

	// Add subcommands
	rootCmd.AddCommand(createVersionCommand())
	rootCmd.AddCommand(createConfigCommand())

	// Global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "configuration file path (default: config.yaml)")
	rootCmd.PersistentFlags().StringVar(&outputDir, "output-dir", "", "base download directory (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be downloaded without downloading")
	rootCmd.PersistentFlags().BoolVar(&metaOnly, "meta-only", false, "download only JSON metadata files")
	rootCmd.PersistentFlags().IntVar(&limit, "limit", 0, "limit processing to N recordings (0 = no limit)")
	rootCmd.PersistentFlags().BoolVar(&noProgress, "no-progress", false, "disable progress bars and real-time updates")
	rootCmd.PersistentFlags().BoolVar(&compactMode, "compact", false, "use compact progress display")
	rootCmd.PersistentFlags().StringVar(&zoomUser, "zoom-user", "", "process recordings for specific Zoom user email")
	rootCmd.PersistentFlags().StringVar(&boxUser, "box-user", "", "corresponding Box user email for uploads (requires --zoom-user)")

	// Add flag validation
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if limit < 0 {
			return fmt.Errorf("limit must be a positive number or 0, got: %d", limit)
		}
		
		// Validate single user flags
		if (zoomUser != "" && boxUser == "") || (zoomUser == "" && boxUser != "") {
			return fmt.Errorf("both --zoom-user and --box-user must be provided together")
		}
		
		// Validate email format for zoom-user
		if zoomUser != "" && !isValidEmail(zoomUser) {
			return fmt.Errorf("invalid email format for --zoom-user: %s", zoomUser)
		}
		
		// Validate email format for box-user
		if boxUser != "" && !isValidEmail(boxUser) {
			return fmt.Errorf("invalid email format for --box-user: %s", boxUser)
		}
		
		return nil
	}

	return rootCmd
}

// createVersionCommand creates the version subcommand
func createVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  "Display version, commit, and build information for zoom-to-box",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("zoom-to-box version %s\n", version)
			cmd.Printf("Commit: %s\n", commit)
			cmd.Printf("Build date: %s\n", buildDate)
		},
	}
}

// createConfigCommand creates the config help subcommand
func createConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show configuration file structure and examples",
		Long:  "Display the required configuration file structure, authentication methods, environment variables, and comprehensive examples",
		Run: func(cmd *cobra.Command, args []string) {
			configHelp := `Configuration File Structure (config.yaml):

ZOOM API CONFIGURATION (Required):
=================================
zoom:
  account_id: "your_zoom_account_id"       # Zoom Account ID from Server-to-Server OAuth app
  client_id: "your_zoom_client_id"         # Client ID from Server-to-Server OAuth app  
  client_secret: "your_zoom_client_secret" # Client Secret from Server-to-Server OAuth app
  base_url: "https://api.zoom.us/v2"       # Zoom API base URL (default: https://api.zoom.us/v2)

# REQUIRED SCOPES: recording:read, user:read, meeting:read
# Uses Server-to-Server OAuth (account-level access, no user tokens needed)

DOWNLOAD CONFIGURATION:
======================
download:
  output_dir: "./downloads"        # Local download directory (default: ./downloads)
  concurrent_limit: 3              # Max concurrent downloads (default: 3, range: 1-10)
  retry_attempts: 3                # Max retry attempts for failed downloads (default: 3)
  timeout_seconds: 300             # Download timeout in seconds (default: 300 = 5 minutes)

LOGGING CONFIGURATION:
=====================
logging:
  level: "info"                    # Log level: debug, info, warn, error (default: info)
  file: "./zoom-downloader.log"    # Log file path (default: ./zoom-downloader.log)
  console: true                    # Enable console output (default: true)
  json_format: false               # Use JSON log format (default: false)

BOX INTEGRATION (Optional):
==========================
box:
  enabled: false                   # Enable Box uploads (default: false)
  client_id: "your_box_client_id"  # Box OAuth 2.0 client ID
  client_secret: "your_box_client_secret" # Box OAuth 2.0 client secret
  # Note: Files are uploaded to user-specific folders within the service account's root folder

ACTIVE USERS FILTERING (Optional):
=================================
active_users:
  file: "./active_users.txt"       # Path to active users list file
  check_enabled: true              # Enable user filtering (default: true)

# Active users file format (one email per line):
# john.doe@company.com
# jane.smith@company.com
# # Lines starting with # are comments
# admin@company.com
#
# For different Zoom and Box emails, use comma separation:
# john.doe@zoomaccount.com,john.doe@company.com
# admin@zoomaccount.com,admin@company.com

ENVIRONMENT VARIABLES:
=====================

Required Zoom API credentials (override config file):
  ZOOM_ACCOUNT_ID     - Your Zoom account ID
  ZOOM_CLIENT_ID      - Your Zoom OAuth app client ID
  ZOOM_CLIENT_SECRET  - Your Zoom OAuth app client secret
  ZOOM_BASE_URL       - Zoom API base URL (optional)

Optional Box integration:
  BOX_CLIENT_ID     - Box OAuth 2.0 client ID
  BOX_CLIENT_SECRET - Box OAuth 2.0 client secret

Other settings:
  DOWNLOAD_OUTPUT_DIR  - Base download directory

AUTHENTICATION METHODS:
======================

1. Server-to-Server OAuth (Recommended):
   - Account-level access to all users and recordings
   - No user consent required
   - Uses JWT-based authentication
   - Required scopes: recording:read, user:read, meeting:read

2. Environment Variables:
   - Set ZOOM_ACCOUNT_ID, ZOOM_CLIENT_ID, ZOOM_CLIENT_SECRET
   - Overrides any values in config.yaml
   - Useful for CI/CD and containerized deployments

EXAMPLE USAGE:
=============

1. Using configuration file:
   cp config.example.yaml config.yaml
   # Edit config.yaml with your credentials
   zoom-to-box

2. Using environment variables:
   export ZOOM_ACCOUNT_ID="your-account-id"
   export ZOOM_CLIENT_ID="your-client-id"
   export ZOOM_CLIENT_SECRET="your-client-secret"
   zoom-to-box

3. With additional options:
   zoom-to-box --limit 10 --meta-only --verbose
   zoom-to-box --output-dir ./recordings --dry-run

4. Single user processing:
   zoom-to-box --zoom-user=john.doe@company.com --box-user=john.doe@company.com
   zoom-to-box --zoom-user=john.doe@zoomaccount.com --box-user=john.doe@company.com --limit=5

5. Box integration:
   # Set Box OAuth 2.0 credentials in config.yaml or environment variables
   # Enable in config.yaml: box.enabled = true
   export BOX_CLIENT_ID="your_box_client_id"
   export BOX_CLIENT_SECRET="your_box_client_secret"
   zoom-to-box --config config.yaml

DIRECTORY STRUCTURE:
==================
Downloaded files are organized as:
downloads/
├── user.email/
│   └── YYYY/
│       └── MM/
│           └── DD/
│               ├── meeting-topic-HHMM.mp4
│               └── meeting-topic-HHMM.json

Box uploads are organized as:
<service-account-root>/
├── username/
│   └── YYYY/
│       └── MM/
│           └── DD/
│               ├── meeting-topic-HHMM.mp4
│               └── meeting-topic-HHMM.json

TROUBLESHOOTING:
===============
- Ensure your Zoom app has Server-to-Server OAuth enabled
- Verify required scopes are granted: recording:read, user:read, meeting:read
- Check account_id matches your Zoom account (not user ID)
- For Box integration, ensure OAuth 2.0 client credentials are valid

For more information, visit: https://github.com/curtbushko/zoom-to-box
`
			cmd.Print(configHelp)
		},
	}
}

// runDownloadWithProgress executes the download operation with progress reporting
func runDownloadWithProgress(ctx context.Context, cmd *cobra.Command, cfg *config.Config) error {
	// Initialize logging first
	if err := logging.InitializeLogging(cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}
	defer func() {
		if logger := logging.GetDefaultLogger(); logger != nil {
			logger.Close()
		}
	}()

	logger := logging.GetDefaultLogger()

	// Apply command-line overrides to config
	if outputDir != "" {
		cfg.Download.OutputDir = outputDir
	}

	// Handle single user mode
	singleUserConfig := SingleUserConfig{
		Enabled:   zoomUser != "" && boxUser != "",
		ZoomEmail: zoomUser,
		BoxEmail:  boxUser,
	}
	
	if singleUserConfig.Enabled {
		// Log single user mode activation
		if logger != nil {
			logger.InfoWithContext(ctx, "Single user mode activated")
			logger.LogUserAction("single_user_mode", singleUserConfig.ZoomEmail, map[string]interface{}{
				"zoom_email": singleUserConfig.ZoomEmail,
				"box_email":  singleUserConfig.BoxEmail,
			})
		}
		
		// In single user mode, we bypass active user list checking
		cmd.Printf("🎯 Single user mode: processing %s → %s\n", singleUserConfig.ZoomEmail, singleUserConfig.BoxEmail)
	}

	// Create progress configuration based on CLI flags
	progressConfig := progress.NewProgressConfigBuilder().
		WithVerbose(verbose).
		WithCompactMode(compactMode).
		WithFileLogging(cfg.Logging.File != "").
		Build()

	// Disable progress bar if requested or in dry-run mode
	if noProgress || dryRun {
		progressConfig.ShowProgressBar = false
	}

	// Create progress reporter
	var reporter progress.ProgressReporter
	if dryRun {
		// For dry run, just show what would be downloaded without progress tracking
		cmd.Printf("🔍 DRY RUN: Showing what would be downloaded (no files will be saved)\n\n")
		
		// Create a minimal reporter for dry run
		progressConfig.ShowProgressBar = false
		progressConfig.EnableFileLogging = false
		reporter = progress.NewProgressReporter(progressConfig, logger)
	} else {
		// Create full progress reporter with logging integration
		baseReporter := progress.NewProgressReporter(progressConfig, logger)
		reporter = progress.NewLoggingProgressReporter(baseReporter, logger)
	}

	// Start progress tracking
	totalItems := estimateTotalItems(cfg, limit)
	if err := reporter.Start(ctx, totalItems); err != nil {
		return fmt.Errorf("failed to start progress tracking: %w", err)
	}

	// Log session start
	if logger != nil {
		logger.InfoWithContext(ctx, "Starting zoom-to-box download session")
		sessionInfo := map[string]interface{}{
			"total_estimated": totalItems,
			"meta_only":       metaOnly,
			"limit":           limit,
			"dry_run":         dryRun,
			"verbose":         verbose,
			"output_dir":      cfg.Download.OutputDir,
			"single_user_mode": singleUserConfig.Enabled,
		}
		
		if singleUserConfig.Enabled {
			sessionInfo["single_zoom_email"] = singleUserConfig.ZoomEmail
			sessionInfo["single_box_email"] = singleUserConfig.BoxEmail
		}
		
		logger.LogUserAction("session_start", "cli", sessionInfo)
	}

	// Execute real download operations
	if err := performDownloads(ctx, cfg, reporter, singleUserConfig); err != nil {
		return fmt.Errorf("download operation failed: %w", err)
	}

	// Finish progress tracking and show summary
	summary := reporter.Finish()

	// Display results based on actual progress
	if dryRun {
		cmd.Printf("\n🔍 DRY RUN COMPLETED\n")
		
		// Check if there were errors that prevented processing
		if len(summary.ErrorItems) > 0 && summary.CompletedDownloads == 0 {
			cmd.Printf("❌ No recordings could be processed due to errors\n")
			cmd.Printf("Errors encountered: %d\n", len(summary.ErrorItems))
			if verbose {
				for _, errorItem := range summary.ErrorItems {
					cmd.Printf("  - %s: %s\n", errorItem.Item, errorItem.ErrorMsg)
				}
			}
		} else {
			cmd.Printf("Would have processed %d recordings\n", summary.CompletedDownloads+len(summary.SkippedItems))
			if metaOnly {
				cmd.Printf("Would have downloaded metadata files only\n")
			}
		}
	} else {
		// Check if download was actually successful
		if len(summary.ErrorItems) > 0 && summary.CompletedDownloads == 0 {
			cmd.Printf("\n❌ DOWNLOAD FAILED\n")
			cmd.Printf("No recordings could be downloaded due to errors\n")
		} else {
			cmd.Printf("\n✅ DOWNLOAD COMPLETED\n")
		}
		
		// Show summary based on verbosity
		if verbose || summary.FailedDownloads > 0 || len(summary.ErrorItems) > 0 {
			showDetailedSummary(cmd, summary)
		}
	}

	return nil
}

// estimateTotalItems estimates the total number of items to process
func estimateTotalItems(cfg *config.Config, limitFlag int) int {
	// For now, return a reasonable estimate
	// In a future enhancement, this could query the API for accurate counts
	estimated := 50 // Default estimate
	
	if limitFlag > 0 && limitFlag < estimated {
		return limitFlag
	}
	
	return estimated
}

// performDownloads executes the real download process
func performDownloads(ctx context.Context, cfg *config.Config, reporter progress.ProgressReporter, singleUserConfig SingleUserConfig) error {
	logger := logging.GetDefaultLogger()
	
	// Initialize Zoom API client
	auth := zoom.NewServerToServerAuth(cfg.Zoom)
	httpConfig := zoom.HTTPClientConfigFromDownloadConfig(cfg.Download)
	retryClient := zoom.NewRetryHTTPClient(httpConfig)
	authRetryClient := zoom.NewAuthenticatedRetryClient(retryClient, auth)
	zoomClient := zoom.NewZoomClient(authRetryClient, cfg.Zoom.BaseURL)
	
	// Initialize download manager
	downloadManager := download.NewDownloadManager(download.DownloadConfig{
		ConcurrentLimit: cfg.Download.ConcurrentLimit,
		ChunkSize:       64 * 1024, // 64KB chunks
		RetryAttempts:   cfg.Download.RetryAttempts,
		RetryDelay:      1 * time.Second,
		UserAgent:       "zoom-to-box/1.0",
		Timeout:         cfg.Download.TimeoutDuration(),
	})
	
	// Initialize user manager 
	var userManager users.ActiveUserManager
	var err error
	if singleUserConfig.Enabled {
		// For single user mode, create a user manager that accepts all users (empty file path)
		userManager, err = users.NewActiveUserManager(users.ActiveUserConfig{
			FilePath:      "", // Empty path means all users are active
			CaseSensitive: false,
			WatchFile:     false,
		})
	} else {
		// For normal mode, use the configured active users file
		userManager, err = users.NewActiveUserManager(users.ActiveUserConfig{
			FilePath:      cfg.ActiveUsers.File,
			CaseSensitive: false,
			WatchFile:     false, // Disable watching for CLI execution
		})
	}
	if err != nil {
		return fmt.Errorf("failed to initialize user manager: %w", err)
	}
	defer userManager.Close()
	
	// Initialize directory manager
	dirConfig := directory.DirectoryConfig{
		BaseDirectory: cfg.Download.OutputDir,
		CreateDirs:    true,
	}
	dirManager := directory.NewDirectoryManager(dirConfig, userManager)
	
	// Initialize filename sanitizer
	filenameSanitizer := filename.NewFileSanitizer(filename.FileSanitizerOptions{})
	
	// Determine users to process
	var usersToProcess []string
	if singleUserConfig.Enabled {
		usersToProcess = []string{singleUserConfig.ZoomEmail}
		fmt.Printf("📋 Single user mode: Processing recordings for %s\n", singleUserConfig.ZoomEmail)
		if singleUserConfig.BoxEmail != singleUserConfig.ZoomEmail {
			fmt.Printf("📁 Box email mapping: %s → %s\n", singleUserConfig.ZoomEmail, singleUserConfig.BoxEmail)
		}
	} else {
		// Get active users from user manager
		if userManager != nil {
			usersToProcess = userManager.GetActiveUsers()
		}
		if len(usersToProcess) == 0 {
			return fmt.Errorf("no active users found to process")
		}
		fmt.Printf("📋 Processing recordings for %d users\n", len(usersToProcess))
	}
	
	// Process each user
	var totalProcessed int
	var usersWithErrors int
	for _, userEmail := range usersToProcess {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		if logger != nil {
			logger.InfoWithContext(ctx, fmt.Sprintf("Processing user: %s", userEmail))
		}
		
		// Get recordings for this user
		params := zoom.ListRecordingsParams{
			From:     getFromDate(),
			To:       getToDate(),
			PageSize: 300, // Maximum page size
		}
		
		recordings, err := zoomClient.GetAllUserRecordings(ctx, userEmail, params)
		if err != nil {
			if logger != nil {
				logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to get recordings for user %s: %v", userEmail, err))
			}
			reporter.AddError(userEmail, fmt.Errorf("failed to get recordings: %w", err), map[string]interface{}{
				"user_email": userEmail,
			})
			usersWithErrors++
			continue
		}
		
		// Process recordings for this user
		userProcessed, err := processUserRecordings(ctx, userEmail, recordings, cfg, reporter, downloadManager, dirManager, filenameSanitizer, singleUserConfig)
		if err != nil {
			if logger != nil {
				logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to process recordings for user %s: %v", userEmail, err))
			}
			reporter.AddError(userEmail, fmt.Errorf("failed to process recordings: %w", err), map[string]interface{}{
				"user_email": userEmail,
			})
			usersWithErrors++
			continue
		}
		
		totalProcessed += userProcessed
		
		// Check global limit
		if limit > 0 && totalProcessed >= limit {
			break
		}
	}
	
	// Return error if all users failed and no recordings were processed
	if usersWithErrors == len(usersToProcess) && totalProcessed == 0 {
		return fmt.Errorf("failed to process any users: all %d users encountered errors", len(usersToProcess))
	}
	
	return nil
}

// processUserRecordings processes all recordings for a single user
func processUserRecordings(ctx context.Context, userEmail string, recordings []*zoom.Recording, cfg *config.Config, reporter progress.ProgressReporter, downloadManager download.DownloadManager, dirManager directory.DirectoryManager, filenameSanitizer filename.FileSanitizer, singleUserConfig SingleUserConfig) (int, error) {
	logger := logging.GetDefaultLogger()
	processed := 0
	
	for _, recording := range recordings {
		select {
		case <-ctx.Done():
			return processed, ctx.Err()
		default:
		}
		
		// Check global limit
		if limit > 0 && processed >= limit {
			break
		}
		
		// Process each recording file in the meeting
		for _, recordingFile := range recording.RecordingFiles {
			// Skip if no download URL
			if recordingFile.DownloadURL == "" {
				continue
			}
			
			// Skip non-MP4 files unless we want all
			if recordingFile.FileType != "MP4" && !metaOnly {
				continue
			}
			
			// Get meeting date for directory structure
			meetingTime := recording.StartTime
			
			// Create directory path
			var dirPath string
			if singleUserConfig.Enabled {
				// For single user mode, create directory manually using Box email
				userDir := email.ExtractUsername(singleUserConfig.BoxEmail)
				if userDir == "" {
					if logger != nil {
						logger.ErrorWithContext(ctx, fmt.Sprintf("Invalid Box email format: %s", singleUserConfig.BoxEmail))
					}
					reporter.AddError(recording.Topic, fmt.Errorf("invalid Box email format: %s", singleUserConfig.BoxEmail), map[string]interface{}{
						"user_email": userEmail,
						"meeting_id": recording.UUID,
					})
					continue
				}
				dirPath = filepath.Join(cfg.Download.OutputDir, userDir, 
					fmt.Sprintf("%04d", meetingTime.Year()), 
					fmt.Sprintf("%02d", int(meetingTime.Month())), 
					fmt.Sprintf("%02d", meetingTime.Day()))
				
				// Create directory if it doesn't exist
				if err := os.MkdirAll(dirPath, 0755); err != nil {
					if logger != nil {
						logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to create directory for %s: %v", userEmail, err))
					}
					reporter.AddError(recording.Topic, fmt.Errorf("failed to create directory: %w", err), map[string]interface{}{
						"user_email": userEmail,
						"meeting_id": recording.UUID,
					})
					continue
				}
			} else {
				// For normal mode, use directory manager
				dirResult, err := dirManager.GenerateDirectory(userEmail, meetingTime)
				if err != nil {
					if logger != nil {
						logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to create directory for %s: %v", userEmail, err))
					}
					reporter.AddError(recording.Topic, fmt.Errorf("failed to create directory: %w", err), map[string]interface{}{
						"user_email": userEmail,
						"meeting_id": recording.UUID,
					})
					continue
				}
				dirPath = dirResult.FullPath
			}
			
			// Generate filename
			meetingFileName := filenameSanitizer.SanitizeTopic(recording.Topic)
			timeStr := filenameSanitizer.FormatTime(meetingTime)
			filename := fmt.Sprintf("%s-%s.%s", meetingFileName, timeStr, strings.ToLower(recordingFile.FileType))
			filePath := filepath.Join(dirPath, filename)
			
			// Check if file already exists
			if _, err := os.Stat(filePath); err == nil {
				reporter.AddSkipped(progress.SkipReasonAlreadyExists, filename, map[string]interface{}{
					"user_email": userEmail,
					"file_path":  filePath,
				})
				continue
			}
			
			// Skip if meta-only mode and this is not a metadata file
			if metaOnly && recordingFile.FileType == "MP4" {
				reporter.AddSkipped(progress.SkipReasonMetaOnlyMode, filename, map[string]interface{}{
					"user_email": userEmail,
					"file_path":  filePath,
				})
				continue
			}
			
			// Skip if dry run
			if dryRun {
				fmt.Printf("Would download: %s → %s\n", recordingFile.DownloadURL, filePath)
				continue
			}
			
			// Create download request
			downloadReq := download.DownloadRequest{
				ID:          fmt.Sprintf("%s-%s", recording.UUID, recordingFile.ID),
				URL:         recordingFile.DownloadURL,
				Destination: filePath,
				FileSize:    recordingFile.FileSize,
				Headers:     make(map[string]string),
				Metadata: map[string]interface{}{
					"user_email":    userEmail,
					"meeting_id":    recording.UUID,
					"meeting_topic": recording.Topic,
					"file_type":     recordingFile.FileType,
					"filename":      filename,
				},
			}
			
			// Perform download
			progressCallback := func(update download.ProgressUpdate) {
				reporter.UpdateDownload(update)
			}
			
			result, err := downloadManager.Download(ctx, downloadReq, progressCallback)
			if err != nil {
				if logger != nil {
					logger.ErrorWithContext(ctx, fmt.Sprintf("Download failed for %s: %v", filename, err))
				}
				reporter.AddError(filename, err, map[string]interface{}{
					"user_email": userEmail,
					"file_path":  filePath,
					"download_url": recordingFile.DownloadURL,
				})
				continue
			}
			
			if logger != nil {
				logger.InfoWithContext(ctx, fmt.Sprintf("Successfully downloaded %s (%d bytes)", filename, result.BytesDownloaded))
			}
			
			// Also save metadata file
			metadataFilename := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json"
			metadataPath := filepath.Join(dirPath, metadataFilename)
			if err := saveMetadata(recording, metadataPath); err != nil {
				if logger != nil {
					logger.ErrorWithContext(ctx, fmt.Sprintf("Failed to save metadata for %s: %v", filename, err))
				}
			}
			
			processed++
		}
	}
	
	return processed, nil
}

// saveMetadata saves recording metadata to a JSON file
func saveMetadata(recording *zoom.Recording, filepath string) error {
	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	
	return os.WriteFile(filepath, data, 0644)
}

// getFromDate returns the start date for fetching recordings (30 days ago)
func getFromDate() *time.Time {
	from := time.Now().AddDate(0, 0, -30)
	return &from
}

// getToDate returns the end date for fetching recordings (today)
func getToDate() *time.Time {
	to := time.Now()
	return &to
}

// showDetailedSummary displays detailed summary information
func showDetailedSummary(cmd *cobra.Command, summary *progress.Summary) {
	cmd.Printf("\nDetailed Summary:\n")
	cmd.Printf("================\n")
	
	if summary.FailedDownloads > 0 {
		cmd.Printf("❌ Failed downloads (%d):\n", summary.FailedDownloads)
		for _, errorItem := range summary.ErrorItems {
			cmd.Printf("   - %s: %s\n", errorItem.Item, errorItem.ErrorMsg)
		}
		cmd.Printf("\n")
	}
	
	if len(summary.SkippedItems) > 0 {
		skippedByReason := summary.GetSkippedByReason()
		cmd.Printf("⏭️  Skipped items by reason:\n")
		
		for reason, items := range skippedByReason {
			if len(items) > 0 {
				cmd.Printf("   %s: %d items\n", reason.String(), len(items))
				if len(items) <= 5 {
					for _, item := range items {
						cmd.Printf("     - %s\n", item.Item)
					}
				} else {
					for i := 0; i < 3; i++ {
						cmd.Printf("     - %s\n", items[i].Item)
					}
					cmd.Printf("     ... and %d more\n", len(items)-3)
				}
			}
		}
		cmd.Printf("\n")
	}
	
	if len(summary.ActiveDownloads) > 0 {
		cmd.Printf("🔄 Active downloads: %d\n", len(summary.ActiveDownloads))
		for _, download := range summary.ActiveDownloads {
			cmd.Printf("   - %s (%s)\n", download.Filename, download.State.String())
		}
		cmd.Printf("\n")
	}
}

// isValidEmail validates email format using RFC 5322 compliant regex
func isValidEmail(email string) bool {
	if len(email) == 0 || len(email) > 320 {
		return false
	}
	
	// RFC 5322 compliant email regex (simplified but sufficient for most cases)
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}

func main() {
	rootCmd := buildRootCommand()
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

