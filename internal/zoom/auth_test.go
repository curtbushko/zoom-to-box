package zoom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

// TestServerToServerAuth tests the Server-to-Server OAuth authentication
func TestServerToServerAuth(t *testing.T) {
	tests := []struct {
		name           string
		config         config.ZoomConfig
		serverResponse string
		serverStatus   int
		expectedError  bool
		expectedScopes []string
	}{
		{
			name: "successful authentication",
			config: config.ZoomConfig{
				AccountID:    "test_account_id",
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
				BaseURL:      "https://api.zoom.us/v2",
			},
			serverResponse: `{
				"access_token": "test_access_token_123",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "recording:read user:read meeting:read"
			}`,
			serverStatus:   200,
			expectedError:  false,
			expectedScopes: []string{"recording:read", "user:read", "meeting:read"},
		},
		{
			name: "invalid credentials",
			config: config.ZoomConfig{
				AccountID:    "invalid_account",
				ClientID:     "invalid_client",
				ClientSecret: "invalid_secret",
			},
			serverResponse: `{
				"reason": "Invalid client_id or client_secret",
				"error": "invalid_client"
			}`,
			serverStatus:  401,
			expectedError: true,
		},
		{
			name: "invalid account ID",
			config: config.ZoomConfig{
				AccountID:    "invalid_account_id",
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
			},
			serverResponse: `{
				"reason": "Invalid account_id",
				"error": "invalid_request"
			}`,
			serverStatus:  400,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify this is the OAuth token endpoint
				if !strings.HasSuffix(r.URL.Path, "/oauth/token") {
					t.Errorf("Expected OAuth token endpoint, got %s", r.URL.Path)
				}

				// Verify Content-Type
				if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Errorf("Expected Content-Type application/x-www-form-urlencoded, got %s", r.Header.Get("Content-Type"))
				}

				// Verify request method
				if r.Method != "POST" {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				// Parse form data
				if err := r.ParseForm(); err != nil {
					t.Fatalf("Failed to parse form: %v", err)
				}

				// Verify grant type
				if r.Form.Get("grant_type") != "account_credentials" {
					t.Errorf("Expected grant_type 'account_credentials', got %s", r.Form.Get("grant_type"))
				}

				// Verify account_id
				if r.Form.Get("account_id") != tt.config.AccountID {
					t.Errorf("Expected account_id %s, got %s", tt.config.AccountID, r.Form.Get("account_id"))
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			// Update config with test server URL
			testConfig := tt.config
			testConfig.BaseURL = server.URL

			// Create authenticator
			auth := NewServerToServerAuth(testConfig)

			// Test authentication
			ctx := context.Background()
			token, err := auth.GetAccessToken(ctx)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if token.AccessToken != "test_access_token_123" {
				t.Errorf("Expected access token 'test_access_token_123', got %s", token.AccessToken)
			}

			if token.TokenType != "Bearer" {
				t.Errorf("Expected token type 'Bearer', got %s", token.TokenType)
			}

			if len(tt.expectedScopes) > 0 {
				for _, expectedScope := range tt.expectedScopes {
					found := false
					for _, scope := range token.Scopes {
						if scope == expectedScope {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected scope %s not found in token scopes: %v", expectedScope, token.Scopes)
					}
				}
			}
		})
	}
}

// TestJWTGeneration tests JWT token generation for Server-to-Server OAuth
func TestJWTGeneration(t *testing.T) {
	config := config.ZoomConfig{
		AccountID:    "test_account_id",
		ClientID:     "test_client_id",
		ClientSecret: "test_client_secret",
	}

	auth := NewServerToServerAuth(config)

	// Generate JWT
	jwtToken, err := auth.generateJWT()
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Parse and validate JWT
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			t.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.ClientSecret), nil
	})

	if err != nil {
		t.Fatalf("Failed to parse JWT: %v", err)
	}

	if !token.Valid {
		t.Error("JWT token is not valid")
	}

	// Verify claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("Failed to parse JWT claims")
	}

	// Check issuer (iss)
	if claims["iss"] != config.ClientID {
		t.Errorf("Expected iss claim %s, got %v", config.ClientID, claims["iss"])
	}

	// Check expiration (exp) - should be within the next hour
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatal("Expected exp claim to be a number")
	}
	expectedExp := time.Now().Add(time.Hour).Unix()
	if int64(exp) > expectedExp+60 || int64(exp) < expectedExp-60 {
		t.Errorf("Expected exp claim around %d, got %d", expectedExp, int64(exp))
	}

	// Check issued at (iat) - should be recent
	iat, ok := claims["iat"].(float64)
	if !ok {
		t.Fatal("Expected iat claim to be a number")
	}
	now := time.Now().Unix()
	if int64(iat) > now+60 || int64(iat) < now-60 {
		t.Errorf("Expected iat claim around %d, got %d", now, int64(iat))
	}
}

// TestTokenRefresh tests automatic token refresh functionality
func TestTokenRefresh(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		
		// First call returns a token expiring soon
		if callCount == 1 {
			response := `{
				"access_token": "token_1",
				"token_type": "Bearer",
				"expires_in": 1,
				"scope": "recording:read"
			}`
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(response))
		} else if callCount == 2 {
			// Second call returns a fresh token
			response := `{
				"access_token": "token_2",
				"token_type": "Bearer",
				"expires_in": 3600,
				"scope": "recording:read"
			}`
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(response))
		}
	}))
	defer server.Close()

	config := config.ZoomConfig{
		AccountID:    "test_account",
		ClientID:     "test_client",
		ClientSecret: "test_secret",
		BaseURL:      server.URL,
	}

	auth := NewServerToServerAuth(config)
	ctx := context.Background()

	// Get first token
	token1, err := auth.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get first token: %v", err)
	}

	if token1.AccessToken != "token_1" {
		t.Errorf("Expected first token 'token_1', got %s", token1.AccessToken)
	}

	// Wait for token to expire and get a new one
	time.Sleep(2 * time.Second)
	
	token2, err := auth.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get refreshed token: %v", err)
	}

	if token2.AccessToken != "token_2" {
		t.Errorf("Expected refreshed token 'token_2', got %s", token2.AccessToken)
	}

	// Verify we made exactly 2 calls to the server
	if callCount != 2 {
		t.Errorf("Expected 2 server calls, got %d", callCount)
	}
}

// TestAuthenticationHeaders tests that Bearer tokens are properly added to requests
func TestAuthenticationHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			// OAuth endpoint
			response := `{
				"access_token": "test_bearer_token",
				"token_type": "Bearer",
				"expires_in": 3600
			}`
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(response))
		} else {
			// API endpoint - check for Bearer token
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer test_bearer_token" {
				t.Errorf("Expected Authorization header 'Bearer test_bearer_token', got %s", authHeader)
			}
			w.WriteHeader(200)
			w.Write([]byte(`{"success": true}`))
		}
	}))
	defer server.Close()

	config := config.ZoomConfig{
		AccountID:    "test_account",
		ClientID:     "test_client",
		ClientSecret: "test_secret",
		BaseURL:      server.URL,
	}

	auth := NewServerToServerAuth(config)
	ctx := context.Background()

	// Create an authenticated HTTP client
	client := &http.Client{}
	authenticatedClient := &AuthenticatedClient{
		client: client,
		auth:   auth,
	}

	// Make a test API request
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := authenticatedClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make authenticated request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestScopeValidation tests that required scopes are present in tokens
func TestScopeValidation(t *testing.T) {
	tests := []struct {
		name           string
		serverScopes   string
		requiredScopes []string
		shouldError    bool
	}{
		{
			name:           "all required scopes present",
			serverScopes:   "recording:read user:read meeting:read",
			requiredScopes: []string{"recording:read", "user:read"},
			shouldError:    false,
		},
		{
			name:           "missing required scope",
			serverScopes:   "recording:read meeting:read",
			requiredScopes: []string{"recording:read", "user:read"},
			shouldError:    true,
		},
		{
			name:           "no scopes provided",
			serverScopes:   "",
			requiredScopes: []string{"recording:read"},
			shouldError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := `{
					"access_token": "test_token",
					"token_type": "Bearer",
					"expires_in": 3600,
					"scope": "` + tt.serverScopes + `"
				}`
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(response))
			}))
			defer server.Close()

			config := config.ZoomConfig{
				AccountID:    "test_account",
				ClientID:     "test_client",
				ClientSecret: "test_secret",
				BaseURL:      server.URL,
			}

			auth := NewServerToServerAuth(config)
			ctx := context.Background()

			token, err := auth.GetAccessToken(ctx)
			if err != nil {
				t.Fatalf("Failed to get token: %v", err)
			}

			// Validate scopes
			err = auth.ValidateScopes(token, tt.requiredScopes)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for scope validation, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error during scope validation: %v", err)
				}
			}
		})
	}
}

// TestErrorHandling tests various error scenarios
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		serverStatus   int
		expectedError  string
	}{
		{
			name: "network timeout",
			// No server response - will cause timeout
			expectedError: "failed to get access token",
		},
		{
			name:           "malformed JSON response",
			serverResponse: `{"invalid": json}`,
			serverStatus:   200,
			expectedError:  "failed to parse token response",
		},
		{
			name: "API rate limit",
			serverResponse: `{
				"error": "rate_limit_exceeded",
				"reason": "Too many requests"
			}`,
			serverStatus:  429,
			expectedError: "rate_limit_exceeded",
		},
		{
			name: "server error",
			serverResponse: `{
				"error": "internal_server_error",
				"reason": "Server encountered an error"
			}`,
			serverStatus:  500,
			expectedError: "internal_server_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "network timeout" {
				// Test network timeout by using an invalid URL
				config := config.ZoomConfig{
					AccountID:    "test_account",
					ClientID:     "test_client",
					ClientSecret: "test_secret",
					BaseURL:      "http://nonexistent.example.com",
				}

				auth := NewServerToServerAuth(config)
				ctx := context.Background()

				_, err := auth.GetAccessToken(ctx)
				if err == nil {
					t.Error("Expected network error, but got none")
				}
				if !strings.Contains(err.Error(), "failed to get access token") {
					t.Errorf("Expected error to contain 'failed to get access token', got %s", err.Error())
				}
				return
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverStatus)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			config := config.ZoomConfig{
				AccountID:    "test_account",
				ClientID:     "test_client",
				ClientSecret: "test_secret",
				BaseURL:      server.URL,
			}

			auth := NewServerToServerAuth(config)
			ctx := context.Background()

			_, err := auth.GetAccessToken(ctx)
			if err == nil {
				t.Error("Expected error, but got none")
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error to contain '%s', got %s", tt.expectedError, err.Error())
			}
		})
	}
}