package storage

import (
	"context"
	"fmt"
	"io"
	"time"
)

type providerArtifactStore struct {
	provider StorageProvider
}

// NewArtifactStore adapts an existing provider into the narrower artifact interface.
func NewArtifactStore(provider StorageProvider) ArtifactStore {
	if provider == nil {
		return nil
	}
	return &providerArtifactStore{provider: provider}
}

// DefaultArtifactStore returns the process-wide artifact store backed by the configured provider.
func DefaultArtifactStore() (ArtifactStore, error) {
	provider, err := GetProvider()
	if err != nil {
		return nil, err
	}
	return NewArtifactStore(provider), nil
}

// NewArtifactStoreFromConfig creates an artifact store from an explicit Config,
// bypassing the process-wide provider singleton. Use when different SDK clients
// need independent storage backends.
func NewArtifactStoreFromConfig(cfg Config) (ArtifactStore, error) {
	provider, err := NewProviderFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewArtifactStore(provider), nil
}

func (s *providerArtifactStore) Put(ctx context.Context, key string, body []byte, contentType string) (ArtifactRef, error) {
	if err := s.provider.Upload(ctx, key, body, contentType); err != nil {
		return ArtifactRef{}, err
	}
	metadata := inferMetadataForDirectPut(key, contentType, body)
	if err := s.persistMetadata(ctx, metadata); err != nil {
		return ArtifactRef{}, err
	}
	return s.refFromMetadata(ctx, metadata), nil
}

func (s *providerArtifactStore) PutArtifact(ctx context.Context, request ArtifactPutRequest, body []byte) (ArtifactRef, error) {
	key := BuildArtifactKey(request)
	if err := s.provider.Upload(ctx, key, body, request.ContentType); err != nil {
		return ArtifactRef{}, err
	}
	metadata := buildMetadata(request, key, body)
	if err := s.persistMetadata(ctx, metadata); err != nil {
		return ArtifactRef{}, err
	}
	return s.refFromMetadata(ctx, metadata), nil
}

func (s *providerArtifactStore) OpenReader(ctx context.Context, key string) (io.ReadCloser, ArtifactRef, error) {
	reader, info, err := s.provider.OpenReader(ctx, key)
	if err != nil {
		return nil, ArtifactRef{}, err
	}
	metadata, metadataErr := s.readMetadata(ctx, key)
	if metadataErr == nil {
		return reader, s.refFromMetadata(ctx, metadata), nil
	}
	return reader, artifactRefFromInfo(info), nil
}

func (s *providerArtifactStore) Get(ctx context.Context, key string) ([]byte, error) {
	return s.provider.Download(ctx, key)
}

func (s *providerArtifactStore) Stat(ctx context.Context, key string) (ArtifactRef, error) {
	metadata, metadataErr := s.readMetadata(ctx, key)
	if metadataErr == nil {
		return s.refFromMetadata(ctx, metadata), nil
	}
	info, err := s.provider.Stat(ctx, key)
	if err != nil {
		return ArtifactRef{}, err
	}
	return artifactRefFromInfo(info), nil
}

func (s *providerArtifactStore) List(ctx context.Context, options ListOptions) ([]ArtifactRef, error) {
	metadata, err := s.ListMetadata(ctx, options)
	if err != nil {
		return nil, err
	}
	refs := make([]ArtifactRef, 0, len(metadata))
	for _, item := range metadata {
		refs = append(refs, s.refFromMetadata(ctx, item))
	}
	return refs, nil
}

func (s *providerArtifactStore) Metadata(ctx context.Context, key string) (ArtifactMetadata, error) {
	return s.readMetadata(ctx, key)
}

func (s *providerArtifactStore) ListMetadata(ctx context.Context, options ListOptions) ([]ArtifactMetadata, error) {
	objects, err := s.provider.List(ctx, ListOptions{
		Prefix: metadataListPrefix(options.Prefix),
		Limit:  options.Limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]ArtifactMetadata, 0, len(objects))
	for _, object := range objects {
		reader, _, err := s.provider.OpenReader(ctx, object.Key)
		if err != nil {
			return nil, err
		}
		metadata, decodeErr := readMetadataFromReader(reader)
		_ = reader.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		items = append(items, metadata)
	}
	return items, nil
}

func (s *providerArtifactStore) GarbageCollect(ctx context.Context, options GCOptions) (GCReport, error) {
	return garbageCollect(ctx, s, options)
}

func (s *providerArtifactStore) Delete(ctx context.Context, key string) error {
	if err := s.provider.Delete(ctx, key); err != nil {
		return err
	}
	if err := s.provider.Delete(ctx, metadataKeyForArtifact(key)); err != nil {
		return err
	}
	return nil
}

func (s *providerArtifactStore) Exists(ctx context.Context, key string) (bool, error) {
	return s.provider.Exists(ctx, key)
}

func (s *providerArtifactStore) URL(ctx context.Context, key string) (string, error) {
	return s.provider.GetURL(ctx, key)
}

func StorePDFRef(ctx context.Context, store ArtifactStore, data []byte, title string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	return store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:   NamespaceDocuments,
		Filename:    title,
		ContentType: "application/pdf",
		Timestamp:   time.Now().UTC(),
	}, data)
}

func StoreScreenshotRef(ctx context.Context, store ArtifactStore, data []byte, sessionID, pageID string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	key := ScreenshotKey(sessionID, pageID, time.Now().UTC())
	return store.Put(ctx, key, data, "image/png")
}

func StoreDocumentRef(ctx context.Context, store ArtifactStore, data []byte, filename string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	return store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:   NamespaceDocuments,
		Filename:    filename,
		ContentType: DetectContentType(filename),
		Timestamp:   time.Now().UTC(),
	}, data)
}

func StoreRAGDocumentRef(ctx context.Context, store ArtifactStore, data []byte, filename string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	return store.PutArtifact(ctx, ArtifactPutRequest{
		Namespace:   NamespaceRAGDocuments,
		Filename:    filename,
		ContentType: DetectContentType(filename),
		Timestamp:   time.Now().UTC(),
	}, data)
}

// StoreWebArtifactRef persists web-fetched content under the session's artifacts/web/ dir.
func StoreWebArtifactRef(ctx context.Context, store ArtifactStore, data []byte, sessionID, filename, contentType string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	if contentType == "" {
		contentType = DetectContentType(filename)
	}
	key := WebArtifactKey(sessionID, filename, time.Now().UTC())
	return store.Put(ctx, key, data, contentType)
}

// StoreGeneratedImageRef persists an AI-generated image under the session's artifacts/images/ dir.
func StoreGeneratedImageRef(ctx context.Context, store ArtifactStore, data []byte, sessionID, filename, contentType string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	if contentType == "" {
		contentType = DetectContentType(filename)
	}
	key := GeneratedImageKey(sessionID, filename, time.Now().UTC())
	return store.Put(ctx, key, data, contentType)
}

// StoreAudioRef persists a TTS/STT audio file under the session's artifacts/audio/ dir.
func StoreAudioRef(ctx context.Context, store ArtifactStore, data []byte, sessionID, filename, contentType string) (ArtifactRef, error) {
	if store == nil {
		var err error
		store, err = DefaultArtifactStore()
		if err != nil {
			return ArtifactRef{}, err
		}
	}
	if contentType == "" {
		contentType = DetectContentType(filename)
	}
	key := AudioKey(sessionID, filename, time.Now().UTC())
	return store.Put(ctx, key, data, contentType)
}

func copyRefURL(ref ArtifactRef) (string, error) {
	if ref.URL == "" {
		return "", fmt.Errorf("artifact URL is empty")
	}
	return ref.URL, nil
}

func artifactRefFromInfo(info ObjectInfo) ArtifactRef {
	return ArtifactRef{
		Key:         info.Key,
		URL:         info.URL,
		ContentType: info.ContentType,
		Size:        info.Size,
		ModifiedAt:  info.ModifiedAt,
	}
}

func artifactRefFromMetadata(metadata ArtifactMetadata) ArtifactRef {
	return ArtifactRef{
		Key:            metadata.Key,
		URL:            "",
		ContentType:    metadata.ContentType,
		Size:           metadata.Size,
		ModifiedAt:     metadata.ModifiedAt,
		ChecksumSHA256: metadata.ChecksumSHA256,
		ExpiresAt:      metadata.ExpiresAt,
		Namespace:      metadata.Namespace,
	}
}

func (s *providerArtifactStore) refFromMetadata(ctx context.Context, metadata ArtifactMetadata) ArtifactRef {
	ref := artifactRefFromMetadata(metadata)
	if s == nil || s.provider == nil {
		return ref
	}
	url, err := s.provider.GetURL(ctx, metadata.Key)
	if err == nil {
		ref.URL = url
	}
	return ref
}
