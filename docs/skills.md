# Writing Skills

Skills are reusable Markdown files that inject structured instructions into an agent session. They are Seshat's primary extension mechanism for teams: a skill defines a task template, its tool access, lifecycle hooks, and optional shell setup — all in one file.

---

## Quick start

Create a file anywhere under `.claude/skills/` in your project (or `~/.seshat/skills/user/` for personal skills):

```markdown
---
name: "summarise-pr"
description: "Summarise a GitHub pull request for the team."
when_to_use: "When asked to summarise a PR or code review."
allowed-tools:
  - bash
  - read
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
  - read
  - write
  - grep
  - glob
```

Available built-in tool names: `bash`, `read`, `write`, `grep`, `glob`, `web_fetch`, `web_search`, `mcp`, `think`, `agent`, `task`.

### `model`

`string` — Override the session model for this skill.

```yaml
model: "claude-3-5-haiku-20241022"
```

Use a smaller/faster model for skills that don't need full reasoning power.

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

`bool` — Run the skill body as a pure shell/hook script without calling the LLM. Useful for setup-only skills.

```yaml
disable-model-invocation: true
```

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

### `shell`

Shell commands executed at lifecycle points **before** the agent is invoked. Useful for setup/teardown.

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

### `hooks`

Attach other skills at lifecycle events of this skill.

```yaml
hooks:
  before_tool:
    - "validate-workspace"
  after_tool:
    - "audit-log"
  on_error:
    - "notify-slack"
  on_complete:
    - "cleanup-temp"
```

Available hook events: `before`, `after`, `before_tool`, `after_tool`, `on_error`, `on_cancel`, `on_complete`, `tool_allowed`, `tool_denied`.

---

## Skill locations

Skills are loaded from multiple directories with clear precedence:

| Directory | Source | Who manages it |
|---|---|---|
| `{project}/.claude/skills/` | Project | Team (committed to repo) |
| `~/.seshat/skills/user/` | Personal | Individual developer |
| `~/.seshat/skills/managed/` | Admin | Platform operators |
| `~/.seshat/bundled-skills/` | Bundled | Seshat built-ins |

Project skills override personal skills with the same name.

---

## Complete examples

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
model: "claude-3-5-haiku-20241022"
allowed-tools:
  - bash
  - read
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
  - read
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

- **Keep skills focused.** One skill per task. Compose with `hooks:` rather than writing monolithic skills.
- **Use `context: fork`** for anything destructive or long-running to avoid polluting the main session history.
- **Restrict tools** with `allowed-tools:` — the principle of least privilege applies to agents too.
- **Use `shell: before`** to install tools or set up environment instead of asking the agent to do it (more reliable, faster).
- **Test with `disable-model-invocation: true`** to verify shell setup before adding the LLM body.
- **Commit project skills** (`.claude/skills/`) to your repository so the whole team benefits.
