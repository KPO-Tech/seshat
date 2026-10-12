package toolsearch

import (
	"os"
	"strconv"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/tools/contract"
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
