package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const metadataPrefix = "__nexus_meta__"

func metadataKeyForArtifact(key string) string {
	cleaned := strings.Trim(strings.TrimSpace(key), "/")
	return path.Join(metadataPrefix, cleaned) + ".json"
}

func metadataListPrefix(prefix string) string {
	cleaned := strings.Trim(strings.TrimSpace(prefix), "/")
	if cleaned == "" {
		return metadataPrefix
	}
	return path.Join(metadataPrefix, cleaned)
}

func checksumSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func defaultRetentionClass(namespace ArtifactNamespace) ArtifactRetentionClass {
	switch namespace {
	case NamespaceDocuments, NamespaceRAGDocuments:
		return RetentionDurable
	case NamespaceBrowserDownloads:
		return RetentionSession
	case NamespaceBrowserScreenshots, NamespaceWebArtifacts:
		return RetentionTemporary
	default:
		return RetentionDurable
	}
}

func defaultTTL(namespace ArtifactNamespace, class ArtifactRetentionClass) time.Duration {
	switch class {
	case RetentionTemporary:
		return 7 * 24 * time.Hour
	case RetentionSession:
		return 24 * time.Hour
	default:
		return 0
	}
}

func normalizeRetention(request ArtifactPutRequest) (ArtifactRetentionClass, time.Duration) {
	class := request.RetentionClass
	if class == "" {
		class = defaultRetentionClass(request.Namespace)
	}
	ttl := request.TTL
	if ttl <= 0 {
		ttl = defaultTTL(request.Namespace, class)
	}
	return class, ttl
}

func buildMetadata(request ArtifactPutRequest, key string, body []byte) ArtifactMetadata {
	now := request.Timestamp.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	class, ttl := normalizeRetention(request)
	meta := ArtifactMetadata{
		Key:            key,
		Namespace:      normalizeNamespace(request.Namespace),
		Filename:       normalizeArtifactFilename(request.Filename, request.ContentType, defaultArtifactBaseName(normalizeNamespace(request.Namespace))),
		SessionID:      strings.TrimSpace(request.SessionID),
		PageID:         strings.TrimSpace(request.PageID),
		ContentType:    strings.TrimSpace(request.ContentType),
		Size:           int64(len(body)),
		ChecksumSHA256: checksumSHA256(body),
		CreatedAt:      now,
		ModifiedAt:     now,
		RetentionClass: class,
	}
	if ttl > 0 {
		meta.ExpiresAt = now.Add(ttl)
	}
	return meta
}

func inferMetadataForDirectPut(key string, contentType string, body []byte) ArtifactMetadata {
	namespace := inferNamespaceFromKey(key)
	request := ArtifactPutRequest{
		Namespace:      ArtifactNamespace(namespace),
		Filename:       path.Base(strings.TrimSpace(key)),
		ContentType:    contentType,
		Timestamp:      time.Now().UTC(),
		RetentionClass: defaultRetentionClass(ArtifactNamespace(namespace)),
	}
	return buildMetadata(request, key, body)
}

func inferNamespaceFromKey(key string) string {
	cleaned := strings.Trim(strings.TrimSpace(key), "/")
	switch {
	// Session-scoped keys: sessions/{id}/{type}/… → namespace is sessions/{id}/{type}
	case strings.HasPrefix(cleaned, "sessions/"):
		parts := strings.SplitN(cleaned, "/", 4)
		if len(parts) >= 3 {
			return strings.Join(parts[:3], "/")
		}
		return "sessions"
	case strings.HasPrefix(cleaned, string(NamespaceBrowserScreenshots)+"/"):
		return string(NamespaceBrowserScreenshots)
	case strings.HasPrefix(cleaned, string(NamespaceBrowserDownloads)+"/"):
		return string(NamespaceBrowserDownloads)
	case strings.HasPrefix(cleaned, string(NamespaceWebArtifacts)+"/"):
		return string(NamespaceWebArtifacts)
	case strings.HasPrefix(cleaned, string(NamespaceRAGDocuments)+"/"):
		return string(NamespaceRAGDocuments)
	case strings.HasPrefix(cleaned, string(NamespaceDocuments)+"/"):
		return string(NamespaceDocuments)
	default:
		return path.Dir(cleaned)
	}
}

func (s *providerArtifactStore) persistMetadata(ctx context.Context, metadata ArtifactMetadata) error {
	body, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal artifact metadata: %w", err)
	}
	return s.provider.Upload(ctx, metadataKeyForArtifact(metadata.Key), body, "application/json")
}

func (s *providerArtifactStore) readMetadata(ctx context.Context, key string) (ArtifactMetadata, error) {
	body, err := s.provider.Download(ctx, metadataKeyForArtifact(key))
	if err != nil {
		return ArtifactMetadata{}, err
	}
	var metadata ArtifactMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return ArtifactMetadata{}, fmt.Errorf("decode artifact metadata: %w", err)
	}
	return metadata, nil
}

func readMetadataFromReader(reader io.Reader) (ArtifactMetadata, error) {
	var metadata ArtifactMetadata
	if err := json.NewDecoder(reader).Decode(&metadata); err != nil {
		return ArtifactMetadata{}, fmt.Errorf("decode artifact metadata: %w", err)
	}
	return metadata, nil
}
