package vector

import "sort"

// Hybrid search puts the best hits of a keyword ranking and of a vector ranking in one list. The three choices below were
// measured on the retrieval benchmark of SeshatOS (seshat-intelligence/benchmarks/chunk_bench: 192 questions on 19 documents,
// a multilingual embedder):
//
//   - Candidates. Each ranking is read to hybridCandidates(topK) hits, not to topK or twice topK: a chunk that is only 15th in
//     each list is still a good answer, and with 20 candidates it is lost. From 20 to 100 candidates the answer is found
//     among the first five results 3 to 4 points more often (512 and 768 tokens chunks); past 100 nothing changes.
//   - Scores, not ranks. Reciprocal rank fusion keeps nothing of how far apart two hits are, and at 512 and 768 tokens it did
//     no better than the keyword ranking alone. Blending scores is better (MRR +0.04 at 512 tokens, +0.05 at 768).
//   - How the scores are normalised (by the best score, min-max, z-score) made no difference beyond the noise of the
//     benchmark, so the simplest is kept: each list is divided by its best score.
const (
	hybridMinCandidates = 100
	hybridMaxCandidates = 500
	hybridPerResult     = 10
)

// hybridCandidates is how many hits of each ranking a hybrid search reads to give topK results.
func hybridCandidates(topK int) int {
	return min(max(topK*hybridPerResult, hybridMinCandidates), hybridMaxCandidates)
}

// blendHybrid merges a vector ranking and a keyword ranking into one list of at most topK results: each list is divided by its
// best score, the vector one is weighted 1-hybridWeight and the keyword one hybridWeight, and a hit that is in both lists gets
// both. Both lists are expected to be the candidates of a search (see hybridCandidates), best first or not: they are not
// re-sorted. Ties go to the smaller key, so that two runs give the same order.
func blendHybrid(vectorResults, keywordResults []SearchResult, hybridWeight float32, topK int) []SearchResult {
	merged := make(map[string]SearchResult, len(vectorResults)+len(keywordResults))
	vectorBest, keywordBest := bestScore(vectorResults), bestScore(keywordResults)
	for _, result := range vectorResults {
		result.Score = normalizedScore(result.Score, vectorBest) * (1 - hybridWeight)
		merged[result.Record.Key] = result
	}
	for _, result := range keywordResults {
		score := normalizedScore(result.Score, keywordBest) * hybridWeight
		if existing, ok := merged[result.Record.Key]; ok {
			existing.Score += score
			merged[result.Record.Key] = existing
			continue
		}
		result.Score = score
		merged[result.Record.Key] = result
	}
	out := make([]SearchResult, 0, len(merged))
	for _, result := range merged {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Record.Key < out[j].Record.Key
		}
		return out[i].Score > out[j].Score
	})
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out
}

func bestScore(results []SearchResult) float32 {
	best := float32(0)
	for _, result := range results {
		if result.Score > best {
			best = result.Score
		}
	}
	return best
}

// normalizedScore divides a score by the best of its list; a list whose best score is not positive has nothing to give.
func normalizedScore(score, best float32) float32 {
	if best <= 0 {
		return 0
	}
	return score / best
}
