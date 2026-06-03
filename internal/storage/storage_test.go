package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalProvider(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()

	storageCfg := Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	}
	SetConfig(storageCfg)
	defer ResetProvider()

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	key := "test/file.txt"
	content := []byte("hello world")

	err = provider.Upload(ctx, key, content, "text/plain")
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	exists, err := provider.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("File should exist after upload")
	}

	data, err := provider.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("Download content mismatch: got %s, want %s", string(data), string(content))
	}

	url, err := provider.GetURL(ctx, key)
	if err != nil {
		t.Fatalf("GetURL failed: %v", err)
	}
	expectedPath := filepath.Join(tmpDir, key)
	if url != expectedPath {
		t.Errorf("GetURL mismatch: got %s, want %s", url, expectedPath)
	}

	err = provider.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	exists, err = provider.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists after delete failed: %v", err)
	}
	if exists {
		t.Error("File should not exist after delete")
	}
}

func TestLocalProviderDefaultPath(t *testing.T) {
	storageCfg := Config{
		Provider:  ProviderLocal,
		LocalPath: "",
	}
	SetConfig(storageCfg)
	defer ResetProvider()

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	localProvider, ok := provider.(*LocalProvider)
	if !ok {
		t.Fatal("Provider should be LocalProvider")
	}

	if localProvider.basePath != DefaultLocalPath() {
		t.Errorf("Default path should be %s, got %s", DefaultLocalPath(), localProvider.basePath)
	}
}

func TestGetConfigFromEnv(t *testing.T) {
	os.Setenv("NEXUS_STORAGE_PROVIDER", "s3")
	os.Setenv("NEXUS_STORAGE_LOCAL_PATH", "/custom/path")
	os.Setenv("NEXUS_S3_ENDPOINT", "minio:9000")
	os.Setenv("NEXUS_S3_BUCKET", "test-bucket")
	os.Setenv("NEXUS_S3_ACCESS_KEY_ID", "key")
	os.Setenv("NEXUS_S3_SECRET_ACCESS_KEY", "secret")
	os.Setenv("NEXUS_S3_REGION", "us-west-2")
	os.Setenv("NEXUS_S3_KEY_PREFIX", "prefix")

	cfg := GetConfigFromEnv()

	if cfg.Provider != ProviderS3 {
		t.Errorf("Provider should be s3, got %s", cfg.Provider)
	}
	if cfg.LocalPath != "/custom/path" {
		t.Errorf("LocalPath should be /custom/path, got %s", cfg.LocalPath)
	}
	if cfg.S3Endpoint != "minio:9000" {
		t.Errorf("S3Endpoint should be minio:9000, got %s", cfg.S3Endpoint)
	}
	if cfg.S3Bucket != "test-bucket" {
		t.Errorf("S3Bucket should be test-bucket, got %s", cfg.S3Bucket)
	}
	if cfg.S3AccessKeyID != "key" {
		t.Errorf("S3AccessKeyID should be key, got %s", cfg.S3AccessKeyID)
	}
	if cfg.S3SecretAccessKey != "secret" {
		t.Errorf("S3SecretAccessKey should be secret, got %s", cfg.S3SecretAccessKey)
	}
	if cfg.S3Region != "us-west-2" {
		t.Errorf("S3Region should be us-west-2, got %s", cfg.S3Region)
	}
	if cfg.S3KeyPrefix != "prefix" {
		t.Errorf("S3KeyPrefix should be prefix, got %s", cfg.S3KeyPrefix)
	}

	os.Unsetenv("NEXUS_STORAGE_PROVIDER")
	os.Unsetenv("NEXUS_STORAGE_LOCAL_PATH")
	os.Unsetenv("NEXUS_S3_ENDPOINT")
	os.Unsetenv("NEXUS_S3_BUCKET")
	os.Unsetenv("NEXUS_S3_ACCESS_KEY_ID")
	os.Unsetenv("NEXUS_S3_SECRET_ACCESS_KEY")
	os.Unsetenv("NEXUS_S3_REGION")
	os.Unsetenv("NEXUS_S3_KEY_PREFIX")
}

func TestHealthCheck(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	storageCfg := Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	}
	SetConfig(storageCfg)
	defer ResetProvider()

	err := HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestArtifactStorePutReturnsRef(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	SetConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	defer ResetProvider()

	store, err := DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore failed: %v", err)
	}

	ref, err := store.Put(ctx, "artifacts/test.txt", []byte("hello"), "text/plain")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if ref.Key != "artifacts/test.txt" {
		t.Fatalf("unexpected key: %s", ref.Key)
	}
	if ref.URL == "" {
		t.Fatal("expected URL to be populated")
	}
	if ref.Size != 5 {
		t.Fatalf("unexpected size: %d", ref.Size)
	}
	if ref.ModifiedAt.IsZero() {
		t.Fatal("expected modified time to be populated")
	}
}

func TestArtifactStorePutArtifactUsesNamespacedLayout(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	SetConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	defer ResetProvider()

	store, err := DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore failed: %v", err)
	}

	ref, err := store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:   NamespaceBrowserDownloads,
		Filename:    "report.pdf",
		SessionID:   "sess-1",
		PageID:      "page-2",
		ContentType: "application/pdf",
		Timestamp:   time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC),
	}, []byte("pdf-data"))
	if err != nil {
		t.Fatalf("PutArtifact failed: %v", err)
	}

	if !strings.HasPrefix(ref.Key, "artifacts/browser/downloads/sess-1/page-2/2026/05/13/") {
		t.Fatalf("unexpected key layout: %s", ref.Key)
	}
	if !strings.HasSuffix(ref.Key, "-report.pdf") {
		t.Fatalf("unexpected key filename: %s", ref.Key)
	}
}

func TestLocalProviderStatListAndOpenReader(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	provider, err := NewLocalProviderWithConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewLocalProviderWithConfig failed: %v", err)
	}

	files := map[string][]byte{
		"artifacts/web/2026/05/13/file-a.txt": []byte("alpha"),
		"artifacts/web/2026/05/13/file-b.txt": []byte("beta"),
		"documents/2026/05/13/file-c.txt":     []byte("gamma"),
	}
	for key, body := range files {
		if err := provider.Upload(ctx, key, body, "text/plain"); err != nil {
			t.Fatalf("Upload(%s) failed: %v", key, err)
		}
	}

	info, err := provider.Stat(ctx, "artifacts/web/2026/05/13/file-a.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != int64(len(files["artifacts/web/2026/05/13/file-a.txt"])) {
		t.Fatalf("unexpected stat size: %d", info.Size)
	}
	if info.ContentType != "text/plain; charset=utf-8" && info.ContentType != "text/plain" {
		t.Fatalf("unexpected content type: %s", info.ContentType)
	}

	reader, openedInfo, err := provider.OpenReader(ctx, "artifacts/web/2026/05/13/file-b.txt")
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(body) != "beta" {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if openedInfo.Key != "artifacts/web/2026/05/13/file-b.txt" {
		t.Fatalf("unexpected open info key: %s", openedInfo.Key)
	}

	listed, err := provider.List(ctx, ListOptions{Prefix: "artifacts/web", Limit: 10})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 listed files, got %d", len(listed))
	}
	for _, item := range listed {
		if !strings.HasPrefix(item.Key, "artifacts/web/") {
			t.Fatalf("unexpected listed key: %s", item.Key)
		}
	}
}

func TestArtifactStorePersistsMetadataAndChecksum(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	SetConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	defer ResetProvider()

	store, err := DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore failed: %v", err)
	}

	ref, err := store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:      NamespaceWebArtifacts,
		Filename:       "payload.json",
		ContentType:    "application/json",
		RetentionClass: RetentionTemporary,
		TTL:            2 * time.Hour,
		Timestamp:      time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC),
	}, []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("PutArtifact failed: %v", err)
	}

	if ref.ChecksumSHA256 == "" {
		t.Fatal("expected checksum to be populated")
	}
	if ref.ExpiresAt.IsZero() {
		t.Fatal("expected expires_at to be populated")
	}
	if ref.Namespace != string(NamespaceWebArtifacts) {
		t.Fatalf("unexpected namespace: %s", ref.Namespace)
	}

	meta, err := store.Metadata(ctx, ref.Key)
	if err != nil {
		t.Fatalf("Metadata failed: %v", err)
	}
	if meta.ChecksumSHA256 != ref.ChecksumSHA256 {
		t.Fatalf("unexpected checksum mismatch: %s vs %s", meta.ChecksumSHA256, ref.ChecksumSHA256)
	}
	if meta.RetentionClass != RetentionTemporary {
		t.Fatalf("unexpected retention class: %s", meta.RetentionClass)
	}
	if got := meta.ExpiresAt.Sub(meta.CreatedAt); got != 2*time.Hour {
		t.Fatalf("unexpected ttl window: %s", got)
	}
}

func TestArtifactStoreGarbageCollectDeletesExpiredTemporaryArtifacts(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	SetConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	defer ResetProvider()

	store, err := DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore failed: %v", err)
	}

	expiredRef, err := store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:      NamespaceBrowserScreenshots,
		Filename:       "shot.png",
		ContentType:    "image/png",
		RetentionClass: RetentionTemporary,
		TTL:            time.Hour,
		Timestamp:      time.Date(2026, time.May, 10, 10, 0, 0, 0, time.UTC),
	}, []byte("old"))
	if err != nil {
		t.Fatalf("PutArtifact expired failed: %v", err)
	}
	keptRef, err := store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:      NamespaceDocuments,
		Filename:       "kept.txt",
		ContentType:    "text/plain",
		RetentionClass: RetentionDurable,
		Timestamp:      time.Date(2026, time.May, 13, 10, 0, 0, 0, time.UTC),
	}, []byte("keep"))
	if err != nil {
		t.Fatalf("PutArtifact kept failed: %v", err)
	}

	report, err := store.GarbageCollect(ctx, GCOptions{
		Now: time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("GarbageCollect failed: %v", err)
	}
	if report.Deleted != 1 {
		t.Fatalf("expected 1 deleted artifact, got %d", report.Deleted)
	}
	if report.Scanned < 2 {
		t.Fatalf("expected at least 2 scanned artifacts, got %d", report.Scanned)
	}

	exists, err := store.Exists(ctx, expiredRef.Key)
	if err != nil {
		t.Fatalf("Exists expired failed: %v", err)
	}
	if exists {
		t.Fatal("expected expired artifact to be deleted")
	}
	exists, err = store.Exists(ctx, keptRef.Key)
	if err != nil {
		t.Fatalf("Exists kept failed: %v", err)
	}
	if !exists {
		t.Fatal("expected durable artifact to remain")
	}
}

func TestArtifactStoreGarbageCollectDryRunDoesNotDelete(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	SetConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	defer ResetProvider()

	store, err := DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore failed: %v", err)
	}
	ref, err := store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:      NamespaceBrowserDownloads,
		Filename:       "temp.bin",
		ContentType:    "application/octet-stream",
		RetentionClass: RetentionSession,
		TTL:            time.Minute,
		Timestamp:      time.Date(2026, time.May, 13, 10, 0, 0, 0, time.UTC),
	}, []byte("temp"))
	if err != nil {
		t.Fatalf("PutArtifact failed: %v", err)
	}

	report, err := store.GarbageCollect(ctx, GCOptions{
		Now:    time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("GarbageCollect dry run failed: %v", err)
	}
	if report.Deleted != 1 {
		t.Fatalf("expected 1 dry-run deletion, got %d", report.Deleted)
	}
	exists, err := store.Exists(ctx, ref.Key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("expected artifact to remain after dry-run GC")
	}
}

func TestLocalProviderPreventsTraversalEscapingBasePath(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	provider, err := NewLocalProviderWithConfig(Config{
		Provider:  ProviderLocal,
		LocalPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("NewLocalProviderWithConfig failed: %v", err)
	}

	if err := provider.Upload(ctx, "../escape.txt", []byte("x"), "text/plain"); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "..", "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected traversal target outside base path to remain absent, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "escape.txt")); err != nil {
		t.Fatalf("expected sanitized file inside base path, got err=%v", err)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal.txt", "normal.txt"},
		{"path/to/file.txt", "path_to_file.txt"},
		{"file:with:colons.txt", "file_with_colons.txt"},
		{"file\\with\\backslash.txt", "file_with_backslash.txt"},
		{"multiple/path/to/file.txt", "multiple_path_to_file.txt"},
	}

	for _, tt := range tests {
		result := sanitizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeFilename(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}
