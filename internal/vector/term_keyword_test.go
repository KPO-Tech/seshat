package vector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
)

func TestQueryWords(t *testing.T) {
	t.Parallel()
	got := queryWords("Who owes the ZebraCode? The zebracode, 12 of it, in 2026!", 16)
	want := []string{"who", "owes", "the", "zebracode", "2026"}
	if !slices.Equal(got, want) {
		t.Fatalf("queryWords = %v, want %v", got, want)
	}
	if got := queryWords("a an of 12", 16); len(got) != 0 {
		t.Fatalf("words of one or two characters carry nothing: %v", got)
	}
	if got := queryWords("alpha beta gamma delta", 2); len(got) != 2 {
		t.Fatalf("limit not applied: %v", got)
	}
}

func TestTermKeywordSearchRanksRareWordsFirst(t *testing.T) {
	t.Parallel()
	docs := map[string][]Record{
		"common": {{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}, {Key: "e"}, {Key: "f"}},
		"rare":   {{Key: "f"}},
		"mid":    {{Key: "b"}, {Key: "c"}},
	}
	lookup := func(_ context.Context, word string, limit int) ([]Record, error) {
		if limit != termLookupCap {
			t.Errorf("limit = %d, want %d", limit, termLookupCap)
		}
		return docs[word], nil
	}
	results, err := termKeywordSearch(context.Background(), "common rare mid unknown", 3, lookup)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, r := range results {
		keys = append(keys, r.Record.Key)
	}
	// f holds the rare word and the common one, b and c the middle one: f first, then b and c (tie: smaller key first).
	if want := []string{"f", "b", "c"}; !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v (%+v)", keys, want, results)
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("scores should decrease: %+v", results)
	}
}

func TestTermKeywordSearchErrorAndEmpty(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	lookup := func(_ context.Context, word string, _ int) ([]Record, error) {
		if word == "bad" {
			return nil, boom
		}
		return []Record{{Key: "x"}}, nil
	}
	if _, err := termKeywordSearch(context.Background(), "good bad", 5, lookup); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	results, err := termKeywordSearch(context.Background(), "an of", 5, lookup)
	if err != nil || results != nil {
		t.Fatalf("no usable word: results %v, err %v", results, err)
	}
	// a query with more words than lookups is still answered
	var many string
	for i := 0; i < 40; i++ {
		many += fmt.Sprintf("word%d ", i)
	}
	var calls atomic.Int32
	_, _ = termKeywordSearch(context.Background(), many, 5, func(_ context.Context, _ string, _ int) ([]Record, error) {
		calls.Add(1)
		return nil, nil
	})
	if calls.Load() != maxLookupWords {
		t.Fatalf("lookups = %d, want %d", calls.Load(), maxLookupWords)
	}
}
