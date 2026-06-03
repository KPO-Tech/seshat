package agent

// Built-in agents aligned with OpenClaude
var (
	browseAgentTools = []string{
		"read_file",
		"glob",
		"grep",
		"tree",
		"tool_search",
		"wikipedia",
		"scholarly_search",
		"web_fetch",
		"web_search",
		"web_map",
		"web_crawl",
		"browser_open",
		"browser_navigate",
		"browser_snapshot",
		"browser_extract",
		"browser_list_pages",
		"browser_network_list",
		"browser_list_downloads",
		"browser_search_content",
		"browser_get_network_policy",
		"browser_set_network_policy",
		"browser_select_page",
		"browser_close_page",
		"browser_click",
		"browser_type",
		"browser_press",
		"browser_scroll",
		"browser_wait",
		"browser_screenshot",
	}

	// GeneralPurposeAgent is the general-purpose agent
	GeneralPurposeAgent = BuiltInAgentDefinition{
		AgentType: AgentTypeGeneralPurpose,
		WhenToUse: "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks.",
		Tools:     []string{"*"},
		Source:    AgentSourceBuiltIn,
		BaseDir:   "built-in",
		GetSystemPrompt: func() string {
			return `You are a general-purpose agent for Nexus_AI. Given the user's message, use the tools available to complete the task.
Complete the task fully—don't gold-plate, but don't leave it half-done.

Your strengths:
- Searching for code, configurations, and patterns across large codebases
- Analyzing multiple files to understand system architecture
- Performing multi-step research tasks

Guidelines:
- For file searches: search broadly when you don't know where something lives.
- For analysis: Start broad and narrow down.
- NEVER create files unless they're absolutely necessary.
- NEVER proactively create documentation files unless explicitly requested.`
		},
		MaxTurns: 50,
	}

	// ExploreAgent is the explore agent (read-only)
	ExploreAgent = BuiltInAgentDefinition{
		AgentType: AgentTypeExplore,
		WhenToUse: "Explore codebases to understand architecture, find patterns, or investigate how something works (read-only analysis).",
		Tools:     []string{"read_file", "glob", "grep"},
		Source:    AgentSourceBuiltIn,
		BaseDir:   "built-in",
		GetSystemPrompt: func() string {
			return `You are an Explore agent for Nexus_AI. Your role is to explore and analyze codebases.
YOU NEVER MAKE CHANGES - You ONLY READ AND ANALYZE.
If you need to make changes, respond with your findings and let the caller do the modifications.

Your task is to:
- Understand code structure and architecture
- Find patterns and relationships
- Investigate how features work
- Identify dependencies and interactions

Guidelines:
- Be thorough in your exploration
- Start with the big picture, then drill down
- Document your findings clearly
- Use multiple search strategies to find relevant code`
		},
		MaxTurns: 30,
	}

	// BrowseAgent is the deep research agent for external/web investigation.
	BrowseAgent = BuiltInAgentDefinition{
		AgentType: AgentTypeBrowse,
		WhenToUse: "Perform deep read-only research using web, browser, documentation, and local code context. Good for external investigation and multi-source analysis.",
		Tools:     browseAgentTools,
		Source:    AgentSourceBuiltIn,
		BaseDir:   "built-in",
		GetSystemPrompt: func() string {
			return `You are a Browse agent for Nexus_AI. Your role is deep read-only research across external sources and local code context.
YOU NEVER MAKE CHANGES - You ONLY READ, INVESTIGATE, AND SYNTHESIZE.

Your strengths:
- Searching the web for current information
- Navigating sites and extracting structured information with browser tools
- Comparing multiple external sources
- Combining external research with local repository context

Guidelines:
- Prefer high-signal sources over many weak ones
- Use browser tools when you need exact page interaction or extraction
- Use web search to discover sources, then fetch or browse targeted pages
- Cite or clearly identify the sources you used in your findings
- If local code context matters, inspect only what is relevant and stay read-only
- Do not modify files, run write tools, or perform implementation work`
		},
		MaxTurns: 35,
	}

	// PlanAgent is the plan agent
	PlanAgent = BuiltInAgentDefinition{
		AgentType: AgentTypePlan,
		WhenToUse: "Create detailed implementation plans with step-by-step instructions for features or bug fixes.",
		Tools:     []string{"read_file", "glob", "grep", "write_file", "edit_file"},
		Source:    AgentSourceBuiltIn,
		BaseDir:   "built-in",
		GetSystemPrompt: func() string {
			return `You are a Plan agent for Nexus_AI. Your role is to create detailed implementation plans.
Create clear, actionable plans that can be followed to implement features or fix bugs.

Your task is to:
- Analyze requirements and understand what needs to be built
- Break down the implementation into steps
- Identify files that need to be modified
- Consider edge cases and error handling

Guidelines:
- Be specific about what to implement
- Break down complex tasks into smaller steps
- Consider the existing code structure
- Think about testing and error handling`
		},
		MaxTurns: 20,
	}

	// NexusCoreAgent is the default Nexus Core profile. Selecting it (via
	// agent_slug: "nexus-core") is equivalent to running with no agent slug —
	// the engine uses its standard builder path (identity + rules + workflow +
	// dynamic runtime context). GetSystemPrompt returns "" so the runtime
	// detects "no override needed" and keeps the full builder pipeline intact.
	NexusCoreAgent = BuiltInAgentDefinition{
		AgentType: AgentTypeNexusCore,
		WhenToUse: "Default Nexus AI coding assistant — general software engineering, multi-step tasks, planning, tool orchestration.",
		Tools:     []string{"*"},
		Source:    AgentSourceBuiltIn,
		BaseDir:   "built-in",
		GetSystemPrompt: func() string {
			// Empty string signals the runtime to use the full builder pipeline
			// (stable sections + dynamic runtime context) rather than a flat
			// custom prompt. Non-empty overrides bypass the dynamic sections.
			return ""
		},
		MaxTurns: 100,
	}

	// BuiltInAgents is the list of all built-in agents
	BuiltInAgents = []BuiltInAgentDefinition{
		NexusCoreAgent,
		GeneralPurposeAgent,
		ExploreAgent,
		BrowseAgent,
		PlanAgent,
		VerifyAgent,
	}
)

// GetBuiltInAgentByType returns a built-in agent by type
func GetBuiltInAgentByType(agentType string) *BuiltInAgentDefinition {
	for _, agent := range BuiltInAgents {
		if agent.AgentType == agentType {
			return &agent
		}
	}
	return nil
}

// GetBuiltInAgents returns all built-in agents
func GetBuiltInAgents() []BuiltInAgentDefinition {
	return BuiltInAgents
}

// ListAvailableAgents returns a summary of all built-in agent types.
func ListAvailableAgents() []map[string]any {
	agents := GetBuiltInAgents()
	result := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		result = append(result, map[string]any{
			"type":      a.AgentType,
			"whenToUse": a.WhenToUse,
			"maxTurns":  a.MaxTurns,
		})
	}
	return result
}

// ToAgentDefinition converts a BuiltInAgentDefinition to AgentDefinition
func ToAgentDefinition(builtIn BuiltInAgentDefinition) *AgentDefinition {
	return &AgentDefinition{
		AgentType:       builtIn.AgentType,
		WhenToUse:       builtIn.WhenToUse,
		Tools:           builtIn.Tools,
		Source:          builtIn.Source,
		BaseDir:         builtIn.BaseDir,
		Model:           builtIn.Model,
		MaxTurns:        builtIn.MaxTurns,
		GetSystemPrompt: builtIn.GetSystemPrompt,
	}
}
