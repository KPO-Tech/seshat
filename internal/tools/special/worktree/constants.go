package worktree

// Tool names
const ToolNameEnterWorktree = "enter_worktree"
const ToolNameExitWorktree = "exit_worktree"

// Search hints
const SearchHintEnterWorktree = "create an isolated git worktree and switch into it"
const SearchHintExitWorktree = "exit a worktree session and return to the original directory"

// Descriptions
const DescriptionEnterWorktree = "Creates an isolated worktree (via git or configured hooks) and switches the session into it"
const DescriptionExitWorktree = "Exits a worktree session created by EnterWorktree and restores the original working directory"

// Worktree directory prefix
const WorktreeDirPrefix = ".worktree-"

// Default worktree config
var DefaultWorktreeConfig = WorktreeConfig{
	UseGitWorktree: true,
	CreateBranch:   true,
	DeleteWorktree: true,
}
