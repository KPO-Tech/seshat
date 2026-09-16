package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewNotionConnector(t *testing.T) {
	c := NewNotionConnector(nil)
	if c == nil {
		t.Fatal("expected a non-nil connector")
	}
}

func TestNotionPageTitle_FindsTitlePropertyRegardlessOfName(t *testing.T) {
	page := notionPage{
		Properties: map[string]notionPageProperty{
			"Tags":     {Type: "multi_select"},
			"My Title": {Type: "title", Title: []notionRichText{{PlainText: "Hello "}, {PlainText: "World"}}},
		},
	}
	if got := notionPageTitle(page); got != "Hello World" {
		t.Fatalf("got %q, want %q", got, "Hello World")
	}
}

func TestNotionPageTitle_NoTitlePropertyIsEmpty(t *testing.T) {
	page := notionPage{Properties: map[string]notionPageProperty{"Tags": {Type: "multi_select"}}}
	if got := notionPageTitle(page); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestNotionBlockText_KnownAndUnknownTypes(t *testing.T) {
	para := notionBlock{Type: "paragraph", Paragraph: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "hi"}}}}
	if got := notionBlockText(para); got != "hi" {
		t.Fatalf("paragraph: got %q, want %q", got, "hi")
	}
	heading := notionBlock{Type: "heading_2", Heading2: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "Section"}}}}
	if got := notionBlockText(heading); got != "Section" {
		t.Fatalf("heading_2: got %q, want %q", got, "Section")
	}
	divider := notionBlock{Type: "divider"}
	if got := notionBlockText(divider); got != "" {
		t.Fatalf("divider (unsupported type): got %q, want empty", got)
	}
	emptyParagraph := notionBlock{Type: "paragraph", Paragraph: nil}
	if got := notionBlockText(emptyParagraph); got != "" {
		t.Fatalf("paragraph with nil container: got %q, want empty", got)
	}
}

// notionFakeServer serves /v1/blocks/{id}/children, keyed by a test-supplied
// handler, so fetchPageText's pagination/recursion can be exercised against
// real HTTP without a live Notion workspace.
func notionFakeServer(t *testing.T, handler func(blockID string, startCursor string) notionBlockChildrenResponse) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Notion-Version") != notionAPIVersion {
			t.Errorf("expected Notion-Version header %q, got %q", notionAPIVersion, r.Header.Get("Notion-Version"))
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		// notionBaseURL is overridden to the test server's own root (no
		// "/v1" prefix in the path here), so the path is "blocks/{id}/children".
		blockID := parts[1]
		resp := handler(blockID, r.URL.Query().Get("start_cursor"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchPageText_FlattensNestedBlocks(t *testing.T) {
	// root: [paragraph "Intro", bulleted_list_item "Item 1" (has children: [paragraph "Nested note"])]
	server := notionFakeServer(t, func(blockID, cursor string) notionBlockChildrenResponse {
		switch blockID {
		case "root":
			return notionBlockChildrenResponse{Results: []notionBlock{
				{ID: "b1", Type: "paragraph", Paragraph: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "Intro"}}}},
				{ID: "b2", Type: "bulleted_list_item", HasChildren: true, BulletedListItem: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "Item 1"}}}},
			}}
		case "b2":
			return notionBlockChildrenResponse{Results: []notionBlock{
				{ID: "b3", Type: "paragraph", Paragraph: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "Nested note"}}}},
			}}
		default:
			return notionBlockChildrenResponse{}
		}
	})

	cl := &notionClient{http: server.Client()}
	restoreBaseURL := SetNotionBaseURLForTesting(server.URL)
	defer restoreBaseURL()

	text, err := fetchPageText(context.Background(), cl, "root")
	if err != nil {
		t.Fatalf("fetchPageText: %v", err)
	}
	for _, want := range []string{"Intro", "Item 1", "Nested note"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected flattened text to contain %q, got %q", want, text)
		}
	}
}

func TestFetchPageText_PaginatesWithinOneBlock(t *testing.T) {
	server := notionFakeServer(t, func(blockID, cursor string) notionBlockChildrenResponse {
		if cursor == "" {
			return notionBlockChildrenResponse{
				Results:    []notionBlock{{ID: "p1", Type: "paragraph", Paragraph: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "page one"}}}}},
				HasMore:    true,
				NextCursor: "cursor-2",
			}
		}
		return notionBlockChildrenResponse{
			Results: []notionBlock{{ID: "p2", Type: "paragraph", Paragraph: &notionRichTextContainer{RichText: []notionRichText{{PlainText: "page two"}}}}},
		}
	})
	cl := &notionClient{http: server.Client()}
	defer SetNotionBaseURLForTesting(server.URL)()

	text, err := fetchPageText(context.Background(), cl, "root")
	if err != nil {
		t.Fatalf("fetchPageText: %v", err)
	}
	if !strings.Contains(text, "page one") || !strings.Contains(text, "page two") {
		t.Fatalf("expected both pages' text, got %q", text)
	}
}

func TestFetchPageText_DepthCapStopsRecursion(t *testing.T) {
	// Each block "depth-N" has exactly one child "depth-N+1", chained far
	// deeper than maxNotionBlockDepth - the walk must stop instead of
	// recursing forever (or past the cap).
	server := notionFakeServer(t, func(blockID, cursor string) notionBlockChildrenResponse {
		depth := 0
		if blockID != "root" {
			depth, _ = strconv.Atoi(strings.TrimPrefix(blockID, "depth-"))
		}
		next := depth + 1
		return notionBlockChildrenResponse{Results: []notionBlock{
			{
				ID:          fmt.Sprintf("depth-%d", next),
				Type:        "paragraph",
				HasChildren: true,
				Paragraph:   &notionRichTextContainer{RichText: []notionRichText{{PlainText: fmt.Sprintf("level-%d", next)}}},
			},
		}}
	})
	cl := &notionClient{http: server.Client()}
	defer SetNotionBaseURLForTesting(server.URL)()

	text, err := fetchPageText(context.Background(), cl, "root")
	if err != nil {
		t.Fatalf("fetchPageText: %v", err)
	}
	// maxNotionBlockDepth caps recursion - a level well past it must not appear.
	if strings.Contains(text, fmt.Sprintf("level-%d", maxNotionBlockDepth+50)) {
		t.Fatalf("expected the walk to stop well before an arbitrarily deep level, got %q", text)
	}
	if !strings.Contains(text, "level-1") {
		t.Fatalf("expected at least the first level to be captured, got %q", text)
	}
}

func TestFetchPageText_BlockCountCapStopsWalk(t *testing.T) {
	// One page of far more blocks than maxNotionBlocks, none with children.
	blocks := make([]notionBlock, maxNotionBlocks+500)
	for i := range blocks {
		blocks[i] = notionBlock{ID: fmt.Sprintf("b%d", i), Type: "paragraph", Paragraph: &notionRichTextContainer{RichText: []notionRichText{{PlainText: fmt.Sprintf("item-%d", i)}}}}
	}
	server := notionFakeServer(t, func(blockID, cursor string) notionBlockChildrenResponse {
		return notionBlockChildrenResponse{Results: blocks}
	})
	cl := &notionClient{http: server.Client()}
	defer SetNotionBaseURLForTesting(server.URL)()

	text, err := fetchPageText(context.Background(), cl, "root")
	if err != nil {
		t.Fatalf("fetchPageText: %v", err)
	}
	got := strings.Count(text, "\n")
	if got > maxNotionBlocks {
		t.Fatalf("expected at most %d lines (block count cap), got %d", maxNotionBlocks, got)
	}
	if got == 0 {
		t.Fatal("expected at least some blocks to be captured before the cap")
	}
}

func TestNotionErrorRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "8")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	cl := &notionClient{http: server.Client()}
	err := cl.do(context.Background(), http.MethodGet, server.URL, nil, nil)
	var rl RateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected the error to satisfy RateLimited, got %T", err)
	}
	delay, ok := rl.RetryAfter()
	if !ok || delay != 8*time.Second {
		t.Fatalf("got (%v, %v), want (8s, true)", delay, ok)
	}
}

func TestNotionErrorNonRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cl := &notionClient{http: server.Client()}
	err := cl.do(context.Background(), http.MethodGet, server.URL, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
	var rl RateLimited
	if errors.As(err, &rl) {
		if _, ok := rl.RetryAfter(); ok {
			t.Fatalf("expected a 401 to report RetryAfter ok=false, got ok=true: %v", err)
		}
	}
}
