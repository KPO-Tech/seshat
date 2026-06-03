package tasks

import (
	"context"
	"os"
	"testing"
	"time"
)

// newTestManager creates a minimal Manager for unit tests (no real engine needed for
// quota and status-counting tests since we only call CreateBashTask which does not
// require the engine client).
//
// Uses os.MkdirTemp instead of t.TempDir so that background goroutines started by
// CreateBashTask can finish writing to the output directory after the test returns
// without triggering the Go 1.25 strict TempDir cleanup check.
func newTestManager(t *testing.T, maxConcurrent int) *Manager {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tasks-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) }) //nolint:errcheck // best-effort cleanup
	cfg := &ManagerConfig{
		MaxConcurrentTasks: maxConcurrent,
		TaskTimeout:        30 * time.Second,
		OutputDir:          tmpDir,
	}
	return &Manager{
		tasks:  make(map[TaskID]*Task),
		config: cfg,
	}
}

// injectTask directly inserts a task with a given status into the manager map,
// bypassing the goroutine launcher — useful for setting up quota-check scenarios.
func injectTask(m *Manager, id TaskID, status TaskStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[id] = &Task{
		ID:        id,
		Status:    status,
		CreatedAt: time.Now(),
	}
}

// TestActiveTaskCountLocked verifies that only Pending/Running tasks are counted.
func TestActiveTaskCountLocked(t *testing.T) {
	m := newTestManager(t, 10)
	m.mu.Lock()

	m.tasks["t1"] = &Task{ID: "t1", Status: TaskStatusPending}
	m.tasks["t2"] = &Task{ID: "t2", Status: TaskStatusRunning}
	m.tasks["t3"] = &Task{ID: "t3", Status: TaskStatusCompleted}
	m.tasks["t4"] = &Task{ID: "t4", Status: TaskStatusFailed}
	m.tasks["t5"] = &Task{ID: "t5", Status: TaskStatusKilled}

	count := m.activeTaskCountLocked()
	m.mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 active tasks, got %d", count)
	}
}

// TestCreateBashTask_QuotaCountsOnlyActiveTasks verifies that completed tasks do NOT
// count toward the quota limit — this is the bug we fixed.
func TestCreateBashTask_QuotaCountsOnlyActiveTasks(t *testing.T) {
	m := newTestManager(t, 2)

	// Seed two COMPLETED tasks — should not consume quota slots.
	injectTask(m, "done1", TaskStatusCompleted)
	injectTask(m, "done2", TaskStatusFailed)

	// With len(m.tasks)=2 and MaxConcurrentTasks=2, the OLD code would reject new tasks.
	// With the fix, active count is 0, so the task should be created.
	// Pre-cancel so the background bash goroutine exits quickly and doesn't leave
	// open files after the test returns.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.CreateBashTask(ctx, "echo hello")
	// The task creation itself must succeed — we only care about the quota check.
	if err != nil && err.Error() == "maximum concurrent tasks reached (2)" {
		t.Error("BUG: completed tasks incorrectly consumed quota slots")
	}
}

// TestCreateBashTask_QuotaBlocksWhenActiveTasksFull verifies that when the active
// limit is reached, new tasks are correctly rejected.
func TestCreateBashTask_QuotaBlocksWhenActiveTasksFull(t *testing.T) {
	m := newTestManager(t, 2)

	// Inject two RUNNING tasks to fill the quota.
	injectTask(m, "run1", TaskStatusRunning)
	injectTask(m, "run2", TaskStatusRunning)

	ctx := context.Background()
	_, err := m.CreateBashTask(ctx, "echo should-be-blocked")
	if err == nil {
		t.Error("expected error when active quota is full, got nil")
	}
	if err != nil && err.Error() != "maximum concurrent tasks reached (2)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestCreateBashTask_QuotaAllowsNewAfterCompletion verifies the common lifecycle:
// fill quota → tasks complete → new tasks succeed.
func TestCreateBashTask_QuotaAllowsNewAfterCompletion(t *testing.T) {
	m := newTestManager(t, 1)

	// One completed task does not block new creation.
	injectTask(m, "old1", TaskStatusCompleted)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the goroutine exits quickly

	_, err := m.CreateBashTask(ctx, "echo lifecycle")
	if err != nil && err.Error() == "maximum concurrent tasks reached (1)" {
		t.Error("BUG: completed task should not block new task creation")
	}
}

// TestGetTaskStats verifies stats are consistent with inserted tasks.
func TestGetTaskStats(t *testing.T) {
	m := newTestManager(t, 10)
	injectTask(m, "p1", TaskStatusPending)
	injectTask(m, "r1", TaskStatusRunning)
	injectTask(m, "r2", TaskStatusRunning)
	injectTask(m, "c1", TaskStatusCompleted)
	injectTask(m, "f1", TaskStatusFailed)
	injectTask(m, "k1", TaskStatusKilled)

	stats := m.GetTaskStats()
	if stats.Total != 6 {
		t.Errorf("expected Total=6, got %d", stats.Total)
	}
	if stats.Pending != 1 {
		t.Errorf("expected Pending=1, got %d", stats.Pending)
	}
	if stats.Running != 2 {
		t.Errorf("expected Running=2, got %d", stats.Running)
	}
	if stats.Completed != 1 {
		t.Errorf("expected Completed=1, got %d", stats.Completed)
	}
	if stats.Failed != 1 {
		t.Errorf("expected Failed=1, got %d", stats.Failed)
	}
	if stats.Killed != 1 {
		t.Errorf("expected Killed=1, got %d", stats.Killed)
	}
}
