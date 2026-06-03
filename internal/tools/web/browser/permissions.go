package browser

import (
	"context"
	"net/url"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func browserPermissionResult(
	ctx context.Context,
	manager browsercore.Manager,
	input map[string]any,
	toolCtx tool.ToolUseContext,
	action string,
) types.PermissionResult {
	return webcore.EvaluatePermission(browserPermissionInput(ctx, manager, input, toolCtx, action))
}

func browserPermissionInput(
	ctx context.Context,
	manager browsercore.Manager,
	input map[string]any,
	toolCtx tool.ToolUseContext,
	action string,
) map[string]any {
	currentURL := resolveBrowserPermissionURL(ctx, manager, toolCtx.SessionID, readOptionalString(input, "page_id"))
	enriched := webcore.EnrichBrowserPermissionInput(input, action, currentURL)
	if toolCtx.ExecutionMode != "" {
		enriched["execution_mode"] = toolCtx.ExecutionMode
	}

	// Navigation-like actions prefer the explicit target URL, while page-local actions
	// keep current page context attached separately for host-aware permission rules.
	if rawURL := readOptionalString(input, "url"); rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			enriched["host"] = webcore.NormalizeHost(parsed.Hostname())
			enriched["scheme"] = parsed.Scheme
			enriched["path"] = parsed.EscapedPath()
		}
	}
	return enriched
}

func browserPermissionMatcher(input map[string]any) func(string) bool {
	return webcore.PermissionMatcher(input)
}

func resolveBrowserPermissionURL(
	ctx context.Context,
	manager browsercore.Manager,
	sessionID types.SessionID,
	pageID string,
) string {
	if manager == nil || sessionID == "" {
		return ""
	}
	pages, err := manager.ListPages(ctx, sessionID)
	if err != nil {
		return ""
	}

	if pageID != "" {
		for _, page := range pages {
			if page.ID == pageID {
				return page.URL
			}
		}
	}

	for _, page := range pages {
		if page.Active {
			return page.URL
		}
	}

	if len(pages) == 1 {
		return pages[0].URL
	}
	return ""
}
