package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/fsext"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

// -----------------------------------------------------------------------------
// Notebook Edit Tool
// -----------------------------------------------------------------------------

// NotebookEditToolMessageItem represents a notebook_edit tool call.
type NotebookEditToolMessageItem struct {
	*baseToolMessageItem
}

var _ ToolMessageItem = (*NotebookEditToolMessageItem)(nil)

// NewNotebookEditToolMessageItem creates a new [NotebookEditToolMessageItem].
func NewNotebookEditToolMessageItem(
	sty *styles.Styles,
	toolCall message.ToolCall,
	result *message.ToolResult,
	canceled bool,
) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &NotebookEditToolRenderContext{}, canceled)
}

// NotebookEditToolRenderContext renders notebook_edit tool messages.
type NotebookEditToolRenderContext struct{}

type notebookEditInput struct {
	NotebookPath string `json:"notebook_path"`
	CellID       string `json:"cell_id,omitempty"`
	NewSource    string `json:"new_source"`
	CellType     string `json:"cell_type,omitempty"`
	EditMode     string `json:"edit_mode,omitempty"`
}

type notebookEditOutput struct {
	NotebookPath string `json:"notebook_path"`
	CellID       string `json:"cell_id,omitempty"`
	CellType     string `json:"cell_type"`
	Language     string `json:"language"`
	EditMode     string `json:"edit_mode"`
	Error        string `json:"error,omitempty"`
}

// RenderTool implements the [ToolRenderer] interface.
func (n *NotebookEditToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := cappedToolWidth(width)
	if opts.IsPending() {
		return pendingTool(sty, "Notebook Edit", opts.Anim, opts.Compact)
	}

	var params notebookEditInput
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Notebook Edit", cappedWidth)
	}

	prettyPath := fsext.PrettyPath(params.NotebookPath)

	editMode := params.EditMode
	if editMode == "" {
		editMode = "replace"
	}

	var headerParams []string
	headerParams = append(headerParams, prettyPath)
	if params.CellID != "" {
		headerParams = append(headerParams, fmt.Sprintf("cell %s · %s", params.CellID, editMode))
	} else {
		headerParams = append(headerParams, editMode)
	}

	// Try to get resolved info from the result.
	var out notebookEditOutput
	if opts.HasResult() && opts.Result.Content != "" {
		_ = json.Unmarshal([]byte(opts.Result.Content), &out)
		if out.Error != "" {
			header := toolHeader(sty, opts.Status, "Notebook Edit", cappedWidth, opts.Compact, headerParams...)
			errBody := sty.Tool.Body.Render(sty.Tool.StateCancelled.Render(out.Error))
			return joinToolParts(header, errBody)
		}
	}

	header := toolHeader(sty, opts.Status, "Notebook Edit", cappedWidth, opts.Compact, headerParams...)
	if opts.Compact {
		return header
	}

	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	// For delete mode, no source to show.
	if editMode == "delete" {
		return header
	}

	// Show new_source as code, language from cell_type (code→python, markdown→markdown).
	lang := cellTypeToLanguage(params.CellType, out.Language)
	if params.NewSource == "" {
		return header
	}

	// Synthesise a filename for syntax highlighting.
	fakeFile := "cell." + lang
	body := toolOutputCodeContent(sty, fakeFile, params.NewSource, 0, cappedWidth, opts.ExpandedContent)
	return joinToolParts(header, body)
}

// cellTypeToLanguage maps notebook cell_type + optional resolved language to a
// file extension for syntax highlighting.
func cellTypeToLanguage(cellType, resolvedLang string) string {
	if resolvedLang != "" {
		switch strings.ToLower(resolvedLang) {
		case "python", "python3", "ipython3":
			return "py"
		case "r":
			return "r"
		case "julia":
			return "jl"
		case "javascript", "js":
			return "js"
		case "typescript", "ts":
			return "ts"
		case "markdown":
			return "md"
		}
	}
	switch strings.ToLower(cellType) {
	case "markdown":
		return "md"
	default:
		return "py"
	}
}
