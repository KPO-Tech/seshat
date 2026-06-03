package websearch

import (
	"context"
	"fmt"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	searchcore "github.com/EngineerProjects/nexus-engine/internal/web/search"
	searchproviders "github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
)

const SearchHint = "search the web for current information"

const ToolDescription = `Search the web for current information using a configured search provider. Supports multiple providers (Tavily, Exa, Jina, SearXNG, DuckDuckGo) with automatic fallback. Use WEB_SEARCH_PROVIDER to select a specific provider.

Use this tool when you need current or external information that is not reliably available from repository state alone.

Good uses:
- current events, prices, releases, policies, documentation, news, provider changes
- finding candidate sources before using a more targeted fetch
- gathering a small set of relevant links for the user
- verifying a factual claim before presenting it confidently

Do not use it when:
- the answer should come from the local codebase
- you already have the exact page and should use WebFetch instead
- a simpler local tool can answer the question

Execution guidance:
- start with one precise query before broadening
- include the current year for latest or recent topics
- use domain filters when the user wants official or specific sources
- prefer fewer high-signal searches over many redundant ones
- for sensitive or high-impact claims, prefer official or primary sources first
- do not treat search snippets alone as proof when the exact page should be checked

Suggested workflow:
1. Search to find the best candidate sources.
2. If the claim is important, verify the exact page with WebFetch or browser tools.
3. Only then present the conclusion as established fact.

After using this tool, include a Sources section in the final answer with relevant URLs as markdown links.`

// RunnerFn is an optional hook that replaces the env-based provider chain.
// It receives the query and domain filters and must return a full Output.
// When nil the tool falls back to the env-configured provider chain.
type RunnerFn func(ctx context.Context, query string, allowedDomains, blockedDomains []string) (Output, error)

// Tool implements the WebSearch tool.
type Tool struct {
	service *searchcore.Service
	runner  RunnerFn
}

// SetRunner installs a runtime runner that overrides the env-based provider chain.
// Pass nil to restore the default env-based behaviour.
func (t *Tool) SetRunner(fn RunnerFn) {
	t.runner = fn
}

// NewTool creates a new WebSearch tool.
func NewTool() *Tool {
	return &Tool{
		service: searchcore.NewService(),
	}
}

// NewToolWithProvider creates a WebSearch tool with an explicit provider.
// Deprecated: Use the provider chain system instead.
func NewToolWithProvider(provider searchproviders.SearchProvider) *Tool {
	_ = provider
	return NewTool()
}

// Definition returns the tool definition.
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		Aliases:     []string{"web_search"},
		DisplayName: "Web Search",
		SearchHint:  SearchHint,
		Description: ToolDescription,
		Category:    "web",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query to use",
				},
				"allowed_domains": map[string]any{
					"type":        "array",
					"description": "Only include search results from these domains",
					"items":       map[string]any{"type": "string"},
				},
				"blocked_domains": map[string]any{
					"type":        "array",
					"description": "Never include search results from these domains",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"query"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: true,
	}
}

// Call executes the web search using the provider chain.
func (t *Tool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsedInput, err := parseCallInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsedInput.Validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}

	if permissionCheck != nil && input.ToolContext == nil {
		permissionInput := t.permissionInput(map[string]any{
			"query":           parsedInput.Query,
			"allowed_domains": parsedInput.AllowedDomains,
			"blocked_domains": parsedInput.BlockedDomains,
		}, "")
		if policyDecision := webcore.EvaluatePermission(permissionInput); policyDecision.IsAllowed() {
			permissionCheck = nil
		}
		if permissionCheck != nil {
			permResult := permissionCheck(ctx, types.ToolPermissionRequest{
				ToolName:  ToolName,
				ToolInput: permissionInput,
			})
			if permResult.Behavior != types.PermissionBehaviorAllow {
				reason := permResult.Reason
				if reason == "" {
					reason = "web search requires approval"
				}
				return tool.NewErrorResult(fmt.Errorf("permission denied: %s", reason)), nil
			}
		}
	}

	var output Output
	if t.runner != nil {
		var err error
		output, err = t.runner(ctx, parsedInput.Query, parsedInput.AllowedDomains, parsedInput.BlockedDomains)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("web search failed: %w", err)), nil
		}
	} else {
		providerOutput, err := t.service.Search(ctx, parsedInput.Request())
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("web search failed: %w", err)), nil
		}
		output = Output(providerOutput)
	}

	result := tool.NewJSONResult(output)
	result.Content = formatOutput(output)
	result.Metadata = &tool.ResultMetadata{
		Additional: map[string]any{
			"result_count":         len(output.Results),
			"provider":             output.Provider,
			"mode":                 t.service.ProviderMode(),
			"configured_providers": searchcore.GetConfiguredProviders(),
		},
	}
	return result, nil
}

// Description returns the tool description.
func (t *Tool) Description(ctx context.Context) (string, error) {
	return ToolDescription, nil
}

// ValidateInput validates and normalizes WebSearch input.
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	_ = ctx
	parsedInput, err := parseCallInput(input)
	if err != nil {
		return nil, err
	}
	if err := parsedInput.Validate(); err != nil {
		return nil, err
	}
	return input, nil
}

// IsConcurrencySafe reports that WebSearch requests can run concurrently.
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	_ = input
	return true
}

// IsReadOnly reports that WebSearch does not modify state.
func (t *Tool) IsReadOnly(input map[string]any) bool {
	_ = input
	return true
}

// IsEnabled always returns true. The tool is always registered; it fails gracefully at
// call time if no providers are configured (env or DB-backed).
func (t *Tool) IsEnabled() bool {
	return true
}

// FormatResult serialises the tool output into the tool_result content string.
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// parseCallInput parses the raw input map into an Input struct.
func parseCallInput(parsed map[string]any) (*Input, error) {
	query, ok := parsed["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	return &Input{
		Query:          strings.TrimSpace(query),
		AllowedDomains: readStringSlice(parsed["allowed_domains"]),
		BlockedDomains: readStringSlice(parsed["blocked_domains"]),
	}, nil
}

// readStringSlice converts an interface{} to a string slice.
func readStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return append([]string(nil), strings...)
		}
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			result = append(result, strings.TrimSpace(s))
		}
	}
	return result
}

// formatOutput formats the search results for display.
func formatOutput(output Output) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Query: %s\n", output.Query))
	builder.WriteString(fmt.Sprintf("Provider: %s\n", output.Provider))
	builder.WriteString(fmt.Sprintf("Duration: %.2fs\n", output.DurationSeconds))
	builder.WriteString(fmt.Sprintf("Results: %d\n\n", len(output.Results)))
	for idx, hit := range output.Results {
		builder.WriteString(fmt.Sprintf("%d. %s\n", idx+1, hit.Title))
		builder.WriteString(fmt.Sprintf("   %s\n", hit.URL))
		if hit.Description != "" {
			builder.WriteString(fmt.Sprintf("   %s\n", hit.Description))
		}
		if hit.Source != "" {
			builder.WriteString(fmt.Sprintf("   Source: %s\n", hit.Source))
		}
		builder.WriteString("\n")
	}
	if len(output.Results) == 0 {
		builder.WriteString("No results found.\n")
	}
	return builder.String()
}

// IsEnabled reports whether WebSearch should be enabled from environment.
// Returns true if any provider is configured.
func IsEnabled() bool {
	return searchcore.IsEnabled()
}

// GetProviderMode returns the current provider mode.
func GetProviderMode() string {
	return searchcore.GetProviderMode()
}

// GetConfiguredProviders returns the list of configured providers.
func GetConfiguredProviders() []string {
	return searchcore.GetConfiguredProviders()
}
