// Package zoom provides Zoom API authentication and client functionality
package zoom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

// AccessToken represents an OAuth access token with metadata
type AccessToken struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int       `json:"expires_in"`
	Scopes      []string  `json:"-"` // Parsed from scope string
	ExpiresAt   time.Time `json:"-"` // Calculated expiration time
}

// IsExpired returns true if the token is expired or will expire within the buffer time
func (t *AccessToken) IsExpired(buffer time.Duration) bool {
	return time.Now().Add(buffer).After(t.ExpiresAt)
}

// TokenResponse represents the response from the OAuth token endpoint
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// AuthError represents authentication-related errors
type AuthError struct {
	Type   string
	Reason string
	Err    error
}

func (e *AuthError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("auth error %s: %s (%v)", e.Type, e.Reason, e.Err)
	}
	return fmt.Sprintf("auth error %s: %s", e.Type, e.Reason)
}

// Authenticator defines the interface for Zoom API authentication
type Authenticator interface {
	GetAccessToken(ctx context.Context) (*AccessToken, error)
	ValidateScopes(token *AccessToken, requiredScopes []string) error
}

// ServerToServerAuth implements Server-to-Server OAuth authentication for Zoom
type ServerToServerAuth struct {
	config      config.ZoomConfig
	client      *http.Client
	cachedToken *AccessToken
}

// NewServerToServerAuth creates a new Server-to-Server OAuth authenticator
func NewServerToServerAuth(cfg config.ZoomConfig) *ServerToServerAuth {
	return &ServerToServerAuth{
		config: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetAccessToken obtains or refreshes an access token using Server-to-Server OAuth
func (s *ServerToServerAuth) GetAccessToken(ctx context.Context) (*AccessToken, error) {
	if s.cachedToken != nil && !s.cachedToken.IsExpired(5*time.Minute) {
		return s.cachedToken, nil
	}

	// Generate JWT token
	jwtToken, err := s.generateJWT()
	if err != nil {
		return nil, &AuthError{
			Type:   "jwt_generation",
			Reason: "failed to generate JWT token",
			Err:    err,
		}
	}

	// Prepare OAuth request
	tokenURL := "https://zoom.us/oauth/token"
	data := url.Values{}
	data.Set("grant_type", "account_credentials")
	data.Set("account_id", s.config.AccountID)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, &AuthError{
			Type:   "request_creation",
			Reason: "failed to create OAuth request",
			Err:    err,
		}
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	// Make OAuth request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &AuthError{
			Type:   "request_failed",
			Reason: "failed to get access token",
			Err:    err,
		}
	}
	defer resp.Body.Close()

	// Parse response
	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, &AuthError{
			Type:   "response_parsing",
			Reason: "failed to parse token response",
			Err:    err,
		}
	}

	// Check for OAuth errors
	if tokenResponse.Error != "" {
		return nil, &AuthError{
			Type:   tokenResponse.Error,
			Reason: tokenResponse.Reason,
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &AuthError{
			Type:   "http_error",
			Reason: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, tokenResponse.Reason),
		}
	}

	// Create access token
	token := &AccessToken{
		AccessToken: tokenResponse.AccessToken,
		TokenType:   tokenResponse.TokenType,
		ExpiresIn:   tokenResponse.ExpiresIn,
		ExpiresAt:   time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second),
	}

	// Parse scopes
	if tokenResponse.Scope != "" {
		token.Scopes = strings.Fields(tokenResponse.Scope)
	}

	s.cachedToken = token
	return token, nil
}

// generateJWT generates a JWT token for Server-to-Server OAuth
func (s *ServerToServerAuth) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": s.config.ClientID,                // Issuer (Client ID)
		"exp": now.Add(time.Hour).Unix(),        // Expiration (1 hour from now)
		"iat": now.Unix(),                       // Issued at
		"aud": "zoom",                           // Audience (Zoom)
		"appKey": s.config.ClientID,             // App Key (same as Client ID)
		"tokenExp": now.Add(time.Hour).Unix(),   // Token expiration
		"alg": "HS256",                          // Algorithm
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.ClientSecret))
}

// ValidateScopes validates that the token has all required scopes
func (s *ServerToServerAuth) ValidateScopes(token *AccessToken, requiredScopes []string) error {
	if len(requiredScopes) == 0 {
		return nil
	}

	tokenScopes := make(map[string]bool)
	for _, scope := range token.Scopes {
		tokenScopes[scope] = true
	}

	var missingScopes []string
	for _, required := range requiredScopes {
		if !tokenScopes[required] {
			missingScopes = append(missingScopes, required)
		}
	}

	if len(missingScopes) > 0 {
		return &AuthError{
			Type:   "insufficient_scope",
			Reason: fmt.Sprintf("missing required scopes: %s", strings.Join(missingScopes, ", ")),
		}
	}

	return nil
}

// AuthenticatedClient is an HTTP client that automatically adds authentication headers
type AuthenticatedClient struct {
	client *http.Client
	auth   Authenticator
}

// NewAuthenticatedClient creates an HTTP client with automatic authentication
func NewAuthenticatedClient(client *http.Client, auth Authenticator) *AuthenticatedClient {
	return &AuthenticatedClient{
		client: client,
		auth:   auth,
	}
}

// Do executes an HTTP request with automatic authentication
func (c *AuthenticatedClient) Do(req *http.Request) (*http.Response, error) {
	// Get access token
	token, err := c.auth.GetAccessToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to get access token for request: %w", err)
	}

	// Add Authorization header
	req.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)

	// Execute request
	return c.client.Do(req)
}

// Client returns the underlying HTTP client
func (c *AuthenticatedClient) Client() *http.Client {
	return c.client
}