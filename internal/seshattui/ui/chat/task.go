package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/message"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/session"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/list"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/styles"
	taskTool "github.com/KPO-Tech/seshat-runtime/internal/tools/task"
	"github.com/charmbracelet/x/ansi"
)

type TaskListToolMessageItem struct{ *baseToolMessageItem }
type TaskGetToolMessageItem struct{ *baseToolMessageItem }
type TaskStopToolMessageItem struct{ *baseToolMessageItem }
type PlanTaskListMessageItem struct {
	*list.Versioned
	*highlightableMessageItem
	*cachedMessageItem
	*focusableMessageItem

	id    string
	sty   *styles.Styles
	tasks []planTaskItem
}

var _ ToolMessageItem = (*TaskListToolMessageItem)(nil)
var _ ToolMessageItem = (*TaskGetToolMessageItem)(nil)
var _ ToolMessageItem = (*TaskStopToolMessageItem)(nil)
var _ MessageItem = (*PlanTaskListMessageItem)(nil)

func NewTaskListToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return &TaskListToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &TaskListToolRenderContext{}, canceled)}
}

func NewTaskGetToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return &TaskGetToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &TaskGetToolRenderContext{}, canceled)}
}

func NewTaskStopToolMessageItem(sty *styles.Styles, toolCall message.ToolCall, result *message.ToolResult, canceled bool) ToolMessageItem {
	return &TaskStopToolMessageItem{newBaseToolMessageItem(sty, toolCall, result, &TaskStopToolRenderContext{}, canceled)}
}

type TaskListToolRenderContext struct{}
type TaskGetToolRenderContext struct{}
type TaskStopToolRenderContext struct{}

type planTaskItem struct {
	ID         string
	Subject    string
	Status     string
	ActiveForm string
}

// taskCreateParams is the input of task_create: one task in the top-level
// fields, or several in Tasks.
type taskCreateParams struct {
	Subject    string             `json:"subject"`
	ActiveForm string             `json:"activeForm"`
	Tasks      []taskCreateParams `json:"tasks"`
}

type taskCreatedRef struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
}

type taskCreateResult struct {
	Task  taskCreatedRef   `json:"task"`
	Tasks []taskCreatedRef `json:"tasks"`
}

// taskUpdateParams is the input of task_update: one update in the top-level
// fields, or several in Updates.
type taskUpdateParams struct {
	TaskID     string             `json:"taskId"`
	Subject    string             `json:"subject"`
	Status     string             `json:"status"`
	ActiveForm string             `json:"activeForm"`
	Updates    []taskUpdateParams `json:"updates"`
}

func NewPlanTaskListMessageItem(sty *styles.Styles, messageID string, tasks []planTaskItem) *PlanTaskListMessageItem {
	v := list.NewVersioned()
	return &PlanTaskListMessageItem{
		Versioned:                v,
		highlightableMessageItem: defaultHighlighter(sty, v),
		cachedMessageItem:        &cachedMessageItem{},
		focusableMessageItem:     newFocusableMessageItem(v),
		id:                       messageID + ":plan-tasks",
		sty:                      sty,
		tasks:                    tasks,
	}
}

func BuildPlanTaskListItem(sty *styles.Styles, msg *message.Message, toolResults map[string]message.ToolResult) *PlanTaskListMessageItem {
	tasks := collectPlanTaskItems(msg, toolResults)
	if len(tasks) == 0 {
		return nil
	}
	return NewPlanTaskListMessageItem(sty, msg.ID, tasks)
}

func collectPlanTaskItems(msg *message.Message, toolResults map[string]message.ToolResult) []planTaskItem {
	if msg == nil {
		return nil
	}
	byID := make(map[string]int)
	var tasks []planTaskItem
	anon := 0
	for _, part := range msg.Parts {
		tc, ok := part.(message.ToolCall)
		if !ok {
			continue
		}
		switch tc.Name {
		case taskTool.ToolNameTaskCreate:
			var params taskCreateParams
			_ = json.Unmarshal([]byte(tc.Input), &params)
			var out taskCreateResult
			if result, ok := toolResults[tc.ID]; ok {
				_ = json.Unmarshal([]byte(firstNonEmpty(result.Data, result.Content)), &out)
			}
			requested := params.Tasks
			created := out.Tasks
			if len(requested) == 0 {
				requested = []taskCreateParams{params}
				created = []taskCreatedRef{out.Task}
			}
			for i, p := range requested {
				item := planTaskItem{
					Subject:    p.Subject,
					Status:     taskTool.TaskStatusPending,
					ActiveForm: p.ActiveForm,
				}
				if i < len(created) {
					item.ID = created[i].ID
					if item.Subject == "" {
						item.Subject = created[i].Subject
					}
				}
				if item.ID == "" {
					anon++
					item.ID = fmt.Sprintf("new-%d", anon)
				}
				if item.Subject == "" {
					item.Subject = "Untitled task"
				}
				byID[item.ID] = len(tasks)
				tasks = append(tasks, item)
			}
		case taskTool.ToolNameTaskUpdate:
			var params taskUpdateParams
			_ = json.Unmarshal([]byte(tc.Input), &params)
			updates := params.Updates
			if len(updates) == 0 {
				updates = []taskUpdateParams{params}
			}
			for _, u := range updates {
				tasks = applyTaskUpdate(tasks, byID, u)
			}
		}
	}
	return tasks
}

// applyTaskUpdate applies one update to the list, adding the task when it was
// created before this message.
func applyTaskUpdate(tasks []planTaskItem, byID map[string]int, params taskUpdateParams) []planTaskItem {
	if params.TaskID == "" {
		return tasks
	}
	idx, ok := byID[params.TaskID]
	if !ok {
		item := planTaskItem{ID: params.TaskID, Subject: params.TaskID, Status: taskTool.TaskStatusPending}
		byID[item.ID] = len(tasks)
		tasks = append(tasks, item)
		idx = len(tasks) - 1
	}
	if params.Subject != "" {
		tasks[idx].Subject = params.Subject
	}
	if params.Status != "" {
		tasks[idx].Status = params.Status
	}
	if params.ActiveForm != "" {
		tasks[idx].ActiveForm = params.ActiveForm
	}
	return tasks
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type taskListToolParams struct {
	Status   string `json:"status"`
	ListType string `json:"listType"`
}

type taskListToolMetadataEnvelope struct {
	TaskList taskListToolMetadata `json:"task_list"`
}

type taskListToolMetadata struct {
	ListType        string                           `json:"listType"`
	StatusFilter    string                           `json:"statusFilter"`
	Count           int                              `json:"count"`
	TodoTasks       []taskListTodoToolMetadata       `json:"todoTasks"`
	BackgroundTasks []taskListBackgroundToolMetadata `json:"backgroundTasks"`
	DeletedCount    int                              `json:"deletedCount"`
}

type taskListTodoToolMetadata struct {
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
	Owner      string `json:"owner"`
}

type taskListBackgroundToolMetadata struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Status  string `json:"status"`
}

type taskGetToolMetadataEnvelope struct {
	TaskGet taskGetToolMetadata `json:"task_get"`
}

type taskGetToolMetadata struct {
	TaskType   string                       `json:"taskType"`
	Todo       *taskTool.TaskGetTodoDetails `json:"todo"`
	Background *taskTool.TaskDetails        `json:"background"`
}

type taskStopToolMetadataEnvelope struct {
	TaskStop taskStopToolMetadata `json:"task_stop"`
}

type taskStopToolMetadata struct {
	TaskID   string                        `json:"taskId"`
	TaskType string                        `json:"taskType"`
	Todo     *taskTool.TaskStopTodoDetails `json:"todo"`
	Command  string                        `json:"command"`
	Message  string                        `json:"message"`
}

func (t *TaskListToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Task List", opts.Anim, opts.Compact)
	}
	var params taskListToolParams
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Task List", cappedWidth)
	}
	var meta taskListToolMetadataEnvelope
	_ = json.Unmarshal([]byte(metadataOrEmpty(opts.Result)), &meta)
	if params.ListType == "" {
		params.ListType = meta.TaskList.ListType
	}
	if params.ListType == "" {
		params.ListType = "background"
	}
	if params.Status == "" {
		params.Status = meta.TaskList.StatusFilter
	}
	if params.Status == "" {
		params.Status = "running"
	}

	summary := taskListSummary(params, meta.TaskList)
	header := toolHeader(sty, opts.Status, "Task List", cappedWidth, opts.Compact, summary)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	body := renderTaskListBody(sty, bodyWidth(cappedWidth), meta.TaskList, opts)
	if body == "" {
		return header
	}
	return joinToolParts(header, sty.Tool.Body.Render(body))
}

func (t *TaskGetToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Task Get", opts.Anim, opts.Compact)
	}
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Task Get", cappedWidth)
	}
	header := toolHeader(sty, opts.Status, "Task Get", cappedWidth, opts.Compact, params.TaskID)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	var meta taskGetToolMetadataEnvelope
	_ = json.Unmarshal([]byte(metadataOrEmpty(opts.Result)), &meta)
	if meta.TaskGet.Todo == nil && meta.TaskGet.Background == nil {
		if opts.HasEmptyResult() {
			return header
		}
		body := sty.Tool.Body.Render(toolOutputPlainContent(sty, opts.Result.Content, bodyWidth(cappedWidth), opts.ExpandedContent))
		return joinToolParts(header, body)
	}
	if meta.TaskGet.Todo != nil {
		lines := []string{
			fmt.Sprintf("Subject: %s", meta.TaskGet.Todo.Subject),
			fmt.Sprintf("Status: %s", meta.TaskGet.Todo.Status),
		}
		if meta.TaskGet.Todo.Owner != "" {
			lines = append(lines, fmt.Sprintf("Owner: %s", meta.TaskGet.Todo.Owner))
		}
		if meta.TaskGet.Todo.ActiveForm != "" {
			lines = append(lines, fmt.Sprintf("Active: %s", meta.TaskGet.Todo.ActiveForm))
		}
		if len(meta.TaskGet.Todo.BlockedBy) > 0 {
			lines = append(lines, fmt.Sprintf("Blocked by: %s", strings.Join(meta.TaskGet.Todo.BlockedBy, ", ")))
		}
		if len(meta.TaskGet.Todo.Blocks) > 0 {
			lines = append(lines, fmt.Sprintf("Blocks: %s", strings.Join(meta.TaskGet.Todo.Blocks, ", ")))
		}
		lines = append(lines, fmt.Sprintf("Created: %s", formatUnix(meta.TaskGet.Todo.CreatedAt)))
		lines = append(lines, fmt.Sprintf("Updated: %s", formatUnix(meta.TaskGet.Todo.UpdatedAt)))
		if meta.TaskGet.Todo.Description != "" {
			lines = append(lines, "", meta.TaskGet.Todo.Description)
		}
		body := sty.Tool.Body.Render(toolOutputPlainContent(sty, strings.Join(lines, "\n"), bodyWidth(cappedWidth), opts.ExpandedContent))
		return joinToolParts(header, body)
	}
	lines := []string{
		fmt.Sprintf("Status: %s", meta.TaskGet.Background.Status),
		fmt.Sprintf("Started: %s", formatUnix(meta.TaskGet.Background.StartTime)),
	}
	// A background agent has no command: show what it was asked to do instead.
	if meta.TaskGet.Background.Command != "" {
		lines = append(lines[:1], append([]string{fmt.Sprintf("Command: %s", meta.TaskGet.Background.Command)}, lines[1:]...)...)
	} else if description := strings.TrimSpace(meta.TaskGet.Background.Description); description != "" {
		lines = append(lines[:1], append([]string{fmt.Sprintf("Task: %s", firstTaskLine(description, 120))}, lines[1:]...)...)
	}
	if meta.TaskGet.Background.EndTime != nil {
		lines = append(lines, fmt.Sprintf("Ended: %s", formatUnix(*meta.TaskGet.Background.EndTime)))
	}
	if meta.TaskGet.Background.ExitCode != nil {
		lines = append(lines, fmt.Sprintf("Exit code: %d", *meta.TaskGet.Background.ExitCode))
	}
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, strings.Join(lines, "\n"), bodyWidth(cappedWidth), opts.ExpandedContent))
	return joinToolParts(header, body)
}

func (t *TaskStopToolRenderContext) RenderTool(sty *styles.Styles, width int, opts *ToolRenderOpts) string {
	cappedWidth := width
	if opts.IsPending() {
		return pendingTool(sty, "Task Stop", opts.Anim, opts.Compact)
	}
	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(opts.ToolCall.Input), &params); err != nil {
		return invalidInputContent(sty, opts, "Task Stop", cappedWidth)
	}
	var meta taskStopToolMetadataEnvelope
	_ = json.Unmarshal([]byte(metadataOrEmpty(opts.Result)), &meta)
	headerParam := params.TaskID
	if meta.TaskStop.TaskType != "" {
		headerParam = fmt.Sprintf("%s (%s)", params.TaskID, meta.TaskStop.TaskType)
	}
	if meta.TaskStop.Todo != nil {
		headerParam = fmt.Sprintf("%s (%s)", meta.TaskStop.Todo.Subject, meta.TaskStop.TaskType)
	}
	header := toolHeader(sty, opts.Status, "Task Stop", cappedWidth, opts.Compact, headerParam)
	if opts.Compact {
		return header
	}
	if earlyState, ok := toolEarlyStateContent(sty, opts, cappedWidth); ok {
		return joinToolParts(header, earlyState)
	}
	if meta.TaskStop.Message == "" && opts.HasEmptyResult() {
		return header
	}
	lines := []string{}
	if meta.TaskStop.Message != "" {
		lines = append(lines, meta.TaskStop.Message)
	}
	if meta.TaskStop.Todo != nil {
		if meta.TaskStop.Todo.PreviousStatus != "" {
			lines = append(lines, fmt.Sprintf("Previous status: %s", meta.TaskStop.Todo.PreviousStatus))
		}
	}
	if meta.TaskStop.Command != "" {
		lines = append(lines, fmt.Sprintf("Command: %s", meta.TaskStop.Command))
	}
	if len(lines) == 0 {
		lines = append(lines, opts.Result.Content)
	}
	body := sty.Tool.Body.Render(toolOutputPlainContent(sty, strings.Join(lines, "\n"), bodyWidth(cappedWidth), opts.ExpandedContent))
	return joinToolParts(header, body)
}

func (p *PlanTaskListMessageItem) ID() string { return p.id }

func (p *PlanTaskListMessageItem) Finished() bool { return true }

func (p *PlanTaskListMessageItem) RawRender(width int) string {
	cappedWidth := cappedMessageWidth(width)
	if cached, height, ok := p.getCachedRender(cappedWidth); ok {
		return p.renderHighlighted(cached, cappedWidth, height)
	}
	bodyWidth := max(20, cappedWidth-4)
	title := p.sty.Tool.ResultItemName.Render("Plan")
	count := p.sty.Tool.ResultItemDesc.Render(fmt.Sprintf("%d tasks", len(p.tasks)))
	lines := []string{title + " " + count}
	for _, task := range p.tasks {
		icon := planTaskStatusIcon(p.sty, task.Status)
		subject := task.Subject
		if task.Status == taskTool.TaskStatusInProgress && task.ActiveForm != "" {
			subject = task.ActiveForm
		}
		subject = ansi.Truncate(subject, bodyWidth-4, "...")
		lines = append(lines, fmt.Sprintf("%s %s", icon, p.sty.Tool.ContentText.Render(subject)))
	}
	rendered := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		PaddingLeft(1).
		Width(cappedWidth).
		Render(strings.Join(lines, "\n"))
	p.setCachedRender(rendered, cappedWidth, lipgloss.Height(rendered))
	return p.renderHighlighted(rendered, cappedWidth, lipgloss.Height(rendered))
}

func (p *PlanTaskListMessageItem) Render(width int) string {
	focused := p.sty.Messages.AssistantFocused.Render()
	blurred := p.sty.Messages.AssistantBlurred.Render()
	rendered := p.RawRender(width)
	lines := strings.Split(rendered, "\n")
	prefix := blurred
	if p.focused {
		prefix = focused
	}
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func planTaskStatusIcon(sty *styles.Styles, status string) string {
	switch status {
	case taskTool.TaskStatusCompleted:
		return sty.Tool.TodoCompletedIcon.Render("✓")
	case taskTool.TaskStatusInProgress:
		return sty.Tool.TodoInProgressIcon.Render("●")
	case taskTool.TaskStatusDeleted:
		return sty.Tool.StateCancelled.Render("×")
	default:
		return sty.Tool.TodoPendingIcon.Render("○")
	}
}

func metadataOrEmpty(result *message.ToolResult) string {
	if result == nil {
		return ""
	}
	return result.Metadata
}

func bodyWidth(cappedWidth int) int {
	return cappedWidth - toolBodyLeftPaddingTotal
}

func taskListSummary(params taskListToolParams, meta taskListToolMetadata) string {
	count := meta.Count
	if count == 0 {
		return fmt.Sprintf("%s · no tasks", params.ListType)
	}
	parts := []string{fmt.Sprintf("%s · %d task(s)", params.ListType, count)}
	if len(meta.TodoTasks) > 0 {
		completed := 0
		active := ""
		for _, task := range meta.TodoTasks {
			if task.Status == taskTool.TaskStatusCompleted {
				completed++
			}
			if active == "" && task.Status == taskTool.TaskStatusInProgress {
				if task.ActiveForm != "" {
					active = task.ActiveForm
				} else {
					active = task.Subject
				}
			}
		}
		parts = append(parts, fmt.Sprintf("%d/%d done", completed, len(meta.TodoTasks)))
		if active != "" {
			parts = append(parts, active)
		}
	}
	if len(meta.BackgroundTasks) > 0 {
		running := 0
		for _, task := range meta.BackgroundTasks {
			if task.Status == "running" || task.Status == "backgrounded" {
				running++
			}
		}
		parts = append(parts, fmt.Sprintf("%d active background", running))
	}
	return strings.Join(parts, " · ")
}

func renderTaskListBody(sty *styles.Styles, bodyWidth int, meta taskListToolMetadata, opts *ToolRenderOpts) string {
	sections := []string{}
	if len(meta.TodoTasks) > 0 {
		todos := make([]session.Todo, 0, len(meta.TodoTasks))
		for _, task := range meta.TodoTasks {
			status := session.TodoStatusPending
			switch task.Status {
			case taskTool.TaskStatusCompleted:
				status = session.TodoStatusCompleted
			case taskTool.TaskStatusInProgress:
				status = session.TodoStatusInProgress
			}
			todos = append(todos, session.Todo{Content: task.Subject, Status: status, ActiveForm: task.ActiveForm})
		}
		section := FormatTodosList(sty, todos, styles.SpinnerIcon, bodyWidth)
		if meta.DeletedCount > 0 {
			section = strings.Join([]string{section, fmt.Sprintf("%d deleted hidden", meta.DeletedCount)}, "\n")
		}
		sections = append(sections, section)
	}
	if len(meta.BackgroundTasks) > 0 {
		lines := make([]string, 0, len(meta.BackgroundTasks))
		for _, task := range meta.BackgroundTasks {
			cmd := task.Command
			if len(cmd) > 80 {
				cmd = cmd[:77] + "..."
			}
			lines = append(lines, fmt.Sprintf("[%s] %s - %s", task.Status, task.ID, cmd))
		}
		sections = append(sections, toolOutputPlainContent(sty, strings.Join(lines, "\n"), bodyWidth, opts.ExpandedContent))
	}
	if len(sections) == 0 && opts.HasResult() && opts.Result.Content != "" {
		return toolOutputPlainContent(sty, opts.Result.Content, bodyWidth, opts.ExpandedContent)
	}
	return strings.Join(sections, "\n\n")
}

// firstTaskLine is the first line of s, cut to at most n runes.
func firstTaskLine(s string, n int) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) > n {
		return string(runes[:n-1]) + "…"
	}
	return string(runes)
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return "unknown"
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}
