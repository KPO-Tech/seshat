package longterm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// LLMCaller is the minimal interface for calling the provider API.
// *providers.Client satisfies this — defined here as an interface so the
// extractor can be tested without a live network call.
type LLMCaller interface {
	CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error)
}

// ExtractorConfig controls extraction behaviour.
type ExtractorConfig struct {
	// MinTurns is the minimum number of user turns required before extraction
	// runs. Short sessions rarely contain facts worth persisting.
	// Default: 3.
	MinTurns int

	// MaxTranscriptChars caps the conversation snippet fed to the LLM.
	// Keeps extraction cheap; recent turns are preferred when truncating.
	// Default: 8000.
	MaxTranscriptChars int

	// Model is the LLM model used for extraction. Haiku-class recommended.
	// When Model.Model is empty the request is sent without an explicit model
	// field, relying on the caller's default.
	Model types.ModelIdentifier

	// ExtractionTimeout is the per-call deadline passed to CreateMessage.
	// Default: 45 s.
	ExtractionTimeout time.Duration
}

// DefaultExtractorConfig returns sensible defaults.
func DefaultExtractorConfig() ExtractorConfig {
	return ExtractorConfig{
		MinTurns:           3,
		MaxTranscriptChars: 8000,
		ExtractionTimeout:  45 * time.Second,
	}
}

// Extractor runs LLM-powered extraction of entities and observations from
// session transcripts and persists them in the long-term memory graph.
//
// Extraction is designed to be asynchronous and best-effort: any error is
// logged at DEBUG level and not surfaced to the user.
type Extractor struct {
	store  Store
	caller LLMCaller
	config ExtractorConfig
}

// NewExtractor creates an Extractor. Zero-value config fields are replaced with
// defaults.
func NewExtractor(store Store, caller LLMCaller, cfg ExtractorConfig) *Extractor {
	if cfg.MinTurns <= 0 {
		cfg.MinTurns = 3
	}
	if cfg.MaxTranscriptChars <= 0 {
		cfg.MaxTranscriptChars = 8000
	}
	if cfg.ExtractionTimeout <= 0 {
		cfg.ExtractionTimeout = 45 * time.Second
	}
	return &Extractor{store: store, caller: caller, config: cfg}
}

// Extract analyses the conversation and upserts any extracted entities into the
// knowledge graph for userID.
//
// It is safe to call from a goroutine: all errors are logged and the function
// always returns nil to the caller (extraction failures are non-fatal).
func (e *Extractor) Extract(ctx context.Context, userID string, messages []types.Message) error {
	if e == nil || e.store == nil || e.caller == nil {
		return nil
	}
	if strings.TrimSpace(userID) == "" || len(messages) == 0 {
		return nil
	}
	if countUserTurns(messages) < e.config.MinTurns {
		return nil
	}

	transcript := BuildTranscript(messages, e.config.MaxTranscriptChars)
	if strings.TrimSpace(transcript) == "" {
		return nil
	}

	extractCtx, cancel := context.WithTimeout(ctx, e.config.ExtractionTimeout)
	defer cancel()

	inputs, err := e.callLLM(extractCtx, transcript)
	if err != nil {
		slog.Debug("longterm extraction LLM call failed", "user", userID, "error", err)
		return nil
	}
	if len(inputs) == 0 {
		return nil
	}

	// UpsertEntities creates entities that do not exist yet.
	_, err = e.store.UpsertEntities(ctx, userID, inputs)
	if err != nil {
		slog.Debug("longterm extraction upsert failed", "user", userID, "error", err)
		return nil
	}

	// AddObservations appends new observations to all mentioned entities
	// (new or pre-existing). Duplicate observations are silently skipped.
	obsInputs := toObservationInputs(inputs)
	if len(obsInputs) > 0 {
		if _, err := e.store.AddObservations(ctx, userID, obsInputs); err != nil {
			slog.Debug("longterm extraction add-obs failed", "user", userID, "error", err)
		}
	}
	return nil
}

// ─── LLM call ────────────────────────────────────────────────────────────────

const extractionSystemPrompt = `You are a memory extraction assistant.

Given a conversation, identify facts worth preserving for future sessions.
Return ONLY a JSON object — no prose, no markdown fences.

Format (strictly):
{"entities":[{"name":"...","entity_type":"...","observations":["..."]}]}

Guidelines:
- entity_type: person | project | tool | concept | preference | decision | fact
- observations: concise factual statements, one clear idea each
- Include only persistent facts; skip ephemeral task details (file paths, exact diffs, step progress)
- Skip uncertain or speculative information
- If nothing is worth remembering, return {"entities":[]}`

func (e *Extractor) callLLM(ctx context.Context, transcript string) ([]EntityInput, error) {
	req := types.APIRequest{
		Model:        e.config.Model,
		SystemPrompt: extractionSystemPrompt,
		Messages: []types.Message{
			types.UserMessage("extraction-req", "Conversation transcript:\n\n"+transcript),
		},
		MaxTokens: 1024,
	}

	resp, err := e.caller.CreateMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("nil response from LLM")
	}

	raw := flattenTextContent(resp.Content)
	return ParseLLMResponse(raw)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// ParseLLMResponse extracts EntityInputs from a JSON string that may be wrapped
// in markdown fences or surrounded by explanatory prose.
func ParseLLMResponse(raw string) ([]EntityInput, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	// Strip common markdown fences.
	raw = stripMarkdownFence(raw)

	// Find the outermost JSON object.
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object found in LLM response")
	}
	raw = raw[start : end+1]

	var wrapper struct {
		Entities []struct {
			Name         string   `json:"name"`
			EntityType   string   `json:"entity_type"`
			Observations []string `json:"observations"`
		} `json:"entities"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("parse extraction JSON: %w", err)
	}

	out := make([]EntityInput, 0, len(wrapper.Entities))
	for _, e := range wrapper.Entities {
		name := strings.TrimSpace(e.Name)
		entityType := strings.TrimSpace(e.EntityType)
		if name == "" || entityType == "" {
			continue
		}
		var obs []string
		for _, o := range e.Observations {
			if s := strings.TrimSpace(o); s != "" {
				obs = append(obs, s)
			}
		}
		out = append(out, EntityInput{
			Name:         name,
			EntityType:   entityType,
			Observations: obs,
		})
	}
	return out, nil
}

// BuildTranscript formats messages as a readable transcript, capped at maxChars
// by keeping the most recent content.
func BuildTranscript(messages []types.Message, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 8000
	}
	var lines []string
	for _, m := range messages {
		text := flattenMessageText(m)
		if strings.TrimSpace(text) == "" {
			continue
		}
		role := "User"
		if m.Role == types.RoleAssistant {
			role = "Assistant"
		} else if m.Role != types.RoleUser {
			continue
		}
		// Truncate very long individual messages.
		if len(text) > 1200 {
			text = text[:1200] + "…"
		}
		lines = append(lines, role+": "+text)
	}

	full := strings.Join(lines, "\n\n")
	if utf8.RuneCountInString(full) <= maxChars {
		return full
	}

	// Keep the last maxChars characters (most recent context is most relevant).
	runes := []rune(full)
	return "…\n\n" + string(runes[len(runes)-maxChars:])
}

// countUserTurns counts messages where the role is User.
func countUserTurns(messages []types.Message) int {
	n := 0
	for _, m := range messages {
		if m.Role == types.RoleUser {
			n++
		}
	}
	return n
}

// toObservationInputs converts EntityInputs to ObservationInputs for AddObservations.
func toObservationInputs(inputs []EntityInput) []ObservationInput {
	out := make([]ObservationInput, 0, len(inputs))
	for _, inp := range inputs {
		if len(inp.Observations) == 0 {
			continue
		}
		out = append(out, ObservationInput{
			EntityName: inp.Name,
			Contents:   inp.Observations,
		})
	}
	return out
}

// flattenTextContent joins all TextContent blocks from an API response.
func flattenTextContent(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if tc, ok := b.(types.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// flattenMessageText extracts visible text from a message's content blocks.
func flattenMessageText(m types.Message) string {
	var parts []string
	for _, b := range m.Content {
		switch t := b.(type) {
		case types.TextContent:
			if s := strings.TrimSpace(t.Text); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " ")
}

// stripMarkdownFence removes ```json ... ``` or ``` ... ``` wrappers.
func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json", "```"} {
		if strings.HasPrefix(s, fence) {
			s = strings.TrimPrefix(s, fence)
			if idx := strings.LastIndex(s, "```"); idx >= 0 {
				s = s[:idx]
			}
			return strings.TrimSpace(s)
		}
	}
	return s
}
