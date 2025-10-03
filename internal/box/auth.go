// Package box provides OAuth 2.0 authentication for Box API
package box

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Authenticator defines the interface for Box OAuth 2.0 authentication
type Authenticator interface {
	// RefreshToken refreshes the access token using the refresh token
	RefreshToken(ctx context.Context) error
	
	// GetAccessToken returns the current access token
	GetAccessToken() string
	
	// IsAuthenticated returns true if we have a valid access token
	IsAuthenticated() bool
	
	// GetCredentials returns the current credentials
	GetCredentials() *OAuth2Credentials
	
	// UpdateCredentials updates the stored credentials
	UpdateCredentials(creds *OAuth2Credentials) error
}

// AuthenticatedHTTPClient provides an HTTP client with automatic OAuth token handling
type AuthenticatedHTTPClient interface {
	// Do performs an HTTP request with automatic token refresh
	Do(req *http.Request) (*http.Response, error)
	
	// Get performs a GET request with authentication
	Get(ctx context.Context, url string) (*http.Response, error)
	
	// Post performs a POST request with authentication
	Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error)
	
	// PostJSON performs a POST request with JSON body
	PostJSON(ctx context.Context, url string, payload interface{}) (*http.Response, error)
}

// oauth2Authenticator implements OAuth 2.0 authentication for Box
type oauth2Authenticator struct {
	credentials *OAuth2Credentials
	httpClient  *http.Client
	mutex       sync.RWMutex
	
	// Callbacks for credential updates
	onCredentialsUpdated func(*OAuth2Credentials) error
}

// NewOAuth2Authenticator creates a new OAuth 2.0 authenticator for Box
func NewOAuth2Authenticator(creds *OAuth2Credentials, httpClient *http.Client) Authenticator {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	
	// Set expires_at if not set
	if creds != nil && creds.ExpiresAt.IsZero() && creds.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().Add(time.Duration(creds.ExpiresIn) * time.Second)
	}
	
	return &oauth2Authenticator{
		credentials: creds,
		httpClient:  httpClient,
	}
}

// SetCredentialsUpdateCallback sets a callback to be called when credentials are updated
func (a *oauth2Authenticator) SetCredentialsUpdateCallback(callback func(*OAuth2Credentials) error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.onCredentialsUpdated = callback
}

// RefreshToken refreshes the access token using the refresh token
func (a *oauth2Authenticator) RefreshToken(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	if a.credentials == nil || a.credentials.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}
	
	// Prepare token refresh request
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", a.credentials.RefreshToken)
	data.Set("client_id", a.credentials.ClientID)
	data.Set("client_secret", a.credentials.ClientSecret)
	
	req, err := http.NewRequestWithContext(ctx, "POST", BoxTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token refresh request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zoom-to-box/1.0")
	
	// Make the request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}
	
	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if json.Unmarshal(body, &errorResp) == nil {
			return &BoxError{
				StatusCode: resp.StatusCode,
				Message:    errorResp.Message,
				Code:       errorResp.Code,
				RequestID:  errorResp.RequestID,
				Retryable:  resp.StatusCode >= 500 || resp.StatusCode == 429,
			}
		}
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	// Parse token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}
	
	// Update credentials
	a.credentials.AccessToken = tokenResp.AccessToken
	a.credentials.RefreshToken = tokenResp.RefreshToken
	a.credentials.ExpiresIn = tokenResp.ExpiresIn
	a.credentials.TokenType = tokenResp.TokenType
	a.credentials.Scope = tokenResp.Scope
	a.credentials.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	
	// Call update callback if set
	if a.onCredentialsUpdated != nil {
		if err := a.onCredentialsUpdated(a.credentials); err != nil {
			return fmt.Errorf("failed to update stored credentials: %w", err)
		}
	}
	
	return nil
}

// GetAccessToken returns the current access token
func (a *oauth2Authenticator) GetAccessToken() string {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	
	if a.credentials == nil {
		return ""
	}
	return a.credentials.AccessToken
}

// IsAuthenticated returns true if we have a valid access token
func (a *oauth2Authenticator) IsAuthenticated() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	
	if a.credentials == nil || a.credentials.AccessToken == "" {
		return false
	}
	
	// Check if token is expired
	return !a.credentials.IsExpired()
}

// GetCredentials returns a copy of the current credentials
func (a *oauth2Authenticator) GetCredentials() *OAuth2Credentials {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	
	if a.credentials == nil {
		return nil
	}
	
	// Return a copy to prevent external modification
	creds := *a.credentials
	return &creds
}

// UpdateCredentials updates the stored credentials
func (a *oauth2Authenticator) UpdateCredentials(creds *OAuth2Credentials) error {
	if creds == nil {
		return fmt.Errorf("credentials cannot be nil")
	}
	
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	// Set expires_at if not set
	if creds.ExpiresAt.IsZero() && creds.ExpiresIn > 0 {
		creds.ExpiresAt = time.Now().Add(time.Duration(creds.ExpiresIn) * time.Second)
	}
	
	a.credentials = creds
	return nil
}

// authenticatedHTTPClient provides HTTP client with automatic OAuth token handling
type authenticatedHTTPClient struct {
	authenticator Authenticator
	httpClient    *http.Client
	mutex         sync.RWMutex
}

// NewAuthenticatedHTTPClient creates a new HTTP client with OAuth authentication
func NewAuthenticatedHTTPClient(auth Authenticator, httpClient *http.Client) AuthenticatedHTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	
	return &authenticatedHTTPClient{
		authenticator: auth,
		httpClient:    httpClient,
	}
}

// Do performs an HTTP request with automatic token refresh
func (c *authenticatedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Ensure we have a valid token
	if err := c.ensureValidToken(req.Context()); err != nil {
		return nil, fmt.Errorf("failed to ensure valid token: %w", err)
	}
	
	// Add authorization header
	accessToken := c.authenticator.GetAccessToken()
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	
	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	
	// Check if we got an unauthorized response, try to refresh token once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		
		// Try to refresh token
		if err := c.authenticator.RefreshToken(req.Context()); err != nil {
			return nil, fmt.Errorf("failed to refresh token after 401: %w", err)
		}
		
		// Retry the request with new token
		newAccessToken := c.authenticator.GetAccessToken()
		if newAccessToken != "" {
			req.Header.Set("Authorization", "Bearer "+newAccessToken)
		}
		
		return c.httpClient.Do(req)
	}
	
	return resp, nil
}

// Get performs a GET request with authentication
func (c *authenticatedHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}
	
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zoom-to-box/1.0")
	
	return c.Do(req)
}

// Post performs a POST request with authentication
func (c *authenticatedHTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}
	
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zoom-to-box/1.0")
	
	return c.Do(req)
}

// PostJSON performs a POST request with JSON body
func (c *authenticatedHTTPClient) PostJSON(ctx context.Context, url string, payload interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}
	
	return c.Post(ctx, url, "application/json", bytes.NewReader(jsonData))
}

// ensureValidToken ensures we have a valid access token, refreshing if necessary
func (c *authenticatedHTTPClient) ensureValidToken(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	// Check if we need to refresh the token
	if !c.authenticator.IsAuthenticated() {
		if err := c.authenticator.RefreshToken(ctx); err != nil {
			return fmt.Errorf("failed to refresh token: %w", err)
		}
	}
	
	return nil
}

// TokenRefreshError represents an error during token refresh
type TokenRefreshError struct {
	Err       error
	Retryable bool
}

func (e *TokenRefreshError) Error() string {
	return fmt.Sprintf("token refresh failed: %v", e.Err)
}

func (e *TokenRefreshError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error is retryable
func (e *TokenRefreshError) IsRetryable() bool {
	return e.Retryable
}

// serviceToServiceAuthenticator implements JWT-based service-to-service authentication for Box
type serviceToServiceAuthenticator struct {
	credentials *ServiceToServiceCredentials
	httpClient  *http.Client
	mutex       sync.RWMutex
}

// NewServiceToServiceAuthenticator creates a new service-to-service authenticator for Box
func NewServiceToServiceAuthenticator(creds *ServiceToServiceCredentials, httpClient *http.Client) Authenticator {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	
	return &serviceToServiceAuthenticator{
		credentials: creds,
		httpClient:  httpClient,
	}
}

// RefreshToken generates a new access token using JWT assertion
func (a *serviceToServiceAuthenticator) RefreshToken(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	if a.credentials == nil {
		return fmt.Errorf("no credentials available")
	}
	
	// Generate JWT assertion
	assertion, err := a.generateJWTAssertion()
	if err != nil {
		return fmt.Errorf("failed to generate JWT assertion: %w", err)
	}
	
	// Prepare token request
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	data.Set("assertion", assertion)
	data.Set("client_id", a.credentials.ClientID)
	data.Set("client_secret", a.credentials.ClientSecret)
	
	req, err := http.NewRequestWithContext(ctx, "POST", BoxTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zoom-to-box/1.0")
	
	// Make the request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}
	
	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if json.Unmarshal(body, &errorResp) == nil {
			return &BoxError{
				StatusCode: resp.StatusCode,
				Message:    errorResp.Message,
				Code:       errorResp.Code,
				RequestID:  errorResp.RequestID,
				Retryable:  resp.StatusCode >= 500 || resp.StatusCode == 429,
			}
		}
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	// Parse token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}
	
	// Update credentials
	a.credentials.AccessToken = tokenResp.AccessToken
	a.credentials.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	
	return nil
}

// GetAccessToken returns the current access token
func (a *serviceToServiceAuthenticator) GetAccessToken() string {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	
	if a.credentials == nil {
		return ""
	}
	return a.credentials.AccessToken
}

// IsAuthenticated returns true if we have a valid access token
func (a *serviceToServiceAuthenticator) IsAuthenticated() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	
	if a.credentials == nil || a.credentials.AccessToken == "" {
		return false
	}
	
	return !a.credentials.IsExpired()
}

// GetCredentials returns nil for service-to-service (no OAuth2Credentials)
func (a *serviceToServiceAuthenticator) GetCredentials() *OAuth2Credentials {
	return nil
}

// UpdateCredentials is not supported for service-to-service authentication
func (a *serviceToServiceAuthenticator) UpdateCredentials(creds *OAuth2Credentials) error {
	return fmt.Errorf("UpdateCredentials is not supported for service-to-service authentication")
}

// generateJWTAssertion creates a JWT assertion for service-to-service authentication
func (a *serviceToServiceAuthenticator) generateJWTAssertion() (string, error) {
	// Parse private key
	block, _ := pem.Decode([]byte(a.credentials.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to parse private key PEM")
	}
	
	var privateKey *rsa.PrivateKey
	var err error
	
	switch block.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse PKCS8 private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("private key is not RSA")
		}
	default:
		return "", fmt.Errorf("unsupported private key type: %s", block.Type)
	}
	
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}
	
	// Create JWT header
	header := map[string]interface{}{
		"alg": "RS256",
		"typ": "JWT",
		"kid": a.credentials.KeyID,
	}
	
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT header: %w", err)
	}
	
	// Create JWT payload
	now := time.Now().Unix()
	payload := map[string]interface{}{
		"iss": a.credentials.ClientID,
		"sub": a.credentials.EnterpriseID,
		"box_sub_type": "enterprise",
		"aud": BoxTokenURL,
		"jti": generateJTI(),
		"exp": now + 60, // Token expires in 60 seconds
		"iat": now,
	}
	
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT payload: %w", err)
	}
	
	// Encode header and payload
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)
	
	// Create signature
	signingInput := headerEncoded + "." + payloadEncoded
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, 0, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}
	
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)
	
	return signingInput + "." + signatureEncoded, nil
}

// generateJTI generates a unique JWT ID
func generateJTI() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// Helper functions for error handling

// IsAuthError returns true if the error is an authentication error
func IsAuthError(err error) bool {
	if boxErr, ok := err.(*BoxError); ok {
		return boxErr.Code == ErrorCodeUnauthorized || boxErr.Code == ErrorCodeInvalidGrant || boxErr.Code == ErrorCodeInsufficientScope
	}
	return false
}

// IsRetryableError returns true if the error is retryable
func IsRetryableError(err error) bool {
	if boxErr, ok := err.(*BoxError); ok {
		return boxErr.IsRetryable()
	}
	if refreshErr, ok := err.(*TokenRefreshError); ok {
		return refreshErr.IsRetryable()
	}
	return false
}

// IsRateLimitError returns true if the error is a rate limit error
func IsRateLimitError(err error) bool {
	if boxErr, ok := err.(*BoxError); ok {
		return boxErr.Code == ErrorCodeRateLimitExceeded
	}
	return false
}