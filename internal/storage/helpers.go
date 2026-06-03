package storage

import (
	"context"
	"fmt"
	"io"
)

func StorePDF(ctx context.Context, data []byte, title string) (string, error) {
	ref, err := StorePDFRef(ctx, nil, data, title)
	if err != nil {
		return "", err
	}
	return copyRefURL(ref)
}

func StoreScreenshot(ctx context.Context, data []byte, sessionID, pageID string) (string, error) {
	ref, err := StoreScreenshotRef(ctx, nil, data, sessionID, pageID)
	if err != nil {
		return "", err
	}
	return copyRefURL(ref)
}

func StoreDocument(ctx context.Context, data []byte, filename string) (string, error) {
	ref, err := StoreDocumentRef(ctx, nil, data, filename)
	if err != nil {
		return "", err
	}
	return copyRefURL(ref)
}

func LoadDocument(ctx context.Context, key string) ([]byte, error) {
	provider, err := GetProvider()
	if err != nil {
		return nil, err
	}

	return provider.Download(ctx, key)
}

func DeleteDocument(ctx context.Context, key string) error {
	provider, err := GetProvider()
	if err != nil {
		return err
	}

	return provider.Delete(ctx, key)
}

type FileInfo struct {
	Key    string
	Size   int64
	URL    string
	Exists bool
}

func Exists(ctx context.Context, key string) (bool, error) {
	provider, err := GetProvider()
	if err != nil {
		return false, err
	}
	return provider.Exists(ctx, key)
}

func GetFileURL(ctx context.Context, key string) (string, error) {
	provider, err := GetProvider()
	if err != nil {
		return "", err
	}
	return provider.GetURL(ctx, key)
}

func StatFile(ctx context.Context, key string) (ArtifactRef, error) {
	store, err := DefaultArtifactStore()
	if err != nil {
		return ArtifactRef{}, err
	}
	return store.Stat(ctx, key)
}

func ListFiles(ctx context.Context, prefix string, limit int) ([]ArtifactRef, error) {
	store, err := DefaultArtifactStore()
	if err != nil {
		return nil, err
	}
	return store.List(ctx, ListOptions{Prefix: prefix, Limit: limit})
}

func OpenFileReader(ctx context.Context, key string) (io.ReadCloser, ArtifactRef, error) {
	store, err := DefaultArtifactStore()
	if err != nil {
		return nil, ArtifactRef{}, err
	}
	return store.OpenReader(ctx, key)
}

func ReadFileMetadata(ctx context.Context, key string) (ArtifactMetadata, error) {
	store, err := DefaultArtifactStore()
	if err != nil {
		return ArtifactMetadata{}, err
	}
	return store.Metadata(ctx, key)
}

func RunGarbageCollection(ctx context.Context, options GCOptions) (GCReport, error) {
	store, err := DefaultArtifactStore()
	if err != nil {
		return GCReport{}, err
	}
	return store.GarbageCollect(ctx, options)
}

func CopyFile(ctx context.Context, srcKey, dstKey string) error {
	provider, err := GetProvider()
	if err != nil {
		return err
	}

	data, err := provider.Download(ctx, srcKey)
	if err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return provider.Upload(ctx, dstKey, data, "")
}

func MoveFile(ctx context.Context, srcKey, dstKey string) error {
	if err := CopyFile(ctx, srcKey, dstKey); err != nil {
		return err
	}
	return DeleteDocument(ctx, srcKey)
}

func UploadFile(ctx context.Context, key string, data []byte, contentType string) error {
	provider, err := GetProvider()
	if err != nil {
		return err
	}
	return provider.Upload(ctx, key, data, contentType)
}

func DownloadFile(ctx context.Context, key string) ([]byte, error) {
	provider, err := GetProvider()
	if err != nil {
		return nil, err
	}
	return provider.Download(ctx, key)
}

func DeleteFile(ctx context.Context, key string) error {
	provider, err := GetProvider()
	if err != nil {
		return err
	}
	return provider.Delete(ctx, key)
}
