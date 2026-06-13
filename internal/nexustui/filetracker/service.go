// Package filetracker provides a no-op file read tracking service.
package filetracker

import (
	"context"
	"time"
)

// Service defines the interface for tracking file reads in sessions.
type Service interface {
	RecordRead(ctx context.Context, sessionID, path string)
	LastReadTime(ctx context.Context, sessionID, path string) time.Time
	ListReadFiles(ctx context.Context, sessionID string) ([]string, error)
}

type noopService struct{}

// NewNoopService returns a Service that does nothing.
func NewNoopService() Service { return &noopService{} }

func (*noopService) RecordRead(_ context.Context, _, _ string)             {}
func (*noopService) LastReadTime(_ context.Context, _, _ string) time.Time { return time.Time{} }
func (*noopService) ListReadFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
