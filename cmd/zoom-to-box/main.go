package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/curtbushko/zoom-to-box/internal/box"
	"github.com/curtbushko/zoom-to-box/internal/config"
	"github.com/curtbushko/zoom-to-box/internal/directory"
	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/filename"
	"github.com/curtbushko/zoom-to-box/internal/logging"
	"github.com/curtbushko/zoom-to-box/internal/processor"
	"github.com/curtbushko/zoom-to-box/internal/tracking"
	"github.com/curtbushko/zoom-to-box/internal/users"
	"github.com/curtbushko/zoom-to-box/internal/zoom"
)

var (
	// Version information - will be set during build
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
	
	// Global flags
	configFile        string
	outputDir         string
	verbose           bool
	dryRun            bool
	metaOnly          bool
	zoomUser          string
	boxUser           string
	deleteAfterUpload bool
	continueOnError   bool
	activeUsersFile   string
	limit             int
)

// SingleUserConfig holds configuration for single user mode
type SingleUserConfig struct {
	Enabled   bool
	ZoomEmail string
	BoxEmail  string
}

// DownloadStats tracks download statistics
type DownloadStats struct {
	SuccessCount int
	ErrorCount   int
	SkippedCount int
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
				cmd.Printf("Configuration Issue Detected\n\n")
				
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
					cmd.Printf("Zoom credentials found in environment variables.\n")
					cmd.Printf("You can run 'zoom-to-box' without a config file.\n\n")
				} else {
					cmd.Printf("Alternative: Set environment variables instead of using a config file:\n")
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
				cmd.Printf("Download failed: %v\n", err)
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
	rootCmd.PersistentFlags().StringVar(&zoomUser, "zoom-user", "", "process recordings for specific Zoom user email")
	rootCmd.PersistentFlags().StringVar(&boxUser, "box-user", "", "corresponding Box user email for uploads (requires --zoom-user)")
	rootCmd.PersistentFlags().BoolVar(&deleteAfterUpload, "delete-after-upload", false, "delete local MP4 files after successful Box upload")
	rootCmd.PersistentFlags().BoolVar(&continueOnError, "continue-on-error", true, "continue processing next user even if current user fails")
	rootCmd.PersistentFlags().StringVar(&activeUsersFile, "active-users-file", "", "path to active users file with upload tracking (overrides config)")
	rootCmd.PersistentFlags().IntVar(&limit, "limit", 0, "limit number of recordings to process per user (0 = no limit)")

	// Add flag validation
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
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
  enterprise_id: "your_box_enterprise_id" # Box enterprise ID for client credentials auth
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
  BOX_ENTERPRISE_ID - Box enterprise ID for client credentials auth

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
   zoom-to-box --meta-only --verbose
   zoom-to-box --output-dir ./recordings --dry-run

4. Single user processing:
   zoom-to-box --zoom-user=john.doe@company.com --box-user=john.doe@company.com
   zoom-to-box --zoom-user=john.doe@zoomaccount.com --box-user=john.doe@company.com

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

	// Override active users file if provided
	if activeUsersFile != "" {
		cfg.ActiveUsers.File = activeUsersFile
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
		cmd.Printf("Single user mode: processing %s -> %s\n", singleUserConfig.ZoomEmail, singleUserConfig.BoxEmail)
	}

	// Log session start
	if logger != nil {
		logger.InfoWithContext(ctx, "Starting zoom-to-box download session")
		sessionInfo := map[string]interface{}{
			"meta_only":        metaOnly,
			"dry_run":          dryRun,
			"verbose":          verbose,
			"output_dir":       cfg.Download.OutputDir,
			"single_user_mode": singleUserConfig.Enabled,
		}

		if singleUserConfig.Enabled {
			sessionInfo["single_zoom_email"] = singleUserConfig.ZoomEmail
			sessionInfo["single_box_email"] = singleUserConfig.BoxEmail
		}

		logger.LogUserAction("session_start", "cli", sessionInfo)
	}

	if dryRun {
		cmd.Printf("DRY RUN: Showing what would be downloaded (no files will be saved)\n\n")
	}

	// Execute download operations
	stats, err := performDownloads(ctx, cfg, singleUserConfig)
	if err != nil {
		return fmt.Errorf("download operation failed: %w", err)
	}

	// Display results
	if dryRun {
		cmd.Printf("\nDRY RUN COMPLETED\n")
		if stats.ErrorCount > 0 {
			cmd.Printf("Errors encountered: %d\n", stats.ErrorCount)
		} else {
			cmd.Printf("Would have processed %d recordings\n", stats.SuccessCount+stats.SkippedCount)
			if metaOnly {
				cmd.Printf("Would have downloaded metadata files only\n")
			}
		}
	} else {
		if stats.ErrorCount > 0 && stats.SuccessCount == 0 {
			cmd.Printf("\nDOWNLOAD FAILED\n")
			cmd.Printf("No recordings could be downloaded due to errors\n")
		} else {
			cmd.Printf("\nDOWNLOAD COMPLETED\n")
		}

		if verbose || stats.ErrorCount > 0 {
			cmd.Printf("\nSummary:\n")
			cmd.Printf("- Downloaded: %d\n", stats.SuccessCount)
			if stats.SkippedCount > 0 {
				cmd.Printf("- Skipped: %d\n", stats.SkippedCount)
			}
			if stats.ErrorCount > 0 {
				cmd.Printf("- Failed: %d\n", stats.ErrorCount)
			}
		}
	}

	return nil
}

// performDownloads executes the download process using the processor package
func performDownloads(ctx context.Context, cfg *config.Config, singleUserConfig SingleUserConfig) (*DownloadStats, error) {
	logger := logging.GetDefaultLogger()
	stats := &DownloadStats{}

	// Initialize Zoom API client
	auth := zoom.NewServerToServerAuth(cfg.Zoom)
	httpConfig := zoom.HTTPClientConfigFromDownloadConfig(cfg.Download)
	retryClient := zoom.NewRetryHTTPClient(httpConfig)
	authRetryClient := zoom.NewAuthenticatedRetryClient(retryClient, auth)
	zoomClient := zoom.NewZoomClient(authRetryClient, cfg.Zoom.BaseURL)

	// Initialize download manager
	downloadManager := download.NewDownloadManager(download.DownloadConfig{
		ChunkSize:     64 * 1024, // 64KB chunks
		RetryAttempts: cfg.Download.RetryAttempts,
		RetryDelay:    1 * time.Second,
		UserAgent:     "zoom-to-box/1.0",
		Timeout:       cfg.Download.TimeoutDuration(),
	})

	// Initialize user manager
	userManager, err := users.NewActiveUserManager(users.ActiveUserConfig{
		FilePath:      "", // Empty for single user mode, will use processor directly
		CaseSensitive: false,
		WatchFile:     false,
	})
	if err != nil {
		return stats, fmt.Errorf("failed to initialize user manager: %w", err)
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

	// Initialize Box upload manager if enabled
	var uploadManager box.UploadManager
	if cfg.Box.Enabled {
		// Validate Box configuration
		if cfg.Box.ClientID == "" {
			return stats, fmt.Errorf("box.client_id is required when Box is enabled")
		}
		if cfg.Box.ClientSecret == "" {
			return stats, fmt.Errorf("box.client_secret is required when Box is enabled")
		}

		// Create Box client
		credentials := &box.OAuth2Credentials{
			ClientID:     cfg.Box.ClientID,
			ClientSecret: cfg.Box.ClientSecret,
			EnterpriseID: cfg.Box.EnterpriseID,
		}

		httpClient := &http.Client{
			Timeout: 30 * time.Second,
		}

		auth := box.NewOAuth2Authenticator(credentials, httpClient)
		boxClient := box.NewBoxClient(auth, httpClient)
		uploadManager = box.NewUploadManager(boxClient)

		// Initialize CSV trackers for upload tracking
		globalCSVPath := filepath.Join(cfg.Download.OutputDir, "all-uploads.csv")
		globalCSVTracker, err := tracking.NewGlobalCSVTracker(globalCSVPath)
		if err != nil {
			return stats, fmt.Errorf("failed to create global CSV tracker: %w", err)
		}
		uploadManager.SetGlobalCSVTracker(globalCSVTracker)

		if logger != nil {
			logger.InfoWithContext(ctx, "Box upload integration enabled with CSV tracking")
		}
		fmt.Printf("Box upload integration enabled\n")
	}

	// Create processor
	processorConfig := processor.ProcessorConfig{
		BaseDownloadDir:   cfg.Download.OutputDir,
		BoxEnabled:        cfg.Box.Enabled,
		DeleteAfterUpload: deleteAfterUpload,
		ContinueOnError:   continueOnError,
		MetaOnly:          metaOnly,
		Limit:             limit,
		DryRun:            dryRun,
		Verbose:           verbose,
	}

	userProcessor := processor.NewUserProcessor(
		zoomClient,
		downloadManager,
		dirManager,
		filenameSanitizer,
		uploadManager,
		processorConfig,
	)

	// Handle single user mode vs batch mode
	if singleUserConfig.Enabled {
		// Single user mode
		fmt.Printf("Single user mode: Processing recordings for %s\n", singleUserConfig.ZoomEmail)
		if singleUserConfig.BoxEmail != singleUserConfig.ZoomEmail {
			fmt.Printf("Box email mapping: %s -> %s\n", singleUserConfig.ZoomEmail, singleUserConfig.BoxEmail)
		}

		result, err := userProcessor.ProcessUser(ctx, singleUserConfig.ZoomEmail, singleUserConfig.BoxEmail)
		if err != nil && !continueOnError {
			return stats, fmt.Errorf("failed to process user %s: %w", singleUserConfig.ZoomEmail, err)
		}

		// Convert processor result to download stats
		stats.SuccessCount = result.DownloadedCount
		stats.ErrorCount = result.ErrorCount
		stats.SkippedCount = result.SkippedCount

		return stats, nil
	}

	// Batch mode with active users file
	if cfg.ActiveUsers.File == "" {
		return stats, fmt.Errorf("active users file not configured and no single user specified")
	}

	// Load active users file
	activeUsersFile, err := users.LoadActiveUsersFile(cfg.ActiveUsers.File)
	if err != nil {
		return stats, fmt.Errorf("failed to load active users file: %w", err)
	}

	fmt.Printf("Processing users from active users file: %s\n", cfg.ActiveUsers.File)

	// Process all incomplete users
	summary, err := userProcessor.ProcessAllUsers(ctx, activeUsersFile)
	if err != nil && !continueOnError {
		return stats, fmt.Errorf("failed to process users: %w", err)
	}

	// Convert processor summary to download stats
	stats.SuccessCount = summary.TotalDownloads
	stats.ErrorCount = summary.TotalErrors
	stats.SkippedCount = summary.TotalSkipped

	// Print summary
	fmt.Printf("\nProcessing Summary:\n")
	fmt.Printf("- Total users processed: %d/%d\n", summary.ProcessedUsers, summary.TotalUsers)
	fmt.Printf("- Failed users: %d\n", summary.FailedUsers)
	fmt.Printf("- Total downloads: %d\n", summary.TotalDownloads)
	fmt.Printf("- Total uploads: %d\n", summary.TotalUploads)
	fmt.Printf("- Total deleted: %d\n", summary.TotalDeleted)
	fmt.Printf("- Duration: %v\n", summary.Duration)

	return stats, nil
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

