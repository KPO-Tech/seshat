package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

// GenericToolMessageItem is a message item that represents an unknown tool call.
type GenericToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*GenericToolMessageItem)(nil)

// NewGenericToolMessageItem creates a new [GenericToolMessageItem].
func NewGenericToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &GenericToolRenderContext{}, canceled)
}

// GenericToolRenderContext renders unknown/generic tool messages.
type GenericToolRenderContext struct{}

// RenderTool implements the [ToolRenderer] interface.
func (g *GenericToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width

	// Strip prefix like "default_api:" or similar from the name we display
	rawName := opts.ToolCall.Name
	if idx := strings.Index(rawName, ":"); idx >= 0 {
		rawName = rawName[idx+1:]
	}
	name := humanizedToolName(rawName)

	if opts.IsPending() {
		return pendingTool(sty, name, opts.Anim, opts.Compact)
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, name, cappedWidth)
	}

	toolParams := getGenericToolParams(params)
	header := toolHeader(sty, opts.Status, name, cappedWidth, opts.Compact, toolParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !opts.HasResult() || opts.Result.Content == "" {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal

	if opts.Result.Data != "" && strings.HasPrefix(opts.Result.MIMEType, "image/") {
		body := sty.Tool.Body.Render(toolOutputImageContent(sty, opts.Result.Data, opts.Result.MIMEType))
		return joinToolParts(header, body)
	}

	body := renderToolResultTextContent(sty, opts.Result.Content, toolResultContentWidths{Body: bodyWidth, Diff: cappedWidth}, opts.ExpandedContent)
	return joinToolParts(header, body)
}

func getGenericToolParams(params map[string]any) []string {
	if len(params) == 0 {
		return nil
	}
	// Try to find a primary key
	primaryKeys := []string{
		"path", "filepath", "directorypath", "absolutepath", "targetfile",
		"command", "commandline", "query", "url", "recipient", "agent_id",
		"action", "target",
	}
	var mainKey string
	for _, pk := range primaryKeys {
		for k := range params {
			if strings.EqualFold(k, pk) {
				mainKey = k
				break
			}
		}
		if mainKey != "" {
			break
		}
	}
	// If no primary key found, pick any key (e.g. first one)
	if mainKey == "" {
		for k := range params {
			mainKey = k
			break
		}
	}

	mainVal := formatParamValue(params[mainKey])

	headerParams := []string{mainVal}
	// Add other keys as key-value pairs
	for k, v := range params {
		if k == mainKey {
			continue
		}
		vStr := formatParamValue(v)
		headerParams = append(headerParams, k, vStr)
	}
	return headerParams
}

func formatParamValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case map[string]any, []any:
		if data, err := json.Marshal(val); err == nil {
			return string(data)
		}
	}
	return fmt.Sprintf("%v", v)
}
