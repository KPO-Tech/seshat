package storage

import (
	"context"
	"fmt"
	"time"
)

func garbageCollect(ctx context.Context, store ArtifactStore, options GCOptions) (GCReport, error) {
	if store == nil {
		return GCReport{}, fmt.Errorf("artifact store is nil")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	metadata, err := store.ListMetadata(ctx, ListOptions{Limit: options.Limit})
	if err != nil {
		return GCReport{}, err
	}

	report := GCReport{
		Errors:      make([]string, 0),
		DeletedKeys: make([]string, 0),
	}
	for _, item := range metadata {
		report.Scanned++
		if !gcNamespaceAllowed(item.Namespace, options.Namespaces) || !gcExpired(item, now) {
			report.Kept++
			continue
		}
		if options.DryRun {
			report.Deleted++
			report.DeletedKeys = append(report.DeletedKeys, item.Key)
			continue
		}
		if err := store.Delete(ctx, item.Key); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", item.Key, err))
			continue
		}
		report.Deleted++
		report.DeletedKeys = append(report.DeletedKeys, item.Key)
	}
	return report, nil
}

func gcNamespaceAllowed(namespace string, filters []ArtifactNamespace) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filter == ArtifactNamespace(namespace) {
			return true
		}
	}
	return false
}

func gcExpired(metadata ArtifactMetadata, now time.Time) bool {
	if metadata.ExpiresAt.IsZero() {
		return false
	}
	return !metadata.ExpiresAt.After(now)
}
