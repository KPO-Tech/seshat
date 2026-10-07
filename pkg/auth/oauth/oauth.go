package oauth

import internaloauth "github.com/KPO-Tech/seshat/internal/auth/oauth"

type (
	// Client represents an OAuth client
	Client = internaloauth.Client
	// Config represents OAuth client configuration
	Config = internaloauth.Config
	// DeviceCodeResponse represents the normalised device code response.
	DeviceCodeResponse = internaloauth.DeviceCodeResponse
	// Token represents an OAuth token
	Token = internaloauth.Token
	// TokenResponse represents a token response
	TokenResponse = internaloauth.TokenResponse
	// TokenType represents the type of token
	TokenType = internaloauth.TokenType
)

const (
	TokenTypeAccess  = internaloauth.TokenTypeAccess
	TokenTypeRefresh = internaloauth.TokenTypeRefresh
)

// DefaultOpenAIConfig returns OAuth config for ChatGPT authentication.
// Uses the same device auth flow as Codex CLI (github.com/openai/codex).
// Issuer: https://auth.openai.com
func DefaultOpenAIConfig() *Config {
	return internaloauth.DefaultOpenAIConfig()
}

// NewClient creates a new OAuth client
func NewClient(config *Config) *Client {
	return internaloauth.NewClient(config)
}
