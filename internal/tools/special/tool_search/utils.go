package toolsearch

import (
	"os"
	"strconv"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/contract"
)

func GetToolSearchMode() ToolSearchMode {
	value := os.Getenv(ToolSearchEnvVar)

	if value == "" {
		return ToolSearchModeStandard
	}

	if value == "true" {
		return ToolSearchModeTST
	}

	if value == "false" {
		return ToolSearchModeStandard
	}

	if strings.HasPrefix(value, "auto:") {
		percentStr := strings.TrimPrefix(value, "auto:")
		percent, err := strconv.Atoi(percentStr)
		if err != nil || percent < 0 || percent > 100 {
			return ToolSearchModeTSTAuto
		}
		if percent == 0 {
			return ToolSearchModeTST
		}
		if percent == 100 {
			return ToolSearchModeStandard
		}
		return ToolSearchModeTSTAuto
	}

	if value == "auto" {
		return ToolSearchModeTSTAuto
	}

	return ToolSearchModeTST
}

func IsToolSearchEnabled() bool {
	mode := GetToolSearchMode()
	return mode != ToolSearchModeStandard
}

func IsToolSearchEnabledOptimistic() bool {
	mode := GetToolSearchMode()
	return mode != ToolSearchModeStandard
}

func IsDeferredTool(tool contract.Tool) bool {
	def := tool.Definition()

	if def.AlwaysLoad {
		return false
	}

	if def.IsMCP {
		return true
	}

	if def.Name == ToolSearchToolName {
		return false
	}

	return def.ShouldDefer
}

func GetDeferredTools(tools []contract.Tool) []contract.Tool {
	var deferred []contract.Tool
	for _, tool := range tools {
		if IsDeferredTool(tool) {
			deferred = append(deferred, tool)
		}
	}
	return deferred
}

func GetNonDeferredTools(tools []contract.Tool) []contract.Tool {
	var nonDeferred []contract.Tool
	for _, tool := range tools {
		if !IsDeferredTool(tool) {
			nonDeferred = append(nonDeferred, tool)
		}
	}
	return nonDeferred
}

func ParseAutoPercentage(value string) (int, bool) {
	if !strings.HasPrefix(value, "auto:") {
		return 0, false
	}
	percentStr := strings.TrimPrefix(value, "auto:")
	percent, err := strconv.Atoi(percentStr)
	if err != nil {
		return 0, false
	}
	if percent < 0 || percent > 100 {
		return 0, false
	}
	return percent, true
}

func GetAutoThresholdPercent() int {
	value := os.Getenv(ToolSearchEnvVar)
	if value == "" {
		return DefaultAutoThresholdPercent
	}
	if value == "auto" {
		return DefaultAutoThresholdPercent
	}
	if percent, ok := ParseAutoPercentage(value); ok {
		return percent
	}
	return DefaultAutoThresholdPercent
}

func GetAutoToolSearchCharThreshold(contextWindow int) int {
	percent := GetAutoThresholdPercent()
	return (contextWindow * percent / 100) * 4
}

func ModelSupportsToolReference(model string) bool {
	lower := strings.ToLower(model)
	return !strings.Contains(lower, "haiku")
}

func ExtractDiscoveredToolNames(messages []any) map[string]bool {
	discovered := make(map[string]bool)
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := msgMap["content"].([]any)
		if !ok {
			continue
		}
		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok || blockMap["type"] != "tool_result" {
				continue
			}
			blockContent, ok := blockMap["content"].([]any)
			if !ok {
				continue
			}
			for _, item := range blockContent {
				itemMap, ok := item.(map[string]any)
				if !ok || itemMap["type"] != "tool_reference" {
					continue
				}
				if toolName, ok := itemMap["tool_name"].(string); ok {
					discovered[toolName] = true
				}
			}
		}
	}
	return discovered
}
