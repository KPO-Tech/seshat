package bash

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
)

// These tests spawn a real shell process per case instead of mocking
// BackgroundTaskManager, so they exercise the stdin-pipe/output-file plumbing
// pollForOutput actually runs against, not just the polling logic in isolation.

func newTestTaskManager(t *testing.T) *BackgroundTaskManager {
	t.Helper()
	mgr := NewBackgroundTaskManager(t.TempDir())
	if err := mgr.Init(); err != nil {
		t.Fatalf("init task manager: %v", err)
	}
	prev := globalTaskManager
	globalTaskManager = mgr
	t.Cleanup(func() { globalTaskManager = prev })
	return mgr
}

func TestBackgroundTaskHelperProcess(t *testing.T) {
	if os.Getenv("SESHAT_BASH_TEST_HELPER") != "1" {
		return
	}
	reader := bufio.NewScanner(os.Stdin)
	switch os.Getenv("SESHAT_BASH_TEST_HELPER_MODE") {
	case "prompt-block":
		fmt.Print("> ")
		reader.Scan()
	case "done":
		fmt.Println("done")
	case "interactive":
		fmt.Print("> ")
		reader.Scan()
		fmt.Println("got:" + reader.Text())
		fmt.Print("> ")
		reader.Scan()
		fmt.Println("got:" + reader.Text())
	case "late":
		time.Sleep(300 * time.Millisecond)
		fmt.Println("late-output")
	case "lines":
		for i := 1; i <= 20; i++ {
			fmt.Println("line-" + strconv.Itoa(i))
		}
	default:
		fmt.Println("unknown helper mode")
		os.Exit(2)
	}
	os.Exit(0)
}

func startTestHelperTask(t *testing.T, mgr *BackgroundTaskManager, mode string) *BackgroundTask {
	t.Helper()
	env := append(os.Environ(),
		"SESHAT_BASH_TEST_HELPER=1",
		"SESHAT_BASH_TEST_HELPER_MODE="+mode,
	)
	task, err := mgr.StartBackgroundTaskArgv(context.Background(), os.Args[0], []string{
		"-test.run=TestBackgroundTaskHelperProcess",
		"--",
	}, "", env)
	if err != nil {
		t.Fatalf("StartBackgroundTaskArgv: %v", err)
	}
	return task
}

func TestPollForOutput_EarlyExitsOnPrompt(t *testing.T) {
	mgr := newTestTaskManager(t)
	// Prints a prompt then blocks forever on read (no more input given) -
	// if pollForOutput didn't early-exit on the prompt, this test would have
	// to wait out the full maxWait every time.
	task := startTestHelperTask(t, mgr, "prompt-block")
	t.Cleanup(func() { _ = mgr.KillTask(task.ID) })

	reader, err := NewTaskOutputReaderFrom(mgr, task.ID)
	if err != nil {
		t.Fatalf("NewTaskOutputReaderFrom: %v", err)
	}

	start := time.Now()
	out, status := pollForOutput(context.Background(), mgr, task.ID, reader, 5*time.Second)
	elapsed := time.Since(start)

	if !strings.Contains(out, ">") {
		t.Errorf("expected the prompt in output, got %q", out)
	}
	if status != TaskStatusRunning && status != TaskStatusBackgrounded {
		t.Errorf("expected the task to still be running (blocked on read), got status %v", status)
	}
	// Generous margin over the 100ms poll interval - this just needs to prove
	// we returned long before the 5s ceiling, not pin an exact timing.
	if elapsed > 2*time.Second {
		t.Errorf("expected early exit on prompt detection well under 5s, took %v", elapsed)
	}
}

func TestPollForOutput_ReturnsOnTaskCompletion(t *testing.T) {
	mgr := newTestTaskManager(t)
	task := startTestHelperTask(t, mgr, "done")
	t.Cleanup(func() { _ = mgr.KillTask(task.ID) })

	reader, err := NewTaskOutputReaderFrom(mgr, task.ID)
	if err != nil {
		t.Fatalf("NewTaskOutputReaderFrom: %v", err)
	}

	start := time.Now()
	out, status := pollForOutput(context.Background(), mgr, task.ID, reader, 5*time.Second)
	elapsed := time.Since(start)

	if !strings.Contains(out, "done") {
		t.Errorf("expected %q in output, got %q", "done", out)
	}
	if status != TaskStatusCompleted {
		t.Errorf("expected TaskStatusCompleted, got %v", status)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected early exit on task completion well under 5s, took %v", elapsed)
	}
}

func TestWriteStdinTool_DrivesInteractiveProcess(t *testing.T) {
	mgr := newTestTaskManager(t)
	task := startTestHelperTask(t, mgr, "interactive")
	t.Cleanup(func() { _ = mgr.KillTask(task.ID) })

	// Let the process print its first prompt before we drive it.
	time.Sleep(200 * time.Millisecond)

	wsTool := NewWriteStdinTool()
	start := time.Now()
	result, err := wsTool.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"task_id": task.ID, "input": "hello"},
	}, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("write_stdin Call: %v", err)
	}
	if !strings.Contains(result.Content, "got:hello") {
		t.Errorf("expected the process to echo back our input, got:\n%s", result.Content)
	}
	// Default wait_ms is 500ms; early-exit on the second "> " prompt should
	// land well under that once "got:hello" plus the next prompt are printed.
	if elapsed > 400*time.Millisecond {
		t.Errorf("expected early exit before the 500ms default wait_ms, took %v", elapsed)
	}
}

func TestJobOutputTool_TimeoutWaitsForNewOutput(t *testing.T) {
	mgr := newTestTaskManager(t)
	task := startTestHelperTask(t, mgr, "late")
	t.Cleanup(func() { _ = mgr.KillTask(task.ID) })

	joTool := NewJobOutputTool()

	// Without a timeout, the output usually isn't there yet (job just started).
	immediate, err := joTool.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"job_id": task.ID},
	}, nil)
	if err != nil {
		t.Fatalf("job_output Call (immediate): %v", err)
	}
	if strings.Contains(immediate.Content, "late-output") {
		t.Skip("job finished suspiciously fast; timing assumption for this test didn't hold")
	}

	// With timeout_ms, it should wait for the job to finish and produce it.
	waited, err := joTool.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"job_id": task.ID, "timeout_ms": float64(3000)},
	}, nil)
	if err != nil {
		t.Fatalf("job_output Call (timeout_ms): %v", err)
	}
	if !strings.Contains(waited.Content, "late-output") {
		t.Errorf("expected timeout_ms to wait for the job's eventual output, got:\n%s", waited.Content)
	}
	if !strings.Contains(waited.Content, "status: completed") {
		t.Errorf("expected the job to be reported completed, got:\n%s", waited.Content)
	}
}

func TestJobOutputTool_NegativeOffsetTailsOutput(t *testing.T) {
	mgr := newTestTaskManager(t)
	task := startTestHelperTask(t, mgr, "lines")
	t.Cleanup(func() { _ = mgr.KillTask(task.ID) })

	joTool := NewJobOutputTool()
	result, err := joTool.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"job_id": task.ID, "timeout_ms": float64(3000), "offset": float64(-5)},
	}, nil)
	if err != nil {
		t.Fatalf("job_output Call: %v", err)
	}
	for _, want := range []string{"line-16", "line-17", "line-18", "line-19", "line-20"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("expected tail to contain %q, got:\n%s", want, result.Content)
		}
	}
	if strings.Contains(result.Content, "line-15\n") {
		t.Errorf("expected only the last 5 lines, but line-15 leaked in:\n%s", result.Content)
	}
}

func TestApplyLineOffset(t *testing.T) {
	content := "a\nb\nc\nd\ne"
	if got := applyLineOffset(content, 0); got != content {
		t.Errorf("offset=0 should be a no-op, got %q", got)
	}
	if got := applyLineOffset(content, 3); got != "c\nd\ne" {
		t.Errorf("offset=3 = %q, want %q", got, "c\nd\ne")
	}
	if got := applyLineOffset(content, -2); got != "d\ne" {
		t.Errorf("offset=-2 = %q, want %q", got, "d\ne")
	}
	if got := applyLineOffset(content, -100); got != content {
		t.Errorf("offset beyond start should clamp to full content, got %q", got)
	}
	if got := applyLineOffset(content, 100); got != "" {
		t.Errorf("offset beyond end should return empty, got %q", got)
	}
}

func TestApplyLineOffset_TrailingNewlineDoesNotCreatePhantomLine(t *testing.T) {
	// Buffered job output from `echo` always ends in "\n" - splitting that
	// naively on "\n" produces a spurious empty trailing line, throwing off
	// the line count a negative/tail offset resolves against.
	content := "line-1\nline-2\nline-3\nline-4\nline-5\n"
	got := applyLineOffset(content, -2)
	want := "line-4\nline-5\n"
	if got != want {
		t.Errorf("applyLineOffset(%q, -2) = %q, want %q", content, got, want)
	}
}
