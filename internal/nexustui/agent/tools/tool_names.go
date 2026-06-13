package tools

import "github.com/EngineerProjects/nexus-engine/internal/nexustui/session"

// Tool name constants matching SDK builtin names where applicable.
// These are used by the permission dialog and chat UI to render tool-specific UI.
const (
	BashToolName         = "bash"
	EditToolName         = "edit_file"
	WriteToolName        = "write_file"
	MultiEditToolName    = "multi_edit"
	ViewToolName         = "read_file"
	DownloadToolName     = "download"
	FetchToolName        = "fetch"
	AgenticFetchToolName = "agentic_fetch"
	LSToolName           = "list_directory"
	JobOutputToolName    = "job_output"
	JobKillToolName      = "job_kill"
	GlobToolName         = "glob"
	GrepToolName         = "grep"
	SourcegraphToolName  = "sourcegraph"
	WebFetchToolName     = "web_fetch"
	WebSearchToolName    = "web_search"
	TodosToolName        = "todos"
	AgentToolName        = "agent"

	BashNoOutput = "<no output>"
)

// AgentParams holds the input for an agent tool call.
type AgentParams struct {
	Prompt string `json:"prompt" description:"The task for the agent to perform"`
}

// --- Permission param structs (used by the permission dialog for rendering) ---

// BashPermissionsParams holds the params for a bash tool permission request.
type BashPermissionsParams struct {
	Command     string
	Description string
}

// EditPermissionsParams holds the params for an edit_file permission request.
type EditPermissionsParams struct {
	FilePath   string
	OldContent string
	NewContent string
}

// WritePermissionsParams holds the params for a write_file permission request.
type WritePermissionsParams struct {
	FilePath   string
	OldContent string
	NewContent string
}

// MultiEditPermissionsParams holds the params for a multi_edit permission request.
type MultiEditPermissionsParams struct {
	FilePath   string
	OldContent string
	NewContent string
}

// DownloadPermissionsParams holds the params for a download permission request.
type DownloadPermissionsParams struct {
	URL      string
	FilePath string
	Timeout  int
}

// FetchPermissionsParams holds the params for a web_fetch permission request.
type FetchPermissionsParams struct {
	URL string
}

// AgenticFetchPermissionsParams holds the params for an agentic_fetch permission request.
type AgenticFetchPermissionsParams struct {
	URL    string
	Prompt string
}

// ViewPermissionsParams holds the params for a read_file permission request.
type ViewPermissionsParams struct {
	FilePath string
	Offset   int
	Limit    int
}

// LSPermissionsParams holds the params for a list_directory permission request.
type LSPermissionsParams struct {
	Path   string
	Ignore []string
}

// --- Tool input param structs (used by chat UI to deserialise tool call JSON) ---

// BashParams holds the input for a bash tool call.
type BashParams struct {
	Command         string `json:"command"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
}

// BashResponseMetadata holds the metadata returned by a bash tool call.
type BashResponseMetadata struct {
	Background  bool   `json:"background,omitempty"`
	Description string `json:"description,omitempty"`
	ShellID     string `json:"shell_id,omitempty"`
	Output      string `json:"output,omitempty"`
}

// JobOutputParams holds the input for a job_output tool call.
type JobOutputParams struct {
	ShellID string `json:"shell_id"`
}

// JobOutputResponseMetadata holds the metadata returned by a job_output tool call.
type JobOutputResponseMetadata struct {
	Description string `json:"description,omitempty"`
	Command     string `json:"command,omitempty"`
}

// JobKillParams holds the input for a job_kill tool call.
type JobKillParams struct {
	ShellID string `json:"shell_id"`
}

// JobKillResponseMetadata holds the metadata returned by a job_kill tool call.
type JobKillResponseMetadata struct {
	Description string `json:"description,omitempty"`
	Command     string `json:"command,omitempty"`
}

// FetchParams holds the input for a fetch tool call.
type FetchParams struct {
	URL     string `json:"url"`
	Format  string `json:"format,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// WebFetchParams holds the input for a web_fetch tool call.
type WebFetchParams struct {
	URL string `json:"url"`
}

// WebSearchParams holds the input for a web_search tool call.
type WebSearchParams struct {
	Query string `json:"query"`
}

// AgenticFetchParams holds the tool input for an agentic_fetch call.
type AgenticFetchParams struct {
	URL    string `json:"url,omitempty"`
	Prompt string `json:"prompt"`
}

// MultiEditResponseMetadata holds the metadata returned by a multi_edit tool call.
type MultiEditResponseMetadata struct {
	OldContent   string `json:"old_content,omitempty"`
	NewContent   string `json:"new_content,omitempty"`
	EditsFailed  []any  `json:"edits_failed,omitempty"`
	EditsApplied int    `json:"edits_applied,omitempty"`
}

// TodosParams holds the input for a todos tool call.
type TodosParams struct {
	Todos []session.Todo `json:"todos"`
}

// TodosResponseMetadata holds the metadata returned by a todos tool call.
type TodosResponseMetadata struct {
	IsNew         bool           `json:"is_new,omitempty"`
	JustStarted   string         `json:"just_started,omitempty"`
	Total         int            `json:"total,omitempty"`
	Completed     int            `json:"completed,omitempty"`
	Todos         []session.Todo `json:"todos,omitempty"`
	JustCompleted []session.Todo `json:"just_completed,omitempty"`
}

// ViewResourceSkill is the resource type for skill-backed file views.
const ViewResourceSkill = "skill"

// ViewParams holds the input for a read_file tool call.
type ViewParams struct {
	FilePath string `json:"file_path"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// ViewResponseMetadata holds the metadata returned by a read_file tool call.
type ViewResponseMetadata struct {
	Content             string `json:"content,omitempty"`
	FilePath            string `json:"file_path,omitempty"`
	ResourceType        string `json:"resource_type,omitempty"`
	ResourceName        string `json:"resource_name,omitempty"`
	ResourceDescription string `json:"resource_description,omitempty"`
}

// WriteParams holds the input for a write_file tool call.
type WriteParams struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// WriteResponseMetadata holds the metadata returned by a write_file tool call.
type WriteResponseMetadata struct {
	Diff string `json:"diff,omitempty"`
}

// EditParams holds the input for an edit_file tool call.
type EditParams struct {
	FilePath string `json:"file_path"`
}

// EditResponseMetadata holds the metadata returned by an edit_file tool call.
type EditResponseMetadata struct {
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}

// MultiEditParams holds the input for a multi_edit tool call.
type MultiEditParams struct {
	FilePath string `json:"file_path"`
	Edits    []any  `json:"edits,omitempty"`
}

// DownloadParams holds the input for a download tool call.
type DownloadParams struct {
	URL      string `json:"url"`
	FilePath string `json:"file_path,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
}

// GlobParams holds the input for a glob tool call.
type GlobParams struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// GrepParams holds the input for a grep tool call.
type GrepParams struct {
	Pattern     string `json:"pattern"`
	Path        string `json:"path,omitempty"`
	Include     string `json:"include,omitempty"`
	LiteralText bool   `json:"literal_text,omitempty"`
}

// LSParams holds the input for a list_directory tool call.
type LSParams struct {
	Path string `json:"path"`
}

// SourcegraphParams holds the input for a sourcegraph tool call.
type SourcegraphParams struct {
	Query         string `json:"query"`
	Count         int    `json:"count,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

// ResetCache clears any tool-level in-memory caches (e.g. file content, image caches).
func ResetCache() {}
