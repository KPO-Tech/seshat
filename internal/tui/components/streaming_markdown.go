package components

import (
	"strings"

	"charm.land/glamour/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// streamingMarkdown caches a "stable prefix" glamour render so each
// streaming flush only re-renders the trailing portion of the
// document.
//
// The boundary between "stable" and "trailing" is detected by
// [findSafeMarkdownBoundary]: a position immediately after a blank
// line at which we can prove no markdown construct is open
// (fenced code block, list, table, block quote, setext header).
//
// Two renders concatenated are NOT generally equal to a single
// render of the whole document — glamour's wrap state is reset
// between calls. The boundary check is therefore deliberately
// conservative; whenever it has the slightest doubt the call
// falls back to a full render and the cache is left untouched.
//
// Invariants:
//
//   - stablePrefix is always a literal byte prefix of the most
//     recently rendered content. If a new content does not have
//     stablePrefix as its prefix the cache is dropped.
//   - stablePrefixRender is the glamour render of stablePrefix
//     alone, with surrounding whitespace trimmed for clean
//     concatenation.
//   - width is the glamour wrap width that produced
//     stablePrefixRender. A width change drops the cache.
type streamingMarkdown struct {
	width              int
	stablePrefix       string
	stablePrefixRender string
}

// Reset drops every cached field. After Reset the next Render call
// is guaranteed to be a full render.
func (s *streamingMarkdown) Reset() {
	s.width = 0
	s.stablePrefix = ""
	s.stablePrefixRender = ""
}

// Render returns the glamour render of content at the given width,
// reusing the cached stable-prefix render when it is safe to do so.
// On any uncertainty the call falls back to a full render via
// renderer and leaves the cache untouched (or drops it).
//
// The returned string has its trailing newline trimmed.
//
// Concurrency: glamour's Render is stateful and not safe for
// concurrent invocation on a shared renderer. We hold
// [common.LockMarkdownRenderer] for the entire prefix +
// trailing render sequence so other goroutines cannot interleave
// their own Render calls and corrupt goldmark's BlockStack.
func (s *streamingMarkdown) Render(content string, width int, renderer *glamour.TermRenderer) string {
	mu := common.LockMarkdownRenderer(renderer)
	mu.Lock()
	defer mu.Unlock()
	full := func() string {
		out, err := renderer.Render(content)
		if err != nil {
			return content
		}
		return strings.TrimSuffix(out, "\n")
	}

	// Width change OR content not a prefix-extension: drop cache,
	// full render, optionally try to seed a fresh boundary on this
	// call.
	if width != s.width || !strings.HasPrefix(content, s.stablePrefix) {
		s.Reset()
		s.width = width
		out := full()
		s.tryAdvanceFromEmpty(content, width, renderer)
		return out
	}

	boundary := findSafeMarkdownBoundary(content)
	if boundary < 0 {
		// No safe boundary anywhere yet. Full render; do not
		// modify the cache (a future flush may find one).
		return full()
	}

	if boundary <= len(s.stablePrefix) {
		// Cached prefix already covers an at-least-as-late
		// boundary. Render the trailing partial fresh and glue.
		trail := content[len(s.stablePrefix):]
		return glueRenders(s.stablePrefixRender, s.renderTrailing(trail, renderer))
	}

	// boundary > len(stablePrefix): we have a NEW chunk of safe
	// content. Render the new chunk, append to stablePrefixRender,
	// promote the boundary, then render the remaining trail.
	newChunk := content[len(s.stablePrefix):boundary]
	newChunkRender := s.renderTrailing(newChunk, renderer)
	s.stablePrefixRender = glueRenders(s.stablePrefixRender, newChunkRender)
	s.stablePrefix = content[:boundary]

	trail := content[boundary:]
	if trail == "" {
		// boundary == len(content): no trailing content. Returning
		// the cached prefix render directly is correct.
		return s.stablePrefixRender
	}
	return glueRenders(s.stablePrefixRender, s.renderTrailing(trail, renderer))
}

// tryAdvanceFromEmpty seeds the cache from a fresh state. We've
// already paid the cost of a full render of `content`; if there is
// a safe boundary inside it, render the prefix once more (cheap
// relative to the full render we just did) and cache it so the
// next flush can avoid the full work.
func (s *streamingMarkdown) tryAdvanceFromEmpty(content string, width int, renderer *glamour.TermRenderer) {
	boundary := findSafeMarkdownBoundary(content)
	if boundary <= 0 {
		return
	}
	prefix := content[:boundary]
	out, err := renderer.Render(prefix)
	if err != nil {
		return
	}
	s.stablePrefix = prefix
	s.stablePrefixRender = trimGlamourMargins(out)
	s.width = width
}

// renderTrailing renders a trailing partial as a fresh glamour
// document and trims the surrounding whitespace so it can be
// concatenated to a cached prefix render without doubled blank
// lines.
func (s *streamingMarkdown) renderTrailing(text string, renderer *glamour.TermRenderer) string {
	if text == "" {
		return ""
	}
	out, err := renderer.Render(text)
	if err != nil {
		return text
	}
	return trimGlamourMargins(out)
}

// glueRenders concatenates two glamour-rendered fragments with a
// single blank line separator. Glamour outputs typically carry
// their own surrounding margins; trimming on both sides and
// gluing with "\n\n" prevents the visible double-margin seam.
func glueRenders(prefix, trail string) string {
	prefix = trimGlamourMargins(prefix)
	trail = trimGlamourMargins(trail)
	switch {
	case prefix == "" && trail == "":
		return ""
	case prefix == "":
		return trail
	case trail == "":
		return prefix
	default:
		return prefix + "\n\n" + trail
	}
}

// trimGlamourMargins strips leading and trailing whitespace
// (including newlines) from a glamour-rendered fragment.
// Glamour adds a leading blank line for documents that open with
// a heading or paragraph, plus a trailing newline; both must be
// removed before concatenation.
func trimGlamourMargins(s string) string {
	return strings.Trim(s, " \t\n")
}

// findSafeMarkdownBoundary returns the byte offset of the END of
// the latest safe boundary in content, i.e. the offset such that
// content[:boundary] is a valid stable-prefix candidate. The
// returned offset always points immediately after a blank-line
// separator, so concatenating a fresh render of content[boundary:]
// to a cached render of content[:boundary] does not require glamour
// to share state across the cut.
//
// Returns -1 when no safe boundary exists. SAFETY FIRST: any time
// we have the slightest doubt we return -1 and let the caller fall
// back to a full render.
func findSafeMarkdownBoundary(content string) int {
	if len(content) == 0 {
		return -1
	}

	// Iterate every blank-line position from latest to earliest.
	for p := blankLineBefore(content, len(content)); p > 0; p = blankLineBefore(content, p-1) {
		if !isSafeBoundaryAt(content, p) {
			continue
		}
		return p
	}
	return -1
}

// blankLineBefore returns the byte offset of the first character
// AFTER the latest blank-line separator that ends strictly before
// `until`. A blank-line separator is a sequence "\n([ \t]*\n)+"
// — one newline, then one or more lines containing only spaces or
// tabs and terminated by another newline. The returned offset is
// the start of the first non-blank line that follows the
// separator (or the position immediately after the final newline,
// if no further content remains).
//
// Returns -1 when no blank-line separator exists before `until`.
func blankLineBefore(content string, until int) int {
	if until <= 0 {
		return -1
	}
	// Walk backward looking for a newline followed (after optional
	// blank-line content) by another newline. We track the latest
	// newline we've seen; if the next earlier newline has only
	// blank chars between them, we have a blank-line separator
	// and the boundary sits immediately after the latest newline.
	end := until
	for end > 0 {
		nl := strings.LastIndexByte(content[:end], '\n')
		if nl < 0 {
			return -1
		}
		// Look for an earlier newline whose gap to nl is empty
		// or whitespace only.
		prev := strings.LastIndexByte(content[:nl], '\n')
		for prev >= 0 {
			gap := content[prev+1 : nl]
			if isBlankOrSpaces(gap) {
				return nl + 1
			}
			// Gap had non-whitespace; nl is not a blank-line
			// separator. Move up: try with the earlier newline as
			// the new "nl" candidate.
			break
		}
		end = nl
	}
	return -1
}

// isBlankOrSpaces reports whether s consists entirely of spaces
// and tabs (or is empty).
func isBlankOrSpaces(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// isSafeBoundaryAt reports whether content[:p] is a safe stable
// prefix. p must be a blank-line boundary (start of a line, with a
// blank line immediately preceding).
func isSafeBoundaryAt(content string, p int) bool {
	prefix := content[:p]

	// (2) Even number of triple-backtick fence lines.
	if countFenceLines(prefix)%2 != 0 {
		return false
	}

	// (2b) Anywhere-in-prefix hazards: open list (B1), HTML block
	// opener (B2), reference link definition (B3). Any of these
	// anywhere in the prefix forces a fallback.
	if prefixHasOpenHazard(prefix) {
		return false
	}

	// (3) Inspect the last non-blank line of the prefix.
	lastLine := lastNonBlankLine(prefix)
	if lastLine != "" && lineOpensConstruct(lastLine) {
		return false
	}

	// (4) If anything follows, make sure it doesn't look like a
	// setext underline that would retroactively turn the last
	// paragraph of the prefix into a header.
	if rest := content[p:]; rest != "" {
		first := firstNonBlankLine(rest)
		if isSetextUnderlineCandidate(first) {
			return false
		}
	}

	return true
}

// prefixHasOpenHazard reports whether prefix contains any of three
// constructs that cannot be safely cut at a blank-line boundary
// even when the immediately preceding line looks fine.
func prefixHasOpenHazard(prefix string) bool {
	inFence := false
	lines := strings.Split(prefix, "\n")
	for _, line := range lines {
		// Track fenced state so list/html/ref patterns inside a
		// fenced code block do not falsely trigger the hazards.
		if isFenceLine(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		// B1: any list-item marker.
		if isListItemMarker(trimmed) {
			return true
		}
		// B2: HTML block opener.
		if isHTMLBlockOpener(line) {
			return true
		}
		// B3: link reference definition.
		if isLinkRefDefinition(line) {
			return true
		}
	}
	return false
}

// countFenceLines counts lines that begin a fenced code block.
func countFenceLines(s string) int {
	n := 0
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if isFenceLine(line) {
			n++
		}
	}
	return n
}

// isFenceLine reports whether line opens or closes a fenced code
// block.
func isFenceLine(line string) bool {
	// Strip up to 3 spaces of indentation.
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return false
	}
	c := line[i]
	if c != '`' && c != '~' {
		return false
	}
	run := 0
	for i < len(line) && line[i] == c {
		i++
		run++
	}
	return run >= 3
}

// lastNonBlankLine returns the last non-blank line of s, or ""
// when every line is blank.
func lastNonBlankLine(s string) string {
	last := ""
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			last = line
		}
	}
	return last
}

// firstNonBlankLine returns the first non-blank line of s, or ""
// when every line is blank.
func firstNonBlankLine(s string) string {
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

// lineOpensConstruct reports whether line keeps a markdown
// construct open across the boundary.
func lineOpensConstruct(line string) bool {
	// Indented code: a tab, or 4+ leading spaces.
	if len(line) > 0 && line[0] == '\t' {
		return true
	}
	if strings.HasPrefix(line, "    ") {
		return true
	}

	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}

	// Block quote.
	if trimmed[0] == '>' {
		return true
	}

	// List item.
	if isListItemMarker(trimmed) {
		return true
	}

	// Table.
	if strings.ContainsRune(line, '|') {
		return true
	}

	// Setext underline candidate.
	if isSetextUnderlineCandidate(trimmed) {
		return true
	}

	return false
}

// isListItemMarker reports whether line (already left-trimmed)
// starts with a CommonMark list-item marker followed by a space
// or tab.
func isListItemMarker(line string) bool {
	if line == "" {
		return false
	}
	c := line[0]
	if c == '-' || c == '*' || c == '+' {
		if len(line) >= 2 && (line[1] == ' ' || line[1] == '\t') {
			return true
		}
		return false
	}
	// Ordered list: digits followed by '.' or ')' and a space.
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i > 9 {
		return false
	}
	if i >= len(line) {
		return false
	}
	if line[i] != '.' && line[i] != ')' {
		return false
	}
	if i+1 >= len(line) {
		return false
	}
	return line[i+1] == ' ' || line[i+1] == '\t'
}

// isSetextUnderlineCandidate reports whether line (with optional
// leading whitespace) consists entirely of '=' or entirely of '-'
// characters with optional trailing whitespace.
func isSetextUnderlineCandidate(line string) bool {
	// Strip leading whitespace.
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i == len(line) {
		return false
	}
	c := line[i]
	if c != '=' && c != '-' {
		return false
	}
	j := i
	for j < len(line) && line[j] == c {
		j++
	}
	// Allow trailing whitespace.
	for j < len(line) {
		if line[j] != ' ' && line[j] != '\t' {
			return false
		}
		j++
	}
	return j-i >= 1
}

// isHTMLBlockOpener reports whether line begins one of the seven
// CommonMark HTML block patterns.
func isHTMLBlockOpener(line string) bool {
	// Strip up to 3 spaces of indentation.
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	rest := line[i:]
	if len(rest) < 2 || rest[0] != '<' {
		return false
	}

	// Type 2: HTML comment "<!--".
	if strings.HasPrefix(rest, "<!--") {
		return true
	}
	// Type 3: processing instruction "<?".
	if strings.HasPrefix(rest, "<?") {
		return true
	}
	// Type 5: CDATA "<![CDATA[".
	if strings.HasPrefix(rest, "<![CDATA[") {
		return true
	}
	// Type 4: declaration "<!" followed by an ASCII letter.
	if len(rest) >= 3 && rest[1] == '!' && isASCIILetter(rest[2]) {
		return true
	}

	// Type 1: <script | <pre | <style | <textarea
	low := strings.ToLower(rest)
	for _, t := range []string{"<script", "<pre", "<style", "<textarea"} {
		if strings.HasPrefix(low, t) {
			next := byte(0)
			if len(low) > len(t) {
				next = low[len(t)]
			}
			if next == 0 || next == ' ' || next == '\t' || next == '>' {
				return true
			}
		}
	}

	// Types 6 & 7: open or close of a block-level tag.
	j := 1
	if j < len(rest) && rest[j] == '/' {
		j++
	}
	if j >= len(rest) || !isASCIILetter(rest[j]) {
		return false
	}
	return true
}

// isASCIILetter reports whether b is an ASCII letter.
func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isLinkRefDefinition reports whether line matches a CommonMark
// link reference definition opener.
func isLinkRefDefinition(line string) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != '[' {
		return false
	}
	i++
	labelStart := i
	for i < len(line) && line[i] != ']' {
		i++
	}
	if i >= len(line) || i == labelStart {
		return false
	}
	i++
	if i >= len(line) || line[i] != ':' {
		return false
	}
	i++
	// Skip required whitespace.
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i < len(line)
}
