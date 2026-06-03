package webfetch

import (
	"context"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
	fetchcore "github.com/EngineerProjects/nexus-engine/internal/web/fetch"
	"strings"
	"testing"
	"time"
)

func TestDefinitionExposesSnakeCaseAlias(t *testing.T) {
	def := NewTool(nil).Definition()
	found := false
	for _, alias := range def.Aliases {
		if alias == "web_fetch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected web_fetch alias, got %v", def.Aliases)
	}
}

type fakeBrowserManager struct {
	openPageFn     func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error)
	snapshotFn     func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error)
	closeSessionFn func(ctx context.Context, sessionID types.SessionID) error
}

func (f fakeBrowserManager) EnsureSession(ctx context.Context, sessionID types.SessionID) (browsercore.SessionState, error) {
	return browsercore.SessionState{SessionID: sessionID}, nil
}

func (f fakeBrowserManager) OpenPage(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
	return f.openPageFn(ctx, sessionID, url)
}

func (f fakeBrowserManager) Navigate(ctx context.Context, sessionID types.SessionID, pageID string, url string) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) Snapshot(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
	return f.snapshotFn(ctx, sessionID, pageID, options)
}

func (f fakeBrowserManager) Click(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) Type(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) Press(ctx context.Context, sessionID types.SessionID, pageID string, key string) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) Scroll(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScrollOptions) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) Wait(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) Screenshot(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScreenshotOptions) (browsercore.Screenshot, error) {
	return browsercore.Screenshot{}, nil
}

func (f fakeBrowserManager) ListPages(ctx context.Context, sessionID types.SessionID) ([]browsercore.PageInfo, error) {
	return nil, nil
}

func (f fakeBrowserManager) ListNetwork(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.NetworkEntry, error) {
	return nil, nil
}

func (f fakeBrowserManager) ListDownloads(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.DownloadEntry, error) {
	return nil, nil
}

func (f fakeBrowserManager) SearchSnapshots(ctx context.Context, sessionID types.SessionID, query string, limit int) ([]browsercore.SnapshotSearchHit, error) {
	return nil, nil
}

func (f fakeBrowserManager) GetNetworkPolicy(ctx context.Context, sessionID types.SessionID) (browsercore.NetworkPolicy, error) {
	return browsercore.NetworkPolicy{}, nil
}

func (f fakeBrowserManager) SetNetworkPolicy(ctx context.Context, sessionID types.SessionID, policy browsercore.NetworkPolicy) (browsercore.NetworkPolicy, error) {
	return policy, nil
}

func (f fakeBrowserManager) SelectPage(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.PageInfo, error) {
	return browsercore.PageInfo{}, nil
}

func (f fakeBrowserManager) ClosePage(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.SessionState, error) {
	return browsercore.SessionState{}, nil
}

func (f fakeBrowserManager) CloseSession(ctx context.Context, sessionID types.SessionID) error {
	if f.closeSessionFn != nil {
		return f.closeSessionFn(ctx, sessionID)
	}
	return nil
}

func (f fakeBrowserManager) Close() error {
	return nil
}

func TestParseInputNormalizesRenderMode(t *testing.T) {
	parsed, err := parseInput(map[string]any{
		"url":         "https://example.com",
		"prompt":      "extract summary",
		"render_mode": "BROWSER",
	})
	if err != nil {
		t.Fatalf("parseInput returned unexpected error: %v", err)
	}
	if parsed.RenderMode != fetchcore.RenderModeBrowser {
		t.Fatalf("expected render mode %q, got %q", fetchcore.RenderModeBrowser, parsed.RenderMode)
	}
}

func TestCallBrowserModeUsesBrowserManager(t *testing.T) {
	var openedSession types.SessionID
	var closedSession types.SessionID

	fakeManager := fakeBrowserManager{
		openPageFn: func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
			openedSession = sessionID
			return browsercore.PageInfo{
				ID:        "page-1",
				URL:       url,
				Active:    true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
		snapshotFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
			if options.MaxText != fetchcore.BrowserSnapshotTextLimit {
				t.Fatalf("unexpected browser snapshot text limit: %d", options.MaxText)
			}
			return browsercore.Snapshot{
				Page: browsercore.PageInfo{
					ID:    pageID,
					URL:   "https://example.com/rendered",
					Title: "Rendered Example",
				},
				Text: "Rendered browser content",
			}, nil
		},
		closeSessionFn: func(ctx context.Context, sessionID types.SessionID) error {
			closedSession = sessionID
			return nil
		},
	}

	fetchTool := NewTool(&Config{
		BrowserManager:    fakeManager,
		Cache:             DefaultCache(),
		RenderPoolEnabled: false,
	})

	result, err := fetchTool.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"url":         "https://example.com",
			"prompt":      "extract summary",
			"render_mode": "browser",
		},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got error: %v", result.Error)
	}
	if openedSession == "" {
		t.Fatal("expected a browser session to be opened")
	}
	if openedSession != closedSession {
		t.Fatalf("expected browser session cleanup, opened=%q closed=%q", openedSession, closedSession)
	}

	output, ok := result.Data.(Output)
	if !ok {
		t.Fatalf("expected Output result data, got %T", result.Data)
	}
	if output.Mode != fetchcore.RenderModeBrowser {
		t.Fatalf("expected output mode %q, got %q", fetchcore.RenderModeBrowser, output.Mode)
	}
	if output.URL != "https://example.com/rendered" {
		t.Fatalf("expected rendered final URL, got %q", output.URL)
	}
	if !strings.Contains(result.Content, "Rendered browser content") {
		t.Fatalf("unexpected result content: %s", result.Content)
	}
}
