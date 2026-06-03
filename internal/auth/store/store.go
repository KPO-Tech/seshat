package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/auth/types"
)

// ============================================================================
// Store Interface
// ============================================================================

// Store persists authentication credentials
type Store interface {
	// Save credentials for a provider
	SaveCredentials(ctx context.Context, creds *types.Credentials) error

	// Load credentials for a provider
	LoadCredentials(ctx context.Context, provider string) (*types.Credentials, error)

	// Delete credentials for a provider
	DeleteCredentials(ctx context.Context, provider string) error

	// List all providers with stored credentials
	ListProviders(ctx context.Context) ([]string, error)
}

// ============================================================================
// File Store Implementation
// ============================================================================

// FileStore stores credentials in a JSON file
type FileStore struct {
	mu    sync.RWMutex
	path  string
	cache map[string]*types.Credentials
}

// NewFileStore creates a new file-based auth store
func NewFileStore(path string) (*FileStore, error) {
	store := &FileStore{
		path:  path,
		cache: make(map[string]*types.Credentials),
	}

	// Load existing credentials
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load auth store: %w", err)
	}

	return store, nil
}

// load loads credentials from file
func (s *FileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var creds map[string]*types.Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	s.cache = creds
	return nil
}

// save persists credentials to file
func (s *FileStore) save() error {
	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(s.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// SaveCredentials implements Store
func (s *FileStore) SaveCredentials(ctx context.Context, creds *types.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds.UpdatedAt = time.Now()
	if creds.CreatedAt.IsZero() {
		creds.CreatedAt = creds.UpdatedAt
	}

	s.cache[creds.Provider] = creds
	return s.save()
}

// LoadCredentials implements Store
func (s *FileStore) LoadCredentials(ctx context.Context, provider string) (*types.Credentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	creds, ok := s.cache[provider]
	if !ok {
		return nil, fmt.Errorf("no credentials for provider: %s", provider)
	}

	return creds, nil
}

// DeleteCredentials implements Store
func (s *FileStore) DeleteCredentials(ctx context.Context, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cache, provider)
	return s.save()
}

// ListProviders implements Store
func (s *FileStore) ListProviders(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providers := make([]string, 0, len(s.cache))
	for provider := range s.cache {
		providers = append(providers, provider)
	}

	return providers, nil
}

// ============================================================================
// Memory Store (for testing)
// ============================================================================

// MemoryStore is an in-memory store for testing
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]*types.Credentials
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]*types.Credentials),
	}
}

// SaveCredentials implements Store
func (s *MemoryStore) SaveCredentials(ctx context.Context, creds *types.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds.UpdatedAt = time.Now()
	if creds.CreatedAt.IsZero() {
		creds.CreatedAt = creds.UpdatedAt
	}

	s.data[creds.Provider] = creds
	return nil
}

// LoadCredentials implements Store
func (s *MemoryStore) LoadCredentials(ctx context.Context, provider string) (*types.Credentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	creds, ok := s.data[provider]
	if !ok {
		return nil, fmt.Errorf("no credentials for provider: %s", provider)
	}

	return creds, nil
}

// DeleteCredentials implements Store
func (s *MemoryStore) DeleteCredentials(ctx context.Context, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, provider)
	return nil
}

// ListProviders implements Store
func (s *MemoryStore) ListProviders(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providers := make([]string, 0, len(s.data))
	for provider := range s.data {
		providers = append(providers, provider)
	}

	return providers, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// DefaultStorePath returns the default store path
func DefaultStorePath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".nexus", "auth.json")
}
