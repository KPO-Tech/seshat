package common

import (
	"sync"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
)

var (
	markdownCacheMu sync.Mutex
	markdownCache   = map[int]*glamour.TermRenderer{}
	rendererLocksMu sync.Mutex
	rendererLocks   = map[*glamour.TermRenderer]*sync.Mutex{}
)

// MarkdownRenderer returns a memoized glamour renderer for the given width.
// The renderer uses a custom dark style without visible ATX heading prefixes.
func MarkdownRenderer(width int) *glamour.TermRenderer {
	markdownCacheMu.Lock()
	defer markdownCacheMu.Unlock()
	wrappedWidth := ClampInt(width-4, 20, width)
	if r, ok := markdownCache[wrappedWidth]; ok {
		return r
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyleConfig()),
		glamour.WithWordWrap(wrappedWidth),
	)
	if err != nil {
		return nil
	}
	markdownCache[wrappedWidth] = r
	return r
}

// LockMarkdownRenderer returns the mutex guarding a shared glamour renderer.
func LockMarkdownRenderer(r *glamour.TermRenderer) *sync.Mutex {
	rendererLocksMu.Lock()
	defer rendererLocksMu.Unlock()
	if mu, ok := rendererLocks[r]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	rendererLocks[r] = mu
	return mu
}

// InvalidateMarkdownRendererCache drops all cached renderers and locks.
func InvalidateMarkdownRendererCache() {
	markdownCacheMu.Lock()
	defer markdownCacheMu.Unlock()
	rendererLocksMu.Lock()
	defer rendererLocksMu.Unlock()
	markdownCache = map[int]*glamour.TermRenderer{}
	rendererLocks = map[*glamour.TermRenderer]*sync.Mutex{}
}

func markdownStyleConfig() glamouransi.StyleConfig {
	style := glamourstyles.DarkStyleConfig
	empty := ""
	style.H2.Prefix = empty
	style.H3.Prefix = empty
	style.H4.Prefix = empty
	style.H5.Prefix = empty
	style.H6.Prefix = empty
	return style
}
