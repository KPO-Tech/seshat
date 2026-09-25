package rag

import (
	"context"
	"regexp"
	"strings"
)

// QAChunker splits a naturally question/answer-formatted document (FAQs,
// support docs) into one chunk per Q/A pair, instead of arbitrary
// token-count splitting that can separate a question from its own answer
// or merge several unrelated pairs into one chunk. Each pair's chunk
// carries the question in Metadata["qa_question"], for citation/UI display
// without re-parsing the text - the same idea as HeadingChunker's
// Metadata["heading_path"].
//
// Two independent detection strategies are tried, in order:
//  1. Explicit "Q:"/"A:" labels (also "Question:"/"Answer:", numbered
//     "Q1:", and bold "**Q:**" markdown) - the common plain-text FAQ shape.
//  2. Markdown headings ending in "?" (e.g. "## What is your return
//     policy?") - the common Markdown/docling-converted FAQ shape. Reuses
//     HeadingChunker's own heading-detection (matchHeading) and
//     ancestor-path tracking, so a nested "Category > Question?" structure
//     keeps its category context; a heading that does NOT end in "?" still
//     produces an ordinary (non-QA-tagged) section chunk exactly as
//     HeadingChunker would, rather than being silently dropped - QA
//     tagging is additive on top of HeadingChunker's own behavior, never a
//     narrower view of the document.
//
// A document matching neither pattern delegates entirely to Fallback -
// QAChunker is additive, never worse than the fallback's own baseline, the
// same contract HeadingChunker/TableChunker already establish.
type QAChunker struct {
	Profile  ChunkProfile
	Fallback Chunker
}

// NewQAChunker creates a QA-pair-aware chunker using one of Seshat's
// recommended chunk profiles (same defaulting guard as
// NewHeadingChunker/NewTableChunker). The fallback is HeadingChunker, so a
// document with no QA structure at all still gets heading-aware chunking
// rather than plain paragraph splitting.
func NewQAChunker(profile ChunkProfile) *QAChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	return &QAChunker{Profile: profile, Fallback: NewHeadingChunker(profile)}
}

func (c *QAChunker) fallback() Chunker {
	if c != nil && c.Fallback != nil {
		return c.Fallback
	}
	return DefaultChunker()
}

func (c *QAChunker) maxTokens() int {
	if c != nil && c.Profile.MaxTokens > 0 {
		return c.Profile.MaxTokens
	}
	return DefaultChunkProfile().MaxTokens
}

func (c *QAChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	if chunks, ok := c.splitLabeledPairs(ctx, text); ok {
		return chunks, nil
	}
	if chunks, ok := c.splitHeadingPairs(text); ok {
		return chunks, nil
	}
	return c.fallback().Split(ctx, text)
}

// SplitDocument satisfies DocumentChunker by ignoring the original bytes -
// QAChunker works on already-extracted text/patterns, the same as
// HeadingChunker/TableChunker/ParagraphChunker/SemanticChunker.
func (c *QAChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	return c.Split(ctx, doc.Text)
}

// qaQuestionLabel/qaAnswerLabel match a line that *is* a Q/A label -
// optionally bold-markdown-wrapped (bold closes right after the colon,
// e.g. "**Q1:** text", the common convention - not before it), optionally
// numbered ("Q1:", "A2:"), "Question"/"Answer" spelled out or abbreviated,
// colon or period as the separator. The captured group is the label's own
// inline text, if any (a label line can also be bare, e.g. "**Q:**" alone
// with the question on the next line).
//
// Known false-positive: a letter-lettered outline bullet ("a. First
// point") also matches qaAnswerLabel. Harmless on its own - splitLabeledPairs
// only produces a pair from a *complete* question-then-answer sequence, so
// a stray "a."/"q." bullet with no adjacent real Q/A structure never forms
// a pair - but a document that mixes real Q:/A: labels with letter-bulleted
// sub-lists could occasionally misattribute a bullet as part of an answer.
// The same class of heuristic imprecision HeadingChunker's own
// maxHeadingLineRunes guard already accepts elsewhere in this package.
var (
	qaQuestionLabel = regexp.MustCompile(`(?i)^\*{0,2}q(?:uestion)?\s*\d*\s*[:.]\*{0,2}\s*(.*)$`)
	qaAnswerLabel   = regexp.MustCompile(`(?i)^\*{0,2}a(?:nswer)?\s*\d*\s*[:.]\*{0,2}\s*(.*)$`)
)

// splitLabeledPairs detects the "Q: ...\nA: ..." plain-text FAQ shape. ok is
// false when no complete pair was found at all (nothing to do here - Split
// falls through to the next strategy).
func (c *QAChunker) splitLabeledPairs(_ context.Context, text string) ([]Chunk, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	const (
		stateBefore = iota
		stateQuestion
		stateAnswer
	)

	type qaPair struct{ question, answer string }
	var (
		pairs    []qaPair
		preamble []string
		curQ     strings.Builder
		curA     strings.Builder
		state    = stateBefore
	)
	flushPair := func() {
		q := strings.TrimSpace(curQ.String())
		a := strings.TrimSpace(curA.String())
		if q != "" && a != "" {
			pairs = append(pairs, qaPair{question: q, answer: a})
		}
		curQ.Reset()
		curA.Reset()
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if m := qaQuestionLabel.FindStringSubmatch(line); m != nil {
			if state == stateAnswer {
				flushPair()
			}
			if curQ.Len() > 0 {
				curQ.WriteString("\n")
			}
			curQ.WriteString(m[1])
			state = stateQuestion
			continue
		}
		if m := qaAnswerLabel.FindStringSubmatch(line); m != nil {
			if curA.Len() > 0 {
				curA.WriteString("\n")
			}
			curA.WriteString(m[1])
			state = stateAnswer
			continue
		}
		if line == "" {
			continue
		}
		switch state {
		case stateBefore:
			preamble = append(preamble, line)
		case stateQuestion:
			curQ.WriteString("\n")
			curQ.WriteString(line)
		case stateAnswer:
			curA.WriteString("\n")
			curA.WriteString(line)
		}
	}
	flushPair()

	if len(pairs) == 0 {
		return nil, false
	}

	var chunks []Chunk
	position := 0
	if len(preamble) > 0 {
		preambleText := strings.TrimSpace(strings.Join(preamble, "\n"))
		for _, piece := range splitBodyByTokenBudget(preambleText, c.maxTokens()) {
			chunks = append(chunks, Chunk{Text: piece, Position: position})
			position++
		}
	}
	for _, p := range pairs {
		combined := "Q: " + p.question + "\nA: " + p.answer
		for _, piece := range splitBodyByTokenBudget(combined, c.maxTokens()) {
			chunks = append(chunks, Chunk{
				Text:     piece,
				Position: position,
				Metadata: map[string]string{"qa_question": p.question},
			})
			position++
		}
	}
	return chunks, true
}

// splitHeadingPairs detects the Markdown-heading FAQ shape: a heading line
// (matchHeading, shared with HeadingChunker) ending in "?" starts a QA
// pair; its body (everything until the next heading of any level) becomes
// the answer. A heading that does NOT end in "?" still produces an
// ordinary section chunk (same ancestor-path behavior as HeadingChunker),
// so nothing is silently dropped when only some sections are QA-shaped.
// ok is false only when the document has no heading ending in "?" at all -
// pure HeadingChunker-equivalent output wouldn't add anything QAChunker
// doesn't already get by falling through to its own Fallback.
func (c *QAChunker) splitHeadingPairs(text string) ([]Chunk, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	hasQuestionHeading := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if _, ok := matchHeading(line); ok && strings.HasSuffix(line, "?") {
			hasQuestionHeading = true
			break
		}
	}
	if !hasQuestionHeading {
		return nil, false
	}

	maxTokens := c.maxTokens()
	var (
		chunks   []Chunk
		position int
		stack    []headingStackEntry
		body     strings.Builder
	)

	ancestorPath := func() string {
		titles := make([]string, len(stack))
		for i, e := range stack {
			titles[i] = e.title
		}
		return strings.Join(titles, " > ")
	}

	flush := func() {
		sectionBody := strings.TrimSpace(body.String())
		body.Reset()
		if sectionBody == "" {
			return
		}
		path := ancestorPath()
		isQuestion := len(stack) > 0 && strings.HasSuffix(stack[len(stack)-1].title, "?")
		for _, piece := range splitBodyByTokenBudget(sectionBody, maxTokens) {
			chunkText := piece
			if path != "" {
				chunkText = path + "\n\n" + piece
			}
			meta := map[string]string{}
			if path != "" {
				meta["heading_path"] = path
			}
			if isQuestion {
				meta["qa_question"] = stack[len(stack)-1].title
			}
			chunks = append(chunks, Chunk{Text: chunkText, Position: position, Metadata: meta})
			position++
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		level, ok := matchHeading(line)
		if !ok {
			if line != "" {
				body.WriteString(line)
				body.WriteString("\n")
			}
			continue
		}
		flush()
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, headingStackEntry{level: level, title: line})
	}
	flush()

	return chunks, true
}

var _ DocumentChunker = (*QAChunker)(nil)
