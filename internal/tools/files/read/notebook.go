package read

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Notebook constants
const (
	// MaxNotebookCells is the maximum number of cells to read from a notebook
	MaxNotebookCells = 500

	// MaxCellOutputLength is the maximum length of cell output to include
	MaxCellOutputLength = 10000
)

// Notebook represents a parsed Jupyter notebook
type Notebook struct {
	Cells         []NotebookCell   `json:"cells"`
	Metadata      NotebookMetadata `json:"metadata"`
	NBFormat      int              `json:"nbformat"`
	NBFormatMinor int              `json:"nbformat_minor"`
}

// NotebookMetadata represents notebook metadata
type NotebookMetadata struct {
	LanguageInfo *NotebookLanguageInfo `json:"language_info,omitempty"`
	Kernelspec   *NotebookKernelspec   `json:"kernelspec,omitempty"`
	Title        string                `json:"title,omitempty"`
	Author       string                `json:"author,omitempty"`
	Description  string                `json:"description,omitempty"`
}

// NotebookLanguageInfo represents language information
type NotebookLanguageInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version,omitempty"`
	FileExtension string `json:"file_extension,omitempty"`
}

// NotebookKernelspec represents kernel specification
type NotebookKernelspec struct {
	Name     string `json:"name"`
	Display  string `json:"display,omitempty"`
	Language string `json:"language"`
}

// NotebookCell represents a cell in a Jupyter notebook
type NotebookCell struct {
	ID             string           `json:"id,omitempty"`
	CellType       string           `json:"cell_type"`
	Source         []string         `json:"source"`
	Metadata       json.RawMessage  `json:"metadata,omitempty"`
	Outputs        []NotebookOutput `json:"outputs,omitempty"`
	ExecutionCount *int             `json:"execution_count,omitempty"`
}

// NotebookOutput represents output from a code cell
type NotebookOutput struct {
	OutputType string          `json:"output_type"`
	Text       []string        `json:"text,omitempty"`
	Data       map[string]any  `json:"data,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Ename      string          `json:"ename,omitempty"`
	Evalue     string          `json:"evalue,omitempty"`
	Traceback  []string        `json:"traceback,omitempty"`
}

// NotebookResult represents the result of reading a notebook
type NotebookResult struct {
	FilePath      string         `json:"file_path"`
	Cells         []NotebookCell `json:"cells"`
	Language      string         `json:"language,omitempty"`
	CellCount     int            `json:"cell_count"`
	CodeCells     int            `json:"code_cells"`
	MarkdownCells int            `json:"markdown_cells"`
	RawCells      int            `json:"raw_cells"`
}

// IsNotebookExtension checks if a file extension is a notebook extension
func IsNotebookExtension(ext string) bool {
	return strings.ToLower(ext) == ".ipynb"
}

// ReadNotebook reads and parses a Jupyter notebook file
func ReadNotebook(filePath string) (*Notebook, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read notebook: %w", err)
	}

	// Parse JSON
	var notebook Notebook
	if err := json.Unmarshal(data, &notebook); err != nil {
		return nil, fmt.Errorf("failed to parse notebook JSON: %w", err)
	}

	// Validate nbformat
	if notebook.NBFormat < 3 || notebook.NBFormat > 5 {
		return nil, fmt.Errorf("unsupported notebook format: nbformat %d", notebook.NBFormat)
	}

	// Validate cells
	if len(notebook.Cells) > MaxNotebookCells {
		return nil, fmt.Errorf("notebook has too many cells (%d, max %d)", len(notebook.Cells), MaxNotebookCells)
	}

	return &notebook, nil
}

// ParseNotebook parses a notebook into a structured result
func ParseNotebook(notebook *Notebook, filePath string) (*NotebookResult, error) {
	result := &NotebookResult{
		FilePath:  filePath,
		Cells:     notebook.Cells,
		CellCount: len(notebook.Cells),
	}

	// Extract language from metadata
	if notebook.Metadata.LanguageInfo != nil {
		result.Language = notebook.Metadata.LanguageInfo.Name
	}

	// Count cell types
	for _, cell := range notebook.Cells {
		switch cell.CellType {
		case "code":
			result.CodeCells++
		case "markdown":
			result.MarkdownCells++
		case "raw":
			result.RawCells++
		}
	}

	return result, nil
}

// FormatNotebookCell formats a single notebook cell for display
func FormatNotebookCell(cell NotebookCell, index int) string {
	var builder strings.Builder

	// Cell header
	switch cell.CellType {
	case "code":
		if cell.ExecutionCount != nil {
			builder.WriteString(fmt.Sprintf("[Cell %d] Code (executed: %d)\n", index, *cell.ExecutionCount))
		} else {
			builder.WriteString(fmt.Sprintf("[Cell %d] Code (not executed)\n", index))
		}
	case "markdown":
		builder.WriteString(fmt.Sprintf("[Cell %d] Markdown\n", index))
	case "raw":
		builder.WriteString(fmt.Sprintf("[Cell %d] Raw\n", index))
	default:
		builder.WriteString(fmt.Sprintf("[Cell %d] %s\n", index, cell.CellType))
	}

	// Source content
	source := strings.Join(cell.Source, "")
	source = strings.TrimSpace(source)
	if source != "" {
		builder.WriteString(fmt.Sprintf("\n%s\n", source))
	}

	// Outputs for code cells
	if cell.CellType == "code" && len(cell.Outputs) > 0 {
		builder.WriteString("\n[Output]\n")
		for i, output := range cell.Outputs {
			builder.WriteString(formatNotebookOutput(output, i))
		}
	}

	builder.WriteString("\n" + strings.Repeat("-", 60) + "\n")

	return builder.String()
}

// formatNotebookOutput formats a notebook output for display
func formatNotebookOutput(output NotebookOutput, index int) string {
	var builder strings.Builder

	switch output.OutputType {
	case "stream":
		if len(output.Text) > 0 {
			text := strings.Join(output.Text, "")
			if len(text) > MaxCellOutputLength {
				text = text[:MaxCellOutputLength] + "\n... (truncated)"
			}
			builder.WriteString(fmt.Sprintf("  Stream[%d]: %s\n", index, text))
		}

	case "execute_result":
		if len(output.Data) > 0 {
			// Try to find text/plain
			if textData, ok := output.Data["text/plain"]; ok {
				text := formatTextData(textData)
				if len(text) > MaxCellOutputLength {
					text = text[:MaxCellOutputLength] + "\n... (truncated)"
				}
				builder.WriteString(fmt.Sprintf("  Result[%d]: %s\n", index, text))
			} else {
				// Just show keys
				keys := make([]string, 0, len(output.Data))
				for k := range output.Data {
					keys = append(keys, k)
				}
				builder.WriteString(fmt.Sprintf("  Result[%d]: [%s output]\n", index, strings.Join(keys, ", ")))
			}
		}

	case "error":
		builder.WriteString(fmt.Sprintf("  Error[%d]: %s: %s\n", index, output.Ename, output.Evalue))
		if len(output.Traceback) > 0 {
			traceback := strings.Join(output.Traceback, "\n")
			if len(traceback) > MaxCellOutputLength {
				traceback = traceback[:MaxCellOutputLength] + "\n... (truncated)"
			}
			builder.WriteString(fmt.Sprintf("  Traceback:\n%s\n", traceback))
		}

	default:
		builder.WriteString(fmt.Sprintf("  Unknown output type: %s\n", output.OutputType))
	}

	return builder.String()
}

// formatTextData formats text data from output
func formatTextData(data any) string {
	switch v := data.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, "")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// FormatNotebookResult formats a notebook result for display
func FormatNotebookResult(result *NotebookResult) string {
	var builder strings.Builder

	// Header
	builder.WriteString(fmt.Sprintf("Notebook: %s\n", result.FilePath))
	if result.Language != "" {
		builder.WriteString(fmt.Sprintf("Language: %s\n", result.Language))
	}
	builder.WriteString(fmt.Sprintf("Cells: %d total (%d code, %d markdown, %d raw)\n",
		result.CellCount, result.CodeCells, result.MarkdownCells, result.RawCells))
	builder.WriteString(fmt.Sprintf("\n%s\n", strings.Repeat("=", 60)))

	// Format each cell
	for i, cell := range result.Cells {
		builder.WriteString(FormatNotebookCell(cell, i+1))
	}

	return builder.String()
}
