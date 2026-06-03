package webfetch

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	fetchcore "github.com/EngineerProjects/nexus-engine/internal/web/fetch"
)

const SearchHint = "fetch web pages and extract information"

// ToolName is the tool name.
const ToolName = "web_fetch"

// Description is the tool description.
const Description = `Fetch content from a specified URL and process it using a prompt-oriented extraction.

Core behavior:
- Takes a URL and an extraction prompt as input
- Fetches the URL content and converts HTML to markdown when needed
- Returns a structured result suitable for the engine and transcript
- Uses an in-memory cache for repeated requests

Use this tool when you already have a specific URL and need to extract information from that page.

Prefer WebFetch over WebSearch when:
- you already know the page to inspect
- you need details from one or a few exact pages
- you want a targeted extraction prompt instead of broad discovery

Do not use it when:
- you still need to discover relevant URLs first
- the answer should come from the local repository
- the target is a GitHub resource better handled by CLI or dedicated APIs

Usage guidance:
- provide a precise extraction prompt describing exactly what you need
- keep the prompt focused on the page, not on general background knowledge
- ask for concrete fields, statements, dates, compatibility notes, or quoted policy details when those matter
- if the page redirects, follow the returned redirect URL in a new request
- for GitHub URLs, prefer gh via Bash when appropriate
- if an MCP-provided fetch tool exists, prefer it when it is a better fit
- when verifying a sensitive claim, use this tool to confirm the exact wording or details on the source page

Good prompt:
- "Extract the authentication flow described on this page and list the required request/response fields."
- "Extract the documented rate limits, deprecation notice, and effective dates from this page."

Bad prompt:
- "Tell me everything about this website."

Usage notes:
- The URL must be a fully-formed valid URL
- HTTP URLs are automatically upgraded to HTTPS
- The prompt should describe what information you want to extract from the page
- This tool is read-only and does not modify any files
- When a URL redirects to a different host, the tool stops and reports the redirect target
- Preapproved documentation/code hosts can be allowed without prompting depending on the engine policy`

// MaxPromptPreviewLength limits the amount of source content included in the extracted result.
const MaxPromptPreviewLength = 4000

// Tool represents the WebFetch tool.
type Tool struct {
	config  *Config
	service *fetchcore.Service
}

// DefaultToolConfig returns default configuration.
func DefaultToolConfig() *Config {
	return DefaultConfig()
}

// NewTool creates a new WebFetch tool.
func NewTool(config *Config) *Tool {
	if config == nil {
		config = DefaultConfig()
	}

	t := &Tool{
		config:  config,
		service: fetchcore.NewService(config),
	}
	return t
}

// Definition returns the tool definition.
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		Aliases:     []string{"web_fetch"},
		DisplayName: "WebFetch",
		SearchHint:  SearchHint,
		Description: Description,
		Category:    "web",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch content from",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The prompt used to extract or summarize relevant content",
				},
				"render_mode": map[string]any{
					"type":        "string",
					"description": "Optional fetch backend. Use auto or http for the fast HTTP path, or browser for JS-rendered pages.",
				},
			},
			"required": []string{"url", "prompt"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

// Description returns human-readable description.
func (t *Tool) Description(ctx context.Context) (string, error) {
	return Description, nil
}

// ValidateInput validates and normalizes WebFetch input.
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	_ = ctx
	parsedInput, err := parseInput(input)
	if err != nil {
		return nil, err
	}
	if err := parsedInput.Validate(); err != nil {
		return nil, err
	}
	return input, nil
}

// IsConcurrencySafe reports that WebFetch requests can run concurrently.
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	_ = input
	return true
}

// IsReadOnly reports that WebFetch does not modify state.
func (t *Tool) IsReadOnly(input map[string]any) bool {
	_ = input
	return true
}

// IsEnabled returns whether this tool is currently active.
func (t *Tool) IsEnabled() bool { return true }

// FormatResult serialises the tool output into the tool_result content string.
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}
