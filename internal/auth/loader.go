package auth

import (
	"context"
	"os"
	"path/filepath"

	"github.com/EngineerProjects/nexus-engine/internal/auth/store"
	"github.com/EngineerProjects/nexus-engine/internal/auth/types"
)

// ============================================================================
// Config Loader
// ============================================================================

// Loader loads authentication configuration
type Loader struct {
	store  store.Store
	config *types.Config
}

// NewLoader creates a new auth loader
func NewLoader(store store.Store) *Loader {
	return &Loader{
		store: store,
		config: &types.Config{
			Providers: make(map[string]types.ProviderConfig),
		},
	}
}

// GetAPIKey returns the API key for a provider
func (l *Loader) GetAPIKey(ctx context.Context, provider string) (string, error) {
	// First check config
	if providerConfig, hasConfig := l.config.Providers[provider]; hasConfig && providerConfig.APIKey != "" {
		return providerConfig.APIKey, nil
	}

	// Check store
	creds, err := l.store.LoadCredentials(ctx, provider)
	if err != nil {
		return "", err
	}

	return creds.APIKey, nil
}

// SetAPIKey sets the API key for a provider
func (l *Loader) SetAPIKey(ctx context.Context, provider, key string) error {
	// Update config
	l.config.Providers[provider] = types.ProviderConfig{
		AuthMethod: types.AuthMethodAPIKey,
		APIKey:     key,
	}

	// Also save to store
	return l.store.SaveCredentials(ctx, &types.Credentials{
		Provider:   provider,
		AuthMethod: types.AuthMethodAPIKey,
		APIKey:     key,
	})
}

// GetAuthMethod returns the auth method for a provider
func (l *Loader) GetAuthMethod(ctx context.Context, provider string) (types.AuthMethod, error) {
	// First check config
	if providerConfig, hasConfig := l.config.Providers[provider]; hasConfig && providerConfig.AuthMethod != "" {
		return providerConfig.AuthMethod, nil
	}

	// Check store
	creds, err := l.store.LoadCredentials(ctx, provider)
	if err != nil {
		// Default to API key
		return types.AuthMethodAPIKey, nil
	}

	return creds.AuthMethod, nil
}

// SetAuthMethod sets the auth method for a provider
func (l *Loader) SetAuthMethod(ctx context.Context, provider string, method types.AuthMethod) error {
	// Update config
	if l.config.Providers == nil {
		l.config.Providers = make(map[string]types.ProviderConfig)
	}

	l.config.Providers[provider] = types.ProviderConfig{
		AuthMethod: method,
	}

	// Also update store
	return l.store.SaveCredentials(ctx, &types.Credentials{
		Provider:   provider,
		AuthMethod: method,
	})
}

// ListProviders lists all configured providers
func (l *Loader) ListProviders(ctx context.Context) ([]string, error) {
	providers, err := l.store.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	// Add providers from config
	for provider := range l.config.Providers {
		found := false
		for _, p := range providers {
			if p == provider {
				found = true
				break
			}
		}
		if !found {
			providers = append(providers, provider)
		}
	}

	return providers, nil
}

// ============================================================================
// Default Loader
// ============================================================================

var defaultLoader *Loader

// Default returns the default auth loader backed by an encrypted file store.
// Set NEXUS_MASTER_KEY (64 hex chars) to control the encryption key explicitly;
// otherwise a machine-derived key is used automatically.
func Default() (*Loader, error) {
	if defaultLoader != nil {
		return defaultLoader, nil
	}

	path := DefaultConfigPath()
	s, err := store.NewEncryptedFileStore(path)
	if err != nil {
		return nil, err
	}

	defaultLoader = NewLoader(s)
	return defaultLoader, nil
}

// DefaultConfigPath returns the default config path
func DefaultConfigPath() string {
	if path := os.Getenv("NEXUS_AUTH_PATH"); path != "" {
		return path
	}

	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".nexus", "auth.json")
}
