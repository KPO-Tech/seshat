package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Token expiry / refresh logic (no HTTP)
// ---------------------------------------------------------------------------

func TestToken_IsExpired_ExpiredToken(t *testing.T) {
	tok := &Token{
		AccessToken: "tkn",
		ExpiresAt:   time.Now().Add(-time.Minute),
	}
	if !tok.IsExpired() {
		t.Error("expected expired token to report IsExpired() == true")
	}
}

func TestToken_IsExpired_FreshToken(t *testing.T) {
	tok := &Token{
		AccessToken: "tkn",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if tok.IsExpired() {
		t.Error("expected fresh token to report IsExpired() == false")
	}
}

func TestToken_IsExpired_ZeroTime(t *testing.T) {
	// Zero ExpiresAt means the token never expires (non-expiring).
	// IsExpired returns true because time.Until(zero) <= 0 — this is by design;
	// the token does not carry expiry information.
	tok := &Token{AccessToken: "tkn"}
	if !tok.IsExpired() {
		t.Error("zero-expiry token: time.Until(zero) <= 0 so IsExpired should be true")
	}
}

func TestToken_NeedsRefresh_SoonToExpire(t *testing.T) {
	tok := &Token{
		AccessToken: "tkn",
		ExpiresAt:   time.Now().Add(2 * time.Minute), // < 5 min threshold
	}
	if !tok.NeedsRefresh() {
		t.Error("expected token expiring in 2 min to need refresh")
	}
}

func TestToken_NeedsRefresh_FreshToken(t *testing.T) {
	tok := &Token{
		AccessToken: "tkn",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if tok.NeedsRefresh() {
		t.Error("expected token with 1 hour left not to need refresh yet")
	}
}

// ---------------------------------------------------------------------------
// TokenResponse.ToToken conversion
// ---------------------------------------------------------------------------

func TestTokenResponse_ToToken(t *testing.T) {
	resp := &TokenResponse{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		IDToken:      "id-tok",
		ExpiresIn:    3600,
		Scope:        "openid profile",
	}

	tok := resp.ToToken()
	if tok.AccessToken != "access-abc" {
		t.Errorf("AccessToken: got %q, want %q", tok.AccessToken, "access-abc")
	}
	if tok.RefreshToken != "refresh-xyz" {
		t.Errorf("RefreshToken: got %q, want %q", tok.RefreshToken, "refresh-xyz")
	}
	if tok.IDToken != "id-tok" {
		t.Errorf("IDToken: got %q, want %q", tok.IDToken, "id-tok")
	}
	if tok.Scope != "openid profile" {
		t.Errorf("Scope: got %q, want %q", tok.Scope, "openid profile")
	}
	// ExpiresAt must be roughly now + 3600 s (within 5 s tolerance).
	want := time.Now().Add(3600 * time.Second)
	diff := tok.ExpiresAt.Sub(want)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt off by %v", diff)
	}
	if tok.TokenType != TokenTypeAccess {
		t.Errorf("TokenType: got %q, want %q", tok.TokenType, TokenTypeAccess)
	}
}

func TestTokenResponse_ToToken_ZeroExpiry(t *testing.T) {
	resp := &TokenResponse{AccessToken: "x", ExpiresIn: 0}
	tok := resp.ToToken()
	if !tok.ExpiresAt.IsZero() {
		t.Errorf("expected zero ExpiresAt when ExpiresIn == 0, got %v", tok.ExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// DeviceCode — mock HTTP
// ---------------------------------------------------------------------------

func TestClient_DeviceCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("DeviceCode: expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"device_auth_id": "dev-code-123",
			"user_code":      "USER-CODE",
			"interval":       "5",
		})
	}))
	defer srv.Close()

	client := NewClient(&Config{
		BaseURL:       srv.URL,
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL + "/deviceauth",
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
	})

	resp, err := client.DeviceCode(context.Background())
	if err != nil {
		t.Fatalf("DeviceCode() error: %v", err)
	}
	if resp.DeviceCode != "dev-code-123" {
		t.Errorf("DeviceCode: got %q, want %q", resp.DeviceCode, "dev-code-123")
	}
	if resp.UserCode != "USER-CODE" {
		t.Errorf("UserCode: got %q, want %q", resp.UserCode, "USER-CODE")
	}
	if resp.Interval != 5 {
		t.Errorf("Interval: got %d, want 5", resp.Interval)
	}
}

func TestClient_DeviceCode_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(&Config{
		BaseURL:       srv.URL,
		ClientID:      "test-client",
		DeviceAuthURL: srv.URL + "/deviceauth",
		HTTPClient:    &http.Client{Timeout: 5 * time.Second},
	})

	_, err := client.DeviceCode(context.Background())
	if err == nil {
		t.Error("expected DeviceCode to fail on 500 response")
	}
}

// ---------------------------------------------------------------------------
// ExchangeDeviceToken — mock HTTP
// ---------------------------------------------------------------------------

func TestClient_ExchangeDeviceToken_Pending(t *testing.T) {
	// 403 response = authorization_pending.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(&Config{
		ClientID:       "cid",
		DeviceTokenURL: srv.URL + "/token",
		HTTPClient:     &http.Client{Timeout: 5 * time.Second},
	})

	_, err := client.ExchangeDeviceToken(context.Background(), "dev-code", "user-code")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if err.Error() != "authorization_pending" {
		t.Errorf("expected 'authorization_pending', got %q", err.Error())
	}
}

func TestClient_ExchangeDeviceToken_NotAuthorizedYet404(t *testing.T) {
	// 404 response is also treated as authorization_pending by OpenAI's API.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(&Config{
		ClientID:       "cid",
		DeviceTokenURL: srv.URL + "/token",
		HTTPClient:     &http.Client{Timeout: 5 * time.Second},
	})

	_, err := client.ExchangeDeviceToken(context.Background(), "dev-code", "user-code")
	if err == nil || err.Error() != "authorization_pending" {
		t.Errorf("expected 'authorization_pending' for 404, got %v", err)
	}
}

func TestClient_ExchangeDeviceToken_FullFlow(t *testing.T) {
	// Device token endpoint returns PKCE material; token endpoint returns tokens.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "final-access",
			RefreshToken: "final-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer tokenSrv.Close()

	deviceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_code": "auth-code-xyz",
			"code_challenge":     "challenge",
			"code_verifier":      "verifier",
		})
	}))
	defer deviceSrv.Close()

	client := NewClient(&Config{
		ClientID:       "cid",
		RedirectURI:    "https://example.com/callback",
		DeviceTokenURL: deviceSrv.URL + "/devicetoken",
		TokenURL:       tokenSrv.URL + "/token",
		HTTPClient:     &http.Client{Timeout: 5 * time.Second},
	})

	resp, err := client.ExchangeDeviceToken(context.Background(), "dev-code", "user-code")
	if err != nil {
		t.Fatalf("ExchangeDeviceToken() error: %v", err)
	}
	if resp.AccessToken != "final-access" {
		t.Errorf("AccessToken: got %q, want %q", resp.AccessToken, "final-access")
	}
	if resp.RefreshToken != "final-refresh" {
		t.Errorf("RefreshToken: got %q, want %q", resp.RefreshToken, "final-refresh")
	}
}

// ---------------------------------------------------------------------------
// RefreshToken — mock HTTP
// ---------------------------------------------------------------------------

func TestClient_RefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("RefreshToken: expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type: got %q, want refresh_token", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    7200,
		})
	}))
	defer srv.Close()

	client := NewClient(&Config{
		ClientID:   "cid",
		TokenURL:   srv.URL + "/token",
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})

	resp, err := client.RefreshToken(context.Background(), "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error: %v", err)
	}
	if resp.AccessToken != "new-access" {
		t.Errorf("AccessToken: got %q, want %q", resp.AccessToken, "new-access")
	}
}

func TestClient_RefreshToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid_grant"))
	}))
	defer srv.Close()

	client := NewClient(&Config{
		ClientID:   "cid",
		TokenURL:   srv.URL + "/token",
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})

	_, err := client.RefreshToken(context.Background(), "bad-token")
	if err == nil {
		t.Error("expected RefreshToken to fail on 401 response")
	}
}
