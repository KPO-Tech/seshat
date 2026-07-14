package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const automationKeyPrefix = "sats_"

// AutomationAPIKeyRow is the DB row for a daemon API key.
type AutomationAPIKeyRow struct {
	ID        string
	KeyHash   string // SHA-256 hex of the raw key
	KeyPrefix string // first 12 chars of raw key for display ("sats_a1b2c3")
	Label     string
	OwnerID   string
	Enabled   bool
	CreatedAt int64
	// ExpiresAt is a Unix timestamp; 0 means no expiry.
	ExpiresAt int64
}

// GenerateAutomationAPIKey creates a new random key with the sats_ prefix.
// Returns the raw key (shown once) and its SHA-256 hash.
func GenerateAutomationAPIKey() (rawKey, hash, prefix string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return
	}
	rawKey = automationKeyPrefix + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(rawKey))
	hash = hex.EncodeToString(sum[:])
	// Display prefix: "sats_" + first 8 hex chars of the random part
	if len(rawKey) >= 13 {
		prefix = rawKey[:13]
	} else {
		prefix = rawKey
	}
	return
}

// ─── CRUD ─────────────────────────────────────────────────────────────────────

func (db *DB) CreateAutomationAPIKey(ctx context.Context, row AutomationAPIKeyRow) error {
	_, err := db.SQL().ExecContext(ctx, `
		INSERT INTO automation_api_keys (id, key_hash, key_prefix, label, owner_id, enabled, created_at, expires_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		row.ID, row.KeyHash, row.KeyPrefix, row.Label, row.OwnerID,
		boolToInt(row.Enabled), row.CreatedAt, row.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create automation api key: %w", err)
	}
	return nil
}

// GetAutomationAPIKeyByHash looks up an active, non-expired key by its SHA-256 hash.
// Returns nil, nil when the key does not exist, is disabled, or is expired.
func (db *DB) GetAutomationAPIKeyByHash(ctx context.Context, hash string) (*AutomationAPIKeyRow, error) {
	now := time.Now().UTC().Unix()
	row := db.SQL().QueryRowContext(ctx, `
		SELECT id, key_hash, key_prefix, label, owner_id, enabled, created_at, expires_at
		FROM automation_api_keys
		WHERE key_hash = ? AND enabled = 1 AND (expires_at = 0 OR expires_at > ?)`,
		hash, now)
	r, err := scanAutomationAPIKeyRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func (db *DB) ListAutomationAPIKeys(ctx context.Context) ([]*AutomationAPIKeyRow, error) {
	rows, err := db.SQL().QueryContext(ctx, `
		SELECT id, key_hash, key_prefix, label, owner_id, enabled, created_at, expires_at
		FROM automation_api_keys
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list automation api keys: %w", err)
	}
	defer rows.Close()

	var result []*AutomationAPIKeyRow
	for rows.Next() {
		r, err := scanAutomationAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (db *DB) RevokeAutomationAPIKey(ctx context.Context, id string) error {
	res, err := db.SQL().ExecContext(ctx,
		`UPDATE automation_api_keys SET enabled = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke automation api key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) DeleteAutomationAPIKey(ctx context.Context, id string) error {
	_, err := db.SQL().ExecContext(ctx,
		`DELETE FROM automation_api_keys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete automation api key: %w", err)
	}
	return nil
}

// ─── Scanner ──────────────────────────────────────────────────────────────────

func scanAutomationAPIKeyRow(s scannable) (*AutomationAPIKeyRow, error) {
	var r AutomationAPIKeyRow
	var enabled int
	err := s.Scan(&r.ID, &r.KeyHash, &r.KeyPrefix, &r.Label, &r.OwnerID, &enabled, &r.CreatedAt, &r.ExpiresAt)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	return &r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// HashAutomationAPIKey returns the SHA-256 hex hash of a raw key.
func HashAutomationAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// MaskAutomationAPIKey returns a display-safe version: "sats_****<last4>".
func MaskAutomationAPIKey(rawKey string) string {
	if len(rawKey) <= len(automationKeyPrefix)+4 {
		return automationKeyPrefix + "****"
	}
	return rawKey[:len(automationKeyPrefix)] + "****" + rawKey[len(rawKey)-4:]
}

// IsAutomationAPIKey reports whether s looks like a sats_ prefixed key.
func IsAutomationAPIKey(s string) bool {
	return len(s) > len(automationKeyPrefix) && s[:len(automationKeyPrefix)] == automationKeyPrefix
}

// UnixNow returns the current unix timestamp.
func UnixNow() int64 { return time.Now().UTC().Unix() }
