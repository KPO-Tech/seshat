package docx

import (
	"regexp"
	"strings"

	editTool "github.com/KPO-Tech/seshat-runtime/internal/tools/files/edit"
)

var xmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// stripTags removes XML tags, leaving only text content - used to score
// fuzzy-match similarity against just the readable text, not the tag
// markup surrounding it.
func stripTags(s string) string {
	return xmlTagPattern.ReplaceAllString(s, "")
}

// findFuzzyMatch is a line-windowed fuzzy search like
// editTool.FindFuzzyMatch, but scores similarity on tag-stripped text
// instead of the raw pretty-printed line. Necessary because
// prettyPrintXML keeps an inline element like
// <w:t xml:space="preserve">some text</w:t> on a single line - a bare-text
// old_string (no XML tags, the natural thing for a model to write) would
// otherwise be diluted by ~40 characters of tag markup surrounding a
// handful of actually-different characters, sinking the similarity score
// for what a reader would call a near-perfect match. The returned
// FuzzyMatch.Text is still the original tag-included line, since that's
// what a caller needs to build a correct replacement.
func findFuzzyMatch(prettyXML, searchString string) (editTool.FuzzyMatch, bool) {
	searchString = strings.TrimRight(searchString, "\n")
	strippedSearch := stripTags(searchString)
	if strippedSearch == "" {
		return editTool.FuzzyMatch{}, false
	}

	lines := strings.Split(prettyXML, "\n")
	searchLines := strings.Split(searchString, "\n")
	windowSize := len(searchLines)
	if windowSize > len(lines) {
		windowSize = len(lines)
	}
	if windowSize == 0 {
		return editTool.FuzzyMatch{}, false
	}

	best := editTool.FuzzyMatch{}
	for start := 0; start+windowSize <= len(lines); start++ {
		candidate := strings.Join(lines[start:start+windowSize], "\n")
		strippedCandidate := stripTags(candidate)
		sim := editTool.LevenshteinSimilarity(strippedSearch, strippedCandidate)
		if sim > best.Similarity {
			best = editTool.FuzzyMatch{Text: candidate, Similarity: sim}
		}
	}

	if best.Similarity < editTool.FuzzyMatchMinSimilarity {
		return editTool.FuzzyMatch{}, false
	}
	return best, true
}
