package box

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockConfig struct {
	boxConfig BoxConfig
}

func (m *mockConfig) GetBoxConfig() BoxConfig {
	return m.boxConfig
}

func TestLoadCredentialsFromFile(t *testing.T) {
	tempDir := t.TempDir()
	
	tests := []struct {
		name          string
		credentials   *OAuth2Credentials
		expectedError string
	}{
		{
			name: "valid credentials with access token",
			credentials: &OAuth2Credentials{
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
				ExpiresIn:    3600,
				TokenType:    "bearer",
				Scope:        "base_explorer base_upload",
			},
		},
		{
			name: "valid credentials with only refresh token",
			credentials: &OAuth2Credentials{
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				RefreshToken: "test-refresh-token",
				TokenType:    "bearer",
				Scope:        "base_explorer base_upload",
			},
		},
		{
			name: "missing client_id",
			credentials: &OAuth2Credentials{
				ClientSecret: "test-secret",
				AccessToken:  "test-access-token",
			},
			expectedError: "client_id is required",
		},
		{
			name: "missing client_secret",
			credentials: &OAuth2Credentials{
				ClientID:    "test-client",
				AccessToken: "test-access-token",
			},
			expectedError: "client_secret is required",
		},
		{
			name: "missing both access_token and refresh_token",
			credentials: &OAuth2Credentials{
				ClientID:     "test-client",
				ClientSecret: "test-secret",
			},
			expectedError: "either access_token or refresh_token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialsFile := filepath.Join(tempDir, "credentials.json")
			
			// Write credentials to file
			data, err := json.Marshal(tt.credentials)
			if err != nil {
				t.Fatalf("Failed to marshal test credentials: %v", err)
			}
			
			if err := os.WriteFile(credentialsFile, data, 0600); err != nil {
				t.Fatalf("Failed to write credentials file: %v", err)
			}
			
			// Test loading
			creds, err := LoadCredentialsFromFile(credentialsFile)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if creds.ClientID != tt.credentials.ClientID {
				t.Errorf("Expected ClientID '%s', got '%s'", tt.credentials.ClientID, creds.ClientID)
			}
			
			if creds.ClientSecret != tt.credentials.ClientSecret {
				t.Errorf("Expected ClientSecret '%s', got '%s'", tt.credentials.ClientSecret, creds.ClientSecret)
			}
		})
	}
}

func TestLoadCredentialsFromFile_FileErrors(t *testing.T) {
	tempDir := t.TempDir()
	
	tests := []struct {
		name          string
		setupFile     func(string) error
		expectedError string
	}{
		{
			name: "file does not exist",
			setupFile: func(string) error {
				return nil // Don't create file
			},
			expectedError: "failed to read credentials file",
		},
		{
			name: "invalid JSON",
			setupFile: func(path string) error {
				return os.WriteFile(path, []byte("invalid json"), 0600)
			},
			expectedError: "failed to parse credentials JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialsFile := filepath.Join(tempDir, "test_credentials.json")
			
			if err := tt.setupFile(credentialsFile); err != nil {
				t.Fatalf("Failed to setup test file: %v", err)
			}
			
			_, err := LoadCredentialsFromFile(credentialsFile)
			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedError)
			} else if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
			}
		})
	}
}

func TestSaveCredentialsToFile(t *testing.T) {
	tempDir := t.TempDir()
	credentialsFile := filepath.Join(tempDir, "credentials.json")
	
	credentials := &OAuth2Credentials{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresIn:    3600,
		TokenType:    "bearer",
		Scope:        "base_explorer base_upload",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	err := SaveCredentialsToFile(credentials, credentialsFile)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	// Verify file was created and has correct content
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		t.Errorf("Failed to read saved credentials file: %v", err)
		return
	}

	var savedCreds OAuth2Credentials
	if err := json.Unmarshal(data, &savedCreds); err != nil {
		t.Errorf("Failed to parse saved credentials: %v", err)
		return
	}

	if savedCreds.ClientID != credentials.ClientID {
		t.Errorf("Expected saved ClientID '%s', got '%s'", credentials.ClientID, savedCreds.ClientID)
	}

	// Test nil credentials
	err = SaveCredentialsToFile(nil, credentialsFile)
	if err == nil {
		t.Error("Expected error for nil credentials, got nil")
	} else if !strings.Contains(err.Error(), "credentials cannot be nil") {
		t.Errorf("Expected error about nil credentials, got '%s'", err.Error())
	}
}

func TestNewBoxClientFromConfig(t *testing.T) {
	
	tests := []struct {
		name          string
		config        *mockConfig
		expectedError string
	}{
		{
			name: "Box disabled",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled: false,
				},
			},
			expectedError: "Box integration is disabled",
		},
		{
			name: "missing client_id",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled:      true,
					ClientID:     "",
					ClientSecret: "test-secret",
				},
			},
			expectedError: "box.client_id is required",
		},
		{
			name: "missing client_secret",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled:      true,
					ClientID:     "test-client",
					ClientSecret: "",
				},
			},
			expectedError: "box.client_secret is required",
		},
		{
			name: "valid configuration",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled:      true,
					ClientID:     "test-client",
					ClientSecret: "test-secret",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No setup needed for new structure
			
			client, err := NewBoxClientFromConfig(tt.config)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if client == nil {
				t.Error("Expected non-nil client")
			}
		})
	}
}

func TestCreateBoxUploadPath(t *testing.T) {
	tests := []struct {
		name         string
		config       *mockConfig
		userAccount  string
		year         string
		month        string
		day          string
		expectedPath string
	}{
		{
			name: "standard path",
			config: &mockConfig{
				boxConfig: BoxConfig{
				},
			},
			userAccount:  "john.doe@example.com",
			year:         "2024",
			month:        "01",
			day:          "15",
			expectedPath: "john.doe@example.com/2024/01/15",
		},
		{
			name: "empty folder ID",
			config: &mockConfig{
				boxConfig: BoxConfig{
				},
			},
			userAccount:  "jane.smith@example.com",
			year:         "2023",
			month:        "12",
			day:          "25",
			expectedPath: "jane.smith@example.com/2023/12/25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := CreateBoxUploadPath(tt.config, tt.userAccount, tt.year, tt.month, tt.day)
			if path != tt.expectedPath {
				t.Errorf("Expected path '%s', got '%s'", tt.expectedPath, path)
			}
		})
	}
}

func TestValidateBoxConfig(t *testing.T) {
	
	tests := []struct {
		name          string
		config        *mockConfig
		expectedError string
	}{
		{
			name: "Box disabled - no validation",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled: false,
				},
			},
		},
		{
			name: "missing client_id",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled:      true,
					ClientID:     "",
					ClientSecret: "test-secret",
				},
			},
			expectedError: "box.client_id is required",
		},
		{
			name: "missing client_secret",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled:      true,
					ClientID:     "test-client",
					ClientSecret: "",
				},
			},
			expectedError: "box.client_secret is required",
		},
		{
			name: "valid configuration",
			config: &mockConfig{
				boxConfig: BoxConfig{
					Enabled:      true,
					ClientID:     "test-client",
					ClientSecret: "test-secret",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBoxConfig(tt.config)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}