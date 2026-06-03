package modes

// ExecutionMode represents how the agent executes tools.
// This is separate from ApprovalMode which determines who approves actions.

// ExecutionMode determines:
// - Whether tools are actually executed or just described
// - How the agent behaves (plan, execute, etc.)
type ExecutionMode string

const (
	// ExecutionModeExecute is the default mode - tools are executed normally.
	ExecutionModeExecute ExecutionMode = "execute"

	// ExecutionModePlan is planning mode - tools are described but not executed.
	// In this mode, the agent only provides descriptions/plans without running them.
	// This is useful for letting users review before actual execution.
	ExecutionModePlan ExecutionMode = "plan"

	// ExecutionModePairProgramming is collaborative mode - the AI works alongside the user
	// with more interactive feedback and suggestions rather than autonomous execution.
	// This mode focuses on collaboration and real-time code review.
	ExecutionModePairProgramming ExecutionMode = "pair_programming"
)

// IsPlanMode returns true if the execution mode is plan mode.
func IsPlanMode(mode ExecutionMode) bool {
	return mode == ExecutionModePlan
}

// IsExecuteMode returns true if the execution mode executes tools.
func IsExecuteMode(mode ExecutionMode) bool {
	return mode == ExecutionModeExecute || mode == ""
}

// IsPairProgrammingMode returns true if the execution mode is pair programming mode.
func IsPairProgrammingMode(mode ExecutionMode) bool {
	return mode == ExecutionModePairProgramming
}

// IsPlanModeString returns true if the execution mode string is plan mode.
func IsPlanModeString(mode string) bool {
	return mode == string(ExecutionModePlan)
}

// IsPairProgrammingModeString returns true if the execution mode string is pair programming mode.
func IsPairProgrammingModeString(mode string) bool {
	return mode == string(ExecutionModePairProgramming)
}

// IsExecuteModeString returns true if the execution mode string is execute mode.
// Matches both "execute" and "" (empty string treated as default execute mode).
func IsExecuteModeString(mode string) bool {
	return mode == string(ExecutionModeExecute) || mode == ""
}
