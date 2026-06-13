package task

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/db"
)

// Task represents a task in the task list.
type Task struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"sessionId,omitempty"`
	Position    int            `json:"position,omitempty"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	Blocks      []string       `json:"blocks,omitempty"`
	BlockedBy   []string       `json:"blockedBy,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// TaskStore provides task storage, using SQLite when configured and falling
// back to an in-memory session-scoped store otherwise.
type TaskStore struct {
	mu        sync.RWMutex
	tasks     map[string]map[string]*Task
	positions map[string]int
	database  *db.DB
	dbPath    string
}

var globalTaskStore = NewTaskStore()

func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks:     make(map[string]map[string]*Task),
		positions: make(map[string]int),
	}
}

func NewSQLiteTaskStore(path string) (*TaskStore, error) {
	store := NewTaskStore()
	if err := store.ConfigureSQLite(path); err != nil {
		return nil, err
	}
	return store, nil
}

func GlobalTaskStore() *TaskStore {
	return globalTaskStore
}

func InitializeGlobalTaskStore(path string) error {
	return globalTaskStore.ConfigureSQLite(path)
}

func (s *TaskStore) ConfigureSQLite(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if path == "" {
		return nil
	}
	if s.database != nil && s.dbPath == path {
		return nil
	}
	database, err := db.Open(context.Background(), db.DefaultSQLiteConfig(path))
	if err != nil {
		return fmt.Errorf("open task sqlite store: %w", err)
	}
	if s.database != nil {
		_ = s.database.Close()
	}
	s.database = database
	s.dbPath = path
	return nil
}

func (s *TaskStore) CreateTask(ctx context.Context, sessionID, subject, description, activeForm string, metadata map[string]any) (*Task, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	now := time.Now().UTC()
	taskID := fmt.Sprintf("%d", now.UnixNano())
	if s.database != nil {
		position, err := s.database.NextSessionTaskPosition(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		task := &Task{
			ID:          taskID,
			SessionID:   sessionID,
			Position:    position,
			Subject:     subject,
			Description: description,
			Status:      TaskStatusPending,
			ActiveForm:  activeForm,
			Owner:       "",
			Blocks:      []string{},
			BlockedBy:   []string{},
			Metadata:    cloneTaskMetadata(metadata),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.database.CreateSessionTask(ctx, taskToDB(task)); err != nil {
			return nil, err
		}
		return cloneTask(task), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[sessionID] == nil {
		s.tasks[sessionID] = make(map[string]*Task)
	}
	s.positions[sessionID]++
	task := &Task{
		ID:          taskID,
		SessionID:   sessionID,
		Position:    s.positions[sessionID],
		Subject:     subject,
		Description: description,
		Status:      TaskStatusPending,
		ActiveForm:  activeForm,
		Blocks:      []string{},
		BlockedBy:   []string{},
		Metadata:    cloneTaskMetadata(metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tasks[sessionID][taskID] = task
	return cloneTask(task), nil
}

func (s *TaskStore) GetTask(ctx context.Context, sessionID, taskID string) (*Task, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	if s.database != nil {
		row, err := s.database.GetSessionTask(ctx, sessionID, taskID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("task not found: %s", taskID)
			}
			return nil, err
		}
		return dbToTask(row), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	sessionTasks := s.tasks[sessionID]
	task, ok := sessionTasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return cloneTask(task), nil
}

func (s *TaskStore) ListTasks(ctx context.Context, sessionID string) ([]*Task, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	if s.database != nil {
		rows, err := s.database.ListSessionTasks(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		tasks := make([]*Task, 0, len(rows))
		for _, row := range rows {
			tasks = append(tasks, dbToTask(row))
		}
		return tasks, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	sessionTasks := s.tasks[sessionID]
	tasks := make([]*Task, 0, len(sessionTasks))
	for _, task := range sessionTasks {
		tasks = append(tasks, cloneTask(task))
	}
	sortTasks(tasks)
	return tasks, nil
}

func (s *TaskStore) UpdateTask(ctx context.Context, sessionID, taskID string, updates map[string]any) (*Task, error) {
	task, err := s.GetTask(ctx, sessionID, taskID)
	if err != nil {
		return nil, err
	}
	applyTaskUpdates(task, updates)
	task.UpdatedAt = time.Now().UTC()
	if s.database != nil {
		if err := s.database.UpdateSessionTask(ctx, taskToDB(task)); err != nil {
			return nil, err
		}
		return cloneTask(task), nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[sessionID] == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	s.tasks[sessionID][taskID] = cloneTask(task)
	return cloneTask(task), nil
}

func (s *TaskStore) DeleteTask(ctx context.Context, sessionID, taskID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	if s.database != nil {
		return s.database.DeleteSessionTask(ctx, sessionID, taskID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[sessionID][taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	delete(s.tasks[sessionID], taskID)
	return nil
}

func (s *TaskStore) BlockTask(ctx context.Context, sessionID, blockerID, blockedID string) error {
	blocker, err := s.GetTask(ctx, sessionID, blockerID)
	if err != nil {
		return err
	}
	blocked, err := s.GetTask(ctx, sessionID, blockedID)
	if err != nil {
		return err
	}
	if !containsString(blocker.Blocks, blockedID) {
		blocker.Blocks = append(blocker.Blocks, blockedID)
	}
	if !containsString(blocked.BlockedBy, blockerID) {
		blocked.BlockedBy = append(blocked.BlockedBy, blockerID)
	}
	if _, err := s.UpdateTask(ctx, sessionID, blockerID, map[string]any{"blocks": blocker.Blocks}); err != nil {
		return err
	}
	if _, err := s.UpdateTask(ctx, sessionID, blockedID, map[string]any{"blockedBy": blocked.BlockedBy}); err != nil {
		return err
	}
	return nil
}

func taskToDB(task *Task) db.SessionTask {
	return db.SessionTask{
		SessionID:     task.SessionID,
		TaskID:        task.ID,
		Position:      task.Position,
		Subject:       task.Subject,
		Description:   task.Description,
		Status:        task.Status,
		ActiveForm:    task.ActiveForm,
		Owner:         task.Owner,
		Blocks:        append([]string(nil), task.Blocks...),
		BlockedBy:     append([]string(nil), task.BlockedBy...),
		Metadata:      cloneTaskMetadata(task.Metadata),
		CreatedAtUnix: task.CreatedAt.Unix(),
		UpdatedAtUnix: task.UpdatedAt.Unix(),
	}
}

func dbToTask(row db.SessionTask) *Task {
	return &Task{
		ID:          row.TaskID,
		SessionID:   row.SessionID,
		Position:    row.Position,
		Subject:     row.Subject,
		Description: row.Description,
		Status:      row.Status,
		ActiveForm:  row.ActiveForm,
		Owner:       row.Owner,
		Blocks:      append([]string(nil), row.Blocks...),
		BlockedBy:   append([]string(nil), row.BlockedBy...),
		Metadata:    cloneTaskMetadata(row.Metadata),
		CreatedAt:   time.Unix(row.CreatedAtUnix, 0).UTC(),
		UpdatedAt:   time.Unix(row.UpdatedAtUnix, 0).UTC(),
	}
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	clone := *task
	clone.Blocks = append([]string(nil), task.Blocks...)
	clone.BlockedBy = append([]string(nil), task.BlockedBy...)
	clone.Metadata = cloneTaskMetadata(task.Metadata)
	return &clone
}

func cloneTaskMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(metadata))
	for k, v := range metadata {
		clone[k] = v
	}
	return clone
}

func applyTaskUpdates(task *Task, updates map[string]any) {
	if subject, ok := updates["subject"].(string); ok {
		task.Subject = subject
	}
	if description, ok := updates["description"].(string); ok {
		task.Description = description
	}
	if activeForm, ok := updates["activeForm"].(string); ok {
		task.ActiveForm = activeForm
	}
	if status, ok := updates["status"].(string); ok {
		task.Status = status
	}
	if owner, ok := updates["owner"].(string); ok {
		task.Owner = owner
	}
	if blocks, ok := updates["blocks"].([]string); ok {
		task.Blocks = append([]string(nil), blocks...)
	}
	if blockedBy, ok := updates["blockedBy"].([]string); ok {
		task.BlockedBy = append([]string(nil), blockedBy...)
	}
	if metadata, ok := updates["metadata"].(map[string]any); ok {
		if task.Metadata == nil {
			task.Metadata = make(map[string]any)
		}
		for k, v := range metadata {
			if v == nil {
				delete(task.Metadata, k)
			} else {
				task.Metadata[k] = v
			}
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortTasks(tasks []*Task) {
	for i := 0; i < len(tasks); i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[j].Position < tasks[i].Position || (tasks[j].Position == tasks[i].Position && tasks[j].CreatedAt.Before(tasks[i].CreatedAt)) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
}
