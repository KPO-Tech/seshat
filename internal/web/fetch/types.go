// Package fetch contains the reusable web fetch core shared by browser- and tool-facing adapters.
package fetch

import (
	"time"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

const (
	// RenderModeAuto keeps the routing policy in charge of choosing the backend.
	RenderModeAuto = webcore.RenderModeAuto
	// RenderModeHTTP forces the fast HTTP path.
	RenderModeHTTP = webcore.RenderModeHTTP
	// RenderModeBrowser forces JS-rendered extraction through the browser runtime.
	RenderModeBrowser = webcore.RenderModeBrowser
)

// Request describes a backend fetch request independent from any specific tool wrapper.
type Request = webcore.FetchRequest

// FetchedContent represents normalized fetched content returned by the core service.
type FetchedContent = webcore.FetchResponse

// RedirectInfo indicates that the caller should explicitly follow a host-changing redirect.
type RedirectInfo = webcore.RedirectInfo

// Config configures the shared fetch service.
type Config struct {
	HTTPClient            HTTPClient
	BrowserManager        webcore.BrowserManager
	ArtifactStore         webcore.ArtifactStore
	Cache                 *Cache
	DecisionCache         *DecisionCache
	Resolver              webcore.HostResolver
	Timeout               time.Duration
	MaxContentLength      int
	MaxRedirects          int
	RenderPoolEnabled     bool
	RenderPoolTTL         time.Duration
	RenderPoolMaxSessions int
}

// DefaultConfig returns a pragmatic default configuration for the shared fetch service.
func DefaultConfig() *Config {
	return &Config{
		Timeout:               60 * time.Second,
		MaxContentLength:      10 * 1024 * 1024,
		MaxRedirects:          10,
		RenderPoolEnabled:     true,
		RenderPoolTTL:         10 * time.Minute,
		RenderPoolMaxSessions: 32,
	}
}
