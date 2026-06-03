package engine

import (
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	defaultBudgetCompletionThreshold = 0.90
	defaultBudgetDiminishingTokens   = 500
)

// budgetDecision keeps the token-budget policy local to the query loop. The
// loop only needs to know whether it should continue and which synthetic user
// nudge to append if it does.
type budgetDecision struct {
	continueLoop bool
	nudgeMessage string
}

func (l *Loop) resolveTurnTokenBudget(req RunRequest) int {
	if req.TurnTokenBudget > 0 {
		return req.TurnTokenBudget
	}
	return l.config.TurnTokenBudget
}

func (l *Loop) budgetCompletionThreshold() float64 {
	if l.config.BudgetCompletionThreshold <= 0 {
		return defaultBudgetCompletionThreshold
	}
	return l.config.BudgetCompletionThreshold
}

func (l *Loop) budgetDiminishingThreshold() int {
	if l.config.BudgetDiminishingThreshold <= 0 {
		return defaultBudgetDiminishingTokens
	}
	return l.config.BudgetDiminishingThreshold
}

// maybeContinueForBudget mirrors the OpenClaude idea of a turn-level token
// budget: continue only while the model is still making forward progress, and
// stop early when the marginal gain is too small.
func (l *Loop) maybeContinueForBudget(state *MutableState, req RunRequest, assistantMessage types.Message) budgetDecision {
	budget := l.resolveTurnTokenBudget(req)
	if budget <= 0 {
		return budgetDecision{}
	}
	if state.BudgetContinuationCount >= l.config.BudgetContinuationLimit && l.config.BudgetContinuationLimit > 0 {
		return budgetDecision{}
	}
	if !l.shouldNudgeContinuation(assistantMessage, len(state.ToolUses)) {
		return budgetDecision{}
	}

	delta := state.TotalTurnTokens - state.LastBudgetCheckTokens
	diminishing := state.BudgetContinuationCount >= 2 &&
		delta < l.budgetDiminishingThreshold() &&
		state.LastBudgetDelta < l.budgetDiminishingThreshold()
	if diminishing {
		return budgetDecision{}
	}

	pct := float64(state.TotalTurnTokens) / float64(budget)
	if pct >= l.budgetCompletionThreshold() {
		return budgetDecision{}
	}

	state.BudgetContinuationCount++
	state.LastBudgetDelta = delta
	state.LastBudgetCheckTokens = state.TotalTurnTokens
	return budgetDecision{
		continueLoop: true,
		nudgeMessage: fmt.Sprintf("Continue with the task. Budget used: %d/%d tokens. Finish the remaining work directly.", state.TotalTurnTokens, budget),
	}
}
