package toolsearch

import (
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
)

// buildSearchText constructs the BM25-indexed text for a single tool.
// Mirrors Codex's default_tool_search_text() + append_function_search_text()
// + append_schema_search_text() from tools/src/tool_search.rs.
//
// Indexed fields (in order):
//  1. tool name (raw)
//  2. tool name with underscores → spaces ("read_file" also matches "read file")
//  3. category / namespace
//  4. SearchHint (short ranking hint)
//  5. DisplayName (if different)
//  6. Description (full)
//  7. parameter names + their descriptions (recursive into schema.Properties)
func buildSearchText(def contract.Definition) string {
	var parts []string
	push := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}

	push(def.Name)
	push(strings.ReplaceAll(def.Name, "_", " "))
	push(def.Category)
	push(def.SearchHint)
	if def.DisplayName != "" && def.DisplayName != def.Name {
		push(def.DisplayName)
		push(strings.ReplaceAll(def.DisplayName, "_", " "))
	}
	push(def.Description)
	appendSchemaSearchText(def.InputSchema, &parts)

	return strings.Join(parts, " ")
}

// appendSchemaSearchText recursively extracts parameter names and descriptions.
// Mirrors Codex's append_schema_search_text().
func appendSchemaSearchText(s schema.JSONSchema, parts *[]string) {
	push := func(v string) {
		if v = strings.TrimSpace(v); v != "" {
			*parts = append(*parts, v)
		}
	}
	push(s.Description)
	for name, prop := range s.Properties {
		push(name)
		push(strings.ReplaceAll(name, "_", " "))
		appendSchemaSearchText(prop, parts)
	}
	if s.Items != nil {
		appendSchemaSearchText(*s.Items, parts)
	}
}

// toolNamespace classifies a tool into a namespace string.
// Mirrors Codex's ToolExposure / source distinction:
// "builtin" | "deferred" | "mcp" | "dynamic"
func toolNamespace(def contract.Definition) string {
	if def.IsMCP {
		return "mcp"
	}
	if def.ShouldDefer {
		return "deferred"
	}
	if def.Category != "" {
		return def.Category
	}
	return "builtin"
}
