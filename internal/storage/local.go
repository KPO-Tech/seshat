package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LocalProvider struct {
	basePath string
}

func NewLocalProvider() (*LocalProvider, error) {
	return NewLocalProviderWithConfig(GetConfigFromEnv())
}

func NewLocalProviderWithConfig(cfg Config) (*LocalProvider, error) {
	basePath := cfg.LocalPath
	if basePath == "" {
		basePath = DefaultLocalPath()
	}

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalProvider{basePath: basePath}, nil
}

func (p *LocalProvider) fullPath(key string) string {
	cleanKey := filepath.Clean("/" + strings.TrimSpace(key))
	cleanKey = strings.TrimPrefix(cleanKey, string(filepath.Separator))
	return filepath.Join(p.basePath, cleanKey)
}

func (p *LocalProvider) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	_ = ctx
	_ = contentType
	fullPath := p.fullPath(key)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(fullPath), ".upload-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpName, fullPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to finalize file write: %w", err)
	}

	return nil
}

func (p *LocalProvider) Download(ctx context.Context, key string) ([]byte, error) {
	reader, _, err := p.OpenReader(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return data, nil
}

func (p *LocalProvider) Delete(ctx context.Context, key string) error {
	fullPath := p.fullPath(key)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

func (p *LocalProvider) GetURL(ctx context.Context, key string) (string, error) {
	_ = ctx
	return p.fullPath(key), nil
}

func (p *LocalProvider) Exists(ctx context.Context, key string) (bool, error) {
	fullPath := p.fullPath(key)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *LocalProvider) OpenReader(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	_ = ctx
	fullPath := p.fullPath(key)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ObjectInfo{}, fmt.Errorf("file not found: %s", key)
		}
		return nil, ObjectInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, ObjectInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}
	return file, p.objectInfoFor(key, info), nil
}

func (p *LocalProvider) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	_ = ctx
	fullPath := p.fullPath(key)
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectInfo{}, fmt.Errorf("file not found: %s", key)
		}
		return ObjectInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}
	return p.objectInfoFor(key, info), nil
}

func (p *LocalProvider) List(ctx context.Context, options ListOptions) ([]ObjectInfo, error) {
	_ = ctx
	root := p.fullPath(options.Prefix)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat list root: %w", err)
	}

	results := make([]ObjectInfo, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		key, err := filepath.Rel(p.basePath, path)
		if err != nil {
			return err
		}
		results = append(results, p.objectInfoFor(filepath.ToSlash(key), info))
		if options.Limit > 0 && len(results) >= options.Limit {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Key < results[j].Key
	})
	return results, nil
}

func (p *LocalProvider) objectInfoFor(key string, info os.FileInfo) ObjectInfo {
	url, _ := p.GetURL(context.Background(), key)
	return ObjectInfo{
		Key:         key,
		URL:         url,
		ContentType: DetectContentType(key),
		Size:        info.Size(),
		ModifiedAt:  info.ModTime().UTC(),
	}
}

var _ StorageProvider = (*LocalProvider)(nil)
