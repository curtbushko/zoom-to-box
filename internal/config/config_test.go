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
				AuthType:     "oauth",
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
				Enabled:  false, // Should default to false
				AuthType: "oauth", // Should default to oauth
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
			if config.Box.AuthType != tt.expectedBox.AuthType {
				t.Errorf("Expected Box AuthType %s, got %s", tt.expectedBox.AuthType, config.Box.AuthType)
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
					ConcurrentLimit: 3,
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
					ConcurrentLimit: 0,
				},
			},
			shouldError: true,
			errorMsg:    "download.concurrent_limit must be greater than 0",
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
					ConcurrentLimit: 3,
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
					Enabled:  false,
					AuthType: "oauth",
				},
				Download: DownloadConfig{
					OutputDir:       "./downloads",
					ConcurrentLimit: 3,
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
			if config.Download.ConcurrentLimit != tt.expectedConfig.Download.ConcurrentLimit {
				t.Errorf("Expected default ConcurrentLimit %d, got %d", tt.expectedConfig.Download.ConcurrentLimit, config.Download.ConcurrentLimit)
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
					ConcurrentLimit: 3,
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
				ConcurrentLimit: 3,
				RetryAttempts:   3,
				TimeoutSeconds:  300,
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

func TestBoxConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid OAuth Box config",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:      true,
					AuthType:     "oauth",
					ClientID:     "box_client_id",
					ClientSecret: "box_client_secret",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
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
			name: "valid service-to-service Box config",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:      true,
					AuthType:     "service-to-service",
					ClientID:     "box_client_id",
					ClientSecret: "box_client_secret",
					PrivateKey:   "-----BEGIN RSA PRIVATE KEY-----\ntest_key\n-----END RSA PRIVATE KEY-----",
					KeyID:        "test_key_id",
					EnterpriseID: "test_enterprise_id",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
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
			name: "disabled Box config should not validate",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled: false,
					// Missing other fields should not cause error when disabled
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
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
			name: "invalid Box auth type",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:  true,
					AuthType: "invalid",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: true,
			errorMsg:    "box configuration validation failed: box.auth_type must be one of: oauth, service-to-service",
		},
		{
			name: "missing Box client_id",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:      true,
					AuthType:     "oauth",
					ClientSecret: "box_client_secret",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: true,
			errorMsg:    "box configuration validation failed: box.client_id is required when Box is enabled",
		},
		{
			name: "missing Box client_secret for OAuth",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:  true,
					AuthType: "oauth",
					ClientID: "box_client_id",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: true,
			errorMsg:    "box configuration validation failed: box.client_secret is required for OAuth authentication",
		},
		{
			name: "missing private_key for service-to-service",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:      true,
					AuthType:     "service-to-service",
					ClientID:     "box_client_id",
					ClientSecret: "box_client_secret",
					KeyID:        "test_key_id",
					EnterpriseID: "test_enterprise_id",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: true,
			errorMsg:    "box configuration validation failed: box.private_key is required for service-to-service authentication",
		},
		{
			name: "missing key_id for service-to-service",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:      true,
					AuthType:     "service-to-service",
					ClientID:     "box_client_id",
					ClientSecret: "box_client_secret",
					PrivateKey:   "-----BEGIN RSA PRIVATE KEY-----\ntest_key\n-----END RSA PRIVATE KEY-----",
					EnterpriseID: "test_enterprise_id",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: true,
			errorMsg:    "box configuration validation failed: box.key_id is required for service-to-service authentication",
		},
		{
			name: "missing enterprise_id for service-to-service",
			config: &Config{
				Zoom: ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
				},
				Box: BoxConfig{
					Enabled:      true,
					AuthType:     "service-to-service",
					ClientID:     "box_client_id",
					ClientSecret: "box_client_secret",
					PrivateKey:   "-----BEGIN RSA PRIVATE KEY-----\ntest_key\n-----END RSA PRIVATE KEY-----",
					KeyID:        "test_key_id",
				},
				Download: DownloadConfig{
					ConcurrentLimit: 3,
					RetryAttempts:   3,
					TimeoutSeconds:  300,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			shouldError: true,
			errorMsg:    "box configuration validation failed: box.enterprise_id is required for service-to-service authentication",
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

func TestBoxConfigDefaults(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		expectedBox    BoxConfig
	}{
		{
			name: "default auth_type when not specified",
			configYAML: `
zoom:
  account_id: "test_account"
  client_id: "test_client"
  client_secret: "test_secret"
box:
  enabled: true
  client_id: "box_client_id"
  client_secret: "box_client_secret"
`,
			expectedBox: BoxConfig{
				Enabled:      true,
				AuthType:     "oauth",
				ClientID:     "box_client_id",
				ClientSecret: "box_client_secret",
			},
		},
		{
			name: "explicit service-to-service auth_type",
			configYAML: `
zoom:
  account_id: "test_account"
  client_id: "test_client"
  client_secret: "test_secret"
box:
  enabled: true
  auth_type: "service-to-service"
  client_id: "box_client_id"
  client_secret: "box_client_secret"
  private_key: "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----"
  key_id: "test_key_id"
  enterprise_id: "test_enterprise_id"
`,
			expectedBox: BoxConfig{
				Enabled:      true,
				AuthType:     "service-to-service",
				ClientID:     "box_client_id",
				ClientSecret: "box_client_secret",
				PrivateKey:   "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
				KeyID:        "test_key_id",
				EnterpriseID: "test_enterprise_id",
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

			// Check Box configuration
			if config.Box.Enabled != tt.expectedBox.Enabled {
				t.Errorf("Expected Box Enabled %t, got %t", tt.expectedBox.Enabled, config.Box.Enabled)
			}
			if config.Box.AuthType != tt.expectedBox.AuthType {
				t.Errorf("Expected Box AuthType %s, got %s", tt.expectedBox.AuthType, config.Box.AuthType)
			}
			if config.Box.ClientID != tt.expectedBox.ClientID {
				t.Errorf("Expected Box ClientID %s, got %s", tt.expectedBox.ClientID, config.Box.ClientID)
			}
			if config.Box.ClientSecret != tt.expectedBox.ClientSecret {
				t.Errorf("Expected Box ClientSecret %s, got %s", tt.expectedBox.ClientSecret, config.Box.ClientSecret)
			}
			if config.Box.PrivateKey != tt.expectedBox.PrivateKey {
				t.Errorf("Expected Box PrivateKey %s, got %s", tt.expectedBox.PrivateKey, config.Box.PrivateKey)
			}
			if config.Box.KeyID != tt.expectedBox.KeyID {
				t.Errorf("Expected Box KeyID %s, got %s", tt.expectedBox.KeyID, config.Box.KeyID)
			}
			if config.Box.EnterpriseID != tt.expectedBox.EnterpriseID {
				t.Errorf("Expected Box EnterpriseID %s, got %s", tt.expectedBox.EnterpriseID, config.Box.EnterpriseID)
			}
		})
	}
}

func TestBoxEnvironmentVariables(t *testing.T) {
	// Set environment variables
	os.Setenv("BOX_AUTH_TYPE", "service-to-service")
	os.Setenv("BOX_CLIENT_ID", "env_box_client")
	os.Setenv("BOX_CLIENT_SECRET", "env_box_secret")
	os.Setenv("BOX_PRIVATE_KEY", "env_private_key")
	os.Setenv("BOX_KEY_ID", "env_key_id")
	os.Setenv("BOX_ENTERPRISE_ID", "env_enterprise_id")
	defer func() {
		os.Unsetenv("BOX_AUTH_TYPE")
		os.Unsetenv("BOX_CLIENT_ID")
		os.Unsetenv("BOX_CLIENT_SECRET")
		os.Unsetenv("BOX_PRIVATE_KEY")
		os.Unsetenv("BOX_KEY_ID")
		os.Unsetenv("BOX_ENTERPRISE_ID")
	}()

	config := &Config{}
	config.loadFromEnvironment()

	if config.Box.AuthType != "service-to-service" {
		t.Errorf("Expected Box AuthType from env %s, got %s", "service-to-service", config.Box.AuthType)
	}
	if config.Box.ClientID != "env_box_client" {
		t.Errorf("Expected Box ClientID from env %s, got %s", "env_box_client", config.Box.ClientID)
	}
	if config.Box.ClientSecret != "env_box_secret" {
		t.Errorf("Expected Box ClientSecret from env %s, got %s", "env_box_secret", config.Box.ClientSecret)
	}
	if config.Box.PrivateKey != "env_private_key" {
		t.Errorf("Expected Box PrivateKey from env %s, got %s", "env_private_key", config.Box.PrivateKey)
	}
	if config.Box.KeyID != "env_key_id" {
		t.Errorf("Expected Box KeyID from env %s, got %s", "env_key_id", config.Box.KeyID)
	}
	if config.Box.EnterpriseID != "env_enterprise_id" {
		t.Errorf("Expected Box EnterpriseID from env %s, got %s", "env_enterprise_id", config.Box.EnterpriseID)
	}
}