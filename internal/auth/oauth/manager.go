package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/auth/store"
	authTypes "github.com/EngineerProjects/nexus-engine/internal/auth/types"
)

// ============================================================================
// OAuth Manager with Auto-Refresh
// ============================================================================

// OAuthManager manages OAuth tokens with auto-refresh
type OAuthManager struct {
	store    store.Store
	provider string
	client   *Client

	mu           sync.RWMutex
	token        *Token
	refreshToken string
	refreshAt    time.Time
	autoRefresh  bool
	stopCh       chan struct{}
}

// NewOAuthManager creates a new OAuth manager
func NewOAuthManager(s store.Store, provider string, cfg *Config) *OAuthManager {
	return &OAuthManager{
		store:       s,
		provider:    provider,
		client:      NewClient(cfg),
		autoRefresh: true,
		stopCh:      make(chan struct{}),
	}
}

// Login starts OAuth authentication and returns the verification URL
func (m *OAuthManager) Login(ctx context.Context) (userCode, url string, err error) {
	handler := NewHandler(m.client)
	userCode, url, err = handler.Start()
	if err != nil {
		return "", "", err
	}
	return userCode, url, nil
}

// Complete stores the token after OAuth completion
func (m *OAuthManager) Complete(ctx context.Context, token *Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.token = token
	if token.RefreshToken != "" {
		m.refreshToken = token.RefreshToken
	}

	// Set refresh time (5 minutes before expiry)
	if !token.ExpiresAt.IsZero() {
		m.refreshAt = token.ExpiresAt.Add(-5 * time.Minute)
	}

	// Start auto-refresh in background
	if m.autoRefresh && token.RefreshToken != "" {
		go m.refreshLoop(ctx)
	}

	return nil
}

// GetToken returns a valid token, refreshing if necessary
func (m *OAuthManager) GetToken(ctx context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if we need to refresh
	if m.refreshToken != "" && !m.refreshAt.IsZero() && time.Now().After(m.refreshAt) {
		// Try to refresh
		m.mu.RUnlock()
		newToken, err := m.refresh(ctx)
		m.mu.RLock()
		if err != nil {
			return "", err
		}
		return newToken.AccessToken, nil
	}

	// Return current token if valid
	if m.token != nil && !m.token.IsExpired() {
		return m.token.AccessToken, nil
	}

	return "", fmt.Errorf("no valid token")
}

// refresh attempts to refresh the token
func (m *OAuthManager) refresh(ctx context.Context) (*Token, error) {
	if m.refreshToken == "" {
		return nil, fmt.Errorf("no refresh token")
	}

	tokenResp, err := m.client.RefreshToken(ctx, m.refreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}

	// Update stored token
	m.token = tokenResp.ToToken()

	// Update refresh time
	if !m.token.ExpiresAt.IsZero() {
		m.refreshAt = m.token.ExpiresAt.Add(-5 * time.Minute)
	}

	return m.token, nil
}

// refreshLoop runs in background to auto-refresh tokens
func (m *OAuthManager) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(4 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			if m.refreshToken != "" {
				tokenResp, err := m.client.RefreshToken(ctx, m.refreshToken)
				if err != nil {
					m.mu.Unlock()
					continue
				}
				m.token = tokenResp.ToToken()
				if !m.token.ExpiresAt.IsZero() {
					m.refreshAt = m.token.ExpiresAt.Add(-5 * time.Minute)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Stop stops the auto-refresh loop
func (m *OAuthManager) Stop() {
	close(m.stopCh)
}

// ============================================================================
// OAuth Store Integration
// ============================================================================

// StoreCredentials stores OAuth credentials in the auth store
func (m *OAuthManager) StoreCredentials(ctx context.Context, creds *authTypes.Credentials) error {
	return m.store.SaveCredentials(ctx, creds)
}

// LoadCredentials loads OAuth credentials from the auth store
func (m *OAuthManager) LoadCredentials(ctx context.Context) (*authTypes.Credentials, error) {
	return m.store.LoadCredentials(ctx, m.provider)
}

// ============================================================================
// Default OAuth Providers
// ============================================================================

// ProviderConfig holds OAuth provider configuration
type ProviderConfig struct {
	BaseURL       string
	ClientID      string
	AuthURL       string
	TokenURL      string
	DeviceCodeURL string
	Scopes        []string
}

// DefaultOpenAIProvider returns the Auth0-based OAuth config for ChatGPT.
func DefaultOpenAIProvider() *ProviderConfig {
	return &ProviderConfig{
		BaseURL:       "https://api.openai.com",
		AuthURL:       "https://auth0.openai.com/authorize",
		TokenURL:      "https://auth0.openai.com/oauth/token",
		DeviceCodeURL: "https://auth0.openai.com/oauth/device/code",
		Scopes:        []string{"openid", "profile", "email", "offline_access"},
	}
}

// DefaultGoogleProvider returns default Google OAuth config
func DefaultGoogleProvider() *ProviderConfig {
	return &ProviderConfig{
		BaseURL:  "https://oauth2.googleapis.com",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		Scopes:   []string{"https://www.googleapis.com/auth/generative.language.tuning"},
	}
}
