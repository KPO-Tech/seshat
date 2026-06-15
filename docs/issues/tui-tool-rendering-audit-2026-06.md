# TUI Tool Rendering Audit — June 2026

Canonical TUI rendering roadmap. Only shows what is not yet done.

## Legend

- `Done` · `Planned` · `Open` · `Impl` (renderer written, validation test pending)
- Chat policy: `hidden` · `meta` · `compact` · `full` · `generic`

---

## Category A — Hidden From Chat

All hidden. No open items.

Done: `enter_plan_mode`, `exit_plan_mode`, `submit_plan`, `task_create`, `task_update`, `enter_worktree`, `exit_worktree`, `request_permissions`, `config`.

---

## Category B — Metadata Only

Show a summary; hide the raw payload.

| Tool(s) | State | Notes |
|---|---|---|
| `nexus_read_skill` | Planned | Skill name + summary; not full body. |

Done: `read_file`, `get_file_metadata`, `read_document_url`, `list_agents`, `memory_search_nodes`, `memory_open_nodes`, `get_goal`, all `browser_*` list/capture tools (`browser_snapshot`, `browser_extract`, `browser_network_list`, `browser_list_downloads`, `browser_list_pages`, `browser_search_content`).

---

## Category C — Compact Chat Items

| Tool(s) | State | Notes |
|---|---|---|
| `nexus_list_skills`, `nexus_validate_skill` | Planned | Count/result rendering. |
| `tool_search` | Impl | Renderer implemented; validation pending (test 11). |
| `mcp_list_resources`, `mcp_read_resource`, `mcp_*` | Impl | Renderers implemented; validation pending (test 11). |
| `rag_search`, `rag_ingest` | Planned | Counts and identifiers. See **Special Tools**. |

Done: `list_directory`, `glob`, `grep`, `web_search`, `web_fetch`, `send_agent_message`, `wait_agent`, `close_agent`, `memory_create_entities`, `memory_add_observations`, `create_goal`, `update_goal`, all `browser_*` action/navigation tools.

---

## Category D — Rich Chat Rendering

| Tool(s) | State | Notes |
|---|---|---|
| `write_stdin` | Planned | Compact shell continuation under the shell family. |
| `resume_agent` | Planned | Multi-agent; renderer exists but no test yet. |
| `brief` | Planned | **Priority 1 — See Special Tools.** Primary agent→user output. |
| `generate_image` | Planned | See **Special Tools**. |
| `tts`, `stt` | Planned | See **Special Tools**. |

Done: `bash`, `job_output`, `job_kill`, `write_file`, `edit_file`, `apply_patch`, `notebook_edit`, `notebook_create`, `notebook_write`, `create_directory`, `remove_file`, `ask_user_question`, `agent`, `spawn_agent`, `browser_screenshot`, `docx`, `monitor`, `code_complete`, `lsp`.

---

## Category E — Generic For Now

| Tool(s) | State | Notes |
|---|---|---|
| `skill` | Planned | Depends on how much of skill execution should be surfaced vs hidden. |

Done: `browser_get_network_policy`, `browser_set_network_policy`.

---

## Special Tools — Renderer Priority

All tools below currently fall through to `GenericToolMessageItem`. Listed in implementation order.

### Group 1 — Critical

| Tool | Tool Name | File | What to render |
|---|---|---|---|
| brief | `brief` | `brief.go` | Header: status badge (normal/proactive) + first line of message. Body: full markdown via `toolOutputMarkdownContent`. Attachments: name + size; icon for images. |

### Group 2 — High value

| Tool | Tool Name | File | What to render |
|---|---|---|---|
| imagegen | `generate_image` | `imagegen.go` | Header: prompt (truncated). Body: `toolOutputImageContent` if `image_base64`, else URL. Show `revised_prompt` if different. |
| stt | `speech_to_text` | `audio.go` | Header: "Speech to Text". Body: transcript (expandable) + `language · duration · model`. |
| tts | `text_to_speech` | `audio.go` | Header: text preview. Body: `content_type · model · N characters`. |

### Group 3 — Structured output

| Tool | Tool Name | File | What to render |
|---|---|---|---|
| rag | `rag_search` | `rag.go` | Header: query + corpus_id. Body: scored result list (`score · chunk preview`). |
| rag | `rag_ingest` | `rag.go` | Header: filename + corpus_id. Body: `N chunks indexed`. Header-only on success. |

Done: `lsp` (`lsp_tools.go`) — header: operation + file:line + summary; body: plain content.

### Group 4 — Simple

Done: `monitor` (`monitor.go`), `code_complete` (`fim.go`), `docx` (`docx.go`).

---

## Pending Validation Tests

| Test | Tools | State |
|---|---|---|
| Test 11 — MCP & Tool Search | `tool_search`, `mcp_list_resources`, `mcp_read_resource`, `mcp_*` | Renderers implemented; test not run yet. |
| Test 13 — Global Audit | Full renderer regression | Not run yet. |
| Test 15 — Goal Tools | `create_goal`, `get_goal`, `update_goal` | Renderers implemented; test not run yet. |
| Test 16 — Docx, Monitor, Code Complete, LSP | `docx`, `monitor`, `code_complete`, `lsp` | Renderers implemented; test not run yet. |

---

## Runtime Inventory

### Shell and background execution
`bash` · `write_stdin` · `job_output` · `job_kill` · `monitor`

### File read, search, and filesystem
`read_file` · `read_document_url` · `glob` · `grep` · `get_file_metadata` · `list_directory` · `create_directory` · `remove_file`

### File mutation and generation
`write_file` · `edit_file` · `apply_patch` · `notebook_edit` · `notebook_create` · `notebook_write` · `docx`

### Web and browser
`web_fetch` · `web_search` · `browser_open` · `browser_navigate` · `browser_snapshot` · `browser_extract` · `browser_list_pages` · `browser_network_list` · `browser_list_downloads` · `browser_search_content` · `browser_get_network_policy` · `browser_set_network_policy` · `browser_select_page` · `browser_close_page` · `browser_click` · `browser_type` · `browser_press` · `browser_scroll` · `browser_wait` · `browser_screenshot`

### Planning, tasks, permissions, and user interaction
`task_stop` · `task_list` · `task_get` · `task_create` · `task_update` · `enter_plan_mode` · `exit_plan_mode` · `submit_plan` · `request_permissions` · `ask_user_question`

### Agent, workspace, and orchestration
`agent` · `spawn_agent` · `resume_agent` · `wait_agent` · `list_agents` · `send_agent_message` · `close_agent` · `enter_worktree` · `exit_worktree`

### System, MCP, skills, and retrieval
`tool_search` · `mcp` · `skill` · `nexus_list_skills` · `nexus_read_skill` · `nexus_validate_skill` · `rag_search` · `rag_ingest`

### LSP, memory, goals, media, and config
`lsp` · `memory_create_entities` · `memory_add_observations` · `memory_search_nodes` · `memory_open_nodes` · `create_goal` · `get_goal` · `update_goal` · `generate_image` · `tts` · `stt` · `code_complete` · `brief` · `config`

---

## Not On The Active Runtime Surface

| Legacy name | Status | Notes |
|---|---|---|
| `todos` | Removed | Renderer constants removed; `FormatTodosList` retained in `todos.go` as shared utility. |
| `multi_edit` | Legacy | TUI renderer exists; canonical runtime uses `apply_patch`, `edit_file`, `write_file`. |
| `download` | Legacy | Not in current builtin registry. |
| `fetch` | Legacy | Superseded by `web_fetch` and `read_document_url`. |
| `agentic_fetch` | Legacy | Not in current builtin registry. |
| `sourcegraph` | Legacy | Not in current builtin registry. |
| `task_output` | Deprecated | Defined in `internal/tools/task/constants.go` but not active in the runtime surface. |
