package agent

// Prompt for the agent tool
const AgentPrompt = `Use this tool to delegate a bounded subtask to a sub-agent.

Sub-agents are best for isolation, parallelism, or focused execution contexts. They do not share your live reasoning state, so every delegated task must be self-contained.

## When to delegate

Use this tool when:
- the subtask is independent and can run in parallel,
- you need read-only exploration before editing,
- you need current or external information verified before making an important claim,
- you want verification in a fresh context,
- the work is large enough that isolating it will reduce parent-turn complexity.

Do NOT delegate when:
- the next blocking step is simple and easier to do directly,
- the prompt would be vague or underspecified,
- the task is tiny enough to complete in 1-2 direct tool calls.

## Agent types

- **general-purpose**: default for complex execution, coding, fixes, and multi-step work
- **explore**: read-only architecture and codebase investigation
- **browse**: read-only external research using web, browser, docs, and targeted local context
- **plan**: create a detailed implementation plan
- **verify**: validate, test, and review results in a fresh context

## Choosing the right agent type

- Use ` + "`explore`" + ` when the evidence should come mainly from the local repository.
- Use ` + "`browse`" + ` when the evidence should come mainly from current docs, provider behavior, release notes, policies, external references, or website interaction.
- Use ` + "`verify`" + ` after implementation or when you need a fresh validation pass.
- Use ` + "`plan`" + ` only when you want a structured implementation plan as the deliverable.
- Use ` + "`general-purpose`" + ` when the subtask combines several of the above and needs broader autonomy.

## How to write a good delegated task

Your ` + "`task`" + ` must include:
- the exact goal,
- relevant file paths, directories, symbols, or error messages,
- constraints like "read-only" or "do not modify files",
- and what the agent must return when done.

Good:
- "Explore ` + "`internal/auth`" + `, ` + "`internal/providers`" + `, and ` + "`cmd/cli`" + `. Report how browser/device auth works, where credentials are persisted, and any tests covering it. Do not modify files."
- "Browse the official provider docs and release notes for model streaming changes. Summarize current behavior, note breaking changes, and include sources. Do not modify files."

Bad:
- "Look into auth"

## Parallel work

For independent subtasks, call this tool multiple times in the same turn.
Use ` + "`run_in_background: true`" + ` when the parent can continue without waiting immediately.

## Parent/sub-agent workflow

- Parent should keep the visible session checklist in ` + "`todo_write`" + `.
- Sub-agents handle focused subtasks and report results back.
- Read all sub-agent outputs before deciding follow-up actions.
- If a sub-agent is gathering evidence, do not restate its conclusions as fact until you have read and integrated the result.

## Parameters

- ` + "`type`" + `: agent type
- ` + "`task`" + `: self-contained task prompt
- ` + "`tools`" + `: optional explicit allow-list
- ` + "`maxTurns`" + `: max turns for the sub-agent
- ` + "`run_in_background`" + `: launch asynchronously
- ` + "`fork`" + `: inherit selected parent transcript/messages

## Examples

// Read-only exploration
agent({ type: "explore", task: "Inspect request handling in cmd/grpc and pkg/grpc. Report entrypoints, service handlers, and any relevant tests. Do not modify files." })

// Parallel background investigations
agent({ type: "explore", task: "Inspect the frontend state flow for session persistence. Report findings only.", run_in_background: true })
agent({ type: "browse", task: "Research the latest official documentation and release notes for the provider API change. Summarize the important breaking changes with sources.", run_in_background: true })
agent({ type: "verify", task: "After the fix lands, run relevant tests and summarize failures or passes.", run_in_background: true })

// Focused implementation
agent({ type: "general-purpose", task: "Implement the sidebar session rename menu in the renderer and report the changed files plus validation steps." })`

// Description returns the tool description
func Description() string {
	return DescriptionAgent
}
