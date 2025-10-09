// Package main provides tests for the zoom-to-box CLI application
package main

import (
	"bytes"
	"os"
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
			name:           "no args shows configuration detection",
			args:           []string{},
			expectedOutput: "Configuration Issue Detected",
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
		"ZOOM API CONFIGURATION (Required):",
		"zoom:",
		"account_id:",
		"client_id:",
		"client_secret:",
		"DOWNLOAD CONFIGURATION:",
		"download:",
		"output_dir:",
		"concurrent_limit:",
		"LOGGING CONFIGURATION:",
		"logging:",
		"level:",
		"BOX INTEGRATION (Optional):",
		"box:",
		"enabled:",
		"ACTIVE USERS FILTERING (Optional):",
		"active_users:",
		"ENVIRONMENT VARIABLES:",
		"ZOOM_ACCOUNT_ID",
		"ZOOM_CLIENT_ID",
		"ZOOM_CLIENT_SECRET",
		"AUTHENTICATION METHODS:",
		"Server-to-Server OAuth",
		"EXAMPLE USAGE:",
		"DIRECTORY STRUCTURE:",
		"TROUBLESHOOTING:",
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
	expectedFlags := []string{"config", "output-dir", "verbose", "dry-run", "meta-only"}

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
		{
			name:        "meta-only flag",
			args:        []string{"--meta-only"},
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

func TestMetaOnlyFlag(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "help shows meta-only flag",
			args:           []string{"--help"},
			expectedOutput: "--meta-only",
			expectError:    false,
		},
		{
			name:           "meta-only flag description in help",
			args:           []string{"--help"},
			expectedOutput: "download only JSON metadata files",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createRootCommand()

			// Capture output
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

			output := buf.String()
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

// TestConfigurationDetection tests the configuration detection and helpful error messages
func TestConfigurationDetection(t *testing.T) {
	tests := []struct {
		name           string
		configFile     string
		envVars        map[string]string
		expectedOutput []string
	}{
		{
			name:       "missing config file shows helpful guidance",
			configFile: "nonexistent.yaml",
			envVars:    map[string]string{},
			expectedOutput: []string{
				"Configuration Issue Detected",
				"Configuration file 'nonexistent.yaml' not found",
				"To get started:",
				"zoom-to-box config",
				"Copy config.example.yaml to config.yaml",
				"Alternative: Set environment variables",
			},
		},
		{
			name:       "environment variables detected shows different message",
			configFile: "nonexistent.yaml",
			envVars: map[string]string{
				"ZOOM_ACCOUNT_ID":     "test-account",
				"ZOOM_CLIENT_ID":      "test-client",
				"ZOOM_CLIENT_SECRET":  "test-secret",
			},
			expectedOutput: []string{
				"Configuration Issue Detected",
				"Zoom credentials found in environment variables",
				"You can run 'zoom-to-box' without a config file",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			cmd := createRootCommand()
			
			// Set config file if specified
			if tt.configFile != "" {
				cmd.SetArgs([]string{"--config", tt.configFile})
			}
			
			// Capture output
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			
			err := cmd.Execute()
			
			// Should not error, just provide helpful output
			if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			
			output := buf.String()
			
			// Check all expected output strings
			for _, expected := range tt.expectedOutput {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got %q", expected, output)
				}
			}
		})
	}
}

// TestEnhancedConfigHelp tests the enhanced configuration help content
func TestEnhancedConfigHelp(t *testing.T) {
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
	
	// Test specific enhanced content sections
	tests := []struct {
		name     string
		content  string
		required bool
	}{
		{"Zoom API section", "ZOOM API CONFIGURATION (Required):", true},
		{"Server-to-Server OAuth info", "Server-to-Server OAuth", true},
		{"Required scopes", "recording:read, user:read, meeting:read", true},
		{"Box integration section", "BOX INTEGRATION (Optional):", true},
		{"Box credentials format", "client_id", true},
		{"Active users section", "ACTIVE USERS FILTERING (Optional):", true},
		{"Environment variables section", "ENVIRONMENT VARIABLES:", true},
		{"Authentication methods", "AUTHENTICATION METHODS:", true},
		{"Example usage section", "EXAMPLE USAGE:", true},
		{"Directory structure", "DIRECTORY STRUCTURE:", true},
		{"Troubleshooting section", "TROUBLESHOOTING:", true},
		{"Comments in YAML", "# Zoom Account ID from Server-to-Server OAuth app", true},
		{"File format examples", "john.doe@company.com", true},
		{"Default values", "(default:", true},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.required && !strings.Contains(output, test.content) {
				t.Errorf("Expected config help to contain %q", test.content)
			}
		})
	}
}

// TestConfigCommandSections tests that all major sections are present in config help
func TestConfigCommandSections(t *testing.T) {
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
	
	// Verify that major sections appear in the expected order
	sections := []string{
		"ZOOM API CONFIGURATION (Required):",
		"DOWNLOAD CONFIGURATION:",
		"LOGGING CONFIGURATION:",
		"BOX INTEGRATION (Optional):",
		"ACTIVE USERS FILTERING (Optional):",
		"ENVIRONMENT VARIABLES:",
		"AUTHENTICATION METHODS:",
		"EXAMPLE USAGE:",
		"DIRECTORY STRUCTURE:",
		"TROUBLESHOOTING:",
	}
	
	lastIndex := -1
	for i, section := range sections {
		index := strings.Index(output, section)
		if index == -1 {
			t.Errorf("Section %d (%q) not found in config help", i, section)
			continue
		}
		if index <= lastIndex {
			t.Errorf("Section %d (%q) appears out of order (index %d vs previous %d)", i, section, index, lastIndex)
		}
		lastIndex = index
	}
}

// createRootCommand creates a fresh root command instance for testing
func createRootCommand() *cobra.Command {
	return buildRootCommand()
}



// TestEmailValidation tests the isValidEmail function
func TestEmailValidation(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		{
			name:     "valid email",
			email:    "john.doe@company.com",
			expected: true,
		},
		{
			name:     "valid email with plus",
			email:    "john.doe+test@company.com",
			expected: true,
		},
		{
			name:     "valid email with underscore",
			email:    "john_doe@company.com",
			expected: true,
		},
		{
			name:     "empty email",
			email:    "",
			expected: false,
		},
		{
			name:     "email without domain",
			email:    "john.doe",
			expected: false,
		},
		{
			name:     "email without username",
			email:    "@company.com",
			expected: false,
		},
		{
			name:     "email with spaces",
			email:    "john doe@company.com",
			expected: false,
		},
		{
			name:     "email too long",
			email:    strings.Repeat("a", 310) + "@company.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidEmail(tt.email)
			if result != tt.expected {
				t.Errorf("isValidEmail(%q) = %v, expected %v", tt.email, result, tt.expected)
			}
		})
	}
}

// TestSingleUserFlags tests the new zoom-user and box-user flags
func TestSingleUserFlags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectError    bool
		expectedOutput string
	}{
		{
			name:           "help shows zoom-user flag",
			args:           []string{"--help"},
			expectError:    false,
			expectedOutput: "--zoom-user",
		},
		{
			name:           "help shows box-user flag",
			args:           []string{"--help"},
			expectError:    false,
			expectedOutput: "--box-user",
		},
		{
			name:           "zoom-user description in help",
			args:           []string{"--help"},
			expectError:    false,
			expectedOutput: "process recordings for specific Zoom user email",
		},
		{
			name:           "box-user description in help",
			args:           []string{"--help"},
			expectError:    false,
			expectedOutput: "corresponding Box user email for uploads",
		},
		{
			name:        "zoom-user without box-user should error",
			args:        []string{"--zoom-user", "test@example.com"},
			expectError: true,
		},
		{
			name:        "box-user without zoom-user should error",
			args:        []string{"--box-user", "test@example.com"},
			expectError: true,
		},
		{
			name:        "invalid zoom-user email should error",
			args:        []string{"--zoom-user", "invalid-email", "--box-user", "test@example.com"},
			expectError: true,
		},
		{
			name:        "invalid box-user email should error",
			args:        []string{"--zoom-user", "test@example.com", "--box-user", "invalid-email"},
			expectError: true,
		},
		{
			name:        "valid zoom-user and box-user should not error during validation",
			args:        []string{"--zoom-user", "zoom@example.com", "--box-user", "box@example.com", "--dry-run"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createRootCommand()
			
			// Capture output
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
			
			if tt.expectedOutput != "" {
				output := buf.String()
				if !strings.Contains(output, tt.expectedOutput) {
					t.Errorf("Expected output to contain %q, got %q", tt.expectedOutput, output)
				}
			}
		})
	}
}

// TestSingleUserModeHelp tests that single user mode help is displayed correctly
func TestSingleUserModeHelp(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "config help shows single user examples",
			args:           []string{"config"},
			expectedOutput: "Single user processing:",
			expectError:    false,
		},
		{
			name:           "config help shows zoom-user example",
			args:           []string{"config"},
			expectedOutput: "--zoom-user=john.doe@company.com",
			expectError:    false,
		},
		{
			name:           "config help shows box-user example",
			args:           []string{"config"},
			expectedOutput: "--box-user=john.doe@company.com",
			expectError:    false,
		},
		{
			name:           "config help shows email mapping format",
			args:           []string{"config"},
			expectedOutput: "john.doe@zoomaccount.com,john.doe@company.com",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createRootCommand()
			
			// Capture output
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
			
			output := buf.String()
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

// TestSingleUserConfig tests the SingleUserConfig struct
func TestSingleUserConfig(t *testing.T) {
	tests := []struct {
		name      string
		zoomUser  string
		boxUser   string
		enabled   bool
	}{
		{
			name:     "both emails provided",
			zoomUser: "zoom@example.com",
			boxUser:  "box@example.com",
			enabled:  true,
		},
		{
			name:     "same email for both",
			zoomUser: "user@example.com",
			boxUser:  "user@example.com",
			enabled:  true,
		},
		{
			name:     "missing zoom email",
			zoomUser: "",
			boxUser:  "box@example.com",
			enabled:  false,
		},
		{
			name:     "missing box email",
			zoomUser: "zoom@example.com",
			boxUser:  "",
			enabled:  false,
		},
		{
			name:     "both emails empty",
			zoomUser: "",
			boxUser:  "",
			enabled:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := SingleUserConfig{
				Enabled:   tt.zoomUser != "" && tt.boxUser != "",
				ZoomEmail: tt.zoomUser,
				BoxEmail:  tt.boxUser,
			}
			
			if config.Enabled != tt.enabled {
				t.Errorf("Expected enabled=%v, got %v", tt.enabled, config.Enabled)
			}
			
			if config.ZoomEmail != tt.zoomUser {
				t.Errorf("Expected ZoomEmail=%q, got %q", tt.zoomUser, config.ZoomEmail)
			}
			
			if config.BoxEmail != tt.boxUser {
				t.Errorf("Expected BoxEmail=%q, got %q", tt.boxUser, config.BoxEmail)
			}
		})
	}
}