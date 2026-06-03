package mcp

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const ListMcpResourcesToolName = "mcp_list_resources"
const ReadMcpResourceToolName = "mcp_read_resource"

type Wrapper struct {
	client     *Client
	serverName string
	nameMode   ToolNameMode
}

func NewWrapper(client *Client, serverName string, options *IntegrationOptions) *Wrapper {
	resolved := normalizeIntegrationOptions(options)
	if client != nil {
		client.SetToolNameMode(resolved.ToolNameMode)
	}
	return &Wrapper{client: client, serverName: serverName, nameMode: resolved.ToolNameMode}
}

func (w *Wrapper) WrapTool(mcpTool Tool) (tool.Tool, error) {
	inputSchema := w.buildInputSchema(mcpTool.InputSchema)
	def := tool.Definition{
		Name:               exposedToolName(w.serverName, mcpTool.Name, w.nameMode),
		DisplayName:        mcpTool.Name,
		Description:        mcpTool.Description,
		Category:           "mcp",
		InputSchema:        inputSchema,
		IsReadOnly:         isReadOnlyMCPTool(mcpTool.Name),
		IsDestructive:      false,
		IsConcurrencySafe:  true,
		RequiresPermission: true,
		Metadata: map[string]any{
			"mcp_server":  w.serverName,
			"mcp_tool":    mcpTool.Name,
			"mcp_wrapper": ToolWrapperMetadata{ServerName: w.serverName, ToolName: mcpTool.Name, NameMode: w.nameMode},
		},
	}
	handler := func(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
		// Build a progress callback that forwards MCP progress notifications as
		// RuntimeEvents so SSE clients receive live updates during long tool calls.
		var onProgress ProgressCallback
		if emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent)); ok && emitter != nil {
			serverName := w.serverName
			toolDisplayName := mcpTool.Name
			onProgress = func(progress float64, total *float64, message string) {
				percent := progress * 100
				if total != nil && *total > 0 {
					percent = (progress / *total) * 100
				}
				meta := map[string]any{"mcp_server": serverName}
				if total != nil {
					meta["total"] = *total
				}
				emitter(types.RuntimeEvent{
					Type: types.RuntimeEventTypeToolProgress,
					ToolProgress: &types.ToolProgress{
						ToolName:        toolDisplayName,
						Stage:           types.ToolProgressStageRunning,
						Message:         message,
						PercentComplete: percent,
						Metadata:        meta,
					},
				})
			}
		}

		result, err := w.client.CallToolWithProgress(ctx, mcpTool.Name, input.Parsed, onProgress)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		canonical := normalizeToolCallResult(result)
		resultStr, err := w.formatMCPResult(&canonical)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		return tool.NewTextResult(resultStr), nil
	}
	aliases := w.generateAliases(mcpTool)
	builtTool, err := tool.NewBuilder(def.Name).
		WithDescription(def.Description).
		WithCategory(def.Category).
		WithInputSchema(def.InputSchema).
		WithAliases(aliases...).
		WithHandler(handler).
		Build()
	if err != nil {
		return nil, err
	}
	return builtTool, nil
}

func (w *Wrapper) WrapTools(mcpTools []Tool) ([]tool.Tool, error) {
	wrapped := make([]tool.Tool, 0, len(mcpTools))
	for _, mcpTool := range mcpTools {
		wrappedTool, err := w.WrapTool(mcpTool)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap tool '%s': %w", mcpTool.Name, err)
		}
		wrapped = append(wrapped, wrappedTool)
	}
	return wrapped, nil
}

func (w *Wrapper) formatMCPResult(result *ToolCallResult) (string, error) {
	if result == nil {
		return "", nil
	}
	if result.Text != "" {
		return result.Text, nil
	}
	if result.Structured != nil {
		return fmt.Sprintf("%v", result.Structured), nil
	}
	if result.Raw != nil {
		return fmt.Sprintf("%v", result.Raw), nil
	}
	return "", nil
}

func (w *Wrapper) buildInputSchema(mcpSchema map[string]any) schema.JSONSchema {
	if mcpSchema == nil {
		return schema.JSONSchema{Type: "object"}
	}
	rawSchema := map[string]any{"type": "object", "properties": mcpSchema}
	if required, ok := mcpSchema["required"].([]string); ok {
		rawSchema["required"] = required
	}
	return schema.FromMap(rawSchema)
}

func (w *Wrapper) generateAliases(mcpTool Tool) []string {
	aliases := []string{mcpTool.Name, fmt.Sprintf("%s.%s", w.serverName, mcpTool.Name)}
	if w.nameMode == ToolNameModeUnprefixed {
		aliases = append(aliases, prefixedToolName(w.serverName, mcpTool.Name))
	}
	return aliases
}

func (w *Wrapper) WrapResource(resource Resource) (tool.Tool, error) {
	def := tool.Definition{
		Name:               w.prefixResourceName(resource.URI),
		DisplayName:        resource.Name,
		Description:        fmt.Sprintf("Read MCP resource: %s", resource.Description),
		Category:           "mcp_resource",
		InputSchema:        schema.FromMap(map[string]any{"type": "object", "properties": map[string]any{"uri": map[string]any{"type": "string", "description": "Resource URI", "default": resource.URI}}}),
		IsReadOnly:         true,
		IsDestructive:      false,
		IsConcurrencySafe:  true,
		RequiresPermission: true,
		Metadata: map[string]any{
			"mcp_server":   w.serverName,
			"mcp_resource": resource.URI,
			"mime_type":    resource.MimeType,
		},
	}
	handler := func(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
		uri := resource.URI
		if input.Parsed != nil {
			if u, ok := input.Parsed["uri"].(string); ok {
				uri = u
			}
		}
		readResult, err := w.client.CanonicalReadResource(ctx, uri)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		return tool.NewTextResult(w.formatResourceContents(readResult.Contents)), nil
	}
	builtTool, err := tool.NewBuilder(def.Name).
		WithDescription(def.Description).
		WithCategory(def.Category).
		WithInputSchema(def.InputSchema).
		ReadOnly().
		WithHandler(handler).
		Build()
	if err != nil {
		return nil, err
	}
	return builtTool, nil
}

func (w *Wrapper) WrapResources(resources []Resource) ([]tool.Tool, error) {
	wrapped := make([]tool.Tool, 0, len(resources))
	for _, resource := range resources {
		wrappedResource, err := w.WrapResource(resource)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap resource '%s': %w", resource.URI, err)
		}
		wrapped = append(wrapped, wrappedResource)
	}
	return wrapped, nil
}

func (w *Wrapper) formatResourceContents(contents []ResourceContent) string {
	result := ""
	for _, content := range contents {
		if content.Text != "" {
			result += content.Text
		} else if content.Blob != "" {
			result += fmt.Sprintf("[Binary data: %d bytes]", len(content.Blob))
		}
		if content.MimeType != "" {
			result += fmt.Sprintf("\n(MIME type: %s)", content.MimeType)
		}
		result += "\n\n"
	}
	return result
}

func (w *Wrapper) prefixResourceName(uri string) string {
	return fmt.Sprintf("mcp_%s_resource_%s", sanitizeName(w.serverName), sanitizeName(uri))
}

func (w *Wrapper) WrapPrompt(prompt Prompt) (tool.Tool, error) {
	def := tool.Definition{
		Name:               w.prefixPromptName(prompt.Name),
		DisplayName:        prompt.Name,
		Description:        fmt.Sprintf("Get MCP prompt: %s", prompt.Description),
		Category:           "mcp_prompt",
		InputSchema:        w.buildPromptInputSchema(prompt),
		IsReadOnly:         true,
		IsDestructive:      false,
		IsConcurrencySafe:  true,
		RequiresPermission: true,
		Metadata: map[string]any{
			"mcp_server": w.serverName,
			"mcp_prompt": prompt.Name,
		},
	}
	handler := func(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
		arguments := make(map[string]any)
		if input.Parsed != nil {
			arguments = input.Parsed
		}
		promptResult, err := w.client.CanonicalPrompt(ctx, prompt.Name, arguments)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		if len(promptResult.Messages) == 0 {
			return tool.NewTextResult(""), nil
		}
		message := promptResult.Messages[0]
		return tool.NewTextResult(fmt.Sprintf("[%s] %v", message.Role, message.Content)), nil
	}
	builtTool, err := tool.NewBuilder(def.Name).
		WithDescription(def.Description).
		WithCategory(def.Category).
		WithInputSchema(def.InputSchema).
		ReadOnly().
		WithHandler(handler).
		Build()
	if err != nil {
		return nil, err
	}
	return builtTool, nil
}

func (w *Wrapper) buildPromptInputSchema(prompt Prompt) schema.JSONSchema {
	properties := make(map[string]any)
	required := make([]string, 0)
	for _, arg := range prompt.Arguments {
		properties[arg.Name] = map[string]any{"type": "string", "description": arg.Description}
		if arg.Required {
			required = append(required, arg.Name)
		}
	}
	return schema.FromMap(map[string]any{"type": "object", "properties": properties, "required": required})
}

func (w *Wrapper) prefixPromptName(name string) string {
	return fmt.Sprintf("mcp_%s_prompt_%s", sanitizeName(w.serverName), sanitizeName(name))
}

func (w *Wrapper) WrapListResourcesTool() (tool.Tool, error) {
	def := tool.Definition{
		Name:              ListMcpResourcesToolName,
		DisplayName:       "ListMcpResources",
		Description:       "List resources exposed by the MCP server",
		Category:          "mcp_resource",
		InputSchema:       schema.FromMap(map[string]any{"type": "object"}),
		IsReadOnly:        true,
		IsConcurrencySafe: true,
	}
	handler := func(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
		resources, err := w.client.ListResources(ctx)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		return tool.NewTextResult(fmt.Sprintf("%v", resources)), nil
	}
	builtTool, err := tool.NewBuilder(def.Name).
		WithDescription(def.Description).
		WithCategory(def.Category).
		WithInputSchema(def.InputSchema).
		ReadOnly().
		WithHandler(handler).
		Build()
	if err != nil {
		return nil, err
	}
	return builtTool, nil
}

func (w *Wrapper) WrapReadResourceTool() (tool.Tool, error) {
	def := tool.Definition{
		Name:              ReadMcpResourceToolName,
		DisplayName:       "ReadMcpResource",
		Description:       "Read a resource exposed by the MCP server",
		Category:          "mcp_resource",
		InputSchema:       schema.FromMap(map[string]any{"type": "object", "properties": map[string]any{"uri": map[string]any{"type": "string"}}, "required": []string{"uri"}}),
		IsReadOnly:        true,
		IsConcurrencySafe: true,
	}
	handler := func(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
		uri, _ := input.Parsed["uri"].(string)
		readResult, err := w.client.CanonicalReadResource(ctx, uri)
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		return tool.NewTextResult(w.formatResourceContents(readResult.Contents)), nil
	}
	builtTool, err := tool.NewBuilder(def.Name).
		WithDescription(def.Description).
		WithCategory(def.Category).
		WithInputSchema(def.InputSchema).
		ReadOnly().
		WithHandler(handler).
		Build()
	if err != nil {
		return nil, err
	}
	return builtTool, nil
}

func (w *Wrapper) WrapAll(ctx context.Context) ([]tool.Tool, error) {
	allTools := make([]tool.Tool, 0)
	mcpTools, err := w.client.ListTools(ctx)
	if err == nil && len(mcpTools) > 0 {
		wrappedTools, err := w.WrapTools(mcpTools)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap tools: %w", err)
		}
		allTools = append(allTools, wrappedTools...)
	}
	resources, err := w.client.ListResources(ctx)
	if err == nil && len(resources) > 0 {
		wrappedResources, err := w.WrapResources(resources)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap resources: %w", err)
		}
		allTools = append(allTools, wrappedResources...)
	}
	prompts, err := w.client.ListPrompts(ctx)
	if err == nil && len(prompts) > 0 {
		for _, prompt := range prompts {
			wrappedPrompt, err := w.WrapPrompt(prompt)
			if err != nil {
				return nil, fmt.Errorf("failed to wrap prompt '%s': %w", prompt.Name, err)
			}
			allTools = append(allTools, wrappedPrompt)
		}
	}
	listResourcesTool, err := w.WrapListResourcesTool()
	if err == nil {
		allTools = append(allTools, listResourcesTool)
	}
	readResourceTool, err := w.WrapReadResourceTool()
	if err == nil {
		allTools = append(allTools, readResourceTool)
	}
	return allTools, nil
}
