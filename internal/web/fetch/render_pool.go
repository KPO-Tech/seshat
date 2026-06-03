package fetch

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type renderSessionPool struct {
	mu         sync.Mutex
	entries    map[string]renderSessionEntry
	ttl        time.Duration
	maxEntries int
	manager    browserManager
}

type browserManager interface {
	CloseSession(ctx context.Context, sessionID types.SessionID) error
}

type renderSessionEntry struct {
	sessionID   types.SessionID
	lastTouched time.Time
}

func newRenderSessionPool(ttl time.Duration, maxEntries int, manager browserManager) *renderSessionPool {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if maxEntries <= 0 {
		maxEntries = 32
	}
	return &renderSessionPool{
		entries:    make(map[string]renderSessionEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		manager:    manager,
	}
}

func (p *renderSessionPool) sessionFor(rawURL string, request Request) types.SessionID {
	if p == nil {
		return tempBrowserSessionID(request)
	}
	now := time.Now().UTC()
	key := renderPoolKey(rawURL)
	p.mu.Lock()
	defer p.mu.Unlock()

	p.evictExpiredLocked(now)
	if existing, ok := p.entries[key]; ok {
		existing.lastTouched = now
		p.entries[key] = existing
		return existing.sessionID
	}

	if len(p.entries) >= p.maxEntries {
		p.evictOldestLocked()
	}
	sessionID := types.SessionID(fmt.Sprintf("%s:renderpool:%s", baseRenderSessionID(request), sanitizeRenderPoolKey(key)))
	p.entries[key] = renderSessionEntry{
		sessionID:   sessionID,
		lastTouched: now,
	}
	return sessionID
}

func (p *renderSessionPool) evictExpiredLocked(now time.Time) {
	if p == nil {
		return
	}
	for key, entry := range p.entries {
		if now.Sub(entry.lastTouched) > p.ttl {
			delete(p.entries, key)
			if p.manager != nil {
				go func(sessionID types.SessionID) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = p.manager.CloseSession(ctx, sessionID)
				}(entry.sessionID)
			}
		}
	}
}

func (p *renderSessionPool) evictOldestLocked() {
	var (
		oldestKey string
		oldest    time.Time
	)
	for key, entry := range p.entries {
		if oldestKey == "" || entry.lastTouched.Before(oldest) {
			oldestKey = key
			oldest = entry.lastTouched
		}
	}
	if oldestKey == "" {
		return
	}
	entry := p.entries[oldestKey]
	delete(p.entries, oldestKey)
	if p.manager != nil {
		go func(sessionID types.SessionID) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.manager.CloseSession(ctx, sessionID)
		}(entry.sessionID)
	}
}

func renderPoolKey(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(rawURL))
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func baseRenderSessionID(request Request) string {
	base := string(request.SessionID)
	if base == "" {
		base = "webfetch"
	}
	return base
}

func sanitizeRenderPoolKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", ".", "_")
	if value == "" {
		return "default"
	}
	return replacer.Replace(value)
}
