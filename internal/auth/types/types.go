package types

import (
	"time"
)

// ============================================================================
// Types
// ============================================================================

// APIProvider represents an LLM provider
type APIProvider string

const (
	APIProviderOpenAI    APIProvider = "openai"
	APIProviderAnthropic APIProvider = "anthropic"
	APIProviderGoogle    APIProvider = "google"
	APIProviderGemini    APIProvider = "gemini"
	APIProviderOllama    APIProvider = "ollama"
	APIProviderAzure     APIProvider = "azure"
	APIProviderZAI       APIProvider = "z"
	APIProviderMiniMax   APIProvider = "minimax"
)

// AuthMethod represents the authentication method used
type AuthMethod string

const (
	AuthMethodAPIKey AuthMethod = "api_key"
	AuthMethodOAuth  AuthMethod = "oauth"
)

// TokenType represents the type of OAuth token
type TokenType string

const (
	TokenTypeAccess  TokenType = "access_token"
	TokenTypeRefresh TokenType = "refresh_token"
	TokenTypeID      TokenType = "id_token"
)

// Token represents an OAuth token
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	TokenType    TokenType `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	WorkspaceID  string    `json:"workspace_id,omitempty"`
}

// IsExpired returns true if the token is expired or will expire within the given duration
func (t *Token) IsExpired(within time.Duration) bool {
	return time.Until(t.ExpiresAt) <= within
}

// NeedsRefresh returns true if the token should be refreshed
func (t *Token) NeedsRefresh() bool {
	// Refresh if expiring within 5 minutes
	return t.IsExpired(5 * time.Minute)
}

// Credentials represents authentication credentials
type Credentials struct {
	Provider   string            `json:"provider"`
	AuthMethod AuthMethod        `json:"auth_method"`
	APIKey     string            `json:"api_key,omitempty"`
	TokenRef   string            `json:"token_ref,omitempty"`
	OAuth      *OAuthCredentials `json:"oauth,omitempty"`
	// Token holds the live OAuth token data (access, refresh, expiry).
	// Populated when AuthMethod == AuthMethodOAuth.
	Token     *Token    `json:"token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OAuthCredentials holds OAuth-specific configuration
type OAuthCredentials struct {
	BaseURL        string   `json:"base_url"`
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret,omitempty"`
	AuthURL        string   `json:"auth_url"`
	TokenURL       string   `json:"token_url"`
	Scopes         []string `json:"scopes"`
	DeviceAuthURL  string   `json:"device_auth_url,omitempty"`
	DeviceTokenURL string   `json:"device_token_url,omitempty"`
}

// Config represents auth configuration
type Config struct {
	DefaultAuth string                    `json:"default_auth"`
	Providers   map[string]ProviderConfig `json:"providers"`
}

// ProviderConfig holds configuration for a specific provider
type ProviderConfig struct {
	AuthMethod AuthMethod        `json:"auth_method"`
	APIKey     string            `json:"api_key"`
	OAuth      *OAuthCredentials `json:"oauth"`
}
