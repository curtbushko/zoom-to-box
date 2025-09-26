package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	// Test internal package imports
	_ "github.com/curtbushko/zoom-to-box/internal/box"
	_ "github.com/curtbushko/zoom-to-box/internal/config"
	_ "github.com/curtbushko/zoom-to-box/internal/zoom"
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
			cmd.Println("zoom-to-box - Use --help for usage information")
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
		Long:  "Display the required configuration file structure, environment variables, and examples",
		Run: func(cmd *cobra.Command, args []string) {
			configHelp := `Configuration File Structure (config.yaml):

zoom:
  account_id: "your-zoom-account-id"
  client_id: "your-zoom-client-id"
  client_secret: "your-zoom-client-secret"
  base_url: "https://api.zoom.us/v2"

download:
  output_dir: "./recordings"
  concurrent_limit: 3
  retry_attempts: 3
  timeout_seconds: 300

logging:
  level: "info"
  file: "zoom-to-box.log"
  console: true
  json_format: false

box:
  enabled: false
  credentials_file: "box_credentials.json"
  folder_id: "0"

active_users:
  enabled: false
  file_path: "active_users.txt"

Environment Variables:

Required Zoom API credentials (can override config file):
  ZOOM_ACCOUNT_ID    - Your Zoom account ID
  ZOOM_CLIENT_ID     - Your Zoom OAuth app client ID  
  ZOOM_CLIENT_SECRET - Your Zoom OAuth app client secret

Optional Box integration:
  BOX_CLIENT_ID      - Box OAuth app client ID
  BOX_CLIENT_SECRET  - Box OAuth app client secret

Example Usage:

1. Create config.yaml with your Zoom credentials
2. Run: zoom-to-box --config config.yaml
3. Or use environment variables: 
   export ZOOM_ACCOUNT_ID=your-account-id
   export ZOOM_CLIENT_ID=your-client-id  
   export ZOOM_CLIENT_SECRET=your-client-secret
   zoom-to-box

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

