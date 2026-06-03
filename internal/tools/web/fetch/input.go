package webfetch

import (
	"fmt"
	"strings"

	fetchcore "github.com/EngineerProjects/nexus-engine/internal/web/fetch"
)

// parseInput validates the raw tool payload and normalizes optional routing hints.
func parseInput(parsed map[string]any) (*Input, error) {
	urlStr, ok := parsed["url"].(string)
	if !ok || strings.TrimSpace(urlStr) == "" {
		return nil, fmt.Errorf("url is required")
	}
	prompt, ok := parsed["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	renderMode := ""
	if value, ok := parsed["render_mode"]; ok {
		mode, isString := value.(string)
		if !isString {
			return nil, fmt.Errorf("render_mode must be a string")
		}
		renderMode = mode
	}

	normalizedMode, err := fetchcore.NormalizeRenderMode(renderMode)
	if err != nil {
		return nil, err
	}

	return &Input{
		URL:        strings.TrimSpace(urlStr),
		Prompt:     strings.TrimSpace(prompt),
		RenderMode: normalizedMode,
	}, nil
}

func readOptionalString(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}
