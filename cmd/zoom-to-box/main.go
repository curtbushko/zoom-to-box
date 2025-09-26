package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/curtbushko/zoom-to-box/internal/config"
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
)

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
			_, err := config.LoadConfig(configPath)
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

			// Configuration loaded successfully
			cmd.Printf("✅ Configuration loaded successfully from '%s'\n\n", configPath)
			cmd.Printf("zoom-to-box is ready to download Zoom recordings.\n\n")
			cmd.Printf("Usage examples:\n")
			cmd.Printf("  zoom-to-box                    # Download all recordings\n")
			cmd.Printf("  zoom-to-box --limit 10         # Download only 10 recordings\n")
			cmd.Printf("  zoom-to-box --meta-only        # Download only metadata files\n")
			cmd.Printf("  zoom-to-box --dry-run          # Preview what would be downloaded\n\n")
			cmd.Printf("For all options: zoom-to-box --help\n")
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

	// Add flag validation
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if limit < 0 {
			return fmt.Errorf("limit must be a positive number or 0, got: %d", limit)
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
  credentials_file: "box_credentials.json" # Path to Box OAuth 2.0 credentials file
  folder_id: "0"                   # Target Box folder ID (default: "0" = root folder)

# Box credentials file (box_credentials.json) should contain:
# {
#   "client_id": "your_box_oauth_client_id",
#   "client_secret": "your_box_oauth_client_secret", 
#   "access_token": "your_oauth_access_token",
#   "refresh_token": "your_oauth_refresh_token"
# }

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

ENVIRONMENT VARIABLES:
=====================

Required Zoom API credentials (override config file):
  ZOOM_ACCOUNT_ID     - Your Zoom account ID
  ZOOM_CLIENT_ID      - Your Zoom OAuth app client ID
  ZOOM_CLIENT_SECRET  - Your Zoom OAuth app client secret
  ZOOM_BASE_URL       - Zoom API base URL (optional)

Optional Box integration:
  BOX_CREDENTIALS_FILE - Path to Box credentials file
  BOX_FOLDER_ID        - Target Box folder ID

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

4. Box integration:
   # Set up Box OAuth 2.0 credentials file
   # Enable in config.yaml: box.enabled = true
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
- For Box integration, ensure OAuth 2.0 tokens are valid and not expired

For more information, visit: https://github.com/curtbushko/zoom-to-box
`
			cmd.Print(configHelp)
		},
	}
}

func main() {
	rootCmd := buildRootCommand()
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

