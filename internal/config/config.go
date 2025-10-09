// Package config provides configuration management for the zoom-to-box application
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ZoomConfig holds Zoom API authentication and connection settings
type ZoomConfig struct {
	AccountID    string `yaml:"account_id" json:"account_id"`
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
	BaseURL      string `yaml:"base_url" json:"base_url"`
}

// BoxConfig holds Box API authentication and settings
type BoxConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
	EnterpriseID string `yaml:"enterprise_id" json:"enterprise_id"`
}

// DownloadConfig holds download-related settings
type DownloadConfig struct {
	OutputDir      string `yaml:"output_dir" json:"output_dir"`
	RetryAttempts  int    `yaml:"retry_attempts" json:"retry_attempts"`
	TimeoutSeconds int    `yaml:"timeout_seconds" json:"timeout_seconds"`
}

// TimeoutDuration returns the timeout as a time.Duration
func (d DownloadConfig) TimeoutDuration() time.Duration {
	return time.Duration(d.TimeoutSeconds) * time.Second
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level      string `yaml:"level" json:"level"`
	File       string `yaml:"file" json:"file"`
	Console    bool   `yaml:"console" json:"console"`
	JSONFormat bool   `yaml:"json_format" json:"json_format"`
}

// ActiveUsersConfig holds active users list settings
type ActiveUsersConfig struct {
	File         string `yaml:"file" json:"file"`
	CheckEnabled bool   `yaml:"check_enabled" json:"check_enabled"`
}

// Config represents the complete application configuration
type Config struct {
	Zoom        ZoomConfig        `yaml:"zoom" json:"zoom"`
	Box         BoxConfig         `yaml:"box" json:"box"`
	Download    DownloadConfig    `yaml:"download" json:"download"`
	Logging     LoggingConfig     `yaml:"logging" json:"logging"`
	ActiveUsers ActiveUsersConfig `yaml:"active_users" json:"active_users"`
}

// LoadConfig loads configuration from a YAML file with defaults and environment variable overrides
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{}

	// Load from YAML file
	if err := config.loadFromFile(configPath); err != nil {
		return nil, fmt.Errorf("failed to load config from file: %w", err)
	}

	// Apply defaults
	config.setDefaults()

	// Override with environment variables
	config.loadFromEnvironment()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// loadFromFile loads configuration from a YAML file
func (c *Config) loadFromFile(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return nil
}

// setDefaults applies default values for missing configuration
func (c *Config) setDefaults() {
	// Zoom defaults
	if c.Zoom.BaseURL == "" {
		c.Zoom.BaseURL = "https://api.zoom.us/v2"
	}

	// Box defaults
	// Box.Enabled defaults to false (zero value)

	// Download defaults
	if c.Download.OutputDir == "" {
		c.Download.OutputDir = "./downloads"
	}
	if c.Download.RetryAttempts == 0 {
		c.Download.RetryAttempts = 3
	}
	if c.Download.TimeoutSeconds == 0 {
		c.Download.TimeoutSeconds = 300
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.File == "" {
		c.Logging.File = "./zoom-downloader.log"
	}
	// Console defaults to true (if not explicitly configured)
	// Note: This will always set to true, override in YAML if false is desired
	c.Logging.Console = true

	// Active users defaults
	if c.ActiveUsers.File == "" {
		c.ActiveUsers.File = "./active_users.txt"
	}
	// CheckEnabled defaults to true (if not explicitly configured)
	// Note: This will always set to true, override in YAML if false is desired
	c.ActiveUsers.CheckEnabled = true
}

// loadFromEnvironment overrides configuration with environment variables
func (c *Config) loadFromEnvironment() {
	if val := os.Getenv("ZOOM_ACCOUNT_ID"); val != "" {
		c.Zoom.AccountID = val
	}
	if val := os.Getenv("ZOOM_CLIENT_ID"); val != "" {
		c.Zoom.ClientID = val
	}
	if val := os.Getenv("ZOOM_CLIENT_SECRET"); val != "" {
		c.Zoom.ClientSecret = val
	}
	if val := os.Getenv("ZOOM_BASE_URL"); val != "" {
		c.Zoom.BaseURL = val
	}

	if val := os.Getenv("BOX_CLIENT_ID"); val != "" {
		c.Box.ClientID = val
	}
	if val := os.Getenv("BOX_CLIENT_SECRET"); val != "" {
		c.Box.ClientSecret = val
	}
	if val := os.Getenv("BOX_ENTERPRISE_ID"); val != "" {
		c.Box.EnterpriseID = val
	}

	if val := os.Getenv("DOWNLOAD_OUTPUT_DIR"); val != "" {
		c.Download.OutputDir = val
	}
}

// Validate performs validation on the loaded configuration
func (c *Config) Validate() error {
	// Validate required Zoom configuration
	if c.Zoom.AccountID == "" {
		return fmt.Errorf("zoom.account_id is required")
	}
	if c.Zoom.ClientID == "" {
		return fmt.Errorf("zoom.client_id is required")
	}
	if c.Zoom.ClientSecret == "" {
		return fmt.Errorf("zoom.client_secret is required")
	}

	// Validate download configuration
	if c.Download.RetryAttempts < 0 {
		return fmt.Errorf("download.retry_attempts must be >= 0")
	}
	if c.Download.TimeoutSeconds <= 0 {
		return fmt.Errorf("download.timeout_seconds must be greater than 0")
	}

	// Validate logging configuration
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}

	return nil
}

// GetBoxConfig returns the Box configuration
func (c *Config) GetBoxConfig() BoxConfig {
	return c.Box
}