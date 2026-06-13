package task

import tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"

// Tool name constants
const (
	ToolNameTaskStop   = "task_stop"
	ToolNameTaskList   = "task_list"
	ToolNameTaskGet    = "task_get"
	ToolNameTaskOutput = "task_output"
	ToolNameTaskCreate = "task_create"
	ToolNameTaskUpdate = "task_update"
)

// Search hints
const (
	SearchHintTaskStop   = "stop tracking a task or kill a background task"
	SearchHintTaskList   = "list tracked tasks or background tasks"
	SearchHintTaskGet    = "get details of a tracked task or background task by ID"
	SearchHintTaskOutput = "read output from a background task"
	SearchHintTaskCreate = "create a task in the task list"
	SearchHintTaskUpdate = "update a task in the task list"
)

// Tool descriptions
const (
	ToolDescriptionTaskStop = `
Stop tracking a session task, or stop a running background task by its ID.

Default behavior:
- If the ID matches a session task, the tool stops tracking that task first
- If no session task matches, it falls back to background runtime tasks

Use this tool when:
- A tracked execution step is no longer relevant and should be removed
- A long-running background task must be terminated
- A monitored command is stuck or taking too long

Parameters:
- task_id: The ID of the task to stop
- taskType (optional): 'todo' or 'background' to force one mode
`

	ToolDescriptionTaskList = `
List tracked session tasks and/or background runtime tasks.

Default behavior:
- If the current session already has tracked tasks, the tool defaults to listing those tasks
- Otherwise it defaults to listing running background tasks

Parameters:
- listType: 'todo', 'background', or 'all'
- status: for todo tasks use 'all', 'running', or 'completed'; for background tasks use 'running' or 'completed'

Use this tool to:
- Inspect the current session task checklist
- Review tracked execution progress
- Check running background jobs when needed
`

	ToolDescriptionTaskGet = `
Get detailed information about a specific tracked task or background task by its ID.

Default behavior:
- If the ID matches a session task, the tool returns the tracked task details
- Otherwise it falls back to background runtime task details

Use this tool to:
- Inspect the full details of a tracked execution task
- Review description, owner, dependencies, and timestamps
- Check command/status/exit details for a background task when relevant

Parameters:
- task_id: The ID of the task to inspect
- taskType (optional): 'todo' or 'background' to force one mode
`

	ToolDescriptionTaskOutput = `
DEPRECATED: Prefer using Read tool on the task's output file path instead.

Retrieves output from a running or completed background task.

Use this tool to:
- Read stdout/stderr from a shell task
- Check if a task has completed
- Wait for task completion (blocking mode)

Parameters:
- task_id: The ID of the task to get output from
- block: Whether to wait for completion (default: true)
- timeout: Max wait time in ms (default: 30000)

Returns task output along with status information.
`

	ToolDescriptionTaskCreate = `
Create a new task in the task list to track execution steps.

Task tools are the execution-tracking layer that runs alongside implementation.
They are separate from the Plan system: Plan is for user validation before you
start; tasks are the checklist you follow once you have the green light.

## When to use task_create

**After plan approval** — once the user approves your plan, convert each
implementation step into a task before touching any file. Work through the
list in order: mark in_progress before starting each step, completed when done.

**For multi-step work without a formal plan** — if the task has 3+ distinct
steps but does not require user approval first, create tasks upfront so you
don't lose track mid-way.

**Never skip task creation** on complex work. If you forget a step, you risk
leaving the codebase in a broken intermediate state.

## When NOT to use task_create

- Single-step operations that fit in one or two tool calls
- Pure read/research tasks with no implementation
- When the user gave you a literal list of commands to run

## Task Fields

- **subject**: Brief, actionable title in imperative form ("Add JWT validation")
- **description**: What needs to be done and why
- **activeForm** (optional): Present continuous shown in spinner ("Adding JWT validation")

All tasks start with status 'pending'. Use task_update to transition them.
`

	ToolDescriptionTaskUpdate = `
Update a task's status, details, or dependencies.

## The strict execution discipline

Follow this sequence for every task in your list:
1. Mark the task **in_progress** immediately before you start working on it
2. Do the work
3. Mark it **completed** only when the work is fully done and verified
4. Move to the next task

Never mark a task completed while another implementation step is pending.
Never skip ahead — finish what you started before moving on.

## Status transitions

- **pending → in_progress**: You are about to start this task right now
- **in_progress → completed**: The work is done, tests pass (if applicable)
- **in_progress → pending**: You are blocked and switching to another task first
- **any → deleted**: Task is no longer needed (scope change, duplicate, etc.)

Only mark **completed** when FULLY accomplished. Partial work stays in_progress.

## Fields

- **status**: 'pending', 'in_progress', 'completed', 'deleted'
- **subject**: Update the task title
- **description**: Clarify or update what needs doing
- **owner**: Assign to a specific agent or actor
- **addBlocks**: Mark tasks this one must complete before they can start
- **addBlockedBy**: Mark tasks that must complete before this one can start
`

	// TaskStatus constants
	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in_progress"
	TaskStatusCompleted  = "completed"
	TaskStatusDeleted    = "deleted"
)

func taskSurfaceMetadata() map[string]any {
	return map[string]any{
		"surface_profiles": []string{
			tool.ToolSurfaceProfileMonoRun,
			tool.ToolSurfaceProfileSubagent,
		},
	}
}
