package fetch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

// BrowserSnapshotTextLimit bounds the browser-rendered text payload returned to fetch callers.
const BrowserSnapshotTextLimit = 12000

// Fetch executes the resolved backend and returns a shared fetch response regardless of backend.
func (s *Service) Fetch(ctx context.Context, request Request) (FetchedContent, error) {
	plan, err := s.Prepare(request)
	if err != nil {
		return FetchedContent{}, err
	}

	var fetched FetchedContent
	switch plan.Mode {
	case webcore.RenderModeAuto:
		fetched, err = s.fetchAuto(ctx, plan)
	case webcore.RenderModeBrowser:
		fetched, err = s.fetchViaBrowser(ctx, plan)
	case webcore.RenderModeHTTP:
		fetched, err = s.fetchViaHTTP(ctx, plan.NormalizedURL)
	default:
		return FetchedContent{}, fmt.Errorf("unsupported fetch mode %q", plan.Mode)
	}
	if err != nil {
		return FetchedContent{}, err
	}
	if fetched.Mode == "" {
		fetched.Mode = plan.Mode
	}
	return fetched, nil
}

func (s *Service) fetchAuto(ctx context.Context, plan *fetchPlan) (FetchedContent, error) {
	if s.browserManager != nil && s.decisionCache != nil {
		if cachedMode, ok := s.decisionCache.Get(decisionCacheKey(plan.NormalizedURL)); ok {
			switch cachedMode {
			case webcore.RenderModeHTTP:
				httpFetched, err := s.fetchViaHTTP(ctx, plan.NormalizedURL)
				if err == nil {
					httpFetched.Mode = webcore.RenderModeHTTP
				}
				return httpFetched, err
			case webcore.RenderModeBrowser:
				browserFetched, err := s.fetchViaBrowser(ctx, plan)
				if err == nil {
					browserFetched.Mode = webcore.RenderModeBrowser
				}
				return browserFetched, err
			}
		}
	}

	httpFetched, err := s.fetchViaHTTP(ctx, plan.NormalizedURL)
	if err != nil {
		return FetchedContent{}, err
	}
	if httpFetched.Redirect != nil || shouldPreferHTTP(plan, httpFetched) {
		s.rememberDecision(plan.NormalizedURL, webcore.RenderModeHTTP)
		httpFetched.Mode = webcore.RenderModeHTTP
		return httpFetched, nil
	}

	browserFetched, browserErr := s.fetchViaBrowser(ctx, plan)
	if browserErr != nil {
		s.rememberDecision(plan.NormalizedURL, webcore.RenderModeHTTP)
		httpFetched.Mode = webcore.RenderModeHTTP
		return httpFetched, nil
	}
	s.rememberDecision(plan.NormalizedURL, webcore.RenderModeBrowser)
	browserFetched.Mode = webcore.RenderModeBrowser
	return browserFetched, nil
}

// fetchViaBrowser uses a short-lived isolated browser session so JS-rendered pages do not pollute the agent's main tab set.
func (s *Service) fetchViaBrowser(ctx context.Context, plan *fetchPlan) (FetchedContent, error) {
	manager := s.browserManager
	if manager == nil {
		return FetchedContent{}, fmt.Errorf("browser render mode requires a browser manager")
	}

	pooled := s.renderPool != nil
	sessionID := tempBrowserSessionID(plan.Request)
	if pooled {
		sessionID = s.renderPool.sessionFor(plan.NormalizedURL, plan.Request)
	} else {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = manager.CloseSession(cleanupCtx, sessionID)
		}()
	}

	pageInfo, err := manager.OpenPage(ctx, sessionID, plan.NormalizedURL)
	if err != nil {
		return FetchedContent{}, err
	}
	defer func(pageID string) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = manager.ClosePage(cleanupCtx, sessionID, pageID)
	}(pageInfo.ID)
	snapshot, err := manager.Snapshot(ctx, sessionID, pageInfo.ID, browsercore.SnapshotOptions{MaxText: BrowserSnapshotTextLimit})
	if err != nil {
		return FetchedContent{}, err
	}

	return FetchedContent{
		Content:     snapshot.Text,
		Bytes:       len(snapshot.Text),
		Code:        200,
		CodeText:    "OK (browser snapshot)",
		ContentType: "text/plain; source=browser",
		FinalURL:    snapshot.Page.URL,
	}, nil
}

func tempBrowserSessionID(request Request) types.SessionID {
	base := request.SessionID
	if base == "" {
		base = types.SessionID("webfetch")
	}
	return types.SessionID(fmt.Sprintf("%s:render:%d", base, time.Now().UnixNano()))
}

func shouldPreferHTTP(plan *fetchPlan, fetched FetchedContent) bool {
	if fetched.BrowserRecommended {
		return false
	}
	contentType := strings.ToLower(fetched.ContentType)
	if !strings.Contains(contentType, "html") {
		return true
	}
	text := strings.TrimSpace(strings.ToLower(fetched.Content))
	if text == "" {
		return false
	}
	if len(text) > 1200 {
		return true
	}
	lowerURL := strings.ToLower(plan.NormalizedURL)
	if strings.Contains(lowerURL, "/login") ||
		strings.Contains(lowerURL, "/signin") ||
		strings.Contains(lowerURL, "/auth") ||
		strings.Contains(lowerURL, "/dashboard") ||
		strings.Contains(lowerURL, "/app") {
		return false
	}
	if strings.Contains(text, "enable javascript") ||
		strings.Contains(text, "javascript is required") ||
		strings.Contains(text, "loading...") ||
		strings.Contains(text, "please wait") {
		return false
	}
	return len(text) >= 240
}

func (s *Service) rememberDecision(rawURL string, mode string) {
	if s == nil || s.decisionCache == nil {
		return
	}
	switch mode {
	case webcore.RenderModeHTTP, webcore.RenderModeBrowser:
		s.decisionCache.Set(decisionCacheKey(rawURL), mode)
	}
}
