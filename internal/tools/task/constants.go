package task

import tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"

// Tool name constants
const (
	ToolNameTaskStop   = "task_stop"
	ToolNameTaskList   = "task_list"
	ToolNameTaskGet    = "task_get"
	ToolNameTaskCreate = "task_create"
	ToolNameTaskUpdate = "task_update"
)

// Search hints
const (
	SearchHintTaskStop   = "stop tracking a task or kill a background task"
	SearchHintTaskList   = "list tracked tasks or background tasks"
	SearchHintTaskGet    = "get details of a tracked task or background task by ID"
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
- wait_seconds (optional, background only): wait up to this many seconds (at most 600) for the
  task or agent to finish. This is how to wait for a background agent: do not sleep in a shell.
  Several waits can run in the same response.

For a finished background task the result also carries its output (for an agent, its final
answer) and, for an agent, the description it was given. If the wait ran out before the task
finished, the result says timedOut and the status is still running.
`

	ToolDescriptionTaskCreate = `
Create a new task in the task list to track execution steps.

Task tools are the execution-tracking layer that runs alongside implementation.
They are separate from the Plan system: Plan is for user validation before you
start; tasks are the checklist you follow while you work. Plan mode is NOT a
prerequisite. Most work gets a task list directly, in execute mode.

## When to use task_create

**Multi-step work whose approach is already clear** — if the task has 3+
distinct steps and you do not need the user to validate the approach first,
create the tasks upfront and start executing. Do not enter plan mode merely to
be allowed to track the work: a task list is not a plan, it costs a fraction of
one, and the user sees it as live progress.

**Never skip task creation** on multi-step work. If you forget a step, you risk
leaving the codebase in a broken intermediate state.

## When NOT to use task_create

- Single-step operations that fit in one or two tool calls
- Pure read/research tasks with no implementation
- When the user gave you a literal list of commands to run

## Several tasks at once

Pass the "tasks" field (a list of subject/description items) to create the whole list in one call, in order, instead of one call per task.

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

The user watches this list as your live progress, and it only moves when you
call task_update. Update it after each step, before starting the next one —
not once at the end when everything is already done.

Never mark a task completed while another implementation step is pending.
Never skip ahead — finish what you started before moving on.

## Status transitions

- **pending → in_progress**: You are about to start this task right now
- **in_progress → completed**: The work is done, tests pass (if applicable)
- **in_progress → pending**: You are blocked and switching to another task first
- **any → deleted**: Task is no longer needed (scope change, duplicate, etc.)

Only mark **completed** when FULLY accomplished. Partial work stays in_progress.

## Several tasks at once

Pass the "updates" field (a list of items, each with taskId and the fields to change) to update several tasks in one call, in order. The usual use: mark the finished task completed and the next one in_progress together.

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
