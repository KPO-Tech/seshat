package session

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/pubsub"
	"github.com/zeebo/xxh3"
)

type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
)

// HashID returns the XXH3 hash of a session ID (UUID) as a hex string.
func HashID(id string) string {
	h := xxh3.New()
	_, _ = h.WriteString(id)
	return fmt.Sprintf("%x", h.Sum(nil))
}

type Todo struct {
	ID          string     `json:"id,omitempty"`
	Content     string     `json:"content"`
	Description string     `json:"description,omitempty"`
	Status      TodoStatus `json:"status"`
	ActiveForm  string     `json:"active_form"`
	Owner       string     `json:"owner,omitempty"`
}

// HasIncompleteTodos returns true if there are any non-completed todos.
func HasIncompleteTodos(todos []Todo) bool {
	for _, todo := range todos {
		if todo.Status != TodoStatusCompleted {
			return true
		}
	}
	return false
}

type Session struct {
	ID               string
	ParentSessionID  string
	Title            string
	MessageCount     int64
	PromptTokens     int64
	CompletionTokens int64
	EstimatedUsage   bool
	SummaryMessageID string
	Cost             float64
	Todos            []Todo
	CreatedAt        int64
	UpdatedAt        int64
}

type Service interface {
	pubsub.Subscriber[Session]
	Create(ctx context.Context, title string) (Session, error)
	CreateTitleSession(ctx context.Context, parentSessionID string) (Session, error)
	CreateTaskSession(ctx context.Context, toolCallID, parentSessionID, title string) (Session, error)
	Get(ctx context.Context, id string) (Session, error)
	GetLast(ctx context.Context) (Session, error)
	List(ctx context.Context) ([]Session, error)
	Save(ctx context.Context, session Session) (Session, error)
	UpdateTitleAndUsage(ctx context.Context, sessionID, title string, promptTokens, completionTokens int64, cost float64) error
	Rename(ctx context.Context, id string, title string) error
	Delete(ctx context.Context, id string) error

	// Agent tool session management
	CreateAgentToolSessionID(messageID, toolCallID string) string
	ParseAgentToolSessionID(sessionID string) (messageID string, toolCallID string, ok bool)
	IsAgentToolSession(sessionID string) bool
}

// MarshalTodos / UnmarshalTodos helpers used by workspace adapter.
func MarshalTodos(todos []Todo) ([]byte, error) {
	return json.Marshal(todos)
}

func UnmarshalTodos(data []byte) ([]Todo, error) {
	var todos []Todo
	if err := json.Unmarshal(data, &todos); err != nil {
		return nil, err
	}
	return todos, nil
}
