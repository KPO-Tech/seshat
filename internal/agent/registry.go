package agent

import (
	"context"
	"strings"
	"sync"

	skills "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills"
)

// AgentRegistry is a unified registry for built-in and skill-derived agents.
// Built-in agents are pre-loaded at construction; dynamic agents are loaded
// via LoadFromSkills. Skill-defined agents never shadow built-in ones.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentDefinition
}

// NewAgentRegistry creates a registry pre-populated with all built-in agents.
func NewAgentRegistry() *AgentRegistry {
	r := &AgentRegistry{agents: make(map[string]*AgentDefinition, len(BuiltInAgents))}
	for _, b := range BuiltInAgents {
		def := ToAgentDefinition(b)
		r.agents[def.AgentType] = def
	}
	return r
}

// Register adds or replaces an agent definition. Built-in agents can be
// overridden explicitly via this method (e.g. from tests or admin tooling).
func (r *AgentRegistry) Register(def *AgentDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[def.AgentType] = def
}

// Get returns the agent definition for the given slug, or (nil, false).
func (r *AgentRegistry) Get(slug string) (*AgentDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.agents[slug]
	return def, ok
}

// All returns a snapshot of all registered agent definitions.
func (r *AgentRegistry) All() []*AgentDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AgentDefinition, 0, len(r.agents))
	for _, def := range r.agents {
		result = append(result, def)
	}
	return result
}

// LoadFromSkills scans all skills visible to the given user and registers any
// that carry an `agent:` frontmatter field. Skill-defined agents do not
// override built-in agents (Source == AgentSourceBuiltIn).
func (r *AgentRegistry) LoadFromSkills(cwd, userID string) error {
	all, err := skills.GetAllSkillsForUser(cwd, userID)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range all {
		sk := &all[i]
		if sk.Agent == "" {
			continue
		}
		def := skillToAgentDefinition(sk)
		// Never override built-ins from skill files.
		if existing, ok := r.agents[def.AgentType]; ok && existing.Source == AgentSourceBuiltIn {
			continue
		}
		r.agents[def.AgentType] = def
	}
	return nil
}

// skillToAgentDefinition converts a Skill with a non-empty Agent field into
// an AgentDefinition. The skill's markdown body becomes the agent's system
// prompt; AllowedTools, Model, and Effort map to the corresponding fields.
func skillToAgentDefinition(sk *skills.Skill) *AgentDefinition {
	slug := sk.Agent // e.g. "skill-creator"

	getPromptFn := sk.GetPromptForCommand
	getSystemPrompt := func() string {
		if getPromptFn == nil {
			return ""
		}
		blocks, err := getPromptFn("", context.Background())
		if err != nil || len(blocks) == 0 {
			return ""
		}
		var sb strings.Builder
		for _, b := range blocks {
			sb.WriteString(b.Text)
		}
		return sb.String()
	}

	return &AgentDefinition{
		AgentType:       slug,
		WhenToUse:       firstNonEmpty(sk.WhenToUse, sk.Description),
		Source:          AgentSourceUser,
		BaseDir:         sk.SkillRoot,
		Filename:        sk.Name + "/skill.md",
		Tools:           sk.AllowedTools,
		Model:           sk.Model,
		MaxTurns:        effortToMaxTurns(sk.Effort),
		GetSystemPrompt: getSystemPrompt,
	}
}

// effortToMaxTurns converts a skill effort string to a sensible MaxTurns value.
func effortToMaxTurns(effort string) int {
	switch effort {
	case "minimal":
		return 10
	case "low":
		return 20
	case "high":
		return 100
	case "maximum":
		return 150
	default: // "medium" or unset
		return 50
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
