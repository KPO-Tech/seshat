package toolsearch

import "github.com/EngineerProjects/nexus-engine/internal/tools/registry"

const ToolSearchToolName = "tool_search"

// DefaultMaxResults mirrors Codex's TOOL_SEARCH_DEFAULT_LIMIT = 8.
const DefaultMaxResults = 8

const ToolSearchEnvVar = "ENABLE_TOOL_SEARCH"

type ToolSearchMode string

const (
	ToolSearchModeTST      ToolSearchMode = "tst"
	ToolSearchModeTSTAuto  ToolSearchMode = "tst-auto"
	ToolSearchModeStandard ToolSearchMode = "standard"
)

const DefaultAutoThresholdPercent = 10

// ToolSearchDescription is the model-facing description of the tool_search tool.
// Mirrors Codex's create_tool_search_tool() description mentioning BM25 explicitly.
const ToolSearchDescription = `# Tool discovery

Searches over deferred tool metadata with BM25 and returns matching tools for the next call.

Some tools are not loaded upfront to keep the context window small. Use this tool to find them by keyword before calling them.

## Query forms
- ` + "`select:Name1,Name2`" + ` — fetch these exact tools by name (bypass BM25)
- ` + "`read file`" + ` — free-text BM25 search, returns up to ` + "`max_results`" + ` ranked matches
- ` + "`+slack send`" + ` — ` + "`+`" + `-prefixed terms are required; remaining terms are used for ranking

## When to use
- Before calling a tool that is not visible in your current tool list
- When you need an MCP tool and are unsure of its exact name
- To discover tools by capability ("image generation", "calendar", "git diff")`

type ToolSearchConfig struct {
	Registry *registry.Registry
}

func DefaultToolSearchToolConfig(reg *registry.Registry) *ToolSearchConfig {
	return &ToolSearchConfig{Registry: reg}
}
