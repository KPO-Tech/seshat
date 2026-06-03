package fastmode

import (
	"sync"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskType classifies the urgency/cost profile of a task so the manager
// can decide whether the fast model is appropriate.
type TaskType string

const (
	// TaskTypeClassifier is used for auto-mode permission classification calls.
	TaskTypeClassifier TaskType = "classifier"
	// TaskTypeQuick is used for short, latency-sensitive tool calls.
	TaskTypeQuick TaskType = "quick"
	// TaskTypeMain is used for the primary conversation turn — never substituted.
	TaskTypeMain TaskType = "main"
)

// FastmodeManager applies the fastmode config to model selection decisions.
// It is safe for concurrent use.
type FastmodeManager struct {
	mu  sync.RWMutex
	cfg *FastmodeConfig
}

// NewFastmodeManager creates a manager with the given config.
// Pass nil to use DefaultFastmodeConfig.
func NewFastmodeManager(cfg *FastmodeConfig) *FastmodeManager {
	if cfg == nil {
		cfg = DefaultFastmodeConfig()
	}
	return &FastmodeManager{cfg: cfg}
}

// SelectModel returns the model that should be used for the given task type
// and primary model. For TaskTypeMain, or when fastmode is disabled, the
// primary model is returned unchanged.
func (m *FastmodeManager) SelectModel(task TaskType, primary types.ModelIdentifier) types.ModelIdentifier {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.cfg.Enabled || task == TaskTypeMain {
		return primary
	}
	return m.cfg.FastModel
}

// MaxTokensFor returns the token cap to use for the given task. For main tasks
// zero is returned (meaning: use the caller's default).
func (m *FastmodeManager) MaxTokensFor(task TaskType) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.cfg.Enabled || task == TaskTypeMain {
		return 0
	}
	return m.cfg.MaxTokens
}

// SetEnabled toggles fastmode at runtime without replacing the config.
func (m *FastmodeManager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.Enabled = enabled
}

// IsEnabled reports whether fastmode is active.
func (m *FastmodeManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Enabled
}

// Config returns a copy of the current configuration.
func (m *FastmodeManager) Config() FastmodeConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.cfg
}
