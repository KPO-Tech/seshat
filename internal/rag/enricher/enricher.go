// Package enricher implements RAG's optional chunk-enrichment step: an LLM
// generates synthetic questions per chunk, run concurrently across chunks
// with a bounded worker pool. See rag.Enricher's own doc comment for why
// (closing the vocabulary gap between declarative chunk text and
// question-shaped user queries - RAGFlow's own "Extractor" pipeline stage).
package enricher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/KPO-Tech/seshat/internal/types"
)

// LLMCaller is the minimal interface for calling the provider API.
// *providers.Client satisfies this - defined here as an interface (not
// imported from internal/providers) so LLMEnricher can be tested without a
// live network call, and so internal/rag never has to depend on
// internal/providers. Mirrors internal/memory/longterm's identically-shaped
// LLMCaller, for the same reason.
type LLMCaller interface {
	CreateMessage(ctx context.Context, req types.APIRequest) (*types.APIResponse, error)
}

// Config controls LLMEnricher behavior. Zero-value fields are replaced with
// defaults by New.
type Config struct {
	// Model is the LLM model used for enrichment. A small/fast model is
	// recommended - this runs once per chunk. When Model.Model is empty the
	// request is sent without an explicit model field, relying on the
	// caller's default (same convention as longterm.ExtractorConfig.Model).
	Model types.ModelIdentifier

	// QuestionsPerChunk is how many synthetic questions to request per
	// chunk. Default: 3.
	QuestionsPerChunk int

	// MaxConcurrency bounds how many chunks are enriched at once. Each
	// enrichment call is a separate LLM request, so an unbounded fan-out
	// over a large document would otherwise burst the provider's rate
	// limits. Default: 4.
	MaxConcurrency int

	// Timeout is the per-chunk call deadline. Default: 30s.
	Timeout time.Duration
}

func (cfg Config) withDefaults() Config {
	if cfg.QuestionsPerChunk <= 0 {
		cfg.QuestionsPerChunk = 3
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 4
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return cfg
}

// LLMEnricher generates synthetic questions per chunk via an LLM, run
// concurrently across chunks with a bounded worker pool (Config.MaxConcurrency).
//
// Retries/backoff are intentionally not implemented here: the LLMCaller
// passed in (normally *providers.Client) already retries transient failures
// internally (see Client.sendMessageWithRetry) - re-wrapping that here would
// just double the backoff.
//
// Best-effort per chunk: a single chunk's LLM failure is logged at DEBUG and
// leaves that chunk's result empty rather than failing the whole batch -
// callers opted into enrichment for better recall, not for ingestion to
// become less reliable than before. EnrichChunks only returns an error for
// something more fundamental (context cancellation).
type LLMEnricher struct {
	caller LLMCaller
	config Config
}

// New creates an LLMEnricher. caller is normally *providers.Client.
func New(caller LLMCaller, cfg Config) *LLMEnricher {
	return &LLMEnricher{caller: caller, config: cfg.withDefaults()}
}

func (e *LLMEnricher) EnrichChunks(ctx context.Context, texts []string) ([][]string, error) {
	if e == nil || e.caller == nil || len(texts) == 0 {
		return make([][]string, len(texts)), nil
	}

	results := make([][]string, len(texts))
	sem := make(chan struct{}, e.config.MaxConcurrency)
	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, text string) {
			defer wg.Done()
			defer func() { <-sem }()
			questions, err := e.enrichOne(ctx, text)
			if err != nil {
				slog.Debug("chunk enrichment failed", "error", err)
				return
			}
			// Safe without synchronization: each goroutine writes a
			// distinct index of a slice that is never resized after
			// allocation above.
			results[i] = questions
		}(i, text)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return results, nil
}

// EnricherCacheKey implements rag.EnricherCacheKeyProvider (duck-typed - see
// that interface's doc comment) so a rag.CachedEnricher's cache entries are
// invalidated automatically when the model or questions-per-chunk
// configuration changes.
func (e *LLMEnricher) EnricherCacheKey() string {
	if e == nil {
		return "llm-enricher:v1:nil"
	}
	return fmt.Sprintf("llm-enricher:v1:%s:%d", e.config.Model.Model, e.config.QuestionsPerChunk)
}

// ─── LLM call ────────────────────────────────────────────────────────────────

const enrichmentSystemPrompt = `You are a search-indexing assistant. Given a passage of text, generate short, natural-language questions that this passage directly and completely answers.

Return ONLY a JSON object — no prose, no markdown fences.
Format (strictly): {"questions": ["...", "..."]}

Guidelines:
- Each question should read like something a user might actually type into a search box
- Questions must be answerable from the passage alone, using only its content
- Do not include the answer in the question itself
- If the passage has no clear question it answers, return {"questions": []}`

func (e *LLMEnricher) enrichOne(ctx context.Context, text string) ([]string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	req := types.APIRequest{
		Model:        e.config.Model,
		SystemPrompt: enrichmentSystemPrompt,
		Messages: []types.Message{
			types.UserMessage("enrich-req", fmt.Sprintf(
				"Generate up to %d questions for this passage:\n\n%s",
				e.config.QuestionsPerChunk, text)),
		},
		MaxTokens: 256,
	}
	resp, err := e.caller.CreateMessage(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("nil response from LLM")
	}

	questions, err := parseQuestions(flattenTextContent(resp.Content))
	if err != nil {
		return nil, err
	}
	if len(questions) > e.config.QuestionsPerChunk {
		questions = questions[:e.config.QuestionsPerChunk]
	}
	return questions, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseQuestions extracts questions from a JSON string that may be wrapped
// in markdown fences or surrounded by explanatory prose - mirrors
// longterm.ParseLLMResponse's tolerance for the same reason (LLMs don't
// always follow "return only JSON" instructions strictly).
func parseQuestions(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = stripMarkdownFence(raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object found in LLM response")
	}
	raw = raw[start : end+1]

	var wrapper struct {
		Questions []string `json:"questions"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("parse enrichment JSON: %w", err)
	}

	out := make([]string, 0, len(wrapper.Questions))
	for _, q := range wrapper.Questions {
		if s := strings.TrimSpace(q); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func flattenTextContent(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if tc, ok := b.(types.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

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
