package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

const memoryMaxVisible = 6

// ─── JSON mirrors (subset of longterm package types) ─────────────────────────

type memoryEntity struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entity_type"`
	Observations []string `json:"observations"`
}

type memoryObsResult struct {
	EntityName        string   `json:"entity_name"`
	AddedObservations []string `json:"added_observations"`
}

type memoryGraph struct {
	Entities  []memoryEntity    `json:"entities"`
	Relations []json.RawMessage `json:"relations"`
}

// ─── shared body helpers ─────────────────────────────────────────────────────

// renderMemoryEntityList renders a compact list: "name (type) · N obs".
func renderMemoryEntityList(sty *styles.Styles, entities []memoryEntity, width int, expanded bool) string {
	maxVisible := memoryMaxVisible
	if expanded {
		maxVisible = len(entities)
	}
	shown := min(maxVisible, len(entities))

	var out []string
	for i := 0; i < shown; i++ {
		e := entities[i]
		name := sty.Tool.ResultItemDesc.Render(e.Name)
		meta := ""
		if e.EntityType != "" {
			meta += sty.Tool.ResultItemDesc.Render(" (" + e.EntityType + ")")
		}
		if n := len(e.Observations); n > 0 {
			noun := "obs"
			if n == 1 {
				noun = "ob"
			}
			meta += sty.Tool.ResultItemDesc.Render(fmt.Sprintf(" · %d %s", n, noun))
		}
		line := ansi.Truncate(name+meta, width, "…")
		out = append(out, line)
	}
	if remaining := len(entities) - shown; remaining > 0 {
		out = append(out, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more", remaining)))
	}
	return strings.Join(out, "\n")
}

// renderMemoryObsResults renders "entity_name · +N observations" lines.
func renderMemoryObsResults(sty *styles.Styles, results []memoryObsResult, width int, expanded bool) string {
	maxVisible := memoryMaxVisible
	if expanded {
		maxVisible = len(results)
	}
	shown := min(maxVisible, len(results))

	var out []string
	for i := 0; i < shown; i++ {
		r := results[i]
		name := sty.Tool.ResultItemDesc.Render(r.EntityName)
		meta := ""
		if n := len(r.AddedObservations); n > 0 {
			noun := "observations"
			if n == 1 {
				noun = "observation"
			}
			meta = sty.Tool.ResultItemDesc.Render(fmt.Sprintf(" · +%d %s", n, noun))
		}
		line := ansi.Truncate(name+meta, width, "…")
		out = append(out, line)
	}
	if remaining := len(results) - shown; remaining > 0 {
		out = append(out, sty.Tool.ResultTruncation.Render(fmt.Sprintf("… +%d more", remaining)))
	}
	return strings.Join(out, "\n")
}

// ─── memory_create_entities ──────────────────────────────────────────────────

// MemoryCreateEntitiesToolMessageItem represents a memory_create_entities tool call.
type MemoryCreateEntitiesToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MemoryCreateEntitiesToolMessageItem)(nil)

// NewMemoryCreateEntitiesToolMessageItem creates a new [MemoryCreateEntitiesToolMessageItem].
func NewMemoryCreateEntitiesToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MemoryCreateEntitiesRenderContext{}, canceled)
}

// MemoryCreateEntitiesRenderContext renders memory_create_entities tool messages.
type MemoryCreateEntitiesRenderContext struct{}

// RenderTool implements [ToolRenderer].
func (m *MemoryCreateEntitiesRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Create Memory"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	// Input: count entities requested.
	var inputRaw struct {
		Entities []json.RawMessage `json:"entities"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &inputRaw)

	// Result: array of created Entity objects.
	var created []memoryEntity
	if opts.HasResult() && opts.Result.Content != "" {
		_ = json.Unmarshal([]byte(opts.Result.Content), &created)
	}

	n := len(created)
	if n == 0 {
		n = len(inputRaw.Entities)
	}
	noun := "entities"
	if n == 1 {
		noun = "entity"
	}
	headerParams := []string{fmt.Sprintf("%d %s", n, noun)}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if len(created) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderMemoryEntityList(sty, created, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// ─── memory_add_observations ─────────────────────────────────────────────────

// MemoryAddObservationsToolMessageItem represents a memory_add_observations tool call.
type MemoryAddObservationsToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MemoryAddObservationsToolMessageItem)(nil)

// NewMemoryAddObservationsToolMessageItem creates a new [MemoryAddObservationsToolMessageItem].
func NewMemoryAddObservationsToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MemoryAddObservationsRenderContext{}, canceled)
}

// MemoryAddObservationsRenderContext renders memory_add_observations tool messages.
type MemoryAddObservationsRenderContext struct{}

// RenderTool implements [ToolRenderer].
func (m *MemoryAddObservationsRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Memory Observe"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	// Input: count entity+observations pairs.
	var inputRaw struct {
		Observations []json.RawMessage `json:"observations"`
	}
	_ = json.Unmarshal([]byte(opts.ToolCall.Input), &inputRaw)

	// Result: []ObservationResult.
	var results []memoryObsResult
	if opts.HasResult() && opts.Result.Content != "" {
		_ = json.Unmarshal([]byte(opts.Result.Content), &results)
	}

	n := len(results)
	if n == 0 {
		n = len(inputRaw.Observations)
	}
	noun := "entities"
	if n == 1 {
		noun = "entity"
	}
	headerParams := []string{fmt.Sprintf("%d %s", n, noun)}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if len(results) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderMemoryObsResults(sty, results, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// ─── memory_search_nodes ─────────────────────────────────────────────────────

// MemorySearchNodesToolMessageItem represents a memory_search_nodes tool call.
type MemorySearchNodesToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MemorySearchNodesToolMessageItem)(nil)

// NewMemorySearchNodesToolMessageItem creates a new [MemorySearchNodesToolMessageItem].
func NewMemorySearchNodesToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MemorySearchNodesRenderContext{}, canceled)
}

// MemorySearchNodesRenderContext renders memory_search_nodes tool messages.
type MemorySearchNodesRenderContext struct{}

// RenderTool implements [ToolRenderer].
func (m *MemorySearchNodesRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Memory Search"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, displayName, cappedWidth)
	}

	headerParams := []string{params.Query}

	var graph memoryGraph
	hasParsed := false
	if opts.HasResult() && opts.Result.Content != "" {
		if err := json.Unmarshal([]byte(opts.Result.Content), &graph); err == nil {
			hasParsed = true
			n := len(graph.Entities)
			if n > 0 {
				noun := "results"
				if n == 1 {
					noun = "result"
				}
				headerParams = append(headerParams, fmt.Sprintf("%d %s", n, noun))
			} else {
				headerParams = append(headerParams, "no results")
			}
		}
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !hasParsed || len(graph.Entities) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderMemoryEntityList(sty, graph.Entities, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}

// ─── memory_open_nodes ───────────────────────────────────────────────────────

// MemoryOpenNodesToolMessageItem represents a memory_open_nodes tool call.
type MemoryOpenNodesToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*MemoryOpenNodesToolMessageItem)(nil)

// NewMemoryOpenNodesToolMessageItem creates a new [MemoryOpenNodesToolMessageItem].
func NewMemoryOpenNodesToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &MemoryOpenNodesRenderContext{}, canceled)
}

// MemoryOpenNodesRenderContext renders memory_open_nodes tool messages.
type MemoryOpenNodesRenderContext struct{}

// RenderTool implements [ToolRenderer].
func (m *MemoryOpenNodesRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	const displayName = "Memory Open"
	cappedWidth := width

	if opts.IsPending() {
		return pendingTool(sty, displayName, opts.Anim, opts.Compact)
	}

	var params struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, displayName, cappedWidth)
	}

	n := len(params.Names)
	noun := "nodes"
	if n == 1 {
		noun = "node"
	}
	headerParams := []string{fmt.Sprintf("%d %s", n, noun)}

	var graph memoryGraph
	hasParsed := false
	if opts.HasResult() && opts.Result.Content != "" {
		if err := json.Unmarshal([]byte(opts.Result.Content), &graph); err == nil {
			hasParsed = true
			found := len(graph.Entities)
			// Only mention the found count if it differs from requested.
			if found != n {
				if found == 0 {
					headerParams = append(headerParams, "not found")
				} else {
					headerParams = append(headerParams, fmt.Sprintf("%d found", found))
				}
			}
			if r := len(graph.Relations); r > 0 {
				rnoun := "relations"
				if r == 1 {
					rnoun = "relation"
				}
				headerParams = append(headerParams, fmt.Sprintf("%d %s", r, rnoun))
			}
		}
	}

	header := toolHeader(sty, opts.Status, displayName, cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	if !hasParsed || len(graph.Entities) == 0 {
		return header
	}

	bodyWidth := cappedWidth - toolBodyLeftPaddingTotal
	body := sty.Tool.Body.Render(renderMemoryEntityList(sty, graph.Entities, bodyWidth, opts.ExpandedContent))
	return joinToolParts(header, body)
}
