package providers

import (
	"reflect"
	"testing"

	"github.com/KPO-Tech/seshat/internal/types"
)

func TestThinkTagFilterFeed(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   []thinkSegment
	}{
		{
			name:   "no tags",
			chunks: []string{"hello world"},
			want:   []thinkSegment{{text: "hello world"}},
		},
		{
			name:   "full tag in one chunk",
			chunks: []string{"before <think>reasoning</think> after"},
			want: []thinkSegment{
				{text: "before "},
				{text: "reasoning", thinking: true},
				{text: " after"},
			},
		},
		{
			name:   "open tag split across chunks",
			chunks: []string{"before <thi", "nk>reasoning</think> after"},
			want: []thinkSegment{
				{text: "before "},
				{text: "reasoning", thinking: true},
				{text: " after"},
			},
		},
		{
			name:   "tag split one byte at a time",
			chunks: []string{"x", "<", "t", "h", "i", "n", "k", ">", "r", "<", "/", "t", "h", "i", "n", "k", ">", "y"},
			want: []thinkSegment{
				{text: "x"},
				{text: "r", thinking: true},
				{text: "y"},
			},
		},
		{
			name:   "close tag split across chunks",
			chunks: []string{"<think>reasoning</thi", "nk> after"},
			want: []thinkSegment{
				{text: "reasoning", thinking: true},
				{text: " after"},
			},
		},
		{
			name:   "lookalike text never completes a tag",
			chunks: []string{"I like <thinking out loud> about this"},
			want:   []thinkSegment{{text: "I like <thinking out loud> about this"}},
		},
		{
			name:   "multiple think blocks",
			chunks: []string{"<think>a</think>mid<think>b</think>end"},
			want: []thinkSegment{
				{text: "a", thinking: true},
				{text: "mid"},
				{text: "b", thinking: true},
				{text: "end"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &thinkTagFilter{}
			var got []thinkSegment
			for _, c := range tc.chunks {
				got = append(got, f.feed(c)...)
			}
			got = append(got, f.flush()...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestThinkTagFilterFlushReleasesUnterminatedTag(t *testing.T) {
	f := &thinkTagFilter{}
	got := f.feed("trailing <thi")
	want := []thinkSegment{{text: "trailing "}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	got = f.flush()
	want = []thinkSegment{{text: "<thi"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWrapThinkTagFilterNil(t *testing.T) {
	if wrapThinkTagFilter(nil) != nil {
		t.Fatal("expected nil wrapper for nil onChunk")
	}
}

func TestWrapThinkTagFilterReclassifiesTextDelta(t *testing.T) {
	var got []types.APIResponseChunk
	wrapped := wrapThinkTagFilter(func(c types.APIResponseChunk) {
		got = append(got, c)
	})

	wrapped(types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: "before <think>"})
	wrapped(types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: "reasoning</think> after"})
	wrapped(types.APIResponseChunk{Type: types.APIChunkTypeMessageStop})

	want := []types.APIResponseChunk{
		{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: "before "},
		{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "thinking_delta", Delta: "reasoning"},
		{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: " after"},
		{Type: types.APIChunkTypeMessageStop},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWrapThinkTagFilterPassesOtherDeltaTypesUnchanged(t *testing.T) {
	var got []types.APIResponseChunk
	wrapped := wrapThinkTagFilter(func(c types.APIResponseChunk) {
		got = append(got, c)
	})

	native := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "thinking_delta", Delta: "<think>native</think>"}
	toolInput := types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "input_json_delta", PartialJSON: `{"a":1}`}
	wrapped(native)
	wrapped(toolInput)

	want := []types.APIResponseChunk{native, toolInput}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWrapThinkTagFilterFlushesPendingOnBlockBoundary(t *testing.T) {
	var got []types.APIResponseChunk
	wrapped := wrapThinkTagFilter(func(c types.APIResponseChunk) {
		got = append(got, c)
	})

	wrapped(types.APIResponseChunk{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: "trailing <thi"})
	wrapped(types.APIResponseChunk{Type: types.APIChunkTypeContentBlockStop})

	want := []types.APIResponseChunk{
		{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: "trailing "},
		{Type: types.APIChunkTypeContentBlockDelta, DeltaType: "text_delta", Delta: "<thi"},
		{Type: types.APIChunkTypeContentBlockStop},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
