package agent

// Tool name
const ToolNameAgent = "agent"

// Search hint
const SearchHintAgent = "run a sub-agent to complete a task independently"

// Description
const DescriptionAgent = "Runs a sub-agent to complete a focused task. Each agent type runs with its configured tool surface and can execute across multiple turns."

// Agent types (built-in)
const AgentTypeNexusCore = "nexus-core"
const AgentTypeGeneralPurpose = "general-purpose"
const AgentTypeExplore = "explore"
const AgentTypeBrowse = "browse"
const AgentTypePlan = "plan"
const AgentTypeVerify = "verify"

// DefaultMaxTurns is the turn limit for top-level agents run via the agent tool.
// 200 matches OpenClaude's fork limit and allows complex multi-step tasks to
// complete without hitting an artificial ceiling.
const DefaultMaxTurns = 200

// DefaultSubAgentMaxTurns is the turn limit for sub-agents spawned by another
// agent. Same as top-level: sub-agents can also receive complex delegated tasks.
const DefaultSubAgentMaxTurns = 200

// MaxSubAgentDepth is the default maximum spawn depth allowed before an agent
// tool call is rejected. Prevents infinite delegation chains (A→B→C→…).
//
// Reference values: Codex = 1 (strict), OpenClaude = 2 (hooks only).
// Nexus default = 3: supports A→B→C structures which cover real multi-agent
// patterns (orchestrator → specialist → helper) while blocking runaway chains.
//
// This default can be overridden per-user via UserPreferences.MaxSubAgentDepth
// (stored in DB). The frontend exposes a slider from 1 to MaxAbsoluteSubAgentDepth.
const MaxSubAgentDepth = 3

// MaxAbsoluteSubAgentDepth is the hard upper bound for MaxSubAgentDepth — the
// frontend must clamp user-configured values to this maximum.
const MaxAbsoluteSubAgentDepth = 5

// DefaultSubAgentTimeout is the wall-clock safety net for a single sub-agent
// run. This is NOT a functional limit — it exists to kill a sub-agent that is
// genuinely stuck (LLM unresponsive, infinite loop with no token output).
//
// MaxTurns is the correct tool to bound normal execution. This timeout should
// only fire for pathological cases, which is why it is set to 30 minutes.
//
// Reference values: Codex = 30s, OpenClaude = 60s. Those codebases use short
// values because their sub-agents do shorter tasks. Nexus targets complex
// autonomous coding tasks that legitimately need several minutes per sub-agent.
const DefaultSubAgentTimeout = 30 * 60 // 30 minutes, in seconds
