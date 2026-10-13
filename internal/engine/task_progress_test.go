package engine

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/KPO-Tech/seshat-runtime/internal/modes"
	tasktool "github.com/KPO-Tech/seshat-runtime/internal/tools/task"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// newTaskNudgeTestLoop builds the smallest Loop the task-progress policy needs.
func newTaskNudgeTestLoop(limit int) *Loop {
	l := newNudgeTestLoop()
	l.config.TaskNudgeLimit = limit
	return l
}

// newTaskStoreSession gives the global task store its own file and one session
// id, so the policy reads a real list rather than a stub.
func newTaskStoreSession(t *testing.T, sessionID string) {
	t.Helper()
	path := t.TempDir() + "/tasks.sqlite"
	if err := tasktool.InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	t.Cleanup(func() { _ = tasktool.CloseGlobalTaskStore(path) })
	_ = sessionID
}

func createOpenTask(t *testing.T, sessionID, subject string) {
	t.Helper()
	if _, err := tasktool.GlobalTaskStore().CreateTask(context.Background(), sessionID, subject, "description", "working", nil); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

func TestOpenTasksNudgeMessageNamesTheItemsAndTheOneWayOut(t *testing.T) {
	msg := openTasksNudgeMessage([]string{"Add retry with backoff", "Wire the store"})

	for _, want := range []string{
		"2 unfinished items",
		`"Add retry with backoff"`,
		`"Wire the store"`,
		"in_progress",
		"task_update",
		"blocked on something only the user",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected the nudge to contain %q, got %q", want, msg)
		}
	}
}

func TestOpenTasksNudgeMessageBoundsWhatItQuotes(t *testing.T) {
	open := make([]string, 0, maxListedOpenTasks+2)
	for i := 0; i < maxListedOpenTasks+2; i++ {
		open = append(open, "item")
	}
	long := strings.Repeat("x", maxOpenTaskChars+50)
	open[0] = long

	msg := openTasksNudgeMessage(open)
	if !strings.Contains(msg, "and 2 more") {
		t.Fatalf("expected the overflow to be counted, got %q", msg)
	}
	if !strings.Contains(msg, "...") {
		t.Fatalf("expected the long subject to be elided, got %q", msg)
	}
	if strings.Contains(msg, long) {
		t.Fatal("expected the long subject to be truncated, not quoted whole")
	}
	if got := openTasksNudgeMessage([]string{"one"}); !strings.Contains(got, "1 unfinished item:") {
		t.Fatalf("expected singular wording for one item, got %q", got)
	}
}

func TestMaybeNudgeForOpenTasksSendsTheModelBackToWork(t *testing.T) {
	newTaskStoreSession(t, "s1")
	createOpenTask(t, "s1", "Add retry with backoff")

	loop := newTaskNudgeTestLoop(3)
	state := &MutableState{}
	req := RunRequest{SessionID: types.SessionID("s1")}

	first := loop.maybeNudgeForOpenTasks(context.Background(), state, req)
	if !first.continueLoop {
		t.Fatal("expected a nudge while the task list has an unfinished item")
	}
	if !strings.Contains(first.nudgeMessage, "Add retry with backoff") {
		t.Fatalf("expected the nudge to name the open item, got %q", first.nudgeMessage)
	}
	if state.TaskNudgeCount != 1 {
		t.Fatalf("expected 1 nudge to be counted, got %d", state.TaskNudgeCount)
	}

	// Same list, no progress: the model was just told, so it is not told again.
	if again := loop.maybeNudgeForOpenTasks(context.Background(), state, req); again.continueLoop {
		t.Fatalf("expected no second nudge for an unchanged list, got %q", again.nudgeMessage)
	}

	// A list that moved gets a nudge: that is real progress being tracked.
	createOpenTask(t, "s1", "Wire the store")
	second := loop.maybeNudgeForOpenTasks(context.Background(), state, req)
	if !second.continueLoop {
		t.Fatal("expected a nudge after the list changed")
	}
	if state.TaskNudgeCount != 2 {
		t.Fatalf("expected 2 nudges, got %d", state.TaskNudgeCount)
	}
}

// Waiting for the user's approval is what plan mode is for, so an open task
// list there is not a stall to correct.
func TestMaybeNudgeForOpenTasksStaysSilentInPlanMode(t *testing.T) {
	newTaskStoreSession(t, "s1")
	createOpenTask(t, "s1", "Add retry with backoff")

	loop := newTaskNudgeTestLoop(3)
	state := &MutableState{PermissionContext: &types.PermissionContext{
		ExecutionMode: string(modes.ExecutionModePlan),
	}}

	if decision := loop.maybeNudgeForOpenTasks(context.Background(), state, RunRequest{SessionID: types.SessionID("s1")}); decision.continueLoop {
		t.Fatalf("expected no nudge while plan mode is waiting for approval, got %q", decision.nudgeMessage)
	}
	if state.TaskNudgeCount != 0 {
		t.Fatalf("expected no nudge to be counted in plan mode, got %d", state.TaskNudgeCount)
	}
}

func TestMaybeNudgeForOpenTasksRespectsItsCap(t *testing.T) {
	newTaskStoreSession(t, "s1")
	createOpenTask(t, "s1", "Add retry with backoff")

	loop := newTaskNudgeTestLoop(1)
	state := &MutableState{}
	req := RunRequest{SessionID: types.SessionID("s1")}

	if decision := loop.maybeNudgeForOpenTasks(context.Background(), state, req); !decision.continueLoop {
		t.Fatal("expected the first nudge to fire")
	}
	createOpenTask(t, "s1", "Wire the store")
	if decision := loop.maybeNudgeForOpenTasks(context.Background(), state, req); decision.continueLoop {
		t.Fatalf("expected the cap to stop further nudges, got %q", decision.nudgeMessage)
	}
}

func TestMaybeNudgeForOpenTasksLeavesOtherTurnsAlone(t *testing.T) {
	newTaskStoreSession(t, "s1")
	createOpenTask(t, "s1", "Add retry with backoff")

	loop := newTaskNudgeTestLoop(3)
	req := RunRequest{SessionID: types.SessionID("s1")}

	// A session with no tracked list is not reminded of anything.
	other := &MutableState{}
	if decision := loop.maybeNudgeForOpenTasks(context.Background(), other, RunRequest{SessionID: types.SessionID("unknown")}); decision.continueLoop {
		t.Fatalf("expected no nudge for a session without a task list, got %q", decision.nudgeMessage)
	}

	// A finished list ends the turn normally: completing every item is the
	// normal way out, not an exception to be nudged about.
	done := &MutableState{}
	if _, err := tasktool.GlobalTaskStore().UpdateTask(context.Background(), "s1", taskIDFor(t, "s1"), map[string]any{"status": tasktool.TaskStatusCompleted}); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if decision := loop.maybeNudgeForOpenTasks(context.Background(), done, req); decision.continueLoop {
		t.Fatalf("expected no nudge once every item is completed, got %q", decision.nudgeMessage)
	}
	if done.TaskNudgeCount != 0 {
		t.Fatalf("expected no nudge to be counted, got %d", done.TaskNudgeCount)
	}

	// And the reset path gives a fresh turn its budget back.
	used := &MutableState{TaskNudgeCount: 3, TaskNudgeKey: "stale"}
	resetTaskNudge(used)
	if used.TaskNudgeCount != 0 || used.TaskNudgeKey != "" {
		t.Fatalf("expected resetTaskNudge to clear both fields, got %d/%q", used.TaskNudgeCount, used.TaskNudgeKey)
	}
}

// A subject longer than the bound must be cut on a rune boundary: the previous
// byte slice (subject[:maxOpenTaskChars-1]) landed inside a multi-byte rune for
// accented or non-Latin text and produced a mangled trailing character.
func TestTruncateTaskSubjectNeverSplitsARune(t *testing.T) {
	accents := strings.Repeat("é", 200) // 2 bytes per rune
	got := truncateTaskSubject(accents, maxOpenTaskChars)
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8 after truncation, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected an ellipsis, got %q", got)
	}
	if len(got) > maxOpenTaskChars {
		t.Fatalf("expected at most %d bytes, got %d", maxOpenTaskChars, len(got))
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("expected no replacement character, got %q", got)
	}

	// A subject within the bound is untouched.
	if short := "Add retry with backoff"; truncateTaskSubject(short, maxOpenTaskChars) != short {
		t.Fatalf("expected a short subject to pass through unchanged")
	}
}

// A non-positive limit disables the reminder: 0 means "use the default", but a
// negative value is the explicit off switch a host opts into.
func TestMaybeNudgeForOpenTasksDisabledByNegativeLimit(t *testing.T) {
	newTaskStoreSession(t, "s1")
	createOpenTask(t, "s1", "Add retry with backoff")

	loop := newTaskNudgeTestLoop(-1)
	state := &MutableState{}
	if decision := loop.maybeNudgeForOpenTasks(context.Background(), state, RunRequest{SessionID: types.SessionID("s1")}); decision.continueLoop {
		t.Fatalf("expected a negative limit to disable the nudge, got %q", decision.nudgeMessage)
	}
}

func TestNormalizeLoopConfigTaskNudgeLimit(t *testing.T) {
	if got := normalizeLoopConfig(&LoopConfig{}).TaskNudgeLimit; got != defaultTaskNudgeLimit {
		t.Fatalf("expected an unset limit to default to %d, got %d", defaultTaskNudgeLimit, got)
	}
	if got := normalizeLoopConfig(&LoopConfig{TaskNudgeLimit: -1}).TaskNudgeLimit; got != -1 {
		t.Fatalf("expected a negative limit to survive normalization, got %d", got)
	}
	if got := normalizeLoopConfig(&LoopConfig{TaskNudgeLimit: 5}).TaskNudgeLimit; got != 5 {
		t.Fatalf("expected an explicit limit to survive normalization, got %d", got)
	}
}

func taskIDFor(t *testing.T, sessionID string) string {
	t.Helper()
	tasks, err := tasktool.GlobalTaskStore().ListTasks(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least one task")
	}
	return tasks[0].ID
}
