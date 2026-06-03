package engine

import "github.com/EngineerProjects/nexus-engine/internal/types"

func browserRuntimeEventFromProgress(progress types.ToolProgress) *types.RuntimeEvent {
	if progress.Metadata == nil {
		return nil
	}
	eventKind, _ := progress.Metadata["event_kind"].(string)
	if eventKind != "browser" {
		return nil
	}

	browserEvent := &types.BrowserRuntimeEvent{
		Action:        stringMeta(progress.Metadata, "action"),
		PageID:        stringMeta(progress.Metadata, "page_id"),
		URL:           stringMeta(progress.Metadata, "url"),
		Title:         stringMeta(progress.Metadata, "title"),
		ActivePageID:  stringMeta(progress.Metadata, "active_page_id"),
		PageCount:     intMeta(progress.Metadata, "page_count"),
		ActionCount:   intMeta(progress.Metadata, "action_count"),
		ElementCount:  intMeta(progress.Metadata, "element_count"),
		HeadingCount:  intMeta(progress.Metadata, "heading_count"),
		TextLength:    intMeta(progress.Metadata, "text_length"),
		Bytes:         intMeta(progress.Metadata, "bytes"),
		PersistedPath: stringMeta(progress.Metadata, "persisted_path"),
		PersistedSize: intMeta(progress.Metadata, "persisted_size"),
		FullPage:      boolMeta(progress.Metadata, "full_page"),
	}

	return &types.RuntimeEvent{
		Type:    browserRuntimeEventType(browserEvent.Action),
		Browser: browserEvent,
	}
}

func browserRuntimeEventType(action string) types.RuntimeEventType {
	switch action {
	case "open":
		return types.RuntimeEventTypeBrowserPage
	case "close_page", "select_page", "list_pages":
		return types.RuntimeEventTypeBrowserSession
	case "snapshot", "extract":
		return types.RuntimeEventTypeBrowserSnapshot
	case "screenshot":
		return types.RuntimeEventTypeBrowserScreenshot
	default:
		return types.RuntimeEventTypeBrowserAction
	}
}

func stringMeta(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func boolMeta(metadata map[string]any, key string) bool {
	value, _ := metadata[key].(bool)
	return value
}

func intMeta(metadata map[string]any, key string) int {
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
