package providers

import (
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// thinkOpenTag and thinkCloseTag mark reasoning that a model emitted inline
// in its normal text channel instead of through a provider's structured
// reasoning field (e.g. "reasoning_content" on OpenAI-compatible APIs).
// DeepSeek-R1, QwQ and other locally-served reasoning models commonly do
// this when accessed through openAICompatAdapter/ollamaAdapter - the raw
// tags would otherwise reach the live display as plain text_delta chunks.
const (
	thinkOpenTag  = "<think>"
	thinkCloseTag = "</think>"
)

// thinkTagFilter incrementally reclassifies "<think>...</think>" spans found
// inside streamed text_delta chunks into thinking_delta chunks, for display
// purposes only. It is stateful across chunks because a tag can be split
// across arbitrary chunk boundaries (in the extreme, one byte per chunk).
//
// It deliberately only touches what flows to the live onChunk callback. The
// raw, unreclassified text still reaches each adapter's own aggregation
// (APIChunkAccumulator, buildOpenAIStreamResult, ...), so the persisted
// response and conversation history keep the exact text the model produced -
// matching how the think-tag convention is meant to work: the model sees its
// own prior raw tags on the next turn, not a cleaned-up version of them.
type thinkTagFilter struct {
	inside  bool
	pending string
}

// feed consumes a raw text fragment and returns zero or more segments to
// emit, each tagged with whether it belongs inside a think block.
func (f *thinkTagFilter) feed(s string) []thinkSegment {
	if s == "" {
		return nil
	}
	var out []thinkSegment
	buf := f.pending + s
	f.pending = ""
	for {
		tag := thinkOpenTag
		if f.inside {
			tag = thinkCloseTag
		}
		idx := strings.Index(buf, tag)
		if idx >= 0 {
			if idx > 0 {
				out = append(out, thinkSegment{text: buf[:idx], thinking: f.inside})
			}
			buf = buf[idx+len(tag):]
			f.inside = !f.inside
			continue
		}
		if holdLen := longestTagPrefixSuffix(buf, tag); holdLen > 0 {
			if rest := buf[:len(buf)-holdLen]; rest != "" {
				out = append(out, thinkSegment{text: rest, thinking: f.inside})
			}
			f.pending = buf[len(buf)-holdLen:]
		} else if buf != "" {
			out = append(out, thinkSegment{text: buf, thinking: f.inside})
		}
		break
	}
	return out
}

// flush releases any buffered partial-tag text as plain content in whatever
// mode was active, so a tag that never completes before the stream/block
// ends cannot silently swallow text.
func (f *thinkTagFilter) flush() []thinkSegment {
	if f.pending == "" {
		return nil
	}
	seg := thinkSegment{text: f.pending, thinking: f.inside}
	f.pending = ""
	return []thinkSegment{seg}
}

type thinkSegment struct {
	text     string
	thinking bool
}

// longestTagPrefixSuffix returns the length of the longest suffix of buf
// that is also a proper prefix of tag, or 0 if buf cannot still grow into
// tag across a future chunk boundary.
func longestTagPrefixSuffix(buf, tag string) int {
	maxLen := len(tag) - 1
	if maxLen > len(buf) {
		maxLen = len(buf)
	}
	for l := maxLen; l > 0; l-- {
		if strings.HasSuffix(buf, tag[:l]) {
			return l
		}
	}
	return 0
}

// wrapThinkTagFilter wraps onChunk so that raw "<think>" tags leaking into
// text_delta chunks are reclassified as thinking_delta before reaching the
// caller. Every other chunk type/delta passes through untouched. Returns nil
// when onChunk is nil so the no-callback fast path is unaffected.
func wrapThinkTagFilter(onChunk func(types.APIResponseChunk)) func(types.APIResponseChunk) {
	if onChunk == nil {
		return nil
	}
	filter := &thinkTagFilter{}
	return func(chunk types.APIResponseChunk) {
		if chunk.Type != types.APIChunkTypeContentBlockDelta || (chunk.DeltaType != "" && chunk.DeltaType != "text_delta") {
			if pending := filter.flush(); pending != nil {
				emitThinkSegments(onChunk, pending)
			}
			onChunk(chunk)
			return
		}
		emitThinkSegments(onChunk, filter.feed(chunk.Delta))
	}
}

func emitThinkSegments(onChunk func(types.APIResponseChunk), segments []thinkSegment) {
	for _, seg := range segments {
		deltaType := "text_delta"
		if seg.thinking {
			deltaType = "thinking_delta"
		}
		onChunk(types.APIResponseChunk{
			Type:      types.APIChunkTypeContentBlockDelta,
			Delta:     seg.text,
			DeltaType: deltaType,
		})
	}
}
