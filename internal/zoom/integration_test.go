package zoom

import (
	"testing"

	"github.com/curtbushko/zoom-to-box/internal/config"
)

// TestConfigIntegration tests that the authentication system integrates properly with configuration
func TestConfigIntegration(t *testing.T) {
	// Test configuration from plan.md example
	cfg := config.ZoomConfig{
		AccountID:    "test_account_id",
		ClientID:     "test_client_id", 
		ClientSecret: "test_client_secret",
		BaseURL:      "https://api.zoom.us/v2",
	}

	// Create authenticator with config
	auth := NewServerToServerAuth(cfg)
	if auth == nil {
		t.Fatal("Expected authenticator to be created, got nil")
	}

	// Verify configuration was loaded correctly
	if auth.config.AccountID != cfg.AccountID {
		t.Errorf("Expected AccountID %s, got %s", cfg.AccountID, auth.config.AccountID)
	}
	if auth.config.ClientID != cfg.ClientID {
		t.Errorf("Expected ClientID %s, got %s", cfg.ClientID, auth.config.ClientID)
	}
	if auth.config.ClientSecret != cfg.ClientSecret {
		t.Errorf("Expected ClientSecret %s, got %s", cfg.ClientSecret, auth.config.ClientSecret)
	}
	if auth.config.BaseURL != cfg.BaseURL {
		t.Errorf("Expected BaseURL %s, got %s", cfg.BaseURL, auth.config.BaseURL)
	}

	// Test that JWT generation works with loaded config
	jwt, err := auth.generateJWT()
	if err != nil {
		t.Fatalf("JWT generation failed with loaded config: %v", err)
	}
	if jwt == "" {
		t.Error("Expected JWT to be generated, got empty string")
	}

	// Test scope validation
	token := &AccessToken{
		AccessToken: "test_token",
		TokenType:   "Bearer", 
		Scopes:      []string{"recording:read", "user:read", "meeting:read"},
	}

	requiredScopes := []string{"recording:read", "user:read"}
	err = auth.ValidateScopes(token, requiredScopes)
	if err != nil {
		t.Errorf("Scope validation failed: %v", err)
	}

	t.Log("✅ Configuration integration test completed successfully")
}