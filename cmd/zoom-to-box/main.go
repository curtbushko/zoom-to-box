package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/curtbushko/zoom-to-box/internal/config"
	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/logging"
	"github.com/curtbushko/zoom-to-box/internal/progress"
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
  folder_id: "0"                   # Target Box folder ID (default: "0" = root folder)

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
  BOX_FOLDER_ID     - Target Box folder ID

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

	// Simulate download operations (this would be replaced with actual download logic)
	if err := simulateDownloads(ctx, reporter, totalItems, singleUserConfig); err != nil {
		return fmt.Errorf("download operation failed: %w", err)
	}

	// Finish progress tracking and show summary
	summary := reporter.Finish()

	// Display results
	if dryRun {
		cmd.Printf("\n🔍 DRY RUN COMPLETED\n")
		cmd.Printf("Would have processed %d recordings\n", summary.TotalItems)
		if metaOnly {
			cmd.Printf("Would have downloaded metadata files only\n")
		}
	} else {
		cmd.Printf("\n✅ DOWNLOAD COMPLETED\n")
		
		// Show summary based on verbosity
		if verbose || summary.FailedDownloads > 0 || len(summary.ErrorItems) > 0 {
			showDetailedSummary(cmd, summary)
		}
	}

	return nil
}

// estimateTotalItems estimates the total number of items to process
func estimateTotalItems(cfg *config.Config, limitFlag int) int {
	// This is a placeholder - in real implementation, this would query the Zoom API
	// to get an accurate count of recordings to process
	estimated := 50 // Default estimate
	
	if limitFlag > 0 && limitFlag < estimated {
		return limitFlag
	}
	
	return estimated
}

// simulateDownloads simulates the download process for demonstration
func simulateDownloads(ctx context.Context, reporter progress.ProgressReporter, totalItems int, singleUserConfig SingleUserConfig) error {
	// This is a placeholder that simulates downloads
	// In the real implementation, this would:
	// 1. Initialize Zoom API client
	// 2. Get list of users and recordings
	// 3. Filter based on active users OR use single user mode
	// 4. Download recordings with progress tracking
	// 5. Upload to Box if configured
	
	// In single user mode, we would only process the specified user
	if singleUserConfig.Enabled {
		fmt.Printf("📋 Single user mode: Processing recordings for %s\n", singleUserConfig.ZoomEmail)
		fmt.Printf("📁 Folder structure will use: %s\n", singleUserConfig.BoxEmail)
		fmt.Printf("🔐 Box permissions will be granted to: %s\n", singleUserConfig.BoxEmail)
	}

	for i := 0; i < totalItems; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		filename := fmt.Sprintf("meeting-%d.mp4", i+1)
		
		// Simulate different outcomes
		if i%10 == 9 { // Every 10th item fails
			reporter.AddError(filename, fmt.Errorf("simulated network error"), map[string]interface{}{
				"item_index": i,
				"url":        fmt.Sprintf("https://api.zoom.us/recording/%d", i),
			})
		} else if i%7 == 6 { // Every 7th item is skipped
			var reason progress.SkipReason
			switch i % 3 {
			case 0:
				reason = progress.SkipReasonAlreadyExists
			case 1:
				reason = progress.SkipReasonInactiveUser
			default:
				reason = progress.SkipReasonMetaOnlyMode
			}
			
			reporter.AddSkipped(reason, filename, map[string]interface{}{
				"item_index": i,
				"path":       fmt.Sprintf("/downloads/user/%s", filename),
			})
		} else {
			// Simulate successful download
			downloadID := fmt.Sprintf("download-%d", i)
			
			// Start download
			reporter.UpdateDownload(download.ProgressUpdate{
				DownloadID:      downloadID,
				BytesDownloaded: 0,
				TotalBytes:      1048576, // 1MB
				Speed:           0,
				State:           download.DownloadStateDownloading,
				Timestamp:       time.Now(),
				Metadata:        map[string]interface{}{"filename": filename},
			})
			
			// Simulate progress
			for progress := int64(0); progress <= 1048576; progress += 262144 {
				time.Sleep(50 * time.Millisecond) // Simulate download time
				
				state := download.DownloadStateDownloading
				if progress >= 1048576 {
					state = download.DownloadStateCompleted
				}
				
				reporter.UpdateDownload(download.ProgressUpdate{
					DownloadID:      downloadID,
					BytesDownloaded: progress,
					TotalBytes:      1048576,
					Speed:           float64(progress) / 0.2, // Simulate speed
					State:           state,
					Timestamp:       time.Now(),
					Metadata:        map[string]interface{}{"filename": filename},
				})
			}
		}
		
		time.Sleep(100 * time.Millisecond) // Simulate processing time
	}

	return nil
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

