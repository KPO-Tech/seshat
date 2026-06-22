package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/EngineerProjects/seshat/internal/auth/types"
)

// encryptedEnvelope is the on-disk format for encrypted credentials.
// Files without this wrapper are treated as plaintext (migration path).
type encryptedEnvelope struct {
	Version int    `json:"__seshat_v"`
	Data    string `json:"data"` // base64(nonce || ciphertext)
}

// EncryptedFileStore stores credentials encrypted with AES-256-GCM.
//
// Key precedence:
//  1. SESHAT_MASTER_KEY env var (64 hex chars = 32 bytes)
//  2. Machine-derived key (SHA-256 of machine ID + constant)
//
// Existing plaintext files are read transparently and re-encrypted on the
// next write, providing a seamless migration path.
type EncryptedFileStore struct {
	mu    sync.RWMutex
	path  string
	cache map[string]*types.Credentials
	key   []byte // 32 bytes, AES-256
}

// NewEncryptedFileStore creates a new encrypted file-based auth store.
// Returns an error only if key derivation fails; a missing file is not an error.
func NewEncryptedFileStore(path string) (*EncryptedFileStore, error) {
	key, err := deriveKey()
	if err != nil {
		return nil, fmt.Errorf("derive encryption key: %w", err)
	}

	s := &EncryptedFileStore{
		path:  path,
		cache: make(map[string]*types.Credentials),
		key:   key,
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load encrypted auth store: %w", err)
	}

	return s, nil
}

// load decrypts and parses credentials from disk. Falls back to plaintext
// JSON if the file predates encryption (migration).
func (s *EncryptedFileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var creds map[string]*types.Credentials

	// Detect encrypted envelope vs. legacy plaintext.
	var env encryptedEnvelope
	if json.Unmarshal(data, &env) == nil && env.Version == 1 && env.Data != "" {
		plaintext, err := s.decrypt(env.Data)
		if err != nil {
			return fmt.Errorf("decrypt credentials: %w", err)
		}
		if err := json.Unmarshal(plaintext, &creds); err != nil {
			return fmt.Errorf("unmarshal decrypted credentials: %w", err)
		}
	} else {
		// Plaintext legacy file — parse and schedule re-encryption on next write.
		if err := json.Unmarshal(data, &creds); err != nil {
			return fmt.Errorf("unmarshal credentials: %w", err)
		}
	}

	s.cache = creds
	return nil
}

// save encrypts and persists credentials to disk using an atomic tmp-then-rename write.
func (s *EncryptedFileStore) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	plain, err := json.Marshal(s.cache)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	cipherData, err := s.encrypt(plain)
	if err != nil {
		return fmt.Errorf("encrypt credentials: %w", err)
	}

	envelope := encryptedEnvelope{Version: 1, Data: cipherData}
	out, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// encrypt seals plaintext with AES-256-GCM and returns base64(nonce||ciphertext).
func (s *EncryptedFileStore) encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt opens base64(nonce||ciphertext) with AES-256-GCM.
func (s *EncryptedFileStore) decrypt(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, raw[:ns], raw[ns:], nil)
}

// SaveCredentials implements Store.
func (s *EncryptedFileStore) SaveCredentials(ctx context.Context, creds *types.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	creds.UpdatedAt = time.Now()
	if creds.CreatedAt.IsZero() {
		creds.CreatedAt = creds.UpdatedAt
	}
	s.cache[creds.Provider] = creds
	return s.save()
}

// LoadCredentials implements Store.
func (s *EncryptedFileStore) LoadCredentials(ctx context.Context, provider string) (*types.Credentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	creds, ok := s.cache[provider]
	if !ok {
		return nil, fmt.Errorf("no credentials for provider: %s", provider)
	}
	return creds, nil
}

// DeleteCredentials implements Store.
func (s *EncryptedFileStore) DeleteCredentials(ctx context.Context, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, provider)
	return s.save()
}

// ListProviders implements Store.
func (s *EncryptedFileStore) ListProviders(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	providers := make([]string, 0, len(s.cache))
	for p := range s.cache {
		providers = append(providers, p)
	}
	return providers, nil
}

// deriveKey returns a 32-byte AES key.
// Priority: SESHAT_MASTER_KEY env var → machine-derived key.
func deriveKey() ([]byte, error) {
	if keyHex := os.Getenv("SESHAT_MASTER_KEY"); keyHex != "" {
		raw, err := hexDecode32(keyHex)
		if err != nil {
			return nil, fmt.Errorf("SESHAT_MASTER_KEY must be 64 hex characters (32 bytes): %w", err)
		}
		return raw, nil
	}
	seed := machineID()
	h := sha256.Sum256([]byte("seshat-auth-key-v1\x00" + seed))
	return h[:], nil
}

// hexDecode32 decodes a 64-char hex string into 32 bytes.
func hexDecode32(s string) ([]byte, error) {
	if len(s) != 64 {
		return nil, fmt.Errorf("expected 64 hex chars, got %d", len(s))
	}
	var b [32]byte
	for i := 0; i < 32; i++ {
		hi := hexVal(s[i*2])
		lo := hexVal(s[i*2+1])
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex character at position %d", i*2)
		}
		b[i] = byte(hi<<4 | lo)
	}
	return b[:], nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// machineID returns a stable, machine-specific identifier used for key
// derivation when no explicit key is configured.
// Order: /etc/machine-id (Linux) → /var/db/dstlocal (macOS) → hostname+uid.
func machineID() string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if data, err := os.ReadFile(path); err == nil {
			if id := strings.TrimSpace(string(data)); id != "" {
				return id
			}
		}
	}
	hostname, _ := os.Hostname()
	u, _ := user.Current()
	uid := ""
	if u != nil {
		uid = u.Uid
	}
	return hostname + ":" + uid
}
