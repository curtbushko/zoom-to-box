package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name         string
		configYAML   string
		expectedZoom ZoomConfig
		expectedBox  BoxConfig
		shouldError  bool
	}{
		{
			name: "complete configuration",
			configYAML: `
zoom:
  account_id: "test_account_id"
  client_id: "test_client_id"
  client_secret: "test_client_secret"
  base_url: "https://api.zoom.us/v2"

box:
  enabled: true
  client_id: "test_box_client_id"
  client_secret: "test_box_client_secret"
  folder_id: "test_folder_id"

download:
  output_dir: "./downloads"
  concurrent_limit: 3
  retry_attempts: 3
  timeout_seconds: 300

logging:
  level: "info"
  file: "./zoom-downloader.log"
  console: true
  json_format: false

active_users:
  file: "./active_users.txt"
  check_enabled: true
`,
			expectedZoom: ZoomConfig{
				AccountID:    "test_account_id",
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
				BaseURL:      "https://api.zoom.us/v2",
			},
			expectedBox: BoxConfig{
				Enabled:      true,
				ClientID:     "test_box_client_id",
				ClientSecret: "test_box_client_secret",
			},
			shouldError: false,
		},
		{
			name: "minimal configuration with defaults",
			configYAML: `
zoom:
  account_id: "test_account"
  client_id: "test_client"
  client_secret: "test_secret"
`,
			expectedZoom: ZoomConfig{
				AccountID:    "test_account",
				ClientID:     "test_client",
				ClientSecret: "test_secret",
				BaseURL:      "https://api.zoom.us/v2", // Should default
			},
			expectedBox: BoxConfig{
				Enabled: false, // Should default to false
			},
			shouldError: false,
		},
		{
			name: "missing required zoom fields",
			configYAML: `
zoom:
  account_id: "test_account"
  # Missing client_id and client_secret
`,
			shouldError: true,
		},
		{
			name:        "invalid YAML",
			configYAML:  "invalid: yaml: content: [unclosed",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
			if err != nil {
				t.Fatalf("Failed to create temp config file: %v", err)
			}

			// Load configuration
			config, err := LoadConfig(configPath)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Validate Zoom configuration
			if config.Zoom.AccountID != tt.expectedZoom.AccountID {
				t.Errorf("Expected Zoom AccountID %s, got %s", tt.expectedZoom.AccountID, config.Zoom.AccountID)
			}
			if config.Zoom.ClientID != tt.expectedZoom.ClientID {
				t.Errorf("Expected Zoom ClientID %s, got %s", tt.expectedZoom.ClientID, config.Zoom.ClientID)
			}
			if config.Zoom.ClientSecret != tt.expectedZoom.ClientSecret {
				t.Errorf("Expected Zoom ClientSecret %s, got %s", tt.expectedZoom.ClientSecret, config.Zoom.ClientSecret)
			}
			if config.Zoom.BaseURL != tt.expectedZoom.BaseURL {
				t.Errorf("Expected Zoom BaseURL %s, got %s", tt.expectedZoom.BaseURL, config.Zoom.BaseURL)
			}

			// Validate Box configuration
			if config.Box.Enabled != tt.expectedBox.Enabled {
				t.Errorf("Expected Box Enabled %t, got %t", tt.expectedBox.Enabled, config.Box.Enabled)
			}
			if config.Box.ClientID != tt.expectedBox.ClientID {
				t.Errorf("Expected Box ClientID %s, got %s", tt.expectedBox.ClientID, config.Box.ClientID)
			}
			if config.Box.ClientSecret != tt.expectedBox.ClientSecret {
				t.Errorf("Expected Box ClientSecret %s, got %s", tt.expectedBox.ClientSecret, config.Box.ClientSecret)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
					BaseURL:      "https://api.zoom.us/v2",
				},
				Download: DownloadConfig{
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: false,
		},
		{
			name: "missing zoom account_id",
			config: &Config{
				Zoom: ZoomConfig{
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
			},
			shouldError: true,
			errorMsg:    "zoom.account_id is required",
		},
		{
			name: "missing zoom client_id",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientSecret: "test_secret",
				},
			},
			shouldError: true,
			errorMsg:    "zoom.client_id is required",
		},
		{
			name: "invalid concurrent limit",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Download: DownloadConfig{
				},
			},
			shouldError: true,
			errorMsg:    "download.timeout_seconds must be greater than 0",
		},
		{
			name: "invalid retry attempts",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Download: DownloadConfig{
					RetryAttempts:   -1,
				},
			},
			shouldError: true,
			errorMsg:    "download.retry_attempts must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error, but got none")
					return
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		expectedConfig Config
	}{
		{
			name: "apply defaults for missing sections",
			configYAML: `
zoom:
  account_id: "test_account"
  client_id: "test_client"
  client_secret: "test_secret"
`,
			expectedConfig: Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
					BaseURL:      "https://api.zoom.us/v2",
				},
				Box: BoxConfig{
					Enabled: false,
				},
				Download: DownloadConfig{
					OutputDir:       "./downloads",
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level:      "info",
					File:       "./zoom-downloader.log",
					Console:    true,
					JSONFormat: false,
				},
				ActiveUsers: ActiveUsersConfig{
					File:         "./active_users.txt",
					CheckEnabled: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
			if err != nil {
				t.Fatalf("Failed to create temp config file: %v", err)
			}

			// Load configuration
			config, err := LoadConfig(configPath)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Check defaults were applied
			if config.Download.OutputDir != tt.expectedConfig.Download.OutputDir {
				t.Errorf("Expected default OutputDir %s, got %s", tt.expectedConfig.Download.OutputDir, config.Download.OutputDir)
			}
			if config.Logging.Level != tt.expectedConfig.Logging.Level {
				t.Errorf("Expected default Logging Level %s, got %s", tt.expectedConfig.Logging.Level, config.Logging.Level)
			}
		})
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent_config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent config file, but got none")
	}
}

func TestLoadConfigFromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("ZOOM_ACCOUNT_ID", "env_account")
	os.Setenv("ZOOM_CLIENT_ID", "env_client")
	os.Setenv("ZOOM_CLIENT_SECRET", "env_secret")
	defer func() {
		os.Unsetenv("ZOOM_ACCOUNT_ID")
		os.Unsetenv("ZOOM_CLIENT_ID")
		os.Unsetenv("ZOOM_CLIENT_SECRET")
	}()

	config := &Config{}
	config.loadFromEnvironment()

	if config.Zoom.AccountID != "env_account" {
		t.Errorf("Expected AccountID from env %s, got %s", "env_account", config.Zoom.AccountID)
	}
	if config.Zoom.ClientID != "env_client" {
		t.Errorf("Expected ClientID from env %s, got %s", "env_client", config.Zoom.ClientID)
	}
	if config.Zoom.ClientSecret != "env_secret" {
		t.Errorf("Expected ClientSecret from env %s, got %s", "env_secret", config.Zoom.ClientSecret)
	}
}

func TestTimeoutDuration(t *testing.T) {
	config := &Config{
		Download: DownloadConfig{
			TimeoutSeconds: 300,
		},
	}

	expectedDuration := 300 * time.Second
	if config.Download.TimeoutDuration() != expectedDuration {
		t.Errorf("Expected timeout duration %v, got %v", expectedDuration, config.Download.TimeoutDuration())
	}
}

func TestLogLevelValidation(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}
	
	for _, level := range validLevels {
		t.Run("valid_level_"+level, func(t *testing.T) {
			config := &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Download: DownloadConfig{
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: level,
				},
			}
			
			err := config.Validate()
			if err != nil {
				t.Errorf("Valid log level %s should not cause error: %v", level, err)
			}
		})
	}
	
	t.Run("invalid_log_level", func(t *testing.T) {
		config := &Config{
			Zoom: ZoomConfig{
				AccountID:    "test_account",
				ClientID:     "test_client",
				ClientSecret: "test_secret",
			},
			Download: DownloadConfig{
				RetryAttempts:  3,
				TimeoutSeconds: 300,
			},
			Logging: LoggingConfig{
				Level: "invalid",
			},
		}
		
		err := config.Validate()
		if err == nil {
			t.Error("Invalid log level should cause error")
		}
	})
}