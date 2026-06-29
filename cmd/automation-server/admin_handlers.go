package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/EngineerProjects/seshat/internal/db"
	"github.com/google/uuid"
)

// ─── Request / response types ──────────────────────────────────────────────────

type createAPIKeyRequest struct {
	Label   string `json:"label"`
	OwnerID string `json:"owner_id"`
	// ExpiresInDays sets a TTL for the key. 0 (default) means no expiry.
	ExpiresInDays int `json:"expires_in_days"`
}

type apiKeyResponse struct {
	ID        string `json:"id"`
	KeyPrefix string `json:"key_prefix"`
	Label     string `json:"label"`
	OwnerID   string `json:"owner_id"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	// ExpiresAt is a Unix timestamp; 0 means the key never expires.
	ExpiresAt int64 `json:"expires_at,omitempty"`
	// RawKey is set only on creation and never returned again.
	RawKey string `json:"raw_key,omitempty"`
}

// ─── Handlers ──────────────────────────────────────────────────────────────────

// handleCreateAPIKey generates a new sats_ key for an owner.
// POST /v1/admin/keys
func (s *server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.OwnerID == "" {
		jsonError(w, "owner_id is required", http.StatusBadRequest)
		return
	}

	rawKey, hash, prefix, err := db.GenerateAutomationAPIKey()
	if err != nil {
		jsonError(w, "key generation failed", http.StatusInternalServerError)
		return
	}

	var expiresAt int64
	if req.ExpiresInDays > 0 {
		expiresAt = time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour).Unix()
	}

	row := db.AutomationAPIKeyRow{
		ID:        uuid.New().String(),
		KeyHash:   hash,
		KeyPrefix: prefix,
		Label:     req.Label,
		OwnerID:   req.OwnerID,
		Enabled:   true,
		CreatedAt: db.UnixNow(),
		ExpiresAt: expiresAt,
	}
	if err := s.db.CreateAutomationAPIKey(r.Context(), row); err != nil {
		s.internalError(w, err, "create api key: persist")
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, apiKeyResponse{
		ID:        row.ID,
		KeyPrefix: prefix,
		Label:     row.Label,
		OwnerID:   row.OwnerID,
		Enabled:   true,
		CreatedAt: row.CreatedAt,
		ExpiresAt: row.ExpiresAt,
		RawKey:    rawKey, // shown once, never stored
	})
}

// handleListAPIKeys lists all API keys (without raw values).
// GET /v1/admin/keys
func (s *server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keys, err := s.db.ListAutomationAPIKeys(r.Context())
	if err != nil {
		s.internalError(w, err, "list api keys")
		return
	}
	resp := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, apiKeyResponse{
			ID:        k.ID,
			KeyPrefix: k.KeyPrefix,
			Label:     k.Label,
			OwnerID:   k.OwnerID,
			Enabled:   k.Enabled,
			CreatedAt: k.CreatedAt,
			ExpiresAt: k.ExpiresAt,
		})
	}
	jsonOK(w, resp)
}

// handleRevokeAPIKey soft-deletes a key by ID.
// DELETE /v1/admin/keys/{id}
func (s *server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, "key id required", http.StatusBadRequest)
		return
	}
	if err := s.db.RevokeAutomationAPIKey(r.Context(), id); err != nil {
		jsonError(w, "key not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
