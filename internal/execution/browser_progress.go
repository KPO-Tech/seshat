package execution

import (
	"fmt"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

func browserProgressForResult(toolUse types.ToolUseContent, result tool.CallResult) *types.ToolProgress {
	if result.IsError() || !strings.HasPrefix(toolUse.Name, "browser_") {
		return nil
	}

	progress := types.ToolProgress{
		ToolName:        toolUse.Name,
		ToolUseID:       toolUse.ID,
		Stage:           types.ToolProgressStageRunning,
		PercentComplete: 95,
		Metadata: map[string]any{
			"event_kind": "browser",
			"action":     strings.TrimPrefix(toolUse.Name, "browser_"),
		},
	}

	switch payload := result.Data.(type) {
	case browsercore.PageInfo:
		progress.Message = fmt.Sprintf("%s reached page %s", toolUse.Name, payload.ID)
		progress.Metadata["page_id"] = payload.ID
		progress.Metadata["url"] = payload.URL
		progress.Metadata["title"] = payload.Title
		progress.Metadata["active"] = payload.Active
	case []browsercore.PageInfo:
		progress.Message = fmt.Sprintf("%s listed %d browser pages", toolUse.Name, len(payload))
		progress.Metadata["page_count"] = len(payload)
		if len(payload) > 0 {
			progress.Metadata["active_page_id"] = activePageID(payload)
		}
	case []browsercore.DownloadEntry:
		progress.Message = fmt.Sprintf("%s listed %d browser downloads", toolUse.Name, len(payload))
		progress.Metadata["download_count"] = len(payload)
		if len(payload) > 0 {
			progress.Metadata["page_id"] = payload[0].PageID
			progress.Metadata["persisted_path"] = payload[0].PersistedPath
			progress.Metadata["persisted_size"] = payload[0].PersistedSize
		}
	case browsercore.SessionState:
		progress.Message = fmt.Sprintf("%s updated browser session to %d pages", toolUse.Name, payload.PageCount)
		progress.Metadata["active_page_id"] = payload.ActivePageID
		progress.Metadata["page_count"] = payload.PageCount
		progress.Metadata["action_count"] = payload.ActionCount
	case browsercore.Snapshot:
		progress.Message = fmt.Sprintf("%s captured browser snapshot for %s", toolUse.Name, payload.Page.ID)
		progress.Metadata["page_id"] = payload.Page.ID
		progress.Metadata["url"] = payload.Page.URL
		progress.Metadata["title"] = payload.Page.Title
		progress.Metadata["text_length"] = len(payload.Text)
		progress.Metadata["element_count"] = len(payload.Elements)
		progress.Metadata["heading_count"] = len(payload.Headings)
	case browsercore.Screenshot:
		progress.Message = fmt.Sprintf("%s captured browser screenshot for %s", toolUse.Name, payload.Page.ID)
		progress.Metadata["page_id"] = payload.Page.ID
		progress.Metadata["url"] = payload.Page.URL
		progress.Metadata["bytes"] = payload.Bytes
		progress.Metadata["full_page"] = payload.FullPage
		if payload.PersistedPath != "" {
			progress.Metadata["persisted_path"] = payload.PersistedPath
			progress.Metadata["persisted_size"] = payload.PersistedSize
		}
	default:
		return nil
	}

	return &progress
}

func activePageID(pages []browsercore.PageInfo) string {
	for _, page := range pages {
		if page.Active {
			return page.ID
		}
	}
	return ""
}
