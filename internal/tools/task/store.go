package task

import (
	"fmt"
	"sync"
	"time"
)

// Task represents a task in the task list
type Task struct {
	ID          string         `json:"id"`
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

// TaskStore provides in-memory task storage
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

// Global task store
var globalTaskStore = &TaskStore{
	tasks: make(map[string]*Task),
}

// NewTaskStore creates a new task store
func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[string]*Task),
	}
}

// GlobalTaskStore returns the global task store
func GlobalTaskStore() *TaskStore {
	return globalTaskStore
}

// CreateTask creates a new task
func (s *TaskStore) CreateTask(subject, description, activeForm string, metadata map[string]any) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now()

	task := &Task{
		ID:          taskID,
		Subject:     subject,
		Description: description,
		Status:      TaskStatusPending,
		ActiveForm:  activeForm,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.tasks[taskID] = task
	return task, nil
}

// GetTask retrieves a task by ID
func (s *TaskStore) GetTask(taskID string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return task, nil
}

// ListTasks lists all tasks
func (s *TaskStore) ListTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// UpdateTask updates a task
func (s *TaskStore) UpdateTask(taskID string, updates map[string]any) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

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
		task.Blocks = blocks
	}
	if blockedBy, ok := updates["blockedBy"].([]string); ok {
		task.BlockedBy = blockedBy
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

	task.UpdatedAt = time.Now()
	return task, nil
}

// DeleteTask deletes a task
func (s *TaskStore) DeleteTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	delete(s.tasks, taskID)
	return nil
}

// BlockTask adds a block relationship
func (s *TaskStore) BlockTask(blockerID, blockedID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blocker, ok := s.tasks[blockerID]
	if !ok {
		return fmt.Errorf("task not found: %s", blockerID)
	}
	blocked, ok := s.tasks[blockedID]
	if !ok {
		return fmt.Errorf("task not found: %s", blockedID)
	}

	// Add blockedID to blocker's blocks
	hasBlock := false
	for _, b := range blocker.Blocks {
		if b == blockedID {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		blocker.Blocks = append(blocker.Blocks, blockedID)
	}

	// Add blockerID to blocked's blockedBy
	hasBlockedBy := false
	for _, b := range blocked.BlockedBy {
		if b == blockerID {
			hasBlockedBy = true
			break
		}
	}
	if !hasBlockedBy {
		blocked.BlockedBy = append(blocked.BlockedBy, blockerID)
	}

	return nil
}
