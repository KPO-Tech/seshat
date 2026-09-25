package enricher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/internal/types"
)

// fakeCaller returns a canned response per call, or an error. respFor lets a
// test vary the response based on the request's user message text.
type fakeCaller struct {
	mu          sync.Mutex
	respFor     func(text string) (string, error)
	calls       int32
	concurrency int32 // tracks the max number of in-flight calls observed
	inFlight    int32
}

func (f *fakeCaller) CreateMessage(_ context.Context, req types.APIRequest) (*types.APIResponse, error) {
	atomic.AddInt32(&f.calls, 1)
	n := atomic.AddInt32(&f.inFlight, 1)
	defer atomic.AddInt32(&f.inFlight, -1)
	for {
		max := atomic.LoadInt32(&f.concurrency)
		if n <= max || atomic.CompareAndSwapInt32(&f.concurrency, max, n) {
			break
		}
	}

	var text string
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if tc, ok := b.(types.TextContent); ok {
				text = tc.Text
			}
		}
	}
	resp, err := f.respFor(text)
	if err != nil {
		return nil, err
	}
	return &types.APIResponse{Content: []types.ContentBlock{types.TextContent{Text: resp}}}, nil
}

func TestLLMEnricher_ParsesPlainJSON(t *testing.T) {
	caller := &fakeCaller{respFor: func(string) (string, error) {
		return `{"questions": ["What is the capital of France?", "Which city hosts the Eiffel Tower?"]}`, nil
	}}
	e := New(caller, Config{})

	results, err := e.EnrichChunks(context.Background(), []string{"Paris is the capital of France."})
	if err != nil {
		t.Fatalf("EnrichChunks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0]) != 2 {
		t.Fatalf("expected 2 questions, got %v", results[0])
	}
}

func TestLLMEnricher_ParsesFencedAndProseWrappedJSON(t *testing.T) {
	cases := []string{
		"```json\n{\"questions\": [\"Q1?\"]}\n```",
		"Sure, here you go:\n{\"questions\": [\"Q1?\"]}\nHope that helps!",
	}
	for _, raw := range cases {
		caller := &fakeCaller{respFor: func(string) (string, error) { return raw, nil }}
		e := New(caller, Config{})
		results, err := e.EnrichChunks(context.Background(), []string{"some chunk"})
		if err != nil {
			t.Fatalf("EnrichChunks(%q): %v", raw, err)
		}
		if len(results[0]) != 1 || results[0][0] != "Q1?" {
			t.Fatalf("EnrichChunks(%q) = %v, want [\"Q1?\"]", raw, results[0])
		}
	}
}

func TestLLMEnricher_CapsQuestionsPerChunk(t *testing.T) {
	caller := &fakeCaller{respFor: func(string) (string, error) {
		return `{"questions": ["Q1?", "Q2?", "Q3?", "Q4?", "Q5?"]}`, nil
	}}
	e := New(caller, Config{QuestionsPerChunk: 2})

	results, err := e.EnrichChunks(context.Background(), []string{"chunk"})
	if err != nil {
		t.Fatalf("EnrichChunks: %v", err)
	}
	if len(results[0]) != 2 {
		t.Fatalf("expected 2 questions (capped), got %d: %v", len(results[0]), results[0])
	}
}

func TestLLMEnricher_BestEffortSkipsFailedChunkWithoutFailingBatch(t *testing.T) {
	caller := &fakeCaller{respFor: func(text string) (string, error) {
		if strings.Contains(text, "bad") {
			return "", errors.New("simulated LLM failure")
		}
		return `{"questions": ["Good question?"]}`, nil
	}}
	e := New(caller, Config{})

	results, err := e.EnrichChunks(context.Background(), []string{"good chunk", "bad chunk"})
	if err != nil {
		t.Fatalf("EnrichChunks: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(results[0]) != 1 || results[0][0] != "Good question?" {
		t.Fatalf("results[0] = %v, want [\"Good question?\"]", results[0])
	}
	if len(results[1]) != 0 {
		t.Fatalf("results[1] = %v, want empty (failed chunk)", results[1])
	}
}

func TestLLMEnricher_BoundsConcurrency(t *testing.T) {
	caller := &fakeCaller{respFor: func(string) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return `{"questions": []}`, nil
	}}
	e := New(caller, Config{MaxConcurrency: 2})

	texts := make([]string, 8)
	for i := range texts {
		texts[i] = fmt.Sprintf("chunk %d", i)
	}
	if _, err := e.EnrichChunks(context.Background(), texts); err != nil {
		t.Fatalf("EnrichChunks: %v", err)
	}
	if got := atomic.LoadInt32(&caller.calls); got != int32(len(texts)) {
		t.Fatalf("expected %d calls, got %d", len(texts), got)
	}
	if max := atomic.LoadInt32(&caller.concurrency); max > 2 {
		t.Fatalf("observed concurrency %d exceeds MaxConcurrency=2", max)
	}
}

func TestLLMEnricher_NilCallerReturnsEmptyResults(t *testing.T) {
	e := New(nil, Config{})
	results, err := e.EnrichChunks(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EnrichChunks: %v", err)
	}
	if len(results) != 2 || results[0] != nil || results[1] != nil {
		t.Fatalf("expected 2 nil-slice results, got %v", results)
	}
}

func TestLLMEnricher_EnricherCacheKeyReflectsConfig(t *testing.T) {
	a := New(&fakeCaller{}, Config{Model: types.ModelIdentifier{Model: "haiku"}, QuestionsPerChunk: 3})
	b := New(&fakeCaller{}, Config{Model: types.ModelIdentifier{Model: "haiku"}, QuestionsPerChunk: 5})
	c := New(&fakeCaller{}, Config{Model: types.ModelIdentifier{Model: "sonnet"}, QuestionsPerChunk: 3})

	if a.EnricherCacheKey() == b.EnricherCacheKey() {
		t.Fatalf("expected different keys for different QuestionsPerChunk, got same: %s", a.EnricherCacheKey())
	}
	if a.EnricherCacheKey() == c.EnricherCacheKey() {
		t.Fatalf("expected different keys for different Model, got same: %s", a.EnricherCacheKey())
	}
}
