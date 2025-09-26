package box

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type BoxConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	CredentialsFile string `yaml:"credentials_file" json:"credentials_file"`
	FolderID        string `yaml:"folder_id" json:"folder_id"`
}

type Config interface {
	GetBoxConfig() BoxConfig
}

func NewBoxClientFromConfig(config Config) (BoxClient, error) {
	boxConfig := config.GetBoxConfig()
	
	if !boxConfig.Enabled {
		return nil, fmt.Errorf("Box integration is disabled in configuration")
	}

	if boxConfig.CredentialsFile == "" {
		return nil, fmt.Errorf("box.credentials_file is required when Box is enabled")
	}

	credentials, err := LoadCredentialsFromFile(boxConfig.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load Box credentials: %w", err)
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	auth := NewOAuth2Authenticator(credentials, httpClient)
	client := NewBoxClient(auth, httpClient)

	return client, nil
}

func LoadCredentialsFromFile(credentialsFile string) (*OAuth2Credentials, error) {
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file %s: %w", credentialsFile, err)
	}

	var credentials OAuth2Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	if credentials.ClientID == "" {
		return nil, fmt.Errorf("client_id is required in credentials file")
	}
	if credentials.ClientSecret == "" {
		return nil, fmt.Errorf("client_secret is required in credentials file")
	}
	if credentials.AccessToken == "" && credentials.RefreshToken == "" {
		return nil, fmt.Errorf("either access_token or refresh_token is required in credentials file")
	}

	return &credentials, nil
}

func SaveCredentialsToFile(credentials *OAuth2Credentials, credentialsFile string) error {
	if credentials == nil {
		return fmt.Errorf("credentials cannot be nil")
	}

	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials to JSON: %w", err)
	}

	if err := os.WriteFile(credentialsFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file %s: %w", credentialsFile, err)
	}

	return nil
}

func CreateBoxClientWithCredentialsCallback(config Config, saveCredentials func(*OAuth2Credentials) error) (BoxClient, error) {
	boxConfig := config.GetBoxConfig()
	
	if !boxConfig.Enabled {
		return nil, fmt.Errorf("Box integration is disabled in configuration")
	}

	if boxConfig.CredentialsFile == "" {
		return nil, fmt.Errorf("box.credentials_file is required when Box is enabled")
	}

	credentials, err := LoadCredentialsFromFile(boxConfig.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load Box credentials: %w", err)
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	auth := NewOAuth2Authenticator(credentials, httpClient)
	
	if saveCredentials != nil {
		auth.(*oauth2Authenticator).SetCredentialsUpdateCallback(func(creds *OAuth2Credentials) error {
			if err := saveCredentials(creds); err != nil {
				return fmt.Errorf("failed to save updated credentials: %w", err)
			}
			return SaveCredentialsToFile(creds, boxConfig.CredentialsFile)
		})
	}

	client := NewBoxClient(auth, httpClient)
	return client, nil
}

func CreateBoxUploadPath(config Config, userAccount, year, month, day string) string {
	boxConfig := config.GetBoxConfig()
	if boxConfig.FolderID == "" {
		return fmt.Sprintf("%s/%s/%s/%s", userAccount, year, month, day)
	}
	return fmt.Sprintf("%s/%s/%s/%s", userAccount, year, month, day)
}

func ValidateBoxConfig(config Config) error {
	boxConfig := config.GetBoxConfig()
	
	if !boxConfig.Enabled {
		return nil
	}

	if boxConfig.CredentialsFile == "" {
		return fmt.Errorf("box.credentials_file is required when Box is enabled")
	}

	_, err := LoadCredentialsFromFile(boxConfig.CredentialsFile)
	if err != nil {
		return fmt.Errorf("invalid Box credentials: %w", err)
	}

	return nil
}