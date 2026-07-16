package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/seshattui/fsext"
	"github.com/KPO-Tech/seshat/internal/seshattui/message"
	"github.com/KPO-Tech/seshat/internal/seshattui/ui/styles"
)

// -----------------------------------------------------------------------------
// Notebook Edit Tool
// -----------------------------------------------------------------------------

// ── notebook_edit ─────────────────────────────────────────────────────────────

// NotebookEditToolMessageItem represents a notebook_edit tool call.
type NotebookEditToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*NotebookEditToolMessageItem)(nil)

func NewNotebookEditToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &NotebookEditToolRenderContext{}, canceled)
}

// ── notebook_create ───────────────────────────────────────────────────────────

// NotebookCreateToolMessageItem represents a notebook_create tool call.
type NotebookCreateToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*NotebookCreateToolMessageItem)(nil)

func NewNotebookCreateToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &NotebookCreateToolRenderContext{}, canceled)
}

// ── notebook_write ────────────────────────────────────────────────────────────

// NotebookWriteToolMessageItem represents a notebook_write tool call.
type NotebookWriteToolMessageItem struct{ *baseToolMessageItem }

var _ ToolMessageItem = (*NotebookWriteToolMessageItem)(nil)

func NewNotebookWriteToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return newBaseToolMessageItem(sty, toolCall, result, &NotebookWriteToolRenderContext{}, canceled)
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
	OriginalFile string `json:"original_file,omitempty"`
	UpdatedFile  string `json:"updated_file,omitempty"`
	Error        string `json:"error,omitempty"`
}

// RenderTool implements the [ToolRenderer] interface.
func (n *NotebookEditToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
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

	// Try to get resolved info from the result metadata.
	var out notebookEditOutput
	if opts.HasResult() && opts.Result.Metadata != "" {
		_ = json.Unmarshal([]byte(opts.Result.Metadata), &out)
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

	// Delete mode renders the notebook diff so the removed cell is visible.
	if editMode == "delete" {
		if out.OriginalFile != "" || out.UpdatedFile != "" {
			body := toolOutputDiffContent(sty, prettyPath, out.OriginalFile, out.UpdatedFile, cappedWidth, opts.ExpandedContent)
			return joinToolParts(header, body)
		}
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

// ── notebook_create renderer ──────────────────────────────────────────────────

// NotebookCreateToolRenderContext renders notebook_create tool messages.
type NotebookCreateToolRenderContext struct{}

type notebookCreateInput struct {
	NotebookPath string `json:"notebook_path"`
	Kernel       string `json:"kernel,omitempty"`
	Language     string `json:"language,omitempty"`
	Cells        []struct {
		CellType string `json:"cell_type"`
		Source   string `json:"source"`
	} `json:"cells,omitempty"`
}

type notebookCreateOutput struct {
	NotebookPath string `json:"notebook_path"`
	Kernel       string `json:"kernel"`
	Language     string `json:"language"`
	CellCount    int    `json:"cell_count"`
	Error        string `json:"error,omitempty"`
}

func (n *NotebookCreateToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Create Notebook", opts.Anim, opts.Compact)
	}

	var params notebookCreateInput
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Create Notebook", cappedWidth)
	}

	prettyPath := fsext.PrettyPath(params.NotebookPath)

	var out notebookCreateOutput
	if opts.HasResult() && opts.Result.Metadata != "" {
		_ = json.Unmarshal([]byte(opts.Result.Metadata), &out)
		if out.Error != "" {
			header := toolHeader(sty, opts.Status, "Create Notebook", cappedWidth, opts.Compact, prettyPath)
			errBody := sty.Tool.Body.Render(sty.Tool.StateCancelled.Render(out.Error))
			return joinToolParts(header, errBody)
		}
	}

	header := toolHeader(sty, opts.Status, "Create Notebook", cappedWidth, opts.Compact, prettyPath)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	return header
}

// ── notebook_write renderer ───────────────────────────────────────────────────

// NotebookWriteToolRenderContext renders notebook_write tool messages.
type NotebookWriteToolRenderContext struct{}

type notebookWriteInput struct {
	NotebookPath string `json:"notebook_path"`
	Kernel       string `json:"kernel,omitempty"`
	Language     string `json:"language,omitempty"`
	Cells        []struct {
		CellType string `json:"cell_type"`
		Source   string `json:"source"`
	} `json:"cells"`
}

type notebookWriteOutput struct {
	NotebookPath  string `json:"notebook_path"`
	Kernel        string `json:"kernel"`
	Language      string `json:"language"`
	CellCount     int    `json:"cell_count"`
	CodeCells     int    `json:"code_cells"`
	MarkdownCells int    `json:"markdown_cells"`
	Overwritten   bool   `json:"overwritten"`
	Error         string `json:"error,omitempty"`
}

func (n *NotebookWriteToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Write Notebook", opts.Anim, opts.Compact)
	}

	var params notebookWriteInput
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Write Notebook", cappedWidth)
	}

	prettyPath := fsext.PrettyPath(params.NotebookPath)
	action := "create"
	lang := params.Language
	cellCount := len(params.Cells)

	var out notebookWriteOutput
	if opts.HasResult() && opts.Result.Metadata != "" {
		_ = json.Unmarshal([]byte(opts.Result.Metadata), &out)
		if out.Error != "" {
			header := toolHeader(sty, opts.Status, "Write Notebook", cappedWidth, opts.Compact, prettyPath)
			errBody := sty.Tool.Body.Render(sty.Tool.StateCancelled.Render(out.Error))
			return joinToolParts(header, errBody)
		}
		if out.Overwritten {
			action = "overwrite"
		}
		if out.CellCount > 0 {
			cellCount = out.CellCount
		}
		if out.Language != "" {
			lang = out.Language
		}
	}

	// Embed action and cell count directly in the main param.
	mainParam := fmt.Sprintf("%s · %s · %d cells", prettyPath, action, cellCount)
	if cellCount == 0 {
		mainParam = fmt.Sprintf("%s · %s", prettyPath, action)
	}

	header := toolHeader(sty, opts.Status, "Write Notebook", cappedWidth, opts.Compact, mainParam)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}

	// Show the first non-empty cell as a code preview.
	for _, c := range params.Cells {
		if c.Source != "" {
			cellLang := cellTypeToLanguage(c.CellType, lang)
			fakeFile := "cell." + cellLang
			body := toolOutputCodeContent(sty, fakeFile, c.Source, 0, cappedWidth, opts.ExpandedContent)
			return joinToolParts(header, body)
		}
	}
	return header
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
