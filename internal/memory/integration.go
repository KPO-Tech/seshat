package memory

import (
	"fmt"
	"sync"
)

// ============================================================================
// Memory Integration Layer
// ============================================================================

// IntegrationManager provides a unified interface to the catalog-backed and
// manager-backed memory layers.
type IntegrationManager struct {
	mu            sync.RWMutex
	catalog       *Catalog // Searchable in-memory catalog
	manager       *Manager // File-backed scoped manager
	enableCatalog bool     // Use catalog-backed operations when available
}

// NewIntegrationManager creates a new integration manager
func NewIntegrationManager(enableCatalog bool) (*IntegrationManager, error) {
	// Initialize file-backed manager
	manager, err := NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create memory manager: %w", err)
	}

	// Initialize catalog
	var catalog *Catalog
	if enableCatalog {
		catalog = NewCatalog()
	}

	return &IntegrationManager{
		catalog:       catalog,
		manager:       manager,
		enableCatalog: enableCatalog,
	}, nil
}

// Start starts the catalog-backed layer when enabled.
func (im *IntegrationManager) Start() error {
	if im.catalog != nil {
		im.catalog.Start()
	}
	return nil
}

// Stop stops the catalog-backed layer when enabled.
func (im *IntegrationManager) Stop() {
	if im.catalog != nil {
		im.catalog.Stop()
	}
}

// StorePreference stores a user preference
func (im *IntegrationManager) StorePreference(scope MemoryScope, key, value, source string) error {
	if im.enableCatalog && im.catalog != nil {
		entry := Entry{
			Scope:      scope,
			Type:       MemoryTypePreference,
			Key:        key,
			Value:      value,
			Content:    value,
			Source:     source,
			Tags:       []string{"preference"},
			Confidence: 0.8, // Set high confidence for user preferences
			Importance: 0.5, // Set moderate importance for user preferences
		}
		return im.catalog.StoreEntry(entry)
	}

	return im.manager.LearnPreference(scope, key, value, source)
}

// GetPreferences retrieves preferences for a scope
func (im *IntegrationManager) GetPreferences(scope MemoryScope) []*Entry {
	if im.enableCatalog && im.catalog != nil {
		query := MemoryQuery{
			Types:         []MemoryType{MemoryTypePreference},
			MinConfidence: 0.5,
			Limit:         100,
		}

		result, err := im.catalog.Search(query)
		if err != nil {
			return []*Entry{}
		}

		// Convert []Entry to []*Entry
		entries := make([]*Entry, len(result.Entries))
		for i := range result.Entries {
			entries[i] = &result.Entries[i]
		}
		return entries
	}

	return im.manager.GetPreferences(scope)
}

// LearnToolUsage learns from tool execution.
func (im *IntegrationManager) LearnToolUsage(toolName string, parameters map[string]any, success bool, err error) error {
	if im.enableCatalog && im.catalog != nil {
		return im.catalog.LearnToolUsage(toolName, parameters, success, err)
	}

	return nil
}

// GetToolUsagePatterns retrieves tool usage patterns
func (im *IntegrationManager) GetToolUsagePatterns(toolName string) (*ToolUsageMemory, error) {
	if im.enableCatalog && im.catalog != nil {
		return im.catalog.GetToolUsagePatterns(toolName)
	}

	return nil, fmt.Errorf("tool usage patterns not available in manager mode")
}

// Context returns memory context for inclusion in LLM prompts
func (im *IntegrationManager) Context() string {
	var ctx string

	if im.enableCatalog && im.catalog != nil {
		query := MemoryQuery{
			Types:         []MemoryType{MemoryTypePreference, MemoryTypeInstruction},
			MinImportance: 0.3,
			Limit:         20,
		}

		result, err := im.catalog.Search(query)
		if err == nil && len(result.Entries) > 0 {
			ctx += "## Memory Preferences\n"
			for _, entry := range result.Entries {
				ctx += fmt.Sprintf("- %s: %s (confidence: %.2f)\n", entry.Key, entry.Value, entry.Confidence)
			}
		}
	}

	if ctx == "" {
		ctx += im.manager.Context()
	}

	return ctx
}

// Search performs intelligent search across memory systems
func (im *IntegrationManager) Search(query MemoryQuery) (*MemorySearchResult, error) {
	if im.enableCatalog && im.catalog != nil {
		return im.catalog.Search(query)
	}

	return nil, fmt.Errorf("catalog search not available in manager mode")
}

// Stats returns combined statistics from both layers.
func (im *IntegrationManager) Stats() map[string]interface{} {
	stats := make(map[string]interface{})

	if im.enableCatalog && im.catalog != nil {
		stats["catalog"] = im.catalog.Stats()
	}

	stats["manager"] = "enabled"

	return stats
}

// ExportAll exports all memory from all systems
func (im *IntegrationManager) ExportAll() (map[string][]byte, error) {
	result := make(map[string][]byte)

	if im.enableCatalog && im.catalog != nil {
		data, err := im.catalog.Export()
		if err != nil {
			return result, fmt.Errorf("failed to export catalog memory: %w", err)
		}
		result["catalog"] = data
	}

	result["manager"] = []byte("manager memory stored in files")

	return result, nil
}

// IsCatalogMode returns whether catalog-backed memory is enabled.
func (im *IntegrationManager) IsCatalogMode() bool {
	return im.enableCatalog && im.catalog != nil
}

// EnableCatalog switches to catalog-backed memory mode.
func (im *IntegrationManager) EnableCatalog() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.enableCatalog = true
	if im.catalog == nil {
		im.catalog = NewCatalog()
		im.catalog.Start()
	}
}

// DisableCatalog switches back to manager-backed memory mode.
func (im *IntegrationManager) DisableCatalog() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.enableCatalog = false
}
