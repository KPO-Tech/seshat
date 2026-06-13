package app

import (
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/lsp"
)

// LSPEventType represents the type of LSP event.
type LSPEventType string

const (
	LSPEventStateChanged       LSPEventType = "state_changed"
	LSPEventDiagnosticsChanged LSPEventType = "diagnostics_changed"
)

// LSPEvent represents an event in the LSP system.
type LSPEvent struct {
	Type            LSPEventType
	Name            string
	State           lsp.ServerState
	Error           error
	DiagnosticCount int
}

// LSPClientInfo holds information about an LSP client's state.
type LSPClientInfo struct {
	Name            string
	State           lsp.ServerState
	Error           error
	DiagnosticCount int
	ConnectedAt     time.Time
}

// GetLSPStates returns the current state of all LSP clients (stub — LSP not wired).
func GetLSPStates() map[string]LSPClientInfo {
	return map[string]LSPClientInfo{}
}
