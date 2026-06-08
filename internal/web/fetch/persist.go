package fetch

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
)

func isTextualContentType(contentType string) bool {
	lower := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(lower, "text/"):
		return true
	case strings.Contains(lower, "json"),
		strings.Contains(lower, "xml"),
		strings.Contains(lower, "javascript"),
		strings.Contains(lower, "ecmascript"),
		strings.Contains(lower, "graphql"),
		strings.Contains(lower, "yaml"),
		strings.Contains(lower, "csv"),
		strings.Contains(lower, "x-www-form-urlencoded"):
		return true
	default:
		return false
	}
}

func inferArtifactFilename(rawURL string, contentType string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		if base := path.Base(parsed.Path); base != "" && base != "." && base != "/" {
			return base
		}
	}
	switch {
	case strings.Contains(strings.ToLower(contentType), "pdf"):
		return "document.pdf"
	case strings.Contains(strings.ToLower(contentType), "png"):
		return "image.png"
	case strings.Contains(strings.ToLower(contentType), "jpeg"):
		return "image.jpg"
	case strings.Contains(strings.ToLower(contentType), "json"):
		return "data.json"
	default:
		return "download.bin"
	}
}

func (s *Service) persistArtifact(ctx context.Context, sessionID, finalURL, contentType string, body []byte) (string, int, error) {
	if s.artifactStore == nil || len(body) == 0 {
		return "", 0, nil
	}
	filename := inferArtifactFilename(finalURL, contentType)
	ref, err := storage.StoreWebArtifactRef(ctx, s.artifactStore, body, sessionID, filename, contentType)
	if err != nil {
		return "", 0, fmt.Errorf("persist fetched artifact: %w", err)
	}
	return ref.URL, int(ref.Size), nil
}
