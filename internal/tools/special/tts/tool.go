// Package ttstool provides the text_to_speech tool backed by tts.Generation.
package ttstool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/audio/tts"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	ToolName        = "text_to_speech"
	ToolDescription = `Convert text to speech audio using the configured TTS provider.

Returns a JSON object with:
- "provider": which provider synthesised the audio (e.g. "openai")
- "model": the TTS model used
- "audio_base64": base64-encoded audio data
- "content_type": MIME type of the audio (e.g. "audio/mpeg")
- "characters_used": number of input characters consumed`
)

// Tool implements the text_to_speech built-in tool.
type Tool struct{ generator tts.Generation }

// NewTool creates a text_to_speech Tool. Disabled (IsEnabled=false) when generator is nil.
func NewTool(generator tts.Generation) *Tool { return &Tool{generator: generator} }

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Text to Speech",
		Description: ToolDescription,
		Category:    "audio",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "The text to convert to speech.",
				},
			},
			"required": []string{"text"},
		}),
		Metadata: map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *Tool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	if t.generator == nil {
		return tool.CallResult{Error: fmt.Errorf("text_to_speech: no TTS provider configured")}, nil
	}
	text, _ := input.Parsed["text"].(string)
	if text == "" {
		return tool.CallResult{Error: fmt.Errorf("text_to_speech: text is required")}, nil
	}
	resp, err := t.generator.GenerateAudio(ctx, text)
	if err != nil {
		return tool.CallResult{Error: fmt.Errorf("text_to_speech: %w", err)}, nil
	}
	result := map[string]any{
		"provider":        t.generator.Provider(),
		"model":           resp.Model,
		"audio_base64":    base64.StdEncoding.EncodeToString(resp.AudioData),
		"content_type":    resp.ContentType,
		"characters_used": resp.CharactersUsed,
	}
	encoded, _ := json.Marshal(result)
	return tool.CallResult{Data: result, ContentType: tool.ContentTypeJSON, Content: string(encoded)}, nil
}

func (t *Tool) Description(_ context.Context) (string, error) { return ToolDescription, nil }
func (t *Tool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *Tool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}
func (t *Tool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *Tool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *Tool) IsEnabled() bool                         { return t.generator != nil }
func (t *Tool) FormatResult(data any) string {
	if m, ok := data.(map[string]any); ok {
		ct, _ := m["content_type"].(string)
		chars, _ := m["characters_used"].(int)
		return fmt.Sprintf("Audio synthesised (%s, %d chars)", ct, chars)
	}
	return fmt.Sprintf("%v", data)
}
func (t *Tool) BackfillInput(_ context.Context, in map[string]any) map[string]any { return in }
