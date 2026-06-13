package task

import (
	"fmt"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

func resolveTaskSessionID(input tool.CallInput) (string, error) {
	sessionID := resolveOptionalTaskSessionID(input)
	if sessionID != "" {
		return sessionID, nil
	}
	return "", fmt.Errorf("session ID is required for task tools")
}

func resolveOptionalTaskSessionID(input tool.CallInput) string {
	toolCtx := input.ToolContextValue()
	if toolCtx.SessionID != "" {
		return string(toolCtx.SessionID)
	}
	if input.SessionID != "" {
		return string(input.SessionID)
	}
	return ""
}

func resolveTaskKind(parsed map[string]any, fallback string) string {
	if parsed == nil {
		return fallback
	}
	for _, key := range []string{"taskType", "task_type"} {
		if raw, ok := parsed[key].(string); ok {
			value := strings.ToLower(strings.TrimSpace(raw))
			if value != "" {
				return value
			}
		}
	}
	return fallback
}
