package storage

import (
	"context"
	"log"
	"sync"
	"time"
)

// Reaper runs periodic storage GC for expiring artifact namespaces.
// It stays in internal/storage so hosts can reuse the same logic without
// duplicating timers or expiry policies across CLI, API, and SDK layers.
type Reaper struct {
	store  ArtifactStore
	config ReaperConfig

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func DefaultReaperConfig() ReaperConfig {
	return ReaperConfig{
		Interval: 1 * time.Hour,
		// Only namespaces with globally-shared, time-expiring content need GC.
		// Session-scoped content (screenshots, downloads, web artifacts) is cleaned
		// up by DeleteSessionDir when a session is deleted — no reaper needed.
		Namespaces: []ArtifactNamespace{},
		Limit:      512,
	}
}

func NewReaper(store ArtifactStore, config ReaperConfig) *Reaper {
	if store == nil {
		return nil
	}
	defaults := DefaultReaperConfig()
	if config.Interval <= 0 {
		config.Interval = defaults.Interval
	}
	if len(config.Namespaces) == 0 {
		config.Namespaces = defaults.Namespaces
	}
	if config.Limit <= 0 {
		config.Limit = defaults.Limit
	}
	return &Reaper{
		store:  store,
		config: config,
		done:   make(chan struct{}),
	}
}

func (r *Reaper) Start(parent context.Context) {
	if r == nil || r.store == nil || r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel

	go func() {
		defer close(r.done)
		ticker := time.NewTicker(r.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := r.RunOnce(context.Background()); err != nil {
					log.Printf("[storage] garbage collection failed: %v", err)
				}
			}
		}
	}()
}

func (r *Reaper) RunOnce(ctx context.Context) (GCReport, error) {
	if r == nil || r.store == nil {
		return GCReport{}, nil
	}
	return r.store.GarbageCollect(ctx, GCOptions{
		Namespaces: r.config.Namespaces,
		Limit:      r.config.Limit,
	})
}

func (r *Reaper) Stop() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		waitForDone := false
		if r.cancel != nil {
			r.cancel()
			waitForDone = true
		}
		if waitForDone && r.done != nil {
			<-r.done
		}
	})
}
