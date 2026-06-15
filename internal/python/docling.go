// Package python manages Nexus-owned Python subprocesses (docling-serve, …).
package python

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

const (
	DefaultDoclingPort  = 5001
	DefaultDoclingHost  = "127.0.0.1"
	healthCheckTimeout  = 3 * time.Second
	startupPollInterval = 500 * time.Millisecond
	startupMaxWait      = 60 * time.Second
)

// DoclingManager starts and owns a docling-serve subprocess.
// Call Start() once; the process is killed when the context is cancelled or
// Stop() is called.
type DoclingManager struct {
	venvDir string
	host    string
	port    int

	mu      sync.Mutex
	cmd     *exec.Cmd
	baseURL string
	ready   bool
	readyCh chan struct{} // closed once health check passes
}

// NewDoclingManager creates a manager for the venv at venvDir.
// host:port is where docling-serve will bind (typically 127.0.0.1:5001).
func NewDoclingManager(venvDir, host string, port int) *DoclingManager {
	if host == "" {
		host = DefaultDoclingHost
	}
	if port == 0 {
		port = DefaultDoclingPort
	}
	return &DoclingManager{
		venvDir: venvDir,
		host:    host,
		port:    port,
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		readyCh: make(chan struct{}),
	}
}

// DefaultDoclingManager creates a manager using the Nexus runtime root venv.
// Returns nil if the venv or docling-serve binary is not installed.
// On Linux/macOS the venv lives at ~/.config/nexus-cli/.venv (or
// $NEXUS_RUNTIME_ROOT/.venv). On Windows: %APPDATA%\nexus-cli\.venv.
func DefaultDoclingManager() *DoclingManager {
	venvDir := filepath.Join(runtimepath.ResolveRoot(""), ".venv")
	bin := venvBin(venvDir)
	if _, err := os.Stat(bin); err != nil {
		return nil
	}
	return NewDoclingManager(venvDir, DefaultDoclingHost, DefaultDoclingPort)
}

// BaseURL returns the HTTP base URL of the managed docling-serve instance.
func (m *DoclingManager) BaseURL() string { return m.baseURL }

// IsReady reports whether the server has passed its health check.
func (m *DoclingManager) IsReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

// WaitReady blocks until the server is up or ctx is cancelled.
func (m *DoclingManager) WaitReady(ctx context.Context) error {
	select {
	case <-m.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Start launches docling-serve in the background and immediately returns.
// A goroutine polls the /health endpoint; IsReady() becomes true once it responds.
// The process is killed when ctx is cancelled.
func (m *DoclingManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil {
		return nil // already started
	}

	// If a server is already listening (external process), skip launching.
	if m.isAlive() {
		m.ready = true
		close(m.readyCh)
		return nil
	}

	bin := venvBin(m.venvDir)
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("docling-serve not installed at %s; run: make install-python", bin)
	}

	cmd := exec.CommandContext(ctx, bin, "run",
		"--host", m.host,
		"--port", fmt.Sprintf("%d", m.port),
		"--workers", "1",
	)
	// Silence output unless NEXUS_DOCLING_VERBOSE is set.
	if os.Getenv("NEXUS_DOCLING_VERBOSE") == "" {
		logPath := filepath.Join(runtimepath.ResolveRoot(""), "logs", "docling.log")
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start docling-serve: %w", err)
	}
	m.cmd = cmd

	// Background: wait for process exit (log, set not-ready).
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		m.ready = false
		m.cmd = nil
		m.mu.Unlock()
	}()

	// Background: poll until healthy, then mark ready.
	go func() {
		deadline := time.Now().Add(startupMaxWait)
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(startupPollInterval):
			}
			if m.isAlive() {
				m.mu.Lock()
				if !m.ready {
					m.ready = true
					close(m.readyCh)
				}
				m.mu.Unlock()
				return
			}
		}
	}()

	return nil
}

// Stop kills the managed subprocess if one is running.
func (m *DoclingManager) Stop() {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// isAlive does a cheap GET /health with a short timeout.
func (m *DoclingManager) isAlive() bool {
	client := &http.Client{Timeout: healthCheckTimeout}
	resp, err := client.Get(m.baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// venvBin returns the platform-appropriate path to the docling-serve binary
// inside the given venv directory.
// Linux/macOS: <venvDir>/bin/docling-serve
// Windows:     <venvDir>\Scripts\docling-serve.exe
func venvBin(venvDir string) string {
	if isWindows() {
		return filepath.Join(venvDir, "Scripts", "docling-serve.exe")
	}
	return filepath.Join(venvDir, "bin", "docling-serve")
}

func isWindows() bool {
	// Avoid importing runtime in hot paths; use GOOS baked at compile time.
	return os.PathSeparator == '\\'
}
