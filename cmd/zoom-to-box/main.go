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

var rootCmd = &cobra.Command{
	Use:   "zoom-to-box",
	Short: "A CLI tool to download Zoom cloud recordings",
	Long: `zoom-to-box is a CLI tool that connects to the Zoom API 
and downloads video files with metadata, supporting resume functionality 
and Box integration.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("zoom-to-box - Use --help for usage information")
	},
}

func init() {
	// Global flags will be added here later
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

