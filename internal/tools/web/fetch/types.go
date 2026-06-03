package webfetch

import (
	"fmt"
	"net/url"
	"strings"

	fetchcore "github.com/EngineerProjects/nexus-engine/internal/web/fetch"
)

// Input represents the tool input.
type Input struct {
	URL        string `json:"url"`
	Prompt     string `json:"prompt"`
	RenderMode string `json:"render_mode,omitempty"`
}

// Output represents the tool output.
type Output struct {
	Bytes         int    `json:"bytes"`
	Code          int    `json:"code"`
	CodeText      string `json:"codeText"`
	Result        string `json:"result"`
	DurationMs    int64  `json:"durationMs"`
	URL           string `json:"url"`
	Mode          string `json:"mode,omitempty"`
	PersistedPath string `json:"persisted_path,omitempty"`
	PersistedSize int    `json:"persisted_size,omitempty"`
}

type FetchedContent = fetchcore.FetchedContent
type RedirectInfo = fetchcore.RedirectInfo
type Config = fetchcore.Config
type Cache = fetchcore.Cache
type HTTPClient = fetchcore.HTTPClient

// Validate validates the input.
func (i *Input) Validate() error {
	if strings.TrimSpace(i.URL) == "" {
		return fetchcore.Err("url is required")
	}
	if strings.TrimSpace(i.Prompt) == "" {
		return fetchcore.Err("prompt is required")
	}
	if _, err := fetchcore.NormalizeRenderMode(i.RenderMode); err != nil {
		return err
	}

	parsed, err := url.Parse(strings.TrimSpace(i.URL))
	if err != nil {
		return fmt.Errorf("%w: %v", fetchcore.ErrInvalidURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fetchcore.Err("URL must use http or https")
	}
	if parsed.Host == "" {
		return fetchcore.Err("URL must have a host")
	}

	return nil
}

// DefaultConfig returns the shared fetch-core defaults through the tool package for backward compatibility.
func DefaultConfig() *Config {
	return fetchcore.DefaultConfig()
}

// DefaultCache keeps the old wrapper-level constructor available while delegating to the shared fetch core.
func DefaultCache() *Cache {
	return fetchcore.DefaultCache()
}
