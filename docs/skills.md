# Writing Skills

Skills are reusable Markdown files that inject structured instructions into an agent session. They are Seshat's primary extension mechanism for teams: a skill defines a task template, its tool access, lifecycle hooks, and optional shell setup — all in one file.

---

## Quick start

Create a file anywhere under `.claude/skills/` in your project (or `~/.config/seshat/skills/user/` for personal skills - `~/.config/seshat` moves if `SESHAT_RUNTIME_ROOT` is set):

```markdown
---
name: "summarise-pr"
description: "Summarise a GitHub pull request for the team."
when_to_use: "When asked to summarise a PR or code review."
allowed-tools:
  - bash
  - read_file
---

Read the diff output from the bash tool and write a concise Markdown summary:
- What changed and why
- Potential risks
- Reviewer checklist
```

The agent picks up the skill automatically at next session start. No restart required.

---

## File format

A skill file is **Markdown with a YAML frontmatter block**.

```
---
<frontmatter — YAML>
---

<body — Markdown prompt injected into the agent's system context>
```

The frontmatter is optional; a plain Markdown file is treated as a skill with only its body content.

---

## Frontmatter reference

### `name`

`string | list` — One or more names for the skill. The agent uses these names to match user invocations.

```yaml
name: "go-lint"
# or multiple aliases:
name:
  - "go-lint"
  - "lint-go"
```

### `description`

`string | list` — Human-readable description shown in skill listings.

### `when_to_use`

`string` — Guidance injected into the agent's system prompt about when to invoke the skill automatically.

```yaml
when_to_use: "Use whenever the user mentions linting or code quality for Go files."
```

### `arguments`

Formal argument declaration. Agents can pass arguments when invoking the skill.

```yaml
arguments:
  - name: "file"
    description: "Path to the file to process"
  - name: "format"
    description: "Output format: json or text"
    default: "text"
```

`default` is parsed but not yet applied anywhere - an argument with no value passed at invocation is simply absent, not filled in. Declare it for self-documentation, but don't rely on it.

### `argument-hint`

`string` — Short hint displayed to the user when invoking the skill.

```yaml
argument-hint: "<file> [format]"
```

### `allowed-tools`

`list` — Which tools the skill may use. Restricts tool access to only what is needed. Omit to allow all tools.

```yaml
allowed-tools:
  - bash
  - read_file
  - write_file
  - grep
  - glob
```

Names are matched exactly (or by glob pattern) against the tool's real registered name - there is no alias table, so `read`/`write` match nothing. Common built-in names: `bash`, `read_file`, `write_file`, `edit_file`, `grep`, `glob`, `web_fetch`, `web_search`, `agent`, `task_create`, `task_list`, `task_get`, `task_update`, `task_stop`. MCP tools are namespaced as `mcp__<server>__<tool>`; there is no single tool named `mcp` or `think`.

### `model`

`string` — Intended to override the session model for this skill (e.g. a smaller/faster model for skills that don't need full reasoning power).

```yaml
model: "claude-haiku-5-5"
```

Not yet applied: for `context: fork` the value reaches the forked agent's definition, but nothing downstream uses it to actually select a different provider/model; for the default `inline` context it isn't read at all. Declare it for self-documentation, but the skill still runs on the session's own model today.

### `context`

`"inline" | "fork"` — Execution context.

| Value | Behaviour |
|---|---|
| `inline` (default) | Skill runs in the current session. Shares message history and tool state. |
| `fork` | Skill runs in an isolated sub-session. Result returned to parent. Use for destructive or long-running tasks. |

```yaml
context: fork
```

### `user-invocable`

`bool` — Whether users can explicitly invoke this skill by name. Default `true`.

```yaml
user-invocable: false   # background/policy skill, not user-callable
```

### `disable-model-invocation`

`bool` — Intended to run the skill body as a pure shell/hook script without calling the LLM. Useful for setup-only skills.

```yaml
disable-model-invocation: true
```

Not yet implemented: this depends on `shell`/`hooks` execution (below), which is parsed but does not run yet. Setting this today does not change how the skill executes.

### `version`

`string` — Skill version for change tracking.

```yaml
version: "1.2.0"
```

### `paths`

`list` — Additional filesystem paths the skill is allowed to access (complements `allowed-tools`).

```yaml
paths:
  - "src/"
  - "tests/"
  - "/var/log/app.log"
```

Only takes effect through the public `pkg/skills.GetSkillsForContext` API, which a host application can call to filter skills by the files a turn touches - the normal `skill` tool invocation path does not consult `paths` or call that function automatically.

### `shell`

Shell commands executed at lifecycle points around the skill. Useful for setup/teardown.

```yaml
shell:
  before:
    - "npm install --silent"
    - "go generate ./..."
  after:
    - "rm -rf /tmp/skill-cache"
  on_error:
    - "git checkout ."
  on_complete:
    - "echo 'Skill completed'"
```

**Only `before` runs**, and not as a separate execution path: the skill's prompt gets a setup section telling the agent to run each `before` command itself, through its own Bash tool call, asking for permission first unless already granted - the same way `requires.install-cmd` already works. This goes through the normal permission system instead of bypassing it, which is why no raw shell-exec code was added here.

`after`, `on_error` and `on_complete` are parsed but **not yet implemented** - they would need to fire once the skill's work is actually done or has failed, and there is no such signal today: the skill tool returns this prompt once and the conversation just continues, with nothing tracking when "the skill" itself finishes. Implementing them needs that lifecycle tracking built first, not just more prompt text.

### `hooks`

**Not implemented, and not planned as a separate system.** `hooks:` would attach other skills to lifecycle events of this skill, but it overlaps the existing, separate hook system in `internal/hooks` (see [`docs/`](./) for that system's own docs) that the engine already runs real `pre_tool_use`/`post_tool_use`/`session_start`/etc. events through. Adding a second, skill-scoped hook mechanism alongside it would mean two ways to do the same thing with no clear rule for which one fires when. If you need to react to tool use or session lifecycle events, use `internal/hooks` directly rather than this field - it is parsed into the skill definition for forward compatibility but has no effect.

---

## Skill locations

Skills are loaded from multiple directories with clear precedence:

| Directory | Source | Who manages it |
|---|---|---|
| `{project}/.claude/skills/` | Project | Team (committed to repo) |
| `~/.config/seshat/skills/user/` | Personal | Individual developer |
| `~/.config/seshat/skills/managed/` | Admin | Platform operators |
| `~/.config/seshat/skills/builtin/` | Built-in | Seshat itself |

The root `~/.config/seshat` moves if `SESHAT_RUNTIME_ROOT` is set. A `.claude/skills/` directory below the project root (in a subdirectory, up to 6 levels down) is also discovered automatically, not just at the project root itself.

Project skills override personal skills with the same name.

---

## Complete examples

Example 2 below uses `shell.before`, which runs as described (via the agent's own Bash tool call). Example 4 uses `hooks`, which has no effect (see that field's reference entry above) - everything else in each example works as shown.

### 1. Code formatter

```markdown
---
name: "format-go"
description: "Format all Go files and report what changed."
when_to_use: "When asked to format, clean up, or standardise Go code."
allowed-tools:
  - bash
  - glob
---

Run `gofmt -w` on all `.go` files in the workspace, then list the files that changed.
Report a summary: how many files were formatted, any errors encountered.
```

### 2. Security audit

```markdown
---
name: "security-audit"
description: "Run a static security scan on the codebase."
when_to_use: "Use before any release or when security issues are suspected."
context: fork
model: "claude-haiku-5-5"
allowed-tools:
  - bash
  - read_file
  - grep
shell:
  before:
    - "which gosec || go install github.com/securego/gosec/v2/cmd/gosec@latest"
---

Run `gosec ./...` and analyse the output. For each finding:
1. Explain the vulnerability in plain language
2. Assess actual risk (not just severity label)
3. Suggest a concrete fix with code example

Produce a Markdown report sorted by risk priority.
```

### 3. Git commit helper

```markdown
---
name: "smart-commit"
description: "Generate a conventional commit message from staged changes."
when_to_use: "After `git add`, before `git commit`."
argument-hint: "[scope]"
arguments:
  - name: "scope"
    description: "Optional commit scope (e.g., api, ui, db)"
allowed-tools:
  - bash
---

Run `git diff --cached` to see staged changes.
Write a conventional commit message following this format:
  <type>(<scope>): <short description>
  
  <body: what changed and why, wrapped at 72 chars>

Types: feat, fix, refactor, docs, test, chore.
Print only the commit message, ready to paste into `git commit -m`.
```

### 4. Database migration reviewer

```markdown
---
name: "review-migration"
description: "Review a database migration for safety and correctness."
when_to_use: "Before merging any pull request that contains database migrations."
context: fork
allowed-tools:
  - read_file
  - glob
  - bash
paths:
  - "db/migrations/"
  - "internal/db/"
hooks:
  on_complete:
    - "audit-log"
---

Find all migration files modified in the current branch (`git diff --name-only origin/main`).
For each migration file, review:

1. **Safety**: Will this migration cause downtime on a live database? Check for:
   - Table locks (ALTER TABLE on large tables)
   - Non-nullable columns added without defaults
   - Index creation without CONCURRENT
   - Data loss risk (DROP, TRUNCATE)

2. **Rollback**: Is there a rollback path if the migration fails mid-run?

3. **Idempotency**: Can the migration run twice safely?

Report findings as a Markdown checklist. Mark each item ✓ (safe) or ✗ (risk) with explanation.
```

---

## Tips

- **Keep skills focused.** One skill per task. `hooks:` is not implemented and not planned as a separate system (see above) - compose skills some other way.
- **Use `context: fork`** for anything destructive or long-running to avoid polluting the main session history.
- **Restrict tools** with `allowed-tools:`, using real tool names (`read_file`, not `read`) — the principle of least privilege applies to agents too.
- **Use `shell.before`** for setup the agent should run before starting the skill's task; it still asks for permission like any other Bash call. `shell.after`/`on_error`/`on_complete` and `disable-model-invocation:` are not executed yet.
- **Commit project skills** (`.claude/skills/`) to your repository so the whole team benefits.
