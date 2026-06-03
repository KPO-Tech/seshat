package browser

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

const (
	maxBrowserActionsPerTurn = 32
	maxBrowserRepeatPerTurn  = 5
	browserBudgetTTL         = 2 * time.Hour
)

type browserTurnBudget struct {
	count       int
	lastAction  string
	repeatCount int
	updatedAt   time.Time
}

var (
	browserBudgetMu sync.Mutex
	browserBudgets  = map[string]*browserTurnBudget{}
)

func enforceBrowserTurnBudget(input tool.CallInput, action string, details ...string) error {
	toolCtx := input.ToolContextValue()
	if toolCtx.SessionID == "" || toolCtx.TurnID == "" {
		return nil
	}

	signature := action
	if len(details) > 0 {
		signature += ":" + strings.Join(details, ":")
	}
	key := fmt.Sprintf("%s:%s", toolCtx.SessionID, toolCtx.TurnID)

	browserBudgetMu.Lock()
	defer browserBudgetMu.Unlock()
	cleanupBrowserBudgetsLocked()

	budget := browserBudgets[key]
	if budget == nil {
		budget = &browserTurnBudget{}
		browserBudgets[key] = budget
	}

	if budget.count >= maxBrowserActionsPerTurn {
		return fmt.Errorf("browser turn action limit reached (%d)", maxBrowserActionsPerTurn)
	}
	if budget.lastAction == signature {
		budget.repeatCount++
	} else {
		budget.lastAction = signature
		budget.repeatCount = 1
	}
	if budget.repeatCount > maxBrowserRepeatPerTurn {
		return fmt.Errorf("browser turn repeated action limit reached for %q", action)
	}

	budget.count++
	budget.updatedAt = time.Now().UTC()
	return nil
}

func cleanupBrowserBudgetsLocked() {
	now := time.Now().UTC()
	for key, budget := range browserBudgets {
		if now.Sub(budget.updatedAt) > browserBudgetTTL {
			delete(browserBudgets, key)
		}
	}
}
