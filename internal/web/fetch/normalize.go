package fetch

import (
	"fmt"
	"net/url"
	"strings"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

// NormalizeURL upgrades plain HTTP inputs to HTTPS and ensures the fetch core always receives a canonical URL.
func NormalizeURL(raw string) (string, *url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme == "http" {
		parsed.Scheme = "https"
	}
	if parsed.Scheme != "https" {
		return "", nil, fmt.Errorf("URL must use http or https")
	}
	if parsed.Host == "" {
		return "", nil, fmt.Errorf("URL must have a host")
	}
	if err := webcore.RejectLocalNetworkTarget(parsed); err != nil {
		return "", nil, err
	}
	return parsed.String(), parsed, nil
}

// NormalizeRenderMode converts empty or mixed-case render hints into the canonical routing values.
func NormalizeRenderMode(raw string) (string, error) {
	mode := strings.TrimSpace(strings.ToLower(raw))
	if mode == "" {
		return webcore.RenderModeAuto, nil
	}

	switch mode {
	case webcore.RenderModeAuto, webcore.RenderModeHTTP, webcore.RenderModeBrowser:
		return mode, nil
	default:
		return "", fmt.Errorf(
			"render_mode must be one of %q, %q, or %q",
			webcore.RenderModeAuto,
			webcore.RenderModeHTTP,
			webcore.RenderModeBrowser,
		)
	}
}
