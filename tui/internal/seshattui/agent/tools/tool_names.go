package tools

// Tool name constants matching SDK builtin names where applicable.
// These are used by the permission dialog and chat UI to render tool-specific UI.
const (
	BashToolName            = "bash"
	EditToolName            = "edit_file"
	WriteToolName           = "write_file"
	MultiEditToolName       = "multi_edit"
	ApplyPatchToolName      = "apply_patch"
	ViewToolName            = "read_file"
	RemoveFileToolName      = "remove_file"
	CreateDirectoryToolName = "create_directory"
	GetFileMetadataToolName = "get_file_metadata"
	DownloadToolName        = "download"
	FetchToolName           = "fetch"
	AgenticFetchToolName    = "agentic_fetch"
	LSToolName              = "list_directory"
	JobOutputToolName       = "job_output"
	JobKillToolName         = "job_kill"
	GlobToolName            = "glob"
	GrepToolName            = "grep"
	SourcegraphToolName     = "sourcegraph"
	WebFetchToolName        = "web_fetch"
	WebSearchToolName       = "web_search"
	AgentToolName           = "agent"
	AskUserToolName         = "ask_user_question"
	NotebookEditToolName    = "notebook_edit"
	NotebookCreateToolName  = "notebook_create"
	NotebookWriteToolName   = "notebook_write"

	BashNoOutput = "<no output>"
)

// AskUserOption mirrors an option in an ask_user_question prompt.
type AskUserOption struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

// AskUserQuestion mirrors one question in an ask_user_question survey.
type AskUserQuestion struct {
	Question    string          `json:"question"`
	Header      string          `json:"header"`
	Options     []AskUserOption `json:"options"`
	MultiSelect bool            `json:"multiSelect,omitempty"`
}

// AskUserRequest is published to the askUserBroker for each question set the agent asks.
type AskUserRequest struct {
	ID           string            `json:"id"`
	ToolCallID   string            `json:"tool_call_id"`
	Question     string            `json:"question"`
	Header       string            `json:"header"`
	Options      []AskUserOption   `json:"options"`
	MultiSelect  bool              `json:"multi_select"`
	Questions    []AskUserQuestion `json:"questions,omitempty"`
	IsCustomText bool              `json:"is_custom_text"`
}

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

// NotebookEditPermissionsParams holds the params for a notebook_edit permission request.
type NotebookEditPermissionsParams struct {
	NotebookPath string
	CellID       string
	CellType     string
	EditMode     string
	OldContent   string // full notebook JSON before edit (may be empty if file is new)
	NewSource    string // the new_source being written into the cell
}

// NotebookCreatePermissionsParams holds the params for a notebook_create permission request.
type NotebookCreatePermissionsParams struct {
	NotebookPath string
	Kernel       string
	Language     string
	CellCount    int
	Cells        []NotebookCellPreview
}

// NotebookCellPreview is a lightweight notebook cell payload for permission previews.
type NotebookCellPreview struct {
	CellType string
	Source   string
}

// NotebookWritePermissionsParams holds the params for a notebook_write permission request.
type NotebookWritePermissionsParams struct {
	NotebookPath string
	Kernel       string
	Language     string
	CellCount    int
	Cells        []NotebookCellPreview
	OldContent   string // existing file content when overwriting
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
	URL        string `json:"url"`
	Prompt     string `json:"prompt,omitempty"`
	RenderMode string `json:"render_mode,omitempty"`
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

// ApplyPatchParams holds the input for an apply_patch tool call.
type ApplyPatchParams struct {
	Patch string `json:"patch"`
}

// RemoveFileParams holds the input for a remove_file tool call.
type RemoveFileParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// CreateDirectoryParams holds the input for a create_directory tool call.
type CreateDirectoryParams struct {
	Path string `json:"path"`
}

// GetFileMetadataParams holds the input for a get_file_metadata tool call.
type GetFileMetadataParams struct {
	Path string `json:"path"`
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
