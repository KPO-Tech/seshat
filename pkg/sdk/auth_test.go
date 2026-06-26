package sdk

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	publicoauth "github.com/EngineerProjects/seshat/pkg/auth/oauth"
)

func TestOAuthDeviceFlowSaveAndResolve(t *testing.T) {
	t.Parallel()

	authPath := filepath.Join(t.TempDir(), "auth.json")
	flow, err := NewOAuthDeviceFlow(OAuthDeviceFlowConfig{
		Provider: string(APIProviderCodex),
		AuthPath: authPath,
		Persist:  true,
	})
	if err != nil {
		t.Fatalf("NewOAuthDeviceFlow: %v", err)
	}

	token := &publicoauth.Token{
		AccessToken:  "codex-access-token",
		RefreshToken: "codex-refresh-token",
		TokenType:    publicoauth.TokenTypeAccess,
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}
	if err := flow.SaveToken(context.Background(), token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	key, err := flow.CredentialResolver().ResolveAPIKey(context.Background(), string(APIProviderCodex))
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if key != token.AccessToken {
		t.Fatalf("expected %q, got %q", token.AccessToken, key)
	}
}

func TestStoredCredentialResolverPrefersEnv(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "env-codex-token")
	authPath := filepath.Join(t.TempDir(), "auth.json")
	flow, err := NewOAuthDeviceFlow(OAuthDeviceFlowConfig{
		Provider: string(APIProviderCodex),
		AuthPath: authPath,
		Persist:  true,
	})
	if err != nil {
		t.Fatalf("NewOAuthDeviceFlow: %v", err)
	}

	token := &publicoauth.Token{
		AccessToken: "stored-codex-token",
		TokenType:   publicoauth.TokenTypeAccess,
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	}
	if err := flow.SaveToken(context.Background(), token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	resolver := NewStoredCredentialResolver(StoredCredentialResolverConfig{
		AuthPath: authPath,
		EnvFirst: true,
	})
	key, err := resolver.ResolveAPIKey(context.Background(), string(APIProviderCodex))
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if key != "env-codex-token" {
		t.Fatalf("expected env token, got %q", key)
	}
}

func TestStoredCredentialResolverSupportsCodexAccessTokenAlias(t *testing.T) {
	t.Setenv("CODEX_ACCESS_TOKEN", "alias-codex-token")

	resolver := NewStoredCredentialResolver(StoredCredentialResolverConfig{
		AuthPath: filepath.Join(t.TempDir(), "auth.json"),
		EnvFirst: true,
	})
	key, err := resolver.ResolveAPIKey(context.Background(), string(APIProviderCodex))
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if key != "alias-codex-token" {
		t.Fatalf("expected alias token, got %q", key)
	}
}

func TestNewOAuthDeviceFlowUsesDefaultClientID(t *testing.T) {
	flow, err := NewOAuthDeviceFlow(OAuthDeviceFlowConfig{
		Provider: string(APIProviderCodex),
	})
	if err != nil {
		t.Fatalf("NewOAuthDeviceFlow: %v", err)
	}
	if flow.oauthCfg == nil || flow.oauthCfg.ClientID != DefaultOpenAIDeviceClientID {
		t.Fatalf("expected default client id %q, got %#v", DefaultOpenAIDeviceClientID, flow.oauthCfg)
	}
}
