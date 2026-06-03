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
	SearchHintTaskStop   = "kill a running background task"
	SearchHintTaskList   = "list all background tasks"
	SearchHintTaskGet    = "get details of a background task by ID"
	SearchHintTaskOutput = "read output from a background task"
	SearchHintTaskCreate = "create a task in the task list"
	SearchHintTaskUpdate = "update a task in the task list"
)

// Tool descriptions
const (
	ToolDescriptionTaskStop = `
Stops a running background task by its ID.

Use this tool when:
- You need to terminate a long-running background task
- A monitored command is stuck or taking too long
- You want to stop a task started with Monitor tool

The tool requires:
- task_id: The ID of the task to stop (returned from Monitor or Bash background)

Returns success/failure status with details about the stopped task.
`

	ToolDescriptionTaskList = `
List all running background tasks.

Use this tool to:
- See what background tasks are currently running
- Check the status of Monitor or background Bash tasks
- Identify tasks that can be stopped with TaskStop

Returns a list of tasks with their ID, command, status, and start time.
`

	ToolDescriptionTaskGet = `
Get detailed information about a specific background task by its ID.

Use this tool to:
- Get full details of a running or completed background task
- Check command, status, start/end times, and exit code
- Monitor progress of a specific task

Returns task details including command, status, timestamps, and exit code.
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
