package skills

import (
	"context"
	"strings"
)

type LoadedFrom string

const (
	LoadedFromCommandsDeprecated LoadedFrom = "commands_DEPRECATED"
	LoadedFromSkills             LoadedFrom = "skills"
	LoadedFromPlugin             LoadedFrom = "plugin"
	LoadedFromManaged            LoadedFrom = "managed"
	LoadedFromBundled            LoadedFrom = "bundled"
	LoadedFromMCP                LoadedFrom = "mcp"
)

type SettingSource string

const (
	SettingSourcePolicySettings  SettingSource = "policySettings"
	SettingSourceUserSettings    SettingSource = "userSettings"
	SettingSourceProjectSettings SettingSource = "projectSettings"
	SettingSourcePlugin          SettingSource = "plugin"
)

type SkillSource string

const (
	SourceBundled  SkillSource = "bundled"
	SourceCommands SkillSource = "commands"
	SourcePlugin   SkillSource = "plugin"
	SourceManaged  SkillSource = "managed"
	SourceMCP      SkillSource = "mcp"
)

// SkillScope controls who can invoke a skill and what context it can access.
type SkillScope string

const (
	ScopeUser   SkillScope = "user"   // created/owned by the end-user
	ScopeRepo   SkillScope = "repo"   // scoped to the current project
	ScopeSystem SkillScope = "system" // platform-level (plugins, MCP)
	ScopeAdmin  SkillScope = "admin"  // policy-managed (admin-controlled)
)

type ExecutionContext string

const (
	ExecutionContextInline ExecutionContext = "inline"
	ExecutionContextFork   ExecutionContext = "fork"
)

type FrontmatterData struct {
	Name                   interface{} `yaml:"name"`
	Description            interface{} `yaml:"description"`
	WhenToUse              string      `yaml:"when_to_use"`
	ArgumentHint           string      `yaml:"argument-hint"`
	Arguments              interface{} `yaml:"arguments"`
	AllowedTools           interface{} `yaml:"allowed-tools"`
	Model                  string      `yaml:"model"`
	DisableModelInvocation interface{} `yaml:"disable-model-invocation"`
	UserInvocable          interface{} `yaml:"user-invocable"`
	Version                string      `yaml:"version"`
	Context                string      `yaml:"context"`
	Agent                  string      `yaml:"agent"`
	Effort                 string      `yaml:"effort"`
	Paths                  interface{} `yaml:"paths"`
	Hooks                  interface{} `yaml:"hooks"`
	Shell                  interface{} `yaml:"shell"`

	// Triggers is a list of natural-language phrases that should auto-activate
	// this skill (e.g. ["browse this page", "take a screenshot"]). Inspired by
	// gstack. When the user's prompt contains any trigger phrase, the skill is
	// expanded automatically — no /slash-command needed.
	Triggers interface{} `yaml:"triggers,omitempty"`

	// PreambleTier controls how much context is prepended to the expanded skill
	// prompt. Accepts int (1/2/3) or string ("basic"/"standard"/"full").
	//   1 / basic    — skill content only (default)
	//   2 / standard — + working directory info
	//   3 / full     — + working directory + user preferences block
	PreambleTier interface{} `yaml:"preamble-tier,omitempty"`

	// Requires lists external dependencies the skill needs (Node.js, Python
	// packages, system tools). The agent checks them at runtime and requests
	// user permission before installing any missing dependency.
	Requires interface{} `yaml:"requires,omitempty"`
}

type HooksSettings struct {
	BeforeTool  []string `yaml:"before_tool,omitempty"`
	AfterTool   []string `yaml:"after_tool,omitempty"`
	Before      []string `yaml:"before,omitempty"`
	After       []string `yaml:"after,omitempty"`
	OnError     []string `yaml:"on_error,omitempty"`
	OnCancel    []string `yaml:"on_cancel,omitempty"`
	OnComplete  []string `yaml:"on_complete,omitempty"`
	ToolAllowed []string `yaml:"tool_allowed,omitempty"`
	ToolDenied  []string `yaml:"tool_denied,omitempty"`
}

type FrontmatterShell struct {
	Before     []string `yaml:"before,omitempty"`
	After      []string `yaml:"after,omitempty"`
	OnError    []string `yaml:"on_error,omitempty"`
	OnComplete []string `yaml:"on_complete,omitempty"`
}

// SkillRequirement declares an external dependency (Node.js, Python package…)
// that must be present before the skill can run. If any requirement fails its
// check command, the skill prompt includes a pre-flight section instructing the
// agent to request user permission before installing.
type SkillRequirement struct {
	// Type is a human-readable label: "node", "python", "system", etc.
	Type string `yaml:"type"`
	// Check is a shell command that exits 0 when the dep is satisfied.
	// Example: "node --version", "python3 -c 'import pptx'"
	Check string `yaml:"check"`
	// InstallCmd is run (with user permission) when Check fails.
	// Supports ${NEXUS_SKILL_DIR} substitution.
	InstallCmd string `yaml:"install-cmd"`
	// Packages lists the packages for display purposes only.
	Packages []string `yaml:"packages,omitempty"`
	// Optional marks the requirement as non-blocking: the skill runs even if
	// the dep is missing, but the agent is told some features may be degraded.
	Optional bool `yaml:"optional,omitempty"`
}

type Skill struct {
	Name                        string
	DisplayName                 string
	Description                 string
	HasUserSpecifiedDescription bool
	AllowedTools                []string
	ArgumentHint                string
	ArgNames                    []string
	WhenToUse                   string
	Version                     string
	Model                       string
	DisableModelInvocation      bool
	UserInvocable               bool
	Context                     ExecutionContext
	Agent                       string
	Effort                      string
	Paths                       []string
	ContentLength               int
	IsHidden                    bool
	ProgressMessage             string
	Source                      SkillSource
	LoadedFrom                  LoadedFrom
	SkillRoot                   string
	Hooks                       *HooksSettings
	Shell                       *FrontmatterShell

	// Triggers holds natural-language phrases that auto-activate this skill
	// without a slash command (e.g. "browse this page", "take a screenshot").
	Triggers []string
	// PreambleTier controls context injection depth: 0=default (base dir),
	// 1=basic (none), 2=standard (base dir), 3=full (base dir + context hint).
	PreambleTier int
	// Scope indicates the ownership/access level of this skill.
	Scope SkillScope
	// Requires lists external dependencies that must be present before the
	// skill runs. The agent verifies each and requests user permission to
	// install any that are missing.
	Requires []SkillRequirement

	GetPromptForCommand func(args string, ctx context.Context) ([]ContentBlock, error)
}

// MatchesTrigger reports whether userInput contains any of the skill's trigger
// phrases (case-insensitive substring match).
func (s *Skill) MatchesTrigger(userInput string) bool {
	if len(s.Triggers) == 0 {
		return false
	}
	lower := strings.ToLower(userInput)
	for _, t := range s.Triggers {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(t))) {
			return true
		}
	}
	return false
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type SkillWithPath struct {
	Skill    Skill
	FilePath string
}

type SkillLoader interface {
	LoadSkills() ([]Skill, error)
	GetSkill(name string) (*Skill, error)
	GetSkillDirCommands(cwd string) ([]Skill, error)
}

type BundledSkillDefinition struct {
	Name                   string
	Description            string
	Aliases                []string
	WhenToUse              string
	ArgumentHint           string
	AllowedTools           []string
	Model                  string
	DisableModelInvocation bool
	UserInvocable          bool
	IsEnabled              func() bool
	Hooks                  *HooksSettings
	Context                ExecutionContext
	Agent                  string
	Effort                 string
	Files                  map[string]string
	Triggers               []string
	GetPromptForCommand    func(args string, ctx context.Context) ([]ContentBlock, error)
}
