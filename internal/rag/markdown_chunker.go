package rag

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/KPO-Tech/seshat/internal/utils"
)

// markdownChunker is the engine behind HeadingChunker and TableChunker: it cuts a document, written as markdown (what
// the document readers produce), along its own structure.
//
// The text is first read as blocks: headings, paragraphs, lists, tables and fenced code. Blocks are never cut in the
// middle of what they are: a paragraph is cut between sentences, a list between items, a table between rows (each piece
// repeating the header) and a fenced block between lines (each piece fenced again). The blocks of a section are then
// packed into chunks of at most max tokens, and sections too small to stand alone (under min tokens) are joined to
// their neighbours with their headings written in the text, so no chunk is a title and one line. A chunk carries the
// path of headings it sits under, in its text (for the embedding and the BM25 index) and in
// Metadata["heading_path"] (for display and citation).
type markdownChunker struct {
	max, min, overlap int
	// isolateTables makes every table its own chunk(s), never packed with text (TableChunker).
	isolateTables bool
}

func newMarkdownChunker(profile ChunkProfile, isolateTables bool) markdownChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	overlap := profile.OverlapTokens
	if overlap < 0 {
		overlap = 0
	}
	if overlap > profile.MaxTokens/2 { // more than half a chunk would make every chunk mostly a repeat
		overlap = profile.MaxTokens / 8
	}
	return markdownChunker{max: profile.MaxTokens, min: max(profile.MaxTokens/4, 1), overlap: overlap, isolateTables: isolateTables}
}

// tokens estimates the size of a piece of text, rounded up: the pieces of a chunk are counted one by one and joined by
// a blank line, and a sum of estimates that each round down would let a chunk grow past its limit.
func tokens(text string) int {
	if text == "" {
		return 0
	}
	return utils.CountTokensInText(text) + 1
}

// ---- reading the blocks ----

type blockKind int

const (
	blockParagraph blockKind = iota
	blockList
	blockTable
	blockCode
	blockHeading
)

type mdBlock struct {
	kind  blockKind
	text  string
	lines []string
	level int    // headings
	title string // headings: the title without its markup
}

var (
	atxHeading = regexp.MustCompile(`^(#{1,6})\s+(\S.*)$`)
	listItem   = regexp.MustCompile(`^\s{0,3}(?:[-*+•◦▪]|\d{1,3}[.)])\s+\S`)
	fenceOpen  = regexp.MustCompile("^\\s{0,3}(`{3,}|~{3,})")
	// A heading written as a numbered outline: "1.2 Title", "3.2.1 Title" (several numbers), or "1. Title" (one).
	multiLevelNumber = regexp.MustCompile(`^((?:\d{1,3}\.)+\d{1,3})\.?\s+\S`)
	singleNumber     = regexp.MustCompile(`^(\d{1,2})[.)]\s+\S`)
	// Structural words of legal and technical documents, in the languages the readers meet.
	keywordHeading = regexp.MustCompile(`(?i)^(part|partie|parte|teil|chapter|chapitre|capítulo|capitulo|capitolo|kapitel|hoofdstuk|section|sección|seccion|seção|secao|sezione|abschnitt|article|artículo|articulo|artigo|artikel|articolo|clause|cláusula|clausula|annex|annexe|anexo|anhang|appendix)\s+\S`) //nolint:misspell // Spanish and Portuguese words, with and without accents
)

// keywordLevels gives each structural word a fixed depth, so a document mixing them ("Part", later "Article") nests
// sensibly.
var keywordLevels = map[string]int{
	"part": 1, "partie": 1, "parte": 1, "teil": 1,
	"chapter": 2, "chapitre": 2, "capítulo": 2, "capitulo": 2, "capitolo": 2, "kapitel": 2, "hoofdstuk": 2, //nolint:misspell // Spanish and Portuguese words, with and without accents
	"section": 3, "sección": 3, "seccion": 3, "seção": 3, "secao": 3, "sezione": 3, "abschnitt": 3,
	"article": 4, "artículo": 4, "articulo": 4, "artigo": 4, "artikel": 4, "articolo": 4,
	"clause": 5, "cláusula": 5, "clausula": 5, "annex": 5, "annexe": 5, "anexo": 5, "anhang": 5, "appendix": 5,
}

const (
	maxHeadingRunes = 100
	maxHeadingWords = 14
)

// lineHeading says whether a line standing alone between blank lines is a heading written without markup, and how
// deep. Only a short line that is not a sentence qualifies: "Section 8 of the agreement states that the parties..." is
// a paragraph that happens to start with a keyword.
func lineHeading(line string) (int, bool) {
	line = strings.TrimSpace(line)
	if line == "" || len([]rune(line)) > maxHeadingRunes || len(strings.Fields(line)) > maxHeadingWords {
		return 0, false
	}
	switch last, _ := lastRune(line); last {
	case '.', ';', ',', ':', '!', '?':
		return 0, false
	}
	if m := multiLevelNumber.FindStringSubmatch(line); m != nil {
		return strings.Count(m[1], ".") + 1, true
	}
	if singleNumber.MatchString(line) {
		return 1, true
	}
	if keywordHeading.MatchString(line) {
		word := strings.ToLower(strings.Fields(line)[0])
		if level, ok := keywordLevels[word]; ok {
			return level, true
		}
	}
	return 0, false
}

func lastRune(s string) (rune, bool) {
	r := []rune(s)
	if len(r) == 0 {
		return 0, false
	}
	return r[len(r)-1], true
}

// nextNumberFollows says whether the next non-blank line after index i is the next number of a numbered list
// ("1. Scope" followed by "2. Terms"): then both are items of a list, not headings.
func nextNumberFollows(lines []string, i int, n int) bool {
	for j := i + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		m := singleNumber.FindStringSubmatch(t)
		if m == nil {
			return false
		}
		return atoiSafe(m[1]) == n+1
	}
	return false
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func parseBlocks(text string) []mdBlock {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []mdBlock
	var para, list []string
	flushPara := func() {
		if len(para) > 0 {
			blocks = append(blocks, mdBlock{kind: blockParagraph, text: strings.Join(para, "\n"), lines: para})
			para = nil
		}
	}
	flushList := func() {
		if len(list) > 0 {
			blocks = append(blocks, mdBlock{kind: blockList, text: strings.Join(list, "\n"), lines: list})
			list = nil
		}
	}
	blank := func(j int) bool { return j < 0 || j >= len(lines) || strings.TrimSpace(lines[j]) == "" }

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flushPara()
			flushList()
		case fenceOpen.MatchString(line):
			flushPara()
			flushList()
			marker := fenceOpen.FindStringSubmatch(line)[1]
			code := []string{line}
			for i++; i < len(lines); i++ {
				code = append(code, lines[i])
				t := strings.TrimSpace(lines[i])
				if strings.HasPrefix(t, marker) && strings.Trim(t, marker[:1]) == "" {
					break
				}
			}
			blocks = append(blocks, mdBlock{kind: blockCode, text: strings.Join(code, "\n"), lines: code})
		case atxHeading.MatchString(trimmed):
			flushPara()
			flushList()
			m := atxHeading.FindStringSubmatch(trimmed)
			title := strings.TrimSpace(strings.TrimRight(m[2], "# "))
			if title == "" {
				title = m[2]
			}
			blocks = append(blocks, mdBlock{kind: blockHeading, text: trimmed, level: len(m[1]), title: title})
		case blank(i-1) && blank(i+1) && isLineHeading(lines, i):
			flushPara()
			flushList()
			level, _ := lineHeading(trimmed)
			blocks = append(blocks, mdBlock{kind: blockHeading, text: trimmed, level: level, title: trimmed})
		case isTableRow(line) && i+1 < len(lines) && isTableSeparatorRow(lines[i+1]):
			flushPara()
			flushList()
			start := i
			for i += 2; i < len(lines) && isTableRow(lines[i]); i++ {
			}
			rows := lines[start:i]
			i--
			blocks = append(blocks, mdBlock{kind: blockTable, text: strings.Join(rows, "\n"), lines: rows})
		case listItem.MatchString(line):
			flushPara()
			list = append(list, line)
		case len(list) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			list = append(list, line) // the continuation of an item
		default:
			flushList()
			para = append(para, line)
		}
	}
	flushPara()
	flushList()
	return blocks
}

// isLineHeading is lineHeading for the line at index i, with the numbered-list exception.
func isLineHeading(lines []string, i int) bool {
	trimmed := strings.TrimSpace(lines[i])
	if _, ok := lineHeading(trimmed); !ok {
		return false
	}
	if m := singleNumber.FindStringSubmatch(trimmed); m != nil && !multiLevelNumber.MatchString(trimmed) && nextNumberFollows(lines, i, atoiSafe(m[1])) {
		return false
	}
	return true
}

// ---- cutting a block ----

// splitBlock cuts a block that is too big for a chunk into pieces of at most limit tokens, along its own joints.
func (m markdownChunker) splitBlock(b mdBlock, limit int) []string {
	switch b.kind {
	case blockTable:
		return splitTableByRowBudget(b.lines, limit)
	case blockCode:
		return splitCode(b.lines, limit)
	case blockList:
		return m.splitList(b.lines, limit)
	}
	return packSentences(b.text, limit, m.overlap)
}

// splitList cuts a list between its items; an item bigger than the limit is cut as a paragraph.
func (m markdownChunker) splitList(lines []string, limit int) []string {
	var items []string
	for _, l := range lines {
		if listItem.MatchString(l) || len(items) == 0 {
			items = append(items, l)
		} else {
			items[len(items)-1] += "\n" + l
		}
	}
	var pieces []string
	var cur []string
	curTok := 0
	flush := func() {
		if len(cur) > 0 {
			pieces = append(pieces, strings.Join(cur, "\n"))
			cur, curTok = nil, 0
		}
	}
	for _, it := range items {
		t := tokens(it)
		if t > limit {
			flush()
			pieces = append(pieces, packSentences(it, limit, 0)...)
			continue
		}
		if curTok+t > limit {
			flush()
		}
		cur = append(cur, it)
		curTok += t
	}
	flush()
	return pieces
}

// splitCode cuts a fenced block between lines, fencing each piece again with the same opening line.
func splitCode(lines []string, limit int) []string {
	open := lines[0]
	body := lines[1:]
	closing := "```"
	if n := len(body); n > 0 {
		if t := strings.TrimSpace(body[n-1]); t != "" && strings.Trim(t, t[:1]) == "" && len(t) >= 3 && (t[0] == '`' || t[0] == '~') {
			closing = t
			body = body[:n-1]
		}
	}
	overhead := tokens(open) + tokens(closing) + 2
	var pieces []string
	var cur []string
	curTok := 0
	flush := func() {
		if len(cur) > 0 {
			pieces = append(pieces, open+"\n"+strings.Join(cur, "\n")+"\n"+closing)
			cur, curTok = nil, 0
		}
	}
	for _, l := range body {
		t := tokens(l) + 1
		if len(cur) > 0 && curTok+t+overhead > limit {
			flush()
		}
		cur = append(cur, l)
		curTok += t
	}
	flush()
	if len(pieces) == 0 {
		return []string{strings.Join(lines, "\n")}
	}
	return pieces
}

// splitIntoSentences cuts text after each sentence end, keeping the whitespace that follows with the sentence so that the
// pieces put end to end give the text back. A full stop followed by a lower-case letter does not end a sentence
// ("e.g. this", "etc. and").
func splitIntoSentences(text string) []string {
	runes := []rune(text)
	var out []string
	start := 0
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '.', '!', '?', '…', '。':
		default:
			continue
		}
		j := i + 1
		for j < len(runes) && (runes[j] == '"' || runes[j] == '\'' || runes[j] == ')' || runes[j] == '”' || runes[j] == '»') {
			j++
		}
		if j >= len(runes) || !unicode.IsSpace(runes[j]) {
			continue
		}
		k := j
		for k < len(runes) && unicode.IsSpace(runes[k]) {
			k++
		}
		if k < len(runes) && unicode.IsLower(runes[k]) {
			continue
		}
		out = append(out, string(runes[start:k]))
		start = k
		i = k - 1
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

// packSentences cuts a paragraph into pieces of at most limit tokens between its sentences. The next piece starts
// with the last sentences of the one before, up to overlap tokens, so a question about what lies across the cut is
// still answered by one chunk. A sentence bigger than the limit is cut between words.
func packSentences(text string, limit, overlap int) []string {
	if tokens(text) <= limit {
		return []string{strings.TrimSpace(text)}
	}
	var pieces []string
	var cur []string
	curTok := 0
	emit := func() {
		if s := strings.TrimSpace(strings.Join(cur, "")); s != "" {
			pieces = append(pieces, s)
		}
	}
	for _, s := range splitIntoSentences(text) {
		st := tokens(s)
		if st > limit {
			if len(cur) > 0 {
				emit()
				cur, curTok = nil, 0
			}
			pieces = append(pieces, cutWords(s, limit)...)
			continue
		}
		if curTok+st > limit && len(cur) > 0 {
			emit()
			cur, curTok = overlapTail(cur, overlap)
		}
		cur = append(cur, s)
		curTok += st
	}
	if len(cur) > 0 {
		emit()
	}
	return pieces
}

// overlapTail returns the last sentences of a piece that fit in overlap tokens, and their size.
func overlapTail(sentences []string, overlap int) ([]string, int) {
	if overlap <= 0 {
		return nil, 0
	}
	total := 0
	from := len(sentences)
	for i := len(sentences) - 1; i >= 1; i-- { // never the whole piece: the next piece must move on
		t := tokens(sentences[i])
		if total+t > overlap {
			break
		}
		total += t
		from = i
	}
	return append([]string(nil), sentences[from:]...), total
}

// cutWords cuts one very long sentence between words into pieces of at most limit tokens.
func cutWords(s string, limit int) []string {
	var pieces []string
	var cur []string
	curTok := 0
	for _, w := range strings.Fields(s) {
		t := tokens(w) + 1
		if len(cur) > 0 && curTok+t > limit {
			pieces = append(pieces, strings.Join(cur, " "))
			cur, curTok = nil, 0
		}
		cur = append(cur, w)
		curTok += t
	}
	if len(cur) > 0 {
		pieces = append(pieces, strings.Join(cur, " "))
	}
	return pieces
}

// ---- packing blocks into chunks ----

type section struct {
	path    []string // titles from the top, this section's own last
	heading string   // the heading as written, for a chunk that holds several sections
	chain   []string // the headings of the path as written, from the top: what a chunk that holds several sections writes
	blocks  []mdBlock
	tokens  int
}

type part struct {
	text    string
	section int
	heading bool
	kind    blockKind
}

type pending struct {
	parts    []part
	tokens   int
	sections []int
}

func (p *pending) empty() bool { return len(p.parts) == 0 }

func (m markdownChunker) split(text string) []Chunk {
	secs := m.sections(parseBlocks(text))
	var out []Chunk
	cur := &pending{}

	emit := func(paths [][]string, body, kind string) {
		path := commonPrefix(paths)
		prefix := strings.Join(path, " > ")
		chunkText := body
		if prefix != "" {
			chunkText = prefix + "\n\n" + body
		}
		meta := map[string]string{}
		if prefix != "" {
			meta["heading_path"] = prefix
		}
		if kind != "" {
			meta["chunk_type"] = kind
		}
		out = append(out, Chunk{Text: chunkText, Position: len(out), Metadata: meta})
	}
	flush := func() {
		if cur.empty() {
			return
		}
		var paths [][]string
		for _, si := range cur.sections {
			paths = append(paths, secs[si].path)
		}
		common := len(commonPrefix(paths))
		var bodies []string
		kinds := map[blockKind]bool{}
		for _, p := range cur.parts {
			if p.heading && len(secs[p.section].path) <= common {
				continue // the heading is already in the path
			}
			if p.heading {
				// The headings above this section's own that the other sections of the chunk do not share are written
				// too: an ancestor with nothing of its own under it is in no other part.
				bodies = append(bodies, strings.Join(secs[p.section].chain[common:], "\n\n"))
			} else {
				bodies = append(bodies, p.text)
			}
			if !p.heading {
				kinds[p.kind] = true
			}
		}
		kind := ""
		if len(kinds) == 1 { // a chunk that is only a table, or only code, says so
			for k := range kinds {
				kind = pieceKind(k)
			}
		}
		if body := strings.TrimSpace(strings.Join(bodies, "\n\n")); body != "" {
			emit(paths, body, kind)
		}
		*cur = pending{}
	}

	// flushForBlock closes the chunk before a block that does not go in it. A heading just added for the section the
	// block belongs to goes with the block, not at the end of the chunk before.
	flushForBlock := func(si int) {
		var heading *part
		if n := len(cur.parts); n > 0 && cur.parts[n-1].heading && cur.parts[n-1].section == si {
			h := cur.parts[n-1]
			heading = &h
			cur.tokens -= tokens(h.text)
			cur.parts = cur.parts[:n-1]
			if k := len(cur.sections); k > 0 && cur.sections[k-1] == si {
				cur.sections = cur.sections[:k-1]
			}
		}
		flush()
		if heading != nil { // the heading starts the next chunk, with the block it was written for
			cur.sections = append(cur.sections, si)
			cur.parts = append(cur.parts, *heading)
			cur.tokens += tokens(heading.text)
		}
	}

	for si, sec := range secs {
		if len(sec.blocks) == 0 {
			continue // a heading with nothing under it: its title is in the path of what is under its own sub-headings
		}
		headTok := tokens(strings.Join(sec.chain, "\n\n"))
		if !cur.empty() && cur.tokens >= m.min && (sec.tokens >= m.min || cur.tokens+sec.tokens+headTok > m.max) {
			flush()
		}
		join := func() {
			if len(cur.sections) == 0 || cur.sections[len(cur.sections)-1] != si {
				cur.sections = append(cur.sections, si)
			}
		}
		if len(sec.chain) > 0 {
			join()
			cur.parts = append(cur.parts, part{text: sec.heading, section: si, heading: true})
			cur.tokens += headTok
		}
		reserve := tokens(strings.Join(sec.path, " > ")) + 4
		limit := max(m.max-reserve, 16)

		for _, b := range sec.blocks {
			bt := tokens(b.text)
			if b.kind == blockTable && m.isolateTables {
				flushForBlock(si)
				for _, piece := range splitTableByRowBudget(b.lines, limit) {
					emit([][]string{sec.path}, piece, "table")
				}
				continue
			}
			if cur.tokens+bt+reserve <= m.max {
				join()
				cur.parts = append(cur.parts, part{text: b.text, section: si, kind: b.kind})
				cur.tokens += bt
				continue
			}
			// It does not fit with what is already there: close that chunk (it is not cut for this block's sake).
			flushForBlock(si)
			if bt+reserve <= m.max {
				join()
				cur.parts = append(cur.parts, part{text: b.text, section: si, kind: b.kind})
				cur.tokens += bt
				continue
			}
			pieces := m.splitBlock(b, limit)
			for i, piece := range pieces {
				if i < len(pieces)-1 {
					emit([][]string{sec.path}, piece, pieceKind(b.kind))
					continue
				}
				join()
				cur.parts = append(cur.parts, part{text: piece, section: si, kind: b.kind})
				cur.tokens += tokens(piece)
			}
		}
	}
	flush()
	return m.joinTrailing(out)
}

// pieceKind names the chunk_type of a chunk made of one piece of a table or of code.
func pieceKind(k blockKind) string {
	switch k {
	case blockTable:
		return "table"
	case blockCode:
		return "code"
	}
	return ""
}

// joinTrailing joins a last chunk that is too small to the one before it when they fit together.
func (m markdownChunker) joinTrailing(chunks []Chunk) []Chunk {
	n := len(chunks)
	if n < 2 {
		return chunks
	}
	last, prev := chunks[n-1], chunks[n-2]
	if tokens(last.Text) >= m.min || last.Metadata["chunk_type"] != "" || prev.Metadata["chunk_type"] != "" {
		return chunks
	}
	if tokens(prev.Text)+tokens(last.Text) > m.max {
		return chunks
	}
	prev.Text += "\n\n" + last.Text
	prevPath := splitPath(prev.Metadata["heading_path"])
	lastPath := splitPath(last.Metadata["heading_path"])
	if common := strings.Join(commonPrefix([][]string{prevPath, lastPath}), " > "); common != "" {
		prev.Metadata["heading_path"] = common
	} else {
		delete(prev.Metadata, "heading_path")
	}
	chunks[n-2] = prev
	return chunks[:n-1]
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	return strings.Split(p, " > ")
}

// commonPrefix is the path of headings that all the given paths share from the top.
func commonPrefix(paths [][]string) []string {
	if len(paths) == 0 {
		return nil
	}
	prefix := append([]string(nil), paths[0]...)
	for _, p := range paths[1:] {
		n := 0
		for n < len(prefix) && n < len(p) && prefix[n] == p[n] {
			n++
		}
		prefix = prefix[:n]
	}
	return prefix
}

// sections groups the blocks under the headings they follow. The blocks before the first heading are a section with
// no path.
func (m markdownChunker) sections(blocks []mdBlock) []section {
	type entry struct {
		level int
		title string
		text  string
	}
	var stack []entry
	secs := []section{{}}
	for _, b := range blocks {
		if b.kind != blockHeading {
			cur := &secs[len(secs)-1]
			cur.blocks = append(cur.blocks, b)
			cur.tokens += tokens(b.text)
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].level >= b.level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, entry{level: b.level, title: b.title, text: b.text})
		path := make([]string, len(stack))
		chain := make([]string, len(stack))
		for i, e := range stack {
			path[i] = e.title
			chain[i] = e.text
		}
		secs = append(secs, section{path: path, heading: b.text, chain: chain})
	}
	if len(secs[0].blocks) == 0 {
		secs = secs[1:]
	}
	// A heading with nothing under it keeps its title in the path of what is under its sub-headings. When no
	// sub-heading follows (a title that stands alone, or a line a reader took for a heading), the title would be in no
	// chunk at all: it becomes text, under its parent's path.
	for i := range secs {
		if len(secs[i].blocks) > 0 || len(secs[i].path) == 0 {
			continue
		}
		if i+1 < len(secs) && hasPrefix(secs[i+1].path, secs[i].path) {
			continue
		}
		secs[i].blocks = []mdBlock{{kind: blockParagraph, text: secs[i].heading, lines: []string{secs[i].heading}}}
		secs[i].tokens = tokens(secs[i].heading)
		secs[i].path = secs[i].path[:len(secs[i].path)-1]
		secs[i].chain = secs[i].chain[:len(secs[i].chain)-1]
		secs[i].heading = ""
	}
	return secs
}

func hasPrefix(path, prefix []string) bool {
	if len(path) <= len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}
