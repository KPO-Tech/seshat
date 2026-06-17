package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/tools/special/lsp/lspClient"
	"github.com/EngineerProjects/nexus-engine/internal/tools/special/lsp/lspServerManager"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ToolName is the name of the tool
const ToolName = "lsp"

// SearchHint is the search hint for the tool
const SearchHint = "lsp:definition|lsp:references|lsp:hover|lsp:symbols|lsp:implementations|lsp:incoming_calls|lsp:outgoing_calls|lsp:workspace_symbol"

const Description = `
This tool provides Language Server Protocol (LSP) features for code intelligence.

Available operations:

1. definition - Go to definition of a symbol
   Find the definition of a symbol at the cursor position.

2. references - Find all references to a symbol
   Find all references to a symbol at the cursor position.

3. hover - Get hover information
   Get hover information (documentation, type) for a symbol.

4. symbols - Get document symbols
   List all symbols in the current document (classes, functions, variables, etc.).

5. implementations - Find implementations
   Find all implementations of an interface or abstract method.

6. incoming_calls - Find incoming calls (call hierarchy)
   Find all callers of a function (call hierarchy).

7. outgoing_calls - Find outgoing calls (call hierarchy)
   Find all callees of a function (call hierarchy).

8. workspace_symbol - Search workspace symbols
   Search for symbols across the entire workspace.

Each operation requires a file path and optionally a line/column position.
For symbols/workspace_symbol, position is optional.
`

// Tool implements the LSP tool for code intelligence
type Tool struct {
	// manager is the LSP server manager
	manager *lspServerManager.Manager
	// workingDir is the current working directory
	workingDir string
}

// NewLspTool creates a new LSP tool
func NewLspTool(workingDir string) *Tool {
	return &Tool{
		manager:    lspServerManager.NewManager(workingDir),
		workingDir: workingDir,
	}
}

// Definition returns the tool definition
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "LSP",
		SearchHint:  SearchHint,
		Description: Description,
		Category:    "code_intelligence",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"definition", "references", "hover", "symbols", "implementations", "incoming_calls", "outgoing_calls", "workspace_symbol"},
					"description": "The LSP operation to perform",
				},
				"file_path": map[string]any{
					"type":        "string",
					"description": "File path to perform the operation on",
				},
				"line": map[string]any{
					"type":        "number",
					"description": "Line number (1-based)",
				},
				"column": map[string]any{
					"type":        "number",
					"description": "Column number (1-based)",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Query for workspace_symbol operation",
				},
				"server": map[string]any{
					"type":        "string",
					"description": "Optional: specific LSP server to use (e.g., gopls, rust-analyzer, pyright)",
				},
			},
			"required": []string{"operation", "file_path"},
		}),
		Metadata: map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

// Call executes the tool
func (t *Tool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	// Parse input
	operation, ok := parsed["operation"].(string)
	if !ok || operation == "" {
		return tool.CallResult{
			Error: fmt.Errorf("operation is required"),
		}, nil
	}

	filePath, ok := parsed["file_path"].(string)
	if !ok || filePath == "" {
		return tool.CallResult{
			Error: fmt.Errorf("file_path is required"),
		}, nil
	}

	// Resolve file path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(t.workingDir, filePath)
	}

	// Get line and column
	line := 1
	column := 1
	if l, ok := parsed["line"].(float64); ok {
		line = int(l)
	}
	if c, ok := parsed["column"].(float64); ok {
		column = int(c)
	}

	// Get query for workspace_symbol
	query := ""
	if q, ok := parsed["query"].(string); ok {
		query = q
	}

	// Ensure the file is open in the LSP server
	if err := t.manager.OpenFile(ctx, filePath); err != nil {
		return tool.CallResult{
			Error: fmt.Errorf("failed to open file in LSP server: %w", err),
		}, nil
	}

	// Perform the operation
	var result any
	var err error

	switch operation {
	case "definition":
		result, err = t.doDefinition(ctx, filePath, line, column)
	case "references":
		result, err = t.doReferences(ctx, filePath, line, column)
	case "hover":
		result, err = t.doHover(ctx, filePath, line, column)
	case "symbols":
		result, err = t.doSymbols(ctx, filePath)
	case "implementations":
		result, err = t.doImplementations(ctx, filePath, line, column)
	case "incoming_calls":
		result, err = t.doIncomingCalls(ctx, filePath, line, column)
	case "outgoing_calls":
		result, err = t.doOutgoingCalls(ctx, filePath, line, column)
	case "workspace_symbol":
		result, err = t.doWorkspaceSymbol(ctx, filePath, query)
	default:
		return tool.CallResult{
			Error: fmt.Errorf("unknown operation: %s", operation),
		}, nil
	}

	if err != nil {
		return tool.CallResult{
			Error: err,
		}, nil
	}

	// Format the result
	return tool.CallResult{
		Data: t.formatResult(operation, result),
	}, nil
}

// Description returns a human-readable description
func (t *Tool) Description(ctx context.Context) (string, error) {
	return Description, nil
}

// ValidateInput validates the input
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions
func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only
func (t *Tool) IsReadOnly(input map[string]any) bool {
	return true
}

// IsEnabled returns whether the tool is enabled
func (t *Tool) IsEnabled() bool {
	return true
}

// FormatResult formats the result
func (t *Tool) FormatResult(data any) string {
	return fmt.Sprintf("%v", data)
}

// BackfillInput backfills input
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

func (t *Tool) doDefinition(ctx context.Context, filePath string, line, column int) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)
	position := lspClient.Position{Line: line - 1, Character: column - 1}

	locations, err := client.TextDocumentDefinition(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("definition request failed: %w", err)
	}

	return locations, nil
}

func (t *Tool) doReferences(ctx context.Context, filePath string, line, column int) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)
	position := lspClient.Position{Line: line - 1, Character: column - 1}

	locations, err := client.TextDocumentReferences(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("references request failed: %w", err)
	}

	return locations, nil
}

func (t *Tool) doHover(ctx context.Context, filePath string, line, column int) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)
	position := lspClient.Position{Line: line - 1, Character: column - 1}

	hover, err := client.TextDocumentHover(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("hover request failed: %w", err)
	}

	return hover, nil
}

func (t *Tool) doSymbols(ctx context.Context, filePath string) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)

	symbols, err := client.TextDocumentDocumentSymbol(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("document symbols request failed: %w", err)
	}

	return symbols, nil
}

func (t *Tool) doImplementations(ctx context.Context, filePath string, line, column int) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)
	position := lspClient.Position{Line: line - 1, Character: column - 1}

	locations, err := client.TextDocumentImplementation(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("implementations request failed: %w", err)
	}

	return locations, nil
}

func (t *Tool) doIncomingCalls(ctx context.Context, filePath string, line, column int) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)
	position := lspClient.Position{Line: line - 1, Character: column - 1}

	// First get the call hierarchy item
	items, err := client.TextDocumentPrepareCallHierarchy(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("prepare call hierarchy failed: %w", err)
	}

	if len(items) == 0 {
		return []any{}, nil
	}

	// Then get incoming calls
	incoming, err := client.CallHierarchyIncomingCalls(ctx, items[0])
	if err != nil {
		return nil, fmt.Errorf("incoming calls request failed: %w", err)
	}

	return incoming, nil
}

func (t *Tool) doOutgoingCalls(ctx context.Context, filePath string, line, column int) (any, error) {
	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	uri := lspClient.URIFromPath(filePath)
	position := lspClient.Position{Line: line - 1, Character: column - 1}

	// First get the call hierarchy item
	items, err := client.TextDocumentPrepareCallHierarchy(ctx, uri, position)
	if err != nil {
		return nil, fmt.Errorf("prepare call hierarchy failed: %w", err)
	}

	if len(items) == 0 {
		return []any{}, nil
	}

	// Then get outgoing calls
	outgoing, err := client.CallHierarchyOutgoingCalls(ctx, items[0])
	if err != nil {
		return nil, fmt.Errorf("outgoing calls request failed: %w", err)
	}

	return outgoing, nil
}

func (t *Tool) doWorkspaceSymbol(ctx context.Context, filePath string, query string) (any, error) {
	if query == "" {
		return nil, fmt.Errorf("query is required for workspace_symbol operation")
	}

	server, err := t.manager.InitializeForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}

	client := server.Client
	if client == nil {
		return nil, fmt.Errorf("server %s not initialized", server.Name)
	}

	symbols, err := client.WorkspaceSymbol(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("workspace symbol request failed: %w", err)
	}

	return symbols, nil
}

func (t *Tool) formatResult(operation string, result any) map[string]any {
	output := map[string]any{
		"operation": operation,
		"result":    result,
	}

	// Add human-readable summary
	switch operation {
	case "definition", "references", "implementations":
		if locations, ok := result.([]lspClient.Location); ok {
			output["summary"] = fmt.Sprintf("Found %d location(s)", len(locations))
		} else {
			output["summary"] = "No results"
		}
	case "hover":
		if hover, ok := result.(*lspClient.Hover); ok && hover != nil {
			output["summary"] = formatHover(hover)
		} else {
			output["summary"] = "No hover information"
		}
	case "symbols":
		if symbols, ok := result.([]lspClient.DocumentSymbol); ok {
			output["summary"] = fmt.Sprintf("Found %d symbol(s)", countSymbols(symbols))
		} else {
			output["summary"] = "No symbols found"
		}
	case "incoming_calls", "outgoing_calls":
		output["summary"] = formatCallHierarchy(result)
	case "workspace_symbol":
		if symbols, ok := result.([]lspClient.SymbolInformation); ok {
			output["summary"] = fmt.Sprintf("Found %d symbol(s)", len(symbols))
		} else {
			output["summary"] = "No symbols found"
		}
	}

	return output
}

func formatHover(hover *lspClient.Hover) string {
	if hover == nil {
		return "No hover information"
	}

	switch contents := hover.Contents.(type) {
	case string:
		return strings.TrimSpace(contents)
	case map[string]any:
		if value, ok := contents["value"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return "Hover information available"
}

func countSymbols(symbols []lspClient.DocumentSymbol) int {
	count := len(symbols)
	for _, sym := range symbols {
		count += countSymbols(sym.Children)
	}
	return count
}

func formatCallHierarchy(result any) string {
	return "Call hierarchy information available"
}

// ParsePosition parses a position string (line:column format)
func ParsePosition(pos string) (line, column int, err error) {
	parts := strings.Split(pos, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid position format, expected line:column")
	}

	line, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid line number: %w", err)
	}

	column, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid column number: %w", err)
	}

	return line, column, nil
}
