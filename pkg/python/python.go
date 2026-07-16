// Package python exposes the Seshat-managed Python subprocess layer.
// It re-exports DoclingManager from internal/python so callers outside the
// seshat module (e.g. the product API server) can start and stop
// docling-serve without accessing internal packages.
package python

import internalpython "github.com/KPO-Tech/seshat/internal/python"

const (
	DefaultDoclingPort = internalpython.DefaultDoclingPort
	DefaultDoclingHost = internalpython.DefaultDoclingHost
)

// DoclingManager starts and owns a docling-serve subprocess.
// Use DefaultDoclingManager() for the standard runtime-root venv.
type DoclingManager = internalpython.DoclingManager

// NewDoclingManager creates a manager for the venv at venvDir.
func NewDoclingManager(venvDir, host string, port int) *DoclingManager {
	return internalpython.NewDoclingManager(venvDir, host, port)
}

// DefaultDoclingManager creates a manager using the Seshat runtime root venv
// (~/.config/seshat/.venv or $SESHAT_RUNTIME_ROOT/.venv).
// Returns nil if the venv or docling-serve binary is not installed.
func DefaultDoclingManager() *DoclingManager {
	return internalpython.DefaultDoclingManager()
}
