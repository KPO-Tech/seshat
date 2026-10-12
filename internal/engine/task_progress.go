package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/modes"
	tasktool "github.com/KPO-Tech/seshat-runtime/internal/tools/task"
)

const (
	// defaultTaskNudgeLimit caps how many times one turn sends the model back
	// to its own open task list. Each nudge is a user message the model has to
	// read, so the budget is small on purpose.
	defaultTaskNudgeLimit = 3

	// maxListedOpenTasks and maxOpenTaskChars bound the message itself. A list
	// of twenty long subjects would cost more than the reminder is worth.
	maxListedOpenTasks = 8
	maxOpenTaskChars   = 120
)

// taskNudgeDecision is what the loop needs from the task-progress policy: keep
// going with this message, or let the turn end.
type taskNudgeDecision struct {
	continueLoop bool
	nudgeMessage string
}

// maybeNudgeForOpenTasks runs when the model has produced a response with no
// tool call - the moment it is about to end the turn. If the session's tracked
// task list still has unfinished items, the model is sent back to work instead
// of ending.
//
// This is the enforcement half of "the user watches the list as your live
// progress". The tool descriptions and the system prompt ask for it, and asking
// is not enough: a model that writes its plan once and stops mid-list leaves the
// user with a stale checklist and half the work done. The equivalent mechanism
// in the reference agent (followup-todo-nudge) is what makes its to-do lists
// trustworthy, and this mirrors its two bounds:
//
//   - a per-turn cap (TaskNudgeLimit), so a model that keeps stalling cannot
//     spend the turn being reminded;
//   - a no-progress guard: a nudge is only sent when the open list has changed
//     since the previous one, so a model that ignores the reminder is not asked
//     twice about the same list.
//
// A turn with no open items, or with no tracked list at all, is untouched.
func (l *Loop) maybeNudgeForOpenTasks(ctx context.Context, state *MutableState, req RunRequest) taskNudgeDecision {
	if l == nil || state == nil {
		return taskNudgeDecision{}
	}
	limit := l.config.TaskNudgeLimit
	if limit <= 0 || state.TaskNudgeCount >= limit {
		return taskNudgeDecision{}
	}
	if state.PermissionContext != nil && state.PermissionContext.ExecutionMode == string(modes.ExecutionModePlan) {
		// In plan mode, stopping to wait for the user is the correct terminal
		// state, not an unfinished task: nothing may be implemented until the
		// plan is approved, so sending the model back to work would push it
		// against the mode it is in.
		return taskNudgeDecision{}
	}

	open := sessionOpenTaskSubjects(ctx, string(req.SessionID))
	if len(open) == 0 {
		return taskNudgeDecision{}
	}

	key := strings.Join(open, "\x00")
	if state.TaskNudgeCount > 0 && state.TaskNudgeKey == key {
		// The list has not moved since the last reminder: repeating it would
		// only spend tokens. Whatever blocks the model is not a forgotten
		// checklist item.
		return taskNudgeDecision{}
	}

	state.TaskNudgeCount++
	state.TaskNudgeKey = key
	return taskNudgeDecision{
		continueLoop: true,
		nudgeMessage: openTasksNudgeMessage(open),
	}
}

// sessionOpenTaskSubjects returns the subjects of the session's unfinished
// tracked tasks, in the order the store reports them. It reads the same store
// the TUI renders from, so what the loop sees is what the user sees.
//
// A nil or unconfigured store, an unknown session, or a store error all mean
// "no open tasks": this policy is an assistance, and it must never be the
// reason a turn fails to end.
func sessionOpenTaskSubjects(ctx context.Context, sessionID string) []string {
	store := tasktool.GlobalTaskStore()
	if store == nil || sessionID == "" {
		return nil
	}
	tasks, err := store.ListTasks(ctx, sessionID)
	if err != nil {
		return nil
	}
	open := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.Status != tasktool.TaskStatusPending && task.Status != tasktool.TaskStatusInProgress {
			continue
		}
		subject := strings.TrimSpace(task.Subject)
		if subject == "" {
			subject = "(untitled task)"
		}
		open = append(open, subject)
	}
	return open
}

// openTasksNudgeMessage is deliberately specific: it names the items, says who
// is watching, and states the one condition under which stopping early is
// allowed, so the model is not pushed into inventing work to satisfy it.
func openTasksNudgeMessage(open []string) string {
	listed := make([]string, 0, maxListedOpenTasks)
	for i, subject := range open {
		if i == maxListedOpenTasks {
			break
		}
		if len(subject) > maxOpenTaskChars {
			subject = subject[:maxOpenTaskChars-1] + "..."
		}
		listed = append(listed, fmt.Sprintf("%q", subject))
	}
	more := ""
	if len(open) > maxListedOpenTasks {
		more = fmt.Sprintf(" and %d more", len(open)-maxListedOpenTasks)
	}
	count := fmt.Sprintf("%d unfinished item", len(open))
	if len(open) != 1 {
		count += "s"
	}
	return fmt.Sprintf(
		"You were about to end your turn, but your tracked task list still has %s: %s%s. "+
			"The user watches that list as your progress, and the work is not finished. "+
			"Continue now with the next unfinished item, marking it in_progress before you start it. "+
			"Only end your turn before the list is done if you are blocked on something only the user "+
			"can provide - a decision, credentials, an action on their machine - or if they asked you "+
			"to stop after this step. In that case say plainly what you need. If an item is no longer "+
			"needed, update the list with task_update.",
		count, strings.Join(listed, ", "), more,
	)
}

// resetTaskNudge gives a turn its full nudge budget back. Called where the
// other per-turn recovery counters are reset, so a fresh turn - or a turn
// restarted after a tool loop - is not inheriting the previous one's spend.
func resetTaskNudge(state *MutableState) {
	if state == nil {
		return
	}
	state.TaskNudgeCount = 0
	state.TaskNudgeKey = ""
}
