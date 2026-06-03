package toolsearch

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// BM25 parameters — standard Okapi BM25 values.
const (
	bm25K1 = 1.2  // term saturation
	bm25B  = 0.75 // length normalization
)

// bm25Doc is a single indexed document.
type bm25Doc struct {
	id     int
	tokens []string
	tf     map[string]int // term → count in this doc
}

// bm25SearchResult is one ranked hit.
type bm25SearchResult struct {
	ID    int
	Score float64
}

// BM25Engine is an in-memory BM25 full-text search engine.
// Mirrors Codex's use of the `bm25` Rust crate with Language::English.
type BM25Engine struct {
	docs  []bm25Doc
	df    map[string]int // term → number of docs containing it
	avgdl float64
}

// NewBM25Engine builds a BM25 index from (id, text) pairs.
// The id slice is positional: ids[i] is the external identifier for docs[i].
func NewBM25Engine(texts []string) *BM25Engine {
	e := &BM25Engine{
		docs: make([]bm25Doc, len(texts)),
		df:   make(map[string]int),
	}

	totalLen := 0
	for i, text := range texts {
		tokens := tokenize(text)
		tf := make(map[string]int, len(tokens))
		for _, tok := range tokens {
			tf[tok]++
		}
		e.docs[i] = bm25Doc{id: i, tokens: tokens, tf: tf}
		totalLen += len(tokens)
		for tok := range tf {
			e.df[tok]++
		}
	}

	if len(texts) > 0 {
		e.avgdl = float64(totalLen) / float64(len(texts))
	}
	return e
}

// Search returns up to limit results ranked by BM25 score for query.
// Mirrors Codex's SearchEngine::search(query, limit).
func (e *BM25Engine) Search(query string, limit int) []bm25SearchResult {
	if len(e.docs) == 0 {
		return nil
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	n := float64(len(e.docs))
	scores := make([]bm25SearchResult, 0, len(e.docs))

	for _, doc := range e.docs {
		dl := float64(len(doc.tokens))
		normFactor := 1 - bm25B + bm25B*dl/e.avgdl

		var score float64
		for _, term := range queryTokens {
			df := float64(e.df[term])
			if df == 0 {
				continue
			}
			// IDF with smoothing (same formula Codex's BM25 crate uses by default).
			idf := math.Log((n-df+0.5)/(df+0.5) + 1)

			freq := float64(doc.tf[term])
			// BM25 term score.
			score += idf * (freq * (bm25K1 + 1)) / (freq + bm25K1*normFactor)
		}

		if score > 0 {
			scores = append(scores, bm25SearchResult{ID: doc.id, Score: score})
		}
	}

	// Sort descending by score.
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].ID < scores[j].ID // stable tie-break by insertion order
	})

	if limit > 0 && len(scores) > limit {
		scores = scores[:limit]
	}
	return scores
}

// ─── Tokenizer ────────────────────────────────────────────────────────────────

// tokenize lowercases and splits text on non-alphanumeric boundaries,
// then removes English stop words. Mirrors Language::English in the Rust bm25 crate.
func tokenize(text string) []string {
	lower := strings.ToLower(text)

	// Split on any non-alphanumeric character (same as Codex's English tokenizer).
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	tokens := fields[:0]
	for _, f := range fields {
		if len(f) >= 2 && !englishStopWord[f] {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

// englishStopWord is a minimal English stop-word list.
// Keeping it small avoids filtering useful technical terms.
var englishStopWord = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "by": true, "do": true, "for": true,
	"from": true, "has": true, "have": true, "he": true, "in": true,
	"is": true, "it": true, "its": true, "of": true, "on": true,
	"or": true, "that": true, "the": true, "this": true, "to": true,
	"was": true, "were": true, "will": true, "with": true,
}
