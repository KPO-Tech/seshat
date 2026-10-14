package prompt

import (
	_ "embed"
	"strings"
)

// Each agent's prompt is one markdown file under prompts/, read top to bottom
// like the document it is. A file is embedded rather than written as a Go
// string because the prompts are full of backticks, which a Go raw string
// cannot hold.
//
// Adding an agent: write prompts/<name>.md, embed it below and declare its
// agentPrompt. Only what must read the same for every agent (safety, how to
// answer) goes in prompts/shared.md.

//go:embed prompts/general.md
var generalAgentText string

//go:embed prompts/shared.md
var sharedText string

// agentPrompt is the stable system prompt of one agent: its own text, then the
// shared text.
type agentPrompt struct {
	name string
	text string
}

// generalAgent is the main agent of the terminal UI. Its tools are the ones of
// builtin.GeneralAgentToolNames.
var generalAgent = agentPrompt{name: "general", text: generalAgentText}

// Prompt section priorities: higher comes first (see Assembler). The agent's
// own text leads, the shared text follows, and the dynamic boundary (850)
// closes the stable prefix.
const (
	agentTextPriority  = 1000
	sharedTextPriority = 900
)

// sections returns the stable sections of the agent, in prompt order.
func (a agentPrompt) sections() []Section {
	return []Section{
		{
			Type:      SectionTypeDefault,
			Name:      a.name,
			Content:   strings.TrimSpace(a.text),
			Priority:  agentTextPriority,
			Cacheable: true,
			Enabled:   true,
		},
		{
			Type:      SectionTypeDefault,
			Name:      "shared",
			Content:   strings.TrimSpace(sharedText),
			Priority:  sharedTextPriority,
			Cacheable: true,
			Enabled:   true,
		},
	}
}

// stableSystemPromptSections is the stable prefix of the main agent's prompt.
var stableSystemPromptSections = generalAgent.sections()

// SeshatCoreStablePrompt returns the stable text of the main agent's prompt.
// Use it as a baseline when building a custom agent that extends Seshat rather
// than replacing it entirely.
func SeshatCoreStablePrompt() string {
	parts := make([]string, 0, len(stableSystemPromptSections))
	for _, s := range stableSystemPromptSections {
		if s.Content != "" {
			parts = append(parts, s.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}
