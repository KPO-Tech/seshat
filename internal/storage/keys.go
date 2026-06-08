package storage

import (
	"fmt"
	"mime"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func BuildArtifactKey(request ArtifactPutRequest) string {
	now := request.Timestamp.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	namespace := normalizeNamespace(request.Namespace)
	filename := normalizeArtifactFilename(request.Filename, request.ContentType, defaultArtifactBaseName(namespace))
	datePrefix := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())

	prefixParts := []string{namespace}
	if request.SessionID != "" {
		prefixParts = append(prefixParts, sanitizePathSegment(request.SessionID))
	}
	if request.PageID != "" {
		prefixParts = append(prefixParts, sanitizePathSegment(request.PageID))
	}
	prefixParts = append(prefixParts, datePrefix)

	return path.Join(path.Join(prefixParts...), fmt.Sprintf("%d-%s", now.UnixNano(), filename))
}

func PDFKey(title string, now time.Time) string {
	return BuildArtifactKey(ArtifactPutRequest{
		Namespace:   NamespaceDocuments,
		Filename:    title,
		ContentType: "application/pdf",
		Timestamp:   now,
	})
}

// ScreenshotKey builds a session-scoped storage key for a browser screenshot.
// Layout: sessions/{sessionID}/screenshots/{pageID}/{date}/{timestamp}-screenshot.png
func ScreenshotKey(sessionID, pageID string, now time.Time) string {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	datePrefix := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	parts := []string{"sessions", sanitizePathSegment(sessionID), "screenshots"}
	if pageID != "" {
		parts = append(parts, sanitizePathSegment(pageID))
	}
	parts = append(parts, datePrefix)
	return path.Join(append(parts, fmt.Sprintf("%d-screenshot.png", now.UnixNano()))...)
}

// DownloadKey builds a session-scoped storage key for a browser download.
// Layout: sessions/{sessionID}/tools/{pageID}/{date}/{timestamp}-{filename}
func DownloadKey(sessionID, pageID, filename string, now time.Time) string {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	datePrefix := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	filename = normalizeArtifactFilename(filename, DetectContentType(filename), "download")
	parts := []string{"sessions", sanitizePathSegment(sessionID), "tools"}
	if pageID != "" {
		parts = append(parts, sanitizePathSegment(pageID))
	}
	parts = append(parts, datePrefix)
	return path.Join(append(parts, fmt.Sprintf("%d-%s", now.UnixNano(), filename))...)
}

// WebArtifactKey builds a session-scoped key for web-fetched content.
// Layout: sessions/{sessionID}/artifacts/web/{date}/{timestamp}-{filename}
func WebArtifactKey(sessionID, filename string, now time.Time) string {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	datePrefix := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	filename = normalizeArtifactFilename(filename, DetectContentType(filename), "fetched")
	parts := []string{"sessions", sanitizePathSegment(sessionID), "artifacts", "web", datePrefix}
	return path.Join(append(parts, fmt.Sprintf("%d-%s", now.UnixNano(), filename))...)
}

// GeneratedImageKey builds a session-scoped key for an AI-generated image.
// Layout: sessions/{sessionID}/artifacts/images/{date}/{timestamp}-{filename}
func GeneratedImageKey(sessionID, filename string, now time.Time) string {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	datePrefix := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	filename = normalizeArtifactFilename(filename, DetectContentType(filename), "image")
	parts := []string{"sessions", sanitizePathSegment(sessionID), "artifacts", "images", datePrefix}
	return path.Join(append(parts, fmt.Sprintf("%d-%s", now.UnixNano(), filename))...)
}

// AudioKey builds a session-scoped key for a TTS/STT audio file.
// Layout: sessions/{sessionID}/artifacts/audio/{date}/{timestamp}-{filename}
func AudioKey(sessionID, filename string, now time.Time) string {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	datePrefix := fmt.Sprintf("%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	filename = normalizeArtifactFilename(filename, DetectContentType(filename), "audio")
	parts := []string{"sessions", sanitizePathSegment(sessionID), "artifacts", "audio", datePrefix}
	return path.Join(append(parts, fmt.Sprintf("%d-%s", now.UnixNano(), filename))...)
}

func DocumentKey(filename string, now time.Time) string {
	return BuildArtifactKey(ArtifactPutRequest{
		Namespace:   NamespaceDocuments,
		Filename:    filename,
		ContentType: DetectContentType(filename),
		Timestamp:   now,
	})
}

func DetectContentType(filename string) string {
	contentType := mime.TypeByExtension(path.Ext(filename))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func normalizeNamespace(namespace ArtifactNamespace) string {
	value := strings.Trim(strings.TrimSpace(string(namespace)), "/")
	if value == "" {
		return string(NamespaceDocuments)
	}
	return value
}

func defaultArtifactBaseName(namespace string) string {
	switch namespace {
	case string(NamespaceBrowserScreenshots):
		return "screenshot"
	case string(NamespaceBrowserDownloads):
		return "download"
	case string(NamespaceWebArtifacts):
		return "fetched"
	case string(NamespaceRAGDocuments):
		return "document"
	default:
		return "artifact"
	}
}

func normalizeArtifactFilename(filename string, contentType string, fallbackBase string) string {
	result := sanitizeFilename(filename)
	if result == "" || result == "artifact" {
		result = fallbackBase
	}
	if ext := filepath.Ext(result); ext != "" {
		return result
	}
	if inferred := extensionForContentType(contentType); inferred != "" {
		return result + inferred
	}
	return result
}

func extensionForContentType(contentType string) string {
	extensions, err := mime.ExtensionsByType(strings.TrimSpace(contentType))
	if err != nil || len(extensions) == 0 {
		return ""
	}
	return extensions[0]
}

func sanitizeFilename(name string) string {
	result := strings.TrimSpace(name)
	if result == "" {
		return "artifact"
	}
	var builder strings.Builder
	builder.Grow(len(result))
	for _, r := range result {
		switch r {
		case '/', ':', '\\':
			builder.WriteByte('_')
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func sanitizePathSegment(name string) string {
	return strings.Trim(sanitizeFilename(name), " .")
}
