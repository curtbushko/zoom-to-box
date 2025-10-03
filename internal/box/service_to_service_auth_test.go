package box

import (
	"testing"
	"time"
)

func TestNewServiceToServiceAuthenticator(t *testing.T) {
	creds := &ServiceToServiceCredentials{
		ClientID:     "test_client_id",
		ClientSecret: "test_client_secret",
		PrivateKey:   "test_private_key",
		KeyID:        "test_key_id",
		EnterpriseID: "test_enterprise_id",
	}

	auth := NewServiceToServiceAuthenticator(creds, nil)
	if auth == nil {
		t.Fatal("Expected authenticator, got nil")
	}

	ssAuth, ok := auth.(*serviceToServiceAuthenticator)
	if !ok {
		t.Fatalf("Expected *serviceToServiceAuthenticator, got %T", auth)
	}

	if ssAuth.credentials != creds {
		t.Error("Expected credentials to be set correctly")
	}

	if ssAuth.httpClient == nil {
		t.Error("Expected http client to be initialized")
	}
}

func TestServiceToServiceCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name        string
		credentials *ServiceToServiceCredentials
		expected    bool
	}{
		{
			name: "valid non-expired token",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "valid_token",
				ExpiresAt:   time.Now().Add(10 * time.Minute),
			},
			expected: false,
		},
		{
			name: "expired token",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "expired_token",
				ExpiresAt:   time.Now().Add(-10 * time.Minute),
			},
			expected: true,
		},
		{
			name: "token expires soon",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "expires_soon_token",
				ExpiresAt:   time.Now().Add(2 * time.Minute),
			},
			expected: true,
		},
		{
			name: "zero expiration time",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "token",
				ExpiresAt:   time.Time{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.credentials.IsExpired()
			if result != tt.expected {
				t.Errorf("Expected IsExpired() = %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestServiceToServiceAuthenticator_GetAccessToken(t *testing.T) {
	tests := []struct {
		name        string
		credentials *ServiceToServiceCredentials
		expected    string
	}{
		{
			name: "valid credentials",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "test_token",
			},
			expected: "test_token",
		},
		{
			name:        "nil credentials",
			credentials: nil,
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &serviceToServiceAuthenticator{
				credentials: tt.credentials,
			}

			result := auth.GetAccessToken()
			if result != tt.expected {
				t.Errorf("Expected GetAccessToken() = %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestServiceToServiceAuthenticator_IsAuthenticated(t *testing.T) {
	tests := []struct {
		name        string
		credentials *ServiceToServiceCredentials
		expected    bool
	}{
		{
			name: "valid non-expired token",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "valid_token",
				ExpiresAt:   time.Now().Add(10 * time.Minute),
			},
			expected: true,
		},
		{
			name: "expired token",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "expired_token",
				ExpiresAt:   time.Now().Add(-10 * time.Minute),
			},
			expected: false,
		},
		{
			name: "empty access token",
			credentials: &ServiceToServiceCredentials{
				AccessToken: "",
				ExpiresAt:   time.Now().Add(10 * time.Minute),
			},
			expected: false,
		},
		{
			name:        "nil credentials",
			credentials: nil,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &serviceToServiceAuthenticator{
				credentials: tt.credentials,
			}

			result := auth.IsAuthenticated()
			if result != tt.expected {
				t.Errorf("Expected IsAuthenticated() = %t, got %t", tt.expected, result)
			}
		})
	}
}

func TestServiceToServiceAuthenticator_GetCredentials(t *testing.T) {
	auth := &serviceToServiceAuthenticator{}

	result := auth.GetCredentials()
	if result != nil {
		t.Errorf("Expected GetCredentials() = nil for service-to-service auth, got %v", result)
	}
}

func TestServiceToServiceAuthenticator_UpdateCredentials(t *testing.T) {
	auth := &serviceToServiceAuthenticator{}

	err := auth.UpdateCredentials(&OAuth2Credentials{})
	if err == nil {
		t.Error("Expected UpdateCredentials() to return error for service-to-service auth")
	}

	expectedMsg := "UpdateCredentials is not supported for service-to-service authentication"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message %q, got %q", expectedMsg, err.Error())
	}
}