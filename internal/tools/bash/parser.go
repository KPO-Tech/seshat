package bash

import (
	"strings"
)

// shellOperator groups all recognized shell operators ordered by precedence
// (longer operators must come first to avoid prefix-matching "|" before "||").
var shellOperators = []struct {
	text    string
	segType SegmentType
}{
	{"&&", SegmentTypeLogical},
	{"||", SegmentTypeLogical},
	{"|&", SegmentTypePipe},
	{"|", SegmentTypePipe},
	{">>", SegmentTypeRedirection},
	{"&>>", SegmentTypeRedirection},
	{"&>", SegmentTypeRedirection},
	{">", SegmentTypeRedirection},
	{"<", SegmentTypeRedirection},
	{";", SegmentTypeSequential},
}

// Parser provides command parsing utilities.
type Parser struct{}

// NewParser creates a new command parser.
func NewParser() *Parser { return &Parser{} }

// ParsedCommand represents a parsed command.
type ParsedCommand struct {
	Original        string
	Segments        []CommandSegment
	EnvVars         map[string]string
	HasRedirections bool
	IsCompound      bool
}

// CommandSegment represents a segment of a command.
type CommandSegment struct {
	Text     string
	Operator string
	Type     SegmentType
}

// SegmentType represents the type of a command segment.
type SegmentType string

const (
	SegmentTypeCommand     SegmentType = "command"
	SegmentTypePipe        SegmentType = "pipe"
	SegmentTypeLogical     SegmentType = "logical"
	SegmentTypeSequential  SegmentType = "sequential"
	SegmentTypeRedirection SegmentType = "redirection"
)

// ParseCommand parses a bash command into segments.
func (p *Parser) ParseCommand(command string) *ParsedCommand {
	envVars, _ := parseEnvVars(command)
	segments := p.parseSegments(command)

	hasRedirections := false
	isCompound := false
	for _, seg := range segments {
		if seg.Type == SegmentTypeRedirection {
			hasRedirections = true
		}
		if seg.Type != SegmentTypeCommand && seg.Operator != "" {
			isCompound = true
		}
	}

	return &ParsedCommand{
		Original:        command,
		Segments:        segments,
		EnvVars:         envVars,
		HasRedirections: hasRedirections,
		IsCompound:      isCompound,
	}
}

// parseSegments splits a command into segments at all shell operators,
// correctly skipping operators inside single/double quotes, $() and “.
func (p *Parser) parseSegments(command string) []CommandSegment {
	tokens := splitOnOperators(command)
	if len(tokens) == 0 {
		return []CommandSegment{{Text: strings.TrimSpace(command), Type: SegmentTypeCommand}}
	}

	segments := make([]CommandSegment, 0, len(tokens))
	for i, tok := range tokens {
		text := strings.TrimSpace(tok.text)
		if text == "" && !tok.isOp {
			continue
		}
		if tok.isOp {
			continue
		}
		segType := SegmentTypeCommand
		operator := ""
		if i > 0 {
			// find the operator that preceded this segment
			for j := i - 1; j >= 0; j-- {
				if tokens[j].isOp {
					segType = tokens[j].opType
					operator = tokens[j].operator
					break
				}
			}
		}
		segments = append(segments, CommandSegment{
			Text:     text,
			Operator: operator,
			Type:     segType,
		})
	}

	if len(segments) == 0 {
		return []CommandSegment{{Text: strings.TrimSpace(command), Type: SegmentTypeCommand}}
	}
	return segments
}

// shellToken is a text fragment or a shell operator token.
type shellToken struct {
	text     string
	isOp     bool
	opType   SegmentType
	operator string
}

// splitOnOperators tokenises the command string into alternating text / operator tokens.
// It respects single quotes, double quotes, $( ) nesting, and backtick pairs.
func splitOnOperators(command string) []shellToken {
	type tok = shellToken

	var tokens []tok
	runes := []rune(command)
	n := len(runes)
	var cur strings.Builder

	inSingle := false
	inDouble := false
	depth := 0 // $() and backtick nesting depth
	backtickOpen := false

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, tok{text: cur.String()})
			cur.Reset()
		}
	}

	i := 0
	for i < n {
		r := runes[i]

		// ── Backslash escape (outside single quotes) ───────────────────────
		if !inSingle && r == '\\' && i+1 < n {
			cur.WriteRune(r)
			cur.WriteRune(runes[i+1])
			i += 2
			continue
		}

		// ── Single-quote toggle ────────────────────────────────────────────
		if r == '\'' && !inDouble && depth == 0 {
			inSingle = !inSingle
			cur.WriteRune(r)
			i++
			continue
		}

		// ── Inside single quotes: verbatim ─────────────────────────────────
		if inSingle {
			cur.WriteRune(r)
			i++
			continue
		}

		// ── Double-quote toggle ────────────────────────────────────────────
		if r == '"' && depth == 0 {
			inDouble = !inDouble
			cur.WriteRune(r)
			i++
			continue
		}

		// ── Backtick: toggle depth ─────────────────────────────────────────
		if r == '`' {
			if backtickOpen {
				depth--
				backtickOpen = false
			} else {
				depth++
				backtickOpen = true
			}
			cur.WriteRune(r)
			i++
			continue
		}

		// ── $( increases depth ─────────────────────────────────────────────
		if r == '$' && i+1 < n && runes[i+1] == '(' {
			depth++
			cur.WriteRune(r)
			i++
			continue
		}

		// ── ( inside substitution increases depth ──────────────────────────
		if r == '(' && depth > 0 {
			depth++
			cur.WriteRune(r)
			i++
			continue
		}

		// ── ) decreases depth ──────────────────────────────────────────────
		if r == ')' && depth > 0 {
			depth--
			cur.WriteRune(r)
			i++
			continue
		}

		// ── Inside any quoting / substitution context: verbatim ───────────
		if inDouble || depth > 0 {
			cur.WriteRune(r)
			i++
			continue
		}

		// ── Try to match a shell operator at position i ────────────────────
		matched := false
		for _, op := range shellOperators {
			opRunes := []rune(op.text)
			opLen := len(opRunes)
			if i+opLen > n {
				continue
			}
			match := true
			for j := 0; j < opLen; j++ {
				if runes[i+j] != opRunes[j] {
					match = false
					break
				}
			}
			if match {
				flush()
				tokens = append(tokens, tok{
					text:     op.text,
					isOp:     true,
					opType:   op.segType,
					operator: op.text,
				})
				i += opLen
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		cur.WriteRune(r)
		i++
	}

	flush()
	return tokens
}

// GetCommandType returns the type of command.
func (p *Parser) GetCommandType(command string) CommandType {
	_, actualCommand := parseEnvVars(command)
	fields := strings.Fields(actualCommand)
	if len(fields) == 0 {
		return CommandTypeUnknown
	}
	return classifyCommandName(fields[0])
}

// IsSilentCommand checks if command produces no stdout on success.
func (p *Parser) IsSilentCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "mv", "cp", "rm", "mkdir", "rmdir", "chmod", "chown",
		"chgrp", "touch", "ln", "cd", "export", "unset", "wait":
		return true
	}
	return false
}

// CommandType represents the type of command.
type CommandType string

const (
	CommandTypeUnknown        CommandType = "unknown"
	CommandTypeRead           CommandType = "read"
	CommandTypeSearch         CommandType = "search"
	CommandTypeWrite          CommandType = "write"
	CommandTypeStateChange    CommandType = "state_change"
	CommandTypeVersionControl CommandType = "version_control"
)

// classifyCommandName maps a bare command name to its CommandType.
// Single source of truth shared by Parser and SecurityValidator.
func classifyCommandName(cmd string) CommandType {
	switch cmd {
	case "cat", "head", "tail", "less", "more",
		"ls", "tree", "du", "stat", "file", "wc",
		"which", "whereis", "locate", "diff", "echo",
		"printf", "pwd", "env", "printenv", "type", "command":
		return CommandTypeRead

	case "grep", "rg", "ag", "ack", "find", "fd":
		return CommandTypeSearch

	case "mv", "cp", "mkdir", "touch", "rm", "chmod",
		"chown", "chgrp", "ln", "tee", "dd", "truncate",
		"install", "rename", "rmdir":
		return CommandTypeWrite

	case "cd", "export", "unset", "source", ".":
		return CommandTypeStateChange

	case "git", "svn", "hg", "fossil":
		return CommandTypeVersionControl
	}
	return CommandTypeUnknown
}

// parseEnvVars is the package-level canonical implementation — used by Parser
// and SecurityValidator to strip leading VAR=value tokens from a command.
func parseEnvVars(command string) (env map[string]string, remainder string) {
	env = make(map[string]string)
	fields := strings.Fields(command)
	for i, field := range fields {
		if idx := strings.IndexByte(field, '='); idx > 0 && !strings.HasPrefix(field, "-") {
			key := field[:idx]
			// A bare VAR=value token must look like an identifier (no special chars).
			if isIdentifier(key) {
				env[key] = field[idx+1:]
				continue
			}
		}
		return env, strings.Join(fields[i:], " ")
	}
	return env, command
}

// isIdentifier returns true if s is a valid shell variable name.
func isIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, r := range s {
		if i == 0 && (r >= '0' && r <= '9') {
			return false
		}
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}
