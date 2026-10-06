package vector

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
)

// Qdrant and Chroma have no ranked keyword search to call: they can only say which documents hold a word. termKeywordSearch
// builds a ranking from that: one lookup per word of the query (at most termLookupCap documents each), a document scoring the
// sum of the weights of the query words it holds, a rare word weighing more than one that is in many documents. It is the
// IDF part of BM25 without the term frequency and the length normalisation, which these stores cannot give.
const (
	// termLookupCap is how many documents a lookup of one word returns. A word that reaches it is in a large part of the
	// collection and weighs little.
	termLookupCap = 200
	// maxLookupWords bounds the lookups of one search.
	maxLookupWords = 16
)

// wordLookup returns the documents (text and metadata included) that hold the word, at most limit of them.
type wordLookup func(ctx context.Context, word string, limit int) ([]Record, error)

// queryWords returns the distinct words of a query worth looking up, lowercase: three letters or digits at least, since a
// shorter word is in every document and says nothing.
func queryWords(text string, limit int) []string {
	seen := map[string]bool{}
	var words []string
	for _, word := range pgWordRe.FindAllString(strings.ToLower(text), -1) {
		if len([]rune(word)) < 3 || seen[word] {
			continue
		}
		seen[word] = true
		words = append(words, word)
		if len(words) == limit {
			break
		}
	}
	return words
}

// termKeywordSearch ranks the documents that hold at least one word of the query, best first, at most limit of them. The
// lookups run together; the first error is returned.
func termKeywordSearch(ctx context.Context, queryText string, limit int, lookup wordLookup) ([]SearchResult, error) {
	words := queryWords(queryText, maxLookupWords)
	if len(words) == 0 {
		return nil, nil
	}
	found := make([][]Record, len(words))
	errs := make([]error, len(words))
	var wg sync.WaitGroup
	for i, word := range words {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found[i], errs[i] = lookup(ctx, word, termLookupCap)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	scores := map[string]*SearchResult{}
	for i := range words {
		// ln(1 + (cap+1) / (df+0.5)): about 0.7 for a word that fills the cap, 4.9 for a word held by one document.
		weight := float32(math.Log(1 + float64(termLookupCap+1)/(float64(len(found[i]))+0.5)))
		for _, record := range found[i] {
			if hit, ok := scores[record.Key]; ok {
				hit.Score += weight
				continue
			}
			scores[record.Key] = &SearchResult{Record: record, Score: weight}
		}
	}
	results := make([]SearchResult, 0, len(scores))
	for _, hit := range scores {
		results = append(results, *hit)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Record.Key < results[j].Record.Key
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
