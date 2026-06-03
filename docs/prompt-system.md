# Prompt System

The prompt system turns configuration, memory, tools, and stage context into a structured system prompt that is sent to the provider on every model call.

## Two-phase assembly

```
Phase 1 — FetchSystemPromptParts()     called when config changes
Phase 2 — BuildCanonicalPrompt()       called each turn
```

Phase 1 is cached: if tools and config haven't changed, the stable sections are not re-rendered.

---

## Section types

Every section has:

| Field | Purpose |
|---|---|
| `Name` | Unique identifier |
| `Content` | Rendered text (may contain `{{variables}}`) |
| `Priority` | Render order — higher = earlier in the prompt |
| `Type` | `SectionTypeDefault`, `SectionTypeUser`, `SectionTypeDynamic`, `SectionTypeAppend` |
| `Cacheable` | `true` for stable content that sits before the cache boundary |
| `Enabled` | `false` sections are skipped |

---

## Section priority table

```
Priority  Section name           Type       Cacheable  Content
────────────────────────────────────────────────────────────────────────
900       identity               default    ✓          Role definition
850       runtime_contract       default    ✓          Recovery/resume rules
820       working_rules          default    ✓          Coding discipline
800       tool_use               default    ✓          Tool invocation rules
790       output_discipline      default    ✓          Response formatting
780       tool_catalog           default    ✓          Generated tool list

── CACHE BOUNDARY ────────────────────────────────────────────────────
(everything below is re-rendered every turn)

780       runtime_guidance       dynamic    ✗          CWD, session/turn IDs
775       project_instructions   dynamic    ✗          NEXUS.md / AGENTS.md content
770       stage_overlay          dynamic    ✗          Stage-specific text (if stage set)
750       runtime_context        dynamic    ✗          Date, model, deferred tools
730       runtime_memory         dynamic    ✗          Long-term memory entries
```

---

## Variables

Dynamic sections reference variables resolved at build time:

| Variable | Value |
|---|---|
| `{{cwd}}` | Current working directory |
| `{{session_id}}` | Session identifier |
| `{{turn_id}}` | Turn identifier |
| `{{model}}` | Active model name |
| `{{date}}` | ISO 8601 date |
| `{{memory_context}}` | Formatted long-term memory block |
| `{{project_instructions_block}}` | Trimmed NEXUS.md / AGENTS.md content |

---

## Project instructions

When the engine starts a session it looks for an instruction file in the working directory:

```
1. NEXUS.md
2. AGENTS.md
3. .nexus/instructions.md
```

The first non-empty file wins. Content is truncated at 32 KB at the last newline. The content is injected into `{{project_instructions_block}}` at priority 775 (between runtime_guidance and stage_overlay).

---

## Stage overlays

When `Config.PromptStage` is set, a non-cacheable section is added at priority 770:

```go
const (
    StageDefault      ExecutionStage = ""           // no overlay injected
    StageToolCall     ExecutionStage = "tool_call"
    StageToolResult   ExecutionStage = "tool_result"
    StageContinuation ExecutionStage = "continuation"
    StagePlan         ExecutionStage = "plan"
)
```

Custom text for each stage can be provided via `PromptConfig.StageOverrides`.  
An empty override falls back to the built-in template.

---

## Tool hints

`BuildProviderToolDefinitionsWithHints(tools, hints)` appends per-tool guidance to the tool's description before it is sent to the provider:

```go
hints := map[string]string{
    "bash": "Only use for read-only operations in this session.",
}
defs := BuildProviderToolDefinitionsWithHints(toolMap, hints)
// defs[bash].Description == "Execute shell commands.\n\n---\nOnly use for read-only..."
```

---

## SDK surface

```go
type PromptConfig struct {
    SystemPrompt       *string                        // full override (replaces default)
    AppendSystemPrompt *string                        // appended after all sections
    Stage              ExecutionStage                 // active stage
    StageOverrides     map[ExecutionStage]string      // per-stage custom text
    ToolHints          map[string]string              // per-tool extra guidance
}
```

Set on `ClientConfig.PromptConfig` when building the SDK client.

---

## Prompt caching

The stable sections (priorities 900–780) are rendered once and submitted to the provider with a `cache_control: ephemeral` breakpoint. On subsequent turns, the provider can skip re-processing them, reducing latency and cost.

The cache is keyed on the set of enabled tools and the section content hashes. If either changes, the stable prefix is re-sent.
