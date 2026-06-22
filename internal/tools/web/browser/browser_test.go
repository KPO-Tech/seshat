package browser

import (
	"context"
	"strings"
	"testing"
	"time"

	tool "github.com/EngineerProjects/seshat/internal/tools/registry"
	"github.com/EngineerProjects/seshat/internal/types"
	browsercore "github.com/EngineerProjects/seshat/internal/web/browser"
)

type fakeManager struct {
	openPageFn         func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error)
	snapshotFn         func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error)
	listPagesFn        func(ctx context.Context, sessionID types.SessionID) ([]browsercore.PageInfo, error)
	listNetworkFn      func(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.NetworkEntry, error)
	listDownloadsFn    func(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.DownloadEntry, error)
	searchSnapshotsFn  func(ctx context.Context, sessionID types.SessionID, query string, limit int) ([]browsercore.SnapshotSearchHit, error)
	getNetworkPolicyFn func(ctx context.Context, sessionID types.SessionID) (browsercore.NetworkPolicy, error)
	setNetworkPolicyFn func(ctx context.Context, sessionID types.SessionID, policy browsercore.NetworkPolicy) (browsercore.NetworkPolicy, error)
	closePageFn        func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.SessionState, error)
	selectPageFn       func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.PageInfo, error)
	navigateFn         func(ctx context.Context, sessionID types.SessionID, pageID string, url string) (browsercore.PageInfo, error)
	clickFn            func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error)
	typeFn             func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error)
	pressFn            func(ctx context.Context, sessionID types.SessionID, pageID string, key string) (browsercore.PageInfo, error)
	scrollFn           func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScrollOptions) (browsercore.PageInfo, error)
	waitFn             func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error)
	screenshotFn       func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScreenshotOptions) (browsercore.Screenshot, error)
}

func (f fakeManager) EnsureSession(ctx context.Context, sessionID types.SessionID) (browsercore.SessionState, error) {
	return browsercore.SessionState{SessionID: sessionID}, nil
}

func (f fakeManager) OpenPage(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
	if f.openPageFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.openPageFn(ctx, sessionID, url)
}

func (f fakeManager) Navigate(ctx context.Context, sessionID types.SessionID, pageID string, url string) (browsercore.PageInfo, error) {
	if f.navigateFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.navigateFn(ctx, sessionID, pageID, url)
}

func (f fakeManager) Click(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error) {
	if f.clickFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.clickFn(ctx, sessionID, pageID, elementID, revision)
}

func (f fakeManager) Type(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error) {
	if f.typeFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.typeFn(ctx, sessionID, pageID, elementID, revision, text, clear)
}

func (f fakeManager) Press(ctx context.Context, sessionID types.SessionID, pageID string, key string) (browsercore.PageInfo, error) {
	if f.pressFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.pressFn(ctx, sessionID, pageID, key)
}

func (f fakeManager) Scroll(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScrollOptions) (browsercore.PageInfo, error) {
	if f.scrollFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.scrollFn(ctx, sessionID, pageID, options)
}

func (f fakeManager) Wait(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error) {
	if f.waitFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.waitFn(ctx, sessionID, pageID, options)
}

func (f fakeManager) Snapshot(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
	if f.snapshotFn == nil {
		return browsercore.Snapshot{}, nil
	}
	return f.snapshotFn(ctx, sessionID, pageID, options)
}

func (f fakeManager) Screenshot(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScreenshotOptions) (browsercore.Screenshot, error) {
	if f.screenshotFn == nil {
		return browsercore.Screenshot{}, nil
	}
	return f.screenshotFn(ctx, sessionID, pageID, options)
}

func (f fakeManager) ListPages(ctx context.Context, sessionID types.SessionID) ([]browsercore.PageInfo, error) {
	if f.listPagesFn == nil {
		return nil, nil
	}
	return f.listPagesFn(ctx, sessionID)
}

func (f fakeManager) ListNetwork(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.NetworkEntry, error) {
	if f.listNetworkFn == nil {
		return nil, nil
	}
	return f.listNetworkFn(ctx, sessionID, pageID, limit)
}

func (f fakeManager) ListDownloads(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.DownloadEntry, error) {
	if f.listDownloadsFn == nil {
		return nil, nil
	}
	return f.listDownloadsFn(ctx, sessionID, pageID, limit)
}

func (f fakeManager) SearchSnapshots(ctx context.Context, sessionID types.SessionID, query string, limit int) ([]browsercore.SnapshotSearchHit, error) {
	if f.searchSnapshotsFn == nil {
		return nil, nil
	}
	return f.searchSnapshotsFn(ctx, sessionID, query, limit)
}

func (f fakeManager) GetNetworkPolicy(ctx context.Context, sessionID types.SessionID) (browsercore.NetworkPolicy, error) {
	if f.getNetworkPolicyFn == nil {
		return browsercore.NetworkPolicy{}, nil
	}
	return f.getNetworkPolicyFn(ctx, sessionID)
}

func (f fakeManager) SetNetworkPolicy(ctx context.Context, sessionID types.SessionID, policy browsercore.NetworkPolicy) (browsercore.NetworkPolicy, error) {
	if f.setNetworkPolicyFn == nil {
		return browsercore.NetworkPolicy{}, nil
	}
	return f.setNetworkPolicyFn(ctx, sessionID, policy)
}

func (f fakeManager) SelectPage(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.PageInfo, error) {
	if f.selectPageFn == nil {
		return browsercore.PageInfo{}, nil
	}
	return f.selectPageFn(ctx, sessionID, pageID)
}

func (f fakeManager) ClosePage(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.SessionState, error) {
	if f.closePageFn == nil {
		return browsercore.SessionState{}, nil
	}
	return f.closePageFn(ctx, sessionID, pageID)
}

func (f fakeManager) CloseSession(ctx context.Context, sessionID types.SessionID) error {
	return nil
}

func (f fakeManager) Close() error {
	return nil
}

func TestOpenToolUsesSessionContext(t *testing.T) {
	fake := fakeManager{
		openPageFn: func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			if url != "https://example.com" {
				t.Fatalf("unexpected url: %s", url)
			}
			return browsercore.PageInfo{ID: "page-1", URL: url, Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}

	res, err := NewOpenTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"url": "https://example.com"},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if res.Content == "" {
		t.Fatal("expected formatted content")
	}
}

func TestSnapshotToolFormatsElements(t *testing.T) {
	fake := fakeManager{
		clickFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		typeFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		waitFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		snapshotFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
			return browsercore.Snapshot{
				Page: browsercore.PageInfo{ID: "page-1", URL: "https://example.com", Title: "Example"},
				Text: "hello world",
				Elements: []browsercore.ElementInfo{
					{ID: "e1", Role: "button", Name: "Submit"},
				},
			}, nil
		},
	}

	res, err := NewSnapshotTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; got == "" || !containsAll(got, "Example", "hello world", "e1 [button] Submit") {
		t.Fatalf("unexpected snapshot content: %s", got)
	}
}

func TestClickToolUsesElementID(t *testing.T) {
	fake := fakeManager{
		openPageFn: func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		navigateFn: func(ctx context.Context, sessionID types.SessionID, pageID string, url string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		snapshotFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
			return browsercore.Snapshot{}, nil
		},
		listPagesFn: func(ctx context.Context, sessionID types.SessionID) ([]browsercore.PageInfo, error) {
			return nil, nil
		},
		closePageFn: func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.SessionState, error) {
			return browsercore.SessionState{}, nil
		},
		selectPageFn: func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		clickFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			if pageID != "page-1" {
				t.Fatalf("unexpected page id: %s", pageID)
			}
			if elementID != "e3" {
				t.Fatalf("unexpected element id: %s", elementID)
			}
			if revision != "rev-1" {
				t.Fatalf("unexpected revision: %s", revision)
			}
			return browsercore.PageInfo{ID: "page-1", URL: "https://example.com/next", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
		typeFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		waitFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
	}

	res, err := NewClickTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"page_id": "page-1", "element_id": "e3", "revision": "rev-1"},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !strings.Contains(got, "Clicked e3") {
		t.Fatalf("unexpected click content: %s", got)
	}
}

func TestTypeToolPassesTextAndClear(t *testing.T) {
	fake := fakeManager{
		openPageFn: func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		navigateFn: func(ctx context.Context, sessionID types.SessionID, pageID string, url string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		snapshotFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
			return browsercore.Snapshot{}, nil
		},
		listPagesFn: func(ctx context.Context, sessionID types.SessionID) ([]browsercore.PageInfo, error) {
			return nil, nil
		},
		closePageFn: func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.SessionState, error) {
			return browsercore.SessionState{}, nil
		},
		selectPageFn: func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		clickFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		typeFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error) {
			if elementID != "e2" {
				t.Fatalf("unexpected element id: %s", elementID)
			}
			if revision != "rev-2" {
				t.Fatalf("unexpected revision: %s", revision)
			}
			if text != "hello seshat" {
				t.Fatalf("unexpected text: %s", text)
			}
			if !clear {
				t.Fatal("expected clear=true")
			}
			return browsercore.PageInfo{ID: "page-1", URL: "https://example.com/form", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
		waitFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
	}

	res, err := NewTypeTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"page_id": "page-1", "element_id": "e2", "revision": "rev-2", "text": "hello seshat", "clear": true},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !strings.Contains(got, "Typed into e2") {
		t.Fatalf("unexpected type content: %s", got)
	}
}

func TestWaitToolPassesOptions(t *testing.T) {
	fake := fakeManager{
		openPageFn: func(ctx context.Context, sessionID types.SessionID, url string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		navigateFn: func(ctx context.Context, sessionID types.SessionID, pageID string, url string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		snapshotFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.SnapshotOptions) (browsercore.Snapshot, error) {
			return browsercore.Snapshot{}, nil
		},
		listPagesFn: func(ctx context.Context, sessionID types.SessionID) ([]browsercore.PageInfo, error) {
			return nil, nil
		},
		closePageFn: func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.SessionState, error) {
			return browsercore.SessionState{}, nil
		},
		selectPageFn: func(ctx context.Context, sessionID types.SessionID, pageID string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		clickFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		typeFn: func(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (browsercore.PageInfo, error) {
			return browsercore.PageInfo{}, nil
		},
		waitFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.WaitOptions) (browsercore.PageInfo, error) {
			if options.Text != "Ready" {
				t.Fatalf("unexpected wait text: %s", options.Text)
			}
			if options.TimeoutMs != 2500 {
				t.Fatalf("unexpected timeout: %d", options.TimeoutMs)
			}
			return browsercore.PageInfo{ID: "page-1", URL: "https://example.com", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}

	res, err := NewWaitTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"text": "Ready", "timeout_ms": float64(2500)},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !strings.Contains(got, "is ready") {
		t.Fatalf("unexpected wait content: %s", got)
	}
}

func TestPressToolPassesKey(t *testing.T) {
	fake := fakeManager{
		pressFn: func(ctx context.Context, sessionID types.SessionID, pageID string, key string) (browsercore.PageInfo, error) {
			if key != "Enter" {
				t.Fatalf("unexpected key: %s", key)
			}
			return browsercore.PageInfo{ID: "page-1", URL: "https://example.com", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}

	res, err := NewPressTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"key": "Enter"},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !strings.Contains(got, "Pressed Enter") {
		t.Fatalf("unexpected press content: %s", got)
	}
}

func TestScrollToolPassesOptions(t *testing.T) {
	fake := fakeManager{
		scrollFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScrollOptions) (browsercore.PageInfo, error) {
			if options.Direction != "down" {
				t.Fatalf("unexpected direction: %s", options.Direction)
			}
			if options.Amount != 500 {
				t.Fatalf("unexpected amount: %d", options.Amount)
			}
			return browsercore.PageInfo{ID: "page-1", URL: "https://example.com", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}

	res, err := NewScrollTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"direction": "down", "amount": float64(500)},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !strings.Contains(got, "Scrolled browser page") {
		t.Fatalf("unexpected scroll content: %s", got)
	}
}

func TestScreenshotToolFormatsOutput(t *testing.T) {
	fake := fakeManager{
		screenshotFn: func(ctx context.Context, sessionID types.SessionID, pageID string, options browsercore.ScreenshotOptions) (browsercore.Screenshot, error) {
			if !options.FullPage {
				t.Fatal("expected full page screenshot")
			}
			return browsercore.Screenshot{
				Page:       browsercore.PageInfo{ID: "page-1", URL: "https://example.com"},
				MimeType:   "image/png",
				DataBase64: "ZmFrZQ==",
				Bytes:      4,
				FullPage:   true,
			}, nil
		},
	}

	res, err := NewScreenshotTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"full_page": true},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !containsAll(got, "image/png", "full page") {
		t.Fatalf("unexpected screenshot content: %s", got)
	}
}

func TestNetworkListToolFormatsEntries(t *testing.T) {
	fake := fakeManager{
		listNetworkFn: func(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.NetworkEntry, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			if pageID != "page-1" {
				t.Fatalf("unexpected page id: %s", pageID)
			}
			if limit != 10 {
				t.Fatalf("unexpected limit: %d", limit)
			}
			return []browsercore.NetworkEntry{
				{
					Seq:          1,
					PageID:       "page-1",
					URL:          "https://example.com/api",
					Method:       "GET",
					ResourceType: "XHR",
					StatusCode:   200,
					Stage:        "response",
					Timestamp:    time.Now().UTC(),
				},
			}, nil
		},
	}

	res, err := NewNetworkListTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"page_id": "page-1", "limit": float64(10)},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !containsAll(got, "page-1", "GET", "https://example.com/api", "200") {
		t.Fatalf("unexpected network content: %s", got)
	}
}

func TestDownloadListToolFormatsEntries(t *testing.T) {
	fake := fakeManager{
		listDownloadsFn: func(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]browsercore.DownloadEntry, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			if pageID != "page-1" {
				t.Fatalf("unexpected page id: %s", pageID)
			}
			if limit != 5 {
				t.Fatalf("unexpected limit: %d", limit)
			}
			return []browsercore.DownloadEntry{
				{
					GUID:              "dl-1",
					PageID:            "page-1",
					URL:               "https://example.com/report.pdf",
					SuggestedFilename: "report.pdf",
					State:             "completed",
					BytesReceived:     1024,
					TotalBytes:        1024,
					PersistedPath:     "/tmp/report.pdf",
					PersistedSize:     1024,
					StartedAt:         time.Now().UTC(),
					UpdatedAt:         time.Now().UTC(),
				},
			}, nil
		},
	}

	res, err := NewDownloadListTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"page_id": "page-1", "limit": float64(5)},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !containsAll(got, "dl-1", "completed", "report.pdf", "/tmp/report.pdf") {
		t.Fatalf("unexpected download content: %s", got)
	}
}

func TestSearchContentToolFormatsHits(t *testing.T) {
	fake := fakeManager{
		searchSnapshotsFn: func(ctx context.Context, sessionID types.SessionID, query string, limit int) ([]browsercore.SnapshotSearchHit, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			if query != "pricing" {
				t.Fatalf("unexpected query: %s", query)
			}
			if limit != 3 {
				t.Fatalf("unexpected limit: %d", limit)
			}
			return []browsercore.SnapshotSearchHit{
				{
					PageID:   "page-2",
					URL:      "https://example.com/pricing",
					Title:    "Pricing",
					Revision: "rev-4",
					Score:    9,
					Snippet:  "pricing plans and enterprise details",
				},
			}, nil
		},
	}

	res, err := NewSearchContentTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"query": "pricing", "limit": float64(3)},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !containsAll(got, "page-2", "Pricing", "pricing plans", "rev-4") {
		t.Fatalf("unexpected search content: %s", got)
	}
}

func TestGetNetworkPolicyToolFormatsOutput(t *testing.T) {
	fake := fakeManager{
		getNetworkPolicyFn: func(ctx context.Context, sessionID types.SessionID) (browsercore.NetworkPolicy, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			return browsercore.NetworkPolicy{
				BlockedURLs:    []string{"*.png*", "*analytics.example.com*"},
				ResourcePolicy: "text_only",
			}, nil
		},
	}

	res, err := NewGetNetworkPolicyTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !containsAll(got, "text_only", "*.png*", "*analytics.example.com*") {
		t.Fatalf("unexpected policy content: %s", got)
	}
}

func TestSetNetworkPolicyToolPassesPolicy(t *testing.T) {
	fake := fakeManager{
		setNetworkPolicyFn: func(ctx context.Context, sessionID types.SessionID, policy browsercore.NetworkPolicy) (browsercore.NetworkPolicy, error) {
			if sessionID != "sess-1" {
				t.Fatalf("unexpected session id: %s", sessionID)
			}
			if policy.ResourcePolicy != "aggressive" {
				t.Fatalf("unexpected resource policy: %s", policy.ResourcePolicy)
			}
			if len(policy.BlockedURLs) != 2 {
				t.Fatalf("unexpected blocked urls: %#v", policy.BlockedURLs)
			}
			return policy, nil
		},
	}

	res, err := NewSetNetworkPolicyTool(fake).Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{
			"resource_policy": "aggressive",
			"blocked_urls":    []any{"*.png*", "*tracker.example.com*"},
		},
		ToolContext: &tool.ToolUseContext{
			SessionID: "sess-1",
		},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	if res.IsError() {
		t.Fatalf("expected success result, got error: %v", res.Error)
	}
	if got := res.Content; !containsAll(got, "aggressive", "*.png*", "*tracker.example.com*") {
		t.Fatalf("unexpected policy content: %s", got)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
