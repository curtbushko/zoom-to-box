// Package main provides tests for the zoom-to-box CLI application
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "help flag shows help",
			args:           []string{"--help"},
			expectedOutput: "zoom-to-box is a CLI tool that connects to the Zoom API",
			expectError:    false,
		},
		{
			name:           "no args shows basic usage",
			args:           []string{},
			expectedOutput: "zoom-to-box - Use --help for usage information",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test to avoid state pollution
			cmd := createRootCommand()
			
			// Capture output
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			
			// Set args and execute
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			
			// Check error expectation
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			
			// Check output
			output := buf.String()
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := createRootCommand()
	
	// Capture output
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Execute version command
	cmd.SetArgs([]string{"version"})
	err := cmd.Execute()
	
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
	
	output := buf.String()
	if !strings.Contains(output, "zoom-to-box version") {
		t.Errorf("Expected output to contain version info, got %q", output)
	}
}

func TestConfigCommand(t *testing.T) {
	cmd := createRootCommand()
	
	// Capture output
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Execute config command
	cmd.SetArgs([]string{"config"})
	err := cmd.Execute()
	
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
	
	output := buf.String()
	
	// Check that config help contains expected sections
	expectedContent := []string{
		"Configuration File Structure",
		"zoom:",
		"account_id:",
		"client_id:",
		"client_secret:",
		"Environment Variables:",
		"ZOOM_ACCOUNT_ID",
		"ZOOM_CLIENT_ID",
		"ZOOM_CLIENT_SECRET",
	}
	
	for _, content := range expectedContent {
		if !strings.Contains(output, content) {
			t.Errorf("Expected config output to contain %q, got %q", content, output)
		}
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := createRootCommand()
	
	// Test that global flags are defined
	expectedFlags := []string{"config", "output-dir", "verbose", "dry-run"}
	
	for _, flagName := range expectedFlags {
		flag := cmd.PersistentFlags().Lookup(flagName)
		if flag == nil {
			t.Errorf("Expected global flag %q to be defined", flagName)
		}
	}
}

func TestFlagValidation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "valid config path",
			args:        []string{"--config", "/path/to/config.yaml"},
			expectError: false,
		},
		{
			name:        "valid output directory",
			args:        []string{"--output-dir", "/path/to/output"},
			expectError: false,
		},
		{
			name:        "verbose flag",
			args:        []string{"--verbose"},
			expectError: false,
		},
		{
			name:        "dry-run flag",
			args:        []string{"--dry-run"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createRootCommand()
			
			// Capture output to avoid printing during tests
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestHelpCommand(t *testing.T) {
	cmd := createRootCommand()
	
	// Capture output
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	
	// Execute help command
	cmd.SetArgs([]string{"help"})
	err := cmd.Execute()
	
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
	
	output := buf.String()
	
	// Check that help contains expected sections
	expectedContent := []string{
		"zoom-to-box is a CLI tool",
		"Usage:",
		"Available Commands:",
		"Flags:",
	}
	
	for _, content := range expectedContent {
		if !strings.Contains(output, content) {
			t.Errorf("Expected help output to contain %q, got %q", content, output)
		}
	}
}

// createRootCommand creates a fresh root command instance for testing
func createRootCommand() *cobra.Command {
	return buildRootCommand()
}