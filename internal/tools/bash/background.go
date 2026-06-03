package bash

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"golang.org/x/sys/unix"
)

// TaskOutput manages task output storage.
type TaskOutput struct {
	Path         string
	StdoutBuffer []byte
	StderrBuffer []byte
	mu           sync.Mutex
}

// NewTaskOutput creates a new task output.
func NewTaskOutput(path string) *TaskOutput {
	return &TaskOutput{Path: path}
}

// BackgroundTask represents a background task.
type BackgroundTask struct {
	ID        string
	Command   string
	Process   *exec.Cmd
	Output    *TaskOutput
	Status    TaskStatus
	StartTime time.Time
	EndTime   *time.Time
	ExitCode  int

	mu          sync.Mutex
	done        chan struct{}
	cleanupStop chan struct{} // cancels the auto-remove timer
	stdinPipe   io.WriteCloser
}

// TaskStatus represents background task status.
type TaskStatus int

const (
	TaskStatusRunning      TaskStatus = iota // Task is running
	TaskStatusBackgrounded                   // Task was backgrounded
	TaskStatusCompleted                      // Task completed normally
	TaskStatusKilled                         // Task was killed
	TaskStatusTimeout                        // Task timed out
)

// BackgroundTaskManager manages background tasks.
type BackgroundTaskManager struct {
	tasks   map[string]*BackgroundTask
	mu      sync.RWMutex
	taskDir string
}

// globalTaskManager is set by NewTool() so that package-level helpers
// (GlobalTaskManager, NewTaskOutputReader) work for callers that don't
// hold a Tool reference (e.g. the monitor and task-list tools).
var globalTaskManager *BackgroundTaskManager

// GlobalTaskManager returns the manager created by the most recent NewTool() call.
func GlobalTaskManager() *BackgroundTaskManager {
	return globalTaskManager
}

// NewBackgroundTaskManager creates a new, isolated manager.
func NewBackgroundTaskManager(taskDir string) *BackgroundTaskManager {
	if taskDir == "" {
		taskDir = runtimepath.BashTasksDir("")
	}
	return &BackgroundTaskManager{
		tasks:   make(map[string]*BackgroundTask),
		taskDir: taskDir,
	}
}

// Init ensures task directory exists.
func (m *BackgroundTaskManager) Init() error {
	return os.MkdirAll(m.taskDir, 0o755)
}

// StartBackgroundTask starts a command in the background.
func (m *BackgroundTaskManager) StartBackgroundTask(
	ctx context.Context,
	command string,
	workingDir string,
	env []string,
	shell string,
) (*BackgroundTask, error) {
	if err := m.Init(); err != nil {
		return nil, fmt.Errorf("init task dir: %w", err)
	}

	taskID, err := generateTaskID()
	if err != nil {
		return nil, fmt.Errorf("generate task id: %w", err)
	}
	taskPath := filepath.Join(m.taskDir, taskID+".output")

	task := &BackgroundTask{
		ID:          taskID,
		Command:     command,
		Output:      NewTaskOutput(taskPath),
		Status:      TaskStatusRunning,
		StartTime:   time.Now(),
		done:        make(chan struct{}),
		cleanupStop: make(chan struct{}),
	}

	// Create the output file. We open it, pass it to the child, then close our
	// handle — the child inherits the fd through exec and keeps writing to it.
	file, err := os.Create(taskPath)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}

	cmdPath, cmdArgs, sandboxEnv, _ := commandWithLandlock(shell, []string{"-c", command}, workingDir)
	cmd := exec.CommandContext(ctx, cmdPath, cmdArgs...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	cmd.Env = append(env, sandboxEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = file
	cmd.Stderr = file

	// Create a stdin pipe so write_stdin can inject input later.
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		file.Close()
		os.Remove(taskPath)
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		file.Close()
		os.Remove(taskPath)
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Safe to close parent's handle — child already has its own fd copy.
	file.Close()

	task.stdinPipe = stdinPipe
	task.Process = cmd
	m.mu.Lock()
	m.tasks[taskID] = task
	m.mu.Unlock()

	go m.waitForTask(task)

	return task, nil
}

func (m *BackgroundTaskManager) waitForTask(task *BackgroundTask) {
	defer close(task.done)

	// Close the stdin pipe so the subprocess receives EOF if it reads stdin.
	// This must happen before Wait(), otherwise Wait may block on a pending read.
	task.mu.Lock()
	pipe := task.stdinPipe
	task.stdinPipe = nil
	task.mu.Unlock()
	if pipe != nil {
		pipe.Close()
	}

	err := task.Process.Wait()

	task.mu.Lock()
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			task.ExitCode = status.ExitStatus()
		} else {
			task.ExitCode = 1
		}
	}
	task.Status = TaskStatusCompleted
	now := time.Now()
	task.EndTime = &now
	task.mu.Unlock()

	// Auto-remove after 5 minutes unless KillTask already cancelled the timer.
	select {
	case <-task.cleanupStop:
	case <-time.After(5 * time.Minute):
		m.RemoveTask(task.ID)
	}
}

// GetTask retrieves a task by ID.
func (m *BackgroundTaskManager) GetTask(id string) *BackgroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[id]
}

// ListTasks lists all background tasks.
func (m *BackgroundTaskManager) ListTasks() []*BackgroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tasks := make([]*BackgroundTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// KillTask kills a background task and cancels its auto-remove timer.
func (m *BackgroundTaskManager) KillTask(id string) error {
	task := m.GetTask(id)
	if task == nil {
		return fmt.Errorf("task not found: %s", id)
	}

	// Cancel the auto-cleanup goroutine before killing so it doesn't race.
	select {
	case <-task.cleanupStop: // already stopped
	default:
		close(task.cleanupStop)
	}

	return KillTaskProcess(task)
}

// KillTaskProcess kills a task's entire process group.
func KillTaskProcess(task *BackgroundTask) error {
	if task.Process == nil || task.Process.Process == nil {
		return nil
	}

	pid := task.Process.Process.Pid
	pgid, err := unix.Getpgid(pid)
	if err != nil {
		return task.Process.Process.Kill()
	}
	if err := unix.Kill(-pgid, unix.SIGKILL); err != nil {
		return err
	}

	task.mu.Lock()
	task.Status = TaskStatusKilled
	task.mu.Unlock()
	return nil
}

// RemoveTask removes a task and its output file.
func (m *BackgroundTaskManager) RemoveTask(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task, ok := m.tasks[id]; ok {
		if task.Output != nil && task.Output.Path != "" {
			os.Remove(task.Output.Path)
		}
		delete(m.tasks, id)
	}
}

// WaitForTask waits for a task to complete within timeout.
func (m *BackgroundTaskManager) WaitForTask(id string, timeout time.Duration) (*BackgroundTask, error) {
	task := m.GetTask(id)
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	select {
	case <-task.done:
		return task, nil
	case <-time.After(timeout):
		return task, fmt.Errorf("timeout waiting for task %s", id)
	}
}

// WriteStdin writes data to a running task's stdin pipe.
// Returns an error if the task has no stdin pipe (already finished or stdin
// was not set up) or if the write fails (e.g. broken pipe).
func (m *BackgroundTaskManager) WriteStdin(id string, data string) error {
	task := m.GetTask(id)
	if task == nil {
		return fmt.Errorf("task not found: %s", id)
	}

	task.mu.Lock()
	pipe := task.stdinPipe
	status := task.Status
	task.mu.Unlock()

	if pipe == nil {
		if status == TaskStatusCompleted || status == TaskStatusKilled || status == TaskStatusTimeout {
			return fmt.Errorf("task %s has already finished", id)
		}
		return fmt.Errorf("task %s has no stdin pipe", id)
	}

	_, err := io.WriteString(pipe, data)
	if err != nil {
		return fmt.Errorf("write to task %s stdin: %w", id, err)
	}
	return nil
}

// Thread-safe getters on BackgroundTask.

func (t *BackgroundTask) GetStatus() TaskStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status
}

func (t *BackgroundTask) GetExitCode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ExitCode
}

func (t *BackgroundTask) GetEndTime() *time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.EndTime
}

// ── Output reading ─────────────────────────────────────────────────────────────

// TaskOutputReader reads task output incrementally.
type TaskOutputReader struct {
	task *BackgroundTask
	pos  int64
}

// NewTaskOutputReader creates a reader using the global task manager.
// Pass a non-nil manager to use a specific instance instead.
func NewTaskOutputReader(taskID string) (*TaskOutputReader, error) {
	return NewTaskOutputReaderFrom(globalTaskManager, taskID)
}

// NewTaskOutputReaderFrom creates a reader from a specific manager.
func NewTaskOutputReaderFrom(manager *BackgroundTaskManager, taskID string) (*TaskOutputReader, error) {
	if manager == nil {
		return nil, fmt.Errorf("no background task manager available")
	}
	task := manager.GetTask(taskID)
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return &TaskOutputReader{task: task}, nil
}

// ReadOutput reads new output since the last call.
func (r *TaskOutputReader) ReadOutput() (string, error) {
	if r.task.Output == nil || r.task.Output.Path == "" {
		return "", nil
	}
	content, err := os.ReadFile(r.task.Output.Path)
	if err != nil {
		return "", err
	}
	if r.pos >= int64(len(content)) {
		return "", nil
	}
	newOutput := string(content[r.pos:])
	r.pos = int64(len(content))
	return newOutput, nil
}

// IsRunning checks if the task is still running.
func (r *TaskOutputReader) IsRunning() bool {
	st := r.task.GetStatus()
	return st == TaskStatusRunning || st == TaskStatusBackgrounded
}

// ── Helpers ────────────────────────────────────────────────────────────────────

// parseBackgroundCommand strips a trailing & from the command.
func parseBackgroundCommand(command string) (isBackground bool, actualCommand string) {
	command = strings.TrimSpace(command)
	if strings.HasSuffix(command, "&") {
		return true, strings.TrimSpace(command[:len(command)-1])
	}
	fields := strings.Fields(command)
	if len(fields) > 0 && fields[len(fields)-1] == "&" {
		return true, strings.TrimSpace(strings.Join(fields[:len(fields)-1], " "))
	}
	return false, command
}

// generateTaskID returns a cryptographically random 16-byte hex task ID.
func generateTaskID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "task_" + hex.EncodeToString(b), nil
}
