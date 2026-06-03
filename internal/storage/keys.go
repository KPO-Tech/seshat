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

func ScreenshotKey(sessionID, pageID string, now time.Time) string {
	return BuildArtifactKey(ArtifactPutRequest{
		Namespace:   NamespaceBrowserScreenshots,
		Filename:    "screenshot.png",
		SessionID:   sessionID,
		PageID:      pageID,
		ContentType: "image/png",
		Timestamp:   now,
	})
}

func DownloadKey(sessionID, pageID, filename string, now time.Time) string {
	return BuildArtifactKey(ArtifactPutRequest{
		Namespace:   NamespaceBrowserDownloads,
		Filename:    filename,
		SessionID:   sessionID,
		PageID:      pageID,
		ContentType: DetectContentType(filename),
		Timestamp:   now,
	})
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
