package task

// taskListRenderMetadata is persisted into tool-result metadata so the TUI can
// render task_list without reverse-parsing formatted text.
type taskListRenderMetadata struct {
	ListType        string                         `json:"listType"`
	StatusFilter    string                         `json:"statusFilter"`
	Count           int                            `json:"count"`
	TodoTasks       []taskListTodoRenderItem       `json:"todoTasks,omitempty"`
	BackgroundTasks []taskListBackgroundRenderItem `json:"backgroundTasks,omitempty"`
	DeletedCount    int                            `json:"deletedCount,omitempty"`
}

type taskListTodoRenderItem struct {
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
	Owner      string `json:"owner,omitempty"`
}

type taskListBackgroundRenderItem struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Status  string `json:"status"`
}

type TaskGetTodoDetails struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	ActiveForm  string   `json:"activeForm,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Blocks      []string `json:"blocks,omitempty"`
	BlockedBy   []string `json:"blockedBy,omitempty"`
	CreatedAt   int64    `json:"createdAt"`
	UpdatedAt   int64    `json:"updatedAt"`
}

type taskGetRenderMetadata struct {
	TaskType   string              `json:"taskType,omitempty"`
	Todo       *TaskGetTodoDetails `json:"todo,omitempty"`
	Background *TaskDetails        `json:"background,omitempty"`
}

type TaskStopTodoDetails struct {
	ID             string `json:"id"`
	Subject        string `json:"subject"`
	PreviousStatus string `json:"previousStatus,omitempty"`
}

type taskStopRenderMetadata struct {
	TaskID   string               `json:"taskId"`
	TaskType string               `json:"taskType,omitempty"`
	Todo     *TaskStopTodoDetails `json:"todo,omitempty"`
	Command  string               `json:"command,omitempty"`
	Message  string               `json:"message,omitempty"`
}
