package box

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type BoxConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	AuthType     string `yaml:"auth_type" json:"auth_type"`
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
	PrivateKey   string `yaml:"private_key" json:"private_key"`
	KeyID        string `yaml:"key_id" json:"key_id"`
	EnterpriseID string `yaml:"enterprise_id" json:"enterprise_id"`
	FolderID     string `yaml:"folder_id" json:"folder_id"`
}

type Config interface {
	GetBoxConfig() BoxConfig
}

func NewBoxClientFromConfig(config Config) (BoxClient, error) {
	boxConfig := config.GetBoxConfig()
	
	if !boxConfig.Enabled {
		return nil, fmt.Errorf("Box integration is disabled in configuration")
	}

	if boxConfig.ClientID == "" {
		return nil, fmt.Errorf("box.client_id is required when Box is enabled")
	}
	if boxConfig.ClientSecret == "" {
		return nil, fmt.Errorf("box.client_secret is required when Box is enabled")
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	var auth Authenticator

	// Set default auth type if not specified
	authType := boxConfig.AuthType
	if authType == "" {
		authType = "oauth"
	}

	switch authType {
	case "oauth":
		credentials := &OAuth2Credentials{
			ClientID:     boxConfig.ClientID,
			ClientSecret: boxConfig.ClientSecret,
		}
		auth = NewOAuth2Authenticator(credentials, httpClient)
		
	case "service-to-service":
		if boxConfig.PrivateKey == "" {
			return nil, fmt.Errorf("box.private_key is required for service-to-service authentication")
		}
		if boxConfig.KeyID == "" {
			return nil, fmt.Errorf("box.key_id is required for service-to-service authentication") 
		}
		if boxConfig.EnterpriseID == "" {
			return nil, fmt.Errorf("box.enterprise_id is required for service-to-service authentication")
		}
		
		credentials := &ServiceToServiceCredentials{
			ClientID:     boxConfig.ClientID,
			ClientSecret: boxConfig.ClientSecret,
			PrivateKey:   boxConfig.PrivateKey,
			KeyID:        boxConfig.KeyID,
			EnterpriseID: boxConfig.EnterpriseID,
		}
		auth = NewServiceToServiceAuthenticator(credentials, httpClient)
		
	default:
		return nil, fmt.Errorf("unsupported box.auth_type: %s (supported: oauth, service-to-service)", authType)
	}

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

	if boxConfig.ClientID == "" {
		return nil, fmt.Errorf("box.client_id is required when Box is enabled")
	}
	if boxConfig.ClientSecret == "" {
		return nil, fmt.Errorf("box.client_secret is required when Box is enabled")
	}

	credentials := &OAuth2Credentials{
		ClientID:     boxConfig.ClientID,
		ClientSecret: boxConfig.ClientSecret,
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	auth := NewOAuth2Authenticator(credentials, httpClient)
	
	if saveCredentials != nil {
		auth.(*oauth2Authenticator).SetCredentialsUpdateCallback(saveCredentials)
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

	// Set default auth type if not specified
	authType := boxConfig.AuthType
	if authType == "" {
		authType = "oauth"
	}

	// Validate auth type
	validAuthTypes := map[string]bool{
		"oauth":              true,
		"service-to-service": true,
	}
	if !validAuthTypes[authType] {
		return fmt.Errorf("box.auth_type must be one of: oauth, service-to-service")
	}

	// Validate common required fields
	if boxConfig.ClientID == "" {
		return fmt.Errorf("box.client_id is required when Box is enabled")
	}

	// Validate fields based on auth type
	switch authType {
	case "oauth":
		if boxConfig.ClientSecret == "" {
			return fmt.Errorf("box.client_secret is required for OAuth authentication")
		}
	case "service-to-service":
		if boxConfig.ClientSecret == "" {
			return fmt.Errorf("box.client_secret is required for service-to-service authentication")
		}
		if boxConfig.PrivateKey == "" {
			return fmt.Errorf("box.private_key is required for service-to-service authentication")
		}
		if boxConfig.KeyID == "" {
			return fmt.Errorf("box.key_id is required for service-to-service authentication")
		}
		if boxConfig.EnterpriseID == "" {
			return fmt.Errorf("box.enterprise_id is required for service-to-service authentication")
		}
	}

	return nil
}