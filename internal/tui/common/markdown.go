package common

import (
	"fmt"
	"strings"
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

// RenderMarkdown safely renders markdown using the shared cached renderer for
// the requested width. Third-party renderer panics are converted into errors so
// the TUI can fall back to plain text instead of crashing.
func RenderMarkdown(width int, body string) (rendered string, err error) {
	renderer := MarkdownRenderer(width)
	if renderer == nil {
		return "", fmt.Errorf("markdown renderer unavailable")
	}
	mu := LockMarkdownRenderer(renderer)
	mu.Lock()
	defer mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			InvalidateMarkdownRendererCache()
			rendered = ""
			err = fmt.Errorf("markdown render panic: %v", r)
		}
	}()
	rendered, err = renderer.Render(strings.ReplaceAll(body, "\r\n", "\n"))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(rendered, "\n"), nil
}

func markdownStyleConfig() glamouransi.StyleConfig {
	style := glamourstyles.DarkStyleConfig
	empty := ""
	softOrange := "249"
	softAmber := "215"
	nilBg := (*string)(nil)
	bold := true

	style.Heading.Color = &softOrange
	style.Heading.Bold = &bold
	style.Heading.BackgroundColor = nilBg

	style.H1.Color = &softOrange
	style.H1.Bold = &bold
	style.H1.BackgroundColor = nilBg
	style.H1.Prefix = empty

	style.H2.Color = &softOrange
	style.H2.Bold = &bold
	style.H2.BackgroundColor = nilBg
	style.H2.Prefix = empty

	style.H3.Color = &softAmber
	style.H3.Bold = &bold
	style.H3.BackgroundColor = nilBg
	style.H3.Prefix = empty

	style.H4.Color = &softAmber
	style.H4.Bold = &bold
	style.H4.BackgroundColor = nilBg
	style.H4.Prefix = empty

	style.H5.Color = &softAmber
	style.H5.Bold = &bold
	style.H5.BackgroundColor = nilBg
	style.H5.Prefix = empty

	style.H6.Color = &softAmber
	style.H6.Bold = &bold
	style.H6.BackgroundColor = nilBg
	style.H6.Prefix = empty
	return style
}
