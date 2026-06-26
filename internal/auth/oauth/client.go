package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// OAuth Client
// ============================================================================

// Config represents OAuth client configuration
type Config struct {
	BaseURL        string   // Base URL for OAuth provider (auth server root)
	ClientID       string   // OAuth client ID
	ClientSecret   string   // OAuth client secret (optional)
	AuthURL        string   // Authorization endpoint (browser PKCE flow)
	TokenURL       string   // Token endpoint
	DeviceAuthURL  string   // Device usercode endpoint (OpenAI custom API)
	DeviceTokenURL string   // Device token poll endpoint (OpenAI custom API)
	Scopes         []string // OAuth scopes
	RedirectURI    string   // Redirect URI
	HTTPClient     *http.Client
}

// DefaultOpenAIConfig returns OAuth config for ChatGPT authentication.
// Uses the same device auth flow as Codex CLI (github.com/openai/codex).
// Issuer: https://auth.openai.com
func DefaultOpenAIConfig() *Config {
	return &Config{
		BaseURL:  "https://auth.openai.com",
		ClientID: "app_EMoamEEZ73f0CkXaXp7hrann",
		// Browser PKCE flow
		AuthURL:  "https://auth.openai.com/oauth/authorize",
		TokenURL: "https://auth.openai.com/oauth/token",
		// OpenAI custom device auth (NOT standard RFC 8628 Auth0 device flow)
		DeviceAuthURL:  "https://auth.openai.com/api/accounts/deviceauth/usercode",
		DeviceTokenURL: "https://auth.openai.com/api/accounts/deviceauth/token",
		// Codex scopes — api.connectors.* needed for Codex model access
		Scopes:      []string{"openid", "profile", "email", "offline_access", "api.connectors.read", "api.connectors.invoke"},
		RedirectURI: "https://auth.openai.com/deviceauth/callback",
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Client represents an OAuth client
type Client struct {
	config *Config
}

// NewClient creates a new OAuth client
func NewClient(config *Config) *Client {
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{config: config}
}

// ============================================================================
// Device Code Flow (OpenAI custom API)
// ============================================================================

// DeviceCodeResponse represents the normalised device code response.
type DeviceCodeResponse struct {
	DeviceCode              string // maps to device_auth_id in OpenAI's API
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int
}

// DeviceCode initiates OpenAI's device auth flow.
// Calls the OpenAI-specific /api/accounts/deviceauth/usercode endpoint with
// a JSON body — NOT the standard RFC 8628 Auth0 device_code endpoint.
func (c *Client) DeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	body, err := json.Marshal(map[string]string{"client_id": c.config.ClientID})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.DeviceAuthURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, string(b))
	}

	// OpenAI returns {"device_auth_id":"...","user_code":"...","interval":"5"}
	// Note: interval is a JSON string, not a number.
	var raw struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Interval     string `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	interval := 5
	if n, err := strconv.Atoi(strings.TrimSpace(raw.Interval)); err == nil && n > 0 {
		interval = n
	}

	baseURL := strings.TrimSuffix(c.config.BaseURL, "/")
	verificationURL := baseURL + "/codex/device"

	return &DeviceCodeResponse{
		DeviceCode:              raw.DeviceAuthID,
		UserCode:                raw.UserCode,
		VerificationURI:         verificationURL,
		VerificationURIComplete: verificationURL,
		ExpiresIn:               15 * 60, // OpenAI device codes expire in 15 minutes
		Interval:                interval,
	}, nil
}

// ============================================================================
// Token types
// ============================================================================

// TokenResponse represents a token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ToToken converts to auth.Token format
func (r *TokenResponse) ToToken() *Token {
	expiresAt := time.Time{}
	if r.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	}

	return &Token{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		IDToken:      r.IDToken,
		TokenType:    TokenTypeAccess,
		ExpiresAt:    expiresAt,
		Scope:        r.Scope,
	}
}

// Token represents an OAuth token
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	TokenType    TokenType `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// IsExpired returns true if the token is expired
func (t *Token) IsExpired() bool {
	return time.Until(t.ExpiresAt) <= 0
}

// NeedsRefresh returns true if the token should be refreshed
func (t *Token) NeedsRefresh() bool {
	return time.Until(t.ExpiresAt) <= 5*time.Minute
}

// TokenType represents the type of token
type TokenType string

const (
	TokenTypeAccess  TokenType = "access_token"
	TokenTypeRefresh TokenType = "refresh_token"
)

// ============================================================================
// Token Exchange (Device Flow — OpenAI custom)
// ============================================================================

// ExchangeDeviceToken polls OpenAI's device token endpoint.
// On pending: returns an "authorization_pending" error (caller should retry).
// On success: performs the PKCE token exchange and returns full tokens.
// Both deviceCode (device_auth_id) and userCode are required for the poll body.
func (c *Client) ExchangeDeviceToken(ctx context.Context, deviceCode string, userCode string) (*TokenResponse, error) {
	body, err := json.Marshal(map[string]string{
		"device_auth_id": deviceCode,
		"user_code":      userCode,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.DeviceTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// OpenAI signals "not yet authorized" via 403 or 404 status
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("authorization_pending")
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device token poll failed (%d): %s", resp.StatusCode, string(b))
	}

	// Read the full body before parsing. Do NOT log it: the response carries the
	// authorization_code and the PKCE code_verifier, which are secrets.
	rawBody, _ := io.ReadAll(resp.Body)

	// Success: OpenAI returns PKCE material, NOT final tokens directly
	var codeResp struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeChallenge     string `json:"code_challenge"`
		CodeVerifier      string `json:"code_verifier"`
	}
	if err := json.Unmarshal(rawBody, &codeResp); err != nil {
		return nil, fmt.Errorf("decode device token response: %w", err)
	}
	if codeResp.AuthorizationCode == "" {
		return nil, fmt.Errorf("no authorization_code in device token response")
	}

	// Exchange the authorization code for final tokens using PKCE
	return c.exchangeAuthCode(ctx, codeResp.AuthorizationCode, codeResp.CodeVerifier)
}

// exchangeAuthCode performs the PKCE/auth-code token exchange at TokenURL.
// codeVerifier may be empty for non-PKCE flows.
// Retries automatically on transient token_exchange_user_error responses, which
// can occur when the OpenAI backend hasn't fully propagated the authorization yet.
func (c *Client) exchangeAuthCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", c.config.RedirectURI)
	data.Set("client_id", c.config.ClientID)
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}
	if c.config.ClientSecret != "" {
		data.Set("client_secret", c.config.ClientSecret)
	}
	// Do NOT log the encoded body: it carries the authorization code, the PKCE
	// code_verifier, and the client_secret.
	body := data.Encode()

	const maxAttempts = 4
	delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			log.Printf("[oauth] token exchange retry attempt %d/%d", attempt+1, maxAttempts)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delays[attempt-1]):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.TokenURL, strings.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.config.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do request: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var tokenResp TokenResponse
			if decErr := json.NewDecoder(resp.Body).Decode(&tokenResp); decErr != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("decode token response: %w", decErr)
			}
			resp.Body.Close()
			if tokenResp.AccessToken == "" {
				return nil, fmt.Errorf("no access_token in token exchange response")
			}
			log.Printf("[oauth] token exchange succeeded")
			return &tokenResp, nil
		}

		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		errBody := string(b)
		// Log status only — the response body may echo back sensitive request data.
		log.Printf("[oauth] token exchange failed (attempt %d): status=%d", attempt+1, resp.StatusCode)

		// Retry on token_exchange_user_error: OpenAI sometimes returns this transiently
		// when the authorization hasn't fully propagated yet.
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(errBody, "token_exchange_user_error") && attempt < maxAttempts-1 {
			continue
		}

		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, errBody)
	}

	return nil, fmt.Errorf("token exchange failed after %d retries", maxAttempts)
}

// PollToken polls for a token until the user completes auth (blocks; for CLI use).
func (c *Client) PollToken(ctx context.Context, deviceCode, userCode string, interval int) (*TokenResponse, error) {
	if interval < 5 {
		interval = 5
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			token, err := c.ExchangeDeviceToken(ctx, deviceCode, userCode)
			if err != nil {
				if strings.Contains(err.Error(), "authorization_pending") {
					continue
				}
				if strings.Contains(err.Error(), "slow_down") {
					interval *= 2
					ticker.Reset(time.Duration(interval) * time.Second)
					continue
				}
				return nil, err
			}
			return token, nil
		}
	}
}

// ============================================================================
// Token Refresh
// ============================================================================

// RefreshToken refreshes an OAuth token
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", c.config.ClientID)

	if c.config.ClientSecret != "" {
		data.Set("client_secret", c.config.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(b))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in refresh response")
	}
	return &tokenResp, nil
}

// ============================================================================
// Browser PKCE Flow (Authorization Code)
// ============================================================================

// AuthorizeURL generates the authorization URL for the browser PKCE flow.
func (c *Client) AuthorizeURL(state string) string {
	scope := strings.Join(c.config.Scopes, " ")

	v := url.Values{}
	v.Set("client_id", c.config.ClientID)
	v.Set("redirect_uri", c.config.RedirectURI)
	v.Set("response_type", "code")
	v.Set("scope", scope)
	v.Set("state", state)

	return c.config.AuthURL + "?" + v.Encode()
}

// GenerateState generates a random state parameter
func GenerateState() string {
	return uuid.New().String()
}

// ExchangeCode exchanges an authorization code for tokens (browser flow).
// codeVerifier should be provided for PKCE flows; pass empty string otherwise.
func (c *Client) ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	return c.exchangeAuthCode(ctx, code, codeVerifier)
}
