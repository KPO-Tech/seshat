package prompt

import (
	"sort"
	"strconv"
	"strings"
)

// SystemPromptDynamicBoundary is a structural marker, not rendered prompt text.
// Everything before it belongs to the stable prefix; everything after it is
// treated as dynamic runtime context.
const SystemPromptDynamicBoundary = "__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__"

// SectionType represents the type of a prompt section
type SectionType string

const (
	SectionTypeDefault SectionType = "default"
	SectionTypeUser    SectionType = "user"
	SectionTypeSystem  SectionType = "system"
	SectionTypeDynamic SectionType = "dynamic"
	SectionTypeAppend  SectionType = "append"
)

// Section represents a part of the system prompt
type Section struct {
	// Type is the section type
	Type SectionType `json:"type"`

	// Name identifies this section
	Name string `json:"name"`

	// Content is the section content
	Content string `json:"content"`

	// Priority determines order (higher = earlier)
	Priority int `json:"priority"`

	// Cacheable indicates if this section can be cached
	Cacheable bool `json:"cacheable"`

	// DynamicBoundary indicates this section marks the cache boundary
	DynamicBoundary bool `json:"dynamic_boundary"`

	// Enabled indicates if this section is active
	Enabled bool `json:"enabled"`
}

// BuildInput represents input for building a prompt
type BuildInput struct {
	// Sections are the prompt sections
	Sections []Section `json:"sections"`

	// Variables are template variables
	Variables map[string]string `json:"variables,omitempty"`

	// OverrideSystemPrompt overrides the system prompt entirely
	OverrideSystemPrompt *string `json:"override_system_prompt,omitempty"`

	// Model is the model being used
	Model string `json:"model,omitempty"`
}

// BuildResult represents the result of building a prompt
type BuildResult struct {
	// SystemPrompt is the assembled system prompt
	SystemPrompt string `json:"system_prompt"`

	// CacheBreakpoint is where the cache boundary is
	CacheBreakpoint int `json:"cache_breakpoint"`

	// StaticText is the cacheable part
	StaticText string `json:"static_text"`

	// DynamicText is the non-cacheable part
	DynamicText string `json:"dynamic_text"`

	// FullText is the complete prompt
	FullText string `json:"full_text"`

	// VariablesUsed are the variables that were used
	VariablesUsed []string `json:"variables_used,omitempty"`
}

// Assembler assembles system prompts from sections
type Assembler struct {
	// sections are the registered sections
	sections []Section

	// defaultSections are the default sections
	defaultSections []Section
}

// NewAssembler creates a new prompt assembler
func NewAssembler() *Assembler {
	return &Assembler{
		sections:        make([]Section, 0),
		defaultSections: make([]Section, 0),
	}
}

// AddSection adds a section
func (a *Assembler) AddSection(section Section) {
	a.sections = append(a.sections, section)
	a.sortSections()
}

// AddSections adds multiple sections
func (a *Assembler) AddSections(sections []Section) {
	a.sections = append(a.sections, sections...)
	a.sortSections()
}

// SetDefaultSections sets the default sections
func (a *Assembler) SetDefaultSections(sections []Section) {
	a.defaultSections = sections
	a.sortDefaultSections()
}

// Build builds the system prompt.
func (a *Assembler) Build(input BuildInput) (BuildResult, error) {
	// Prefer the explicit section list supplied by the caller. This keeps the
	// prompt builder in control of which sections are rendered and avoids silently
	// duplicating default sections when the assembler already has defaults.
	sections := append([]Section(nil), input.Sections...)
	if len(sections) == 0 {
		sections = append(sections, a.defaultSections...)
	}

	// Apply override if specified.
	if input.OverrideSystemPrompt != nil {
		return BuildResult{
			SystemPrompt:    *input.OverrideSystemPrompt,
			FullText:        *input.OverrideSystemPrompt,
			CacheBreakpoint: 0,
			StaticText:      "",
			DynamicText:     *input.OverrideSystemPrompt,
		}, nil
	}

	enabledSections := filterEnabledSections(sections)

	var staticBuilder strings.Builder
	var dynamicBuilder strings.Builder
	var fullBuilder strings.Builder
	breakpoint := 0
	hitBoundary := false
	variablesUsed := make([]string, 0)

	for _, section := range enabledSections {
		content, used := applyTemplateVariables(section.Content, input.Variables)
		variablesUsed = append(variablesUsed, used...)

		// The boundary is a protocol marker only. It must influence where text is
		// written, but it must never leak into the rendered prompt.
		if section.DynamicBoundary {
			hitBoundary = true
			continue
		}
		if strings.TrimSpace(content) == "" {
			continue
		}

		if hitBoundary || !section.Cacheable {
			dynamicBuilder.WriteString(content)
			dynamicBuilder.WriteString("\n\n")
		} else {
			staticBuilder.WriteString(content)
			staticBuilder.WriteString("\n\n")
			breakpoint += len(content) + 2
		}

		fullBuilder.WriteString(content)
		fullBuilder.WriteString("\n\n")
	}

	systemPrompt := staticBuilder.String() + dynamicBuilder.String()

	return BuildResult{
		SystemPrompt:    strings.TrimSpace(systemPrompt),
		FullText:        strings.TrimSpace(fullBuilder.String()),
		CacheBreakpoint: breakpoint,
		StaticText:      strings.TrimSpace(staticBuilder.String()),
		DynamicText:     strings.TrimSpace(dynamicBuilder.String()),
		VariablesUsed:   dedupeStrings(variablesUsed),
	}, nil
}

func filterEnabledSections(sections []Section) []Section {
	ordered := append([]Section(nil), sections...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Priority > ordered[j].Priority
	})
	enabled := make([]Section, 0, len(ordered))
	for _, section := range ordered {
		if section.Enabled {
			enabled = append(enabled, section)
		}
	}
	return enabled
}

func applyTemplateVariables(content string, variables map[string]string) (string, []string) {
	if len(variables) == 0 || !strings.Contains(content, "{{") {
		return content, nil
	}
	used := make([]string, 0)
	resolved := content
	for key, value := range variables {
		placeholder := "{{" + key + "}}"
		if strings.Contains(resolved, placeholder) {
			resolved = strings.ReplaceAll(resolved, placeholder, value)
			used = append(used, key)
		}
	}
	return resolved, used
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// sortSections sorts sections by priority (descending)
func (a *Assembler) sortSections() {
	// Simple bubble sort
	n := len(a.sections)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if a.sections[j].Priority < a.sections[j+1].Priority {
				a.sections[j], a.sections[j+1] = a.sections[j+1], a.sections[j]
			}
		}
	}
}

// sortDefaultSections sorts default sections by priority (descending)
func (a *Assembler) sortDefaultSections() {
	n := len(a.defaultSections)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if a.defaultSections[j].Priority < a.defaultSections[j+1].Priority {
				a.defaultSections[j], a.defaultSections[j+1] = a.defaultSections[j+1], a.defaultSections[j]
			}
		}
	}
}

// DefaultSystemPromptSections returns the canonical prompt sections in stable order.
func DefaultSystemPromptSections() []Section {
	return canonicalPromptSections()
}

// BuildContext builds context information for the prompt
func BuildContext(sessionID string, turnNumber int, workingDirectory string, availableTools []string) map[string]string {
	sortedTools := append([]string(nil), availableTools...)
	sort.Strings(sortedTools)
	return map[string]string{
		"session_id":        sessionID,
		"turn_number":       strconv.Itoa(turnNumber),
		"working_directory": workingDirectory,
		"available_tools":   strings.Join(sortedTools, ", "),
	}
}

// BuildContextWithGit builds context information including git context
func BuildContextWithGit(sessionID string, turnNumber int, workingDirectory string, availableTools []string, gitRoot string, gitBranch string) map[string]string {
	context := BuildContext(sessionID, turnNumber, workingDirectory, availableTools)
	if gitRoot != "" {
		context["git_root"] = gitRoot
	}
	if gitBranch != "" {
		context["git_branch"] = gitBranch
	}
	return context
}
