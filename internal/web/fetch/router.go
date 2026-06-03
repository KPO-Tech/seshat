package fetch

import (
	"fmt"
	"net/url"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

type fetchPlan struct {
	Request       Request
	NormalizedURL string
	ParsedURL     *url.URL
	IsPreapproved bool
	Mode          string
}

// Prepare resolves normalization, preapproval, and backend selection before network work starts.
func (s *Service) Prepare(request Request) (*fetchPlan, error) {
	normalizedURL, parsedURL, err := NormalizeURL(request.URL)
	if err != nil {
		return nil, err
	}

	mode, err := s.resolveFetchMode(request)
	if err != nil {
		return nil, err
	}

	return &fetchPlan{
		Request:       request,
		NormalizedURL: normalizedURL,
		ParsedURL:     parsedURL,
		IsPreapproved: IsPreapprovedPath(parsedURL.Hostname(), parsedURL.EscapedPath()),
		Mode:          mode,
	}, nil
}

// resolveFetchMode keeps the routing policy reusable across wrappers while hiding browser availability details.
func (s *Service) resolveFetchMode(request Request) (string, error) {
	mode, err := NormalizeRenderMode(request.RenderMode)
	if err != nil {
		return "", err
	}

	switch mode {
	case webcore.RenderModeAuto:
		if s.browserManager == nil {
			return webcore.RenderModeHTTP, nil
		}
		return webcore.RenderModeAuto, nil
	case webcore.RenderModeHTTP:
		return webcore.RenderModeHTTP, nil
	case webcore.RenderModeBrowser:
		if s.browserManager == nil {
			return "", fmt.Errorf("browser render mode requires a browser manager")
		}
		return webcore.RenderModeBrowser, nil
	default:
		return "", fmt.Errorf("unsupported render mode %q", mode)
	}
}
