package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Config for EnterWorktree tool
type EnterWorktreeConfig struct {
	// WorkingDir is the working directory
	WorkingDir string

	// Manager is the worktree manager
	Manager *WorktreeManager
}

// Default config
func DefaultEnterWorktreeConfig() *EnterWorktreeConfig {
	return &EnterWorktreeConfig{
		WorkingDir: ".",
		Manager:    NewWorktreeManager(DefaultWorktreeConfig),
	}
}

// EnterWorktreeTool is the tool for entering a worktree
type EnterWorktreeTool struct {
	config *EnterWorktreeConfig
}

// NewEnterWorktreeTool creates a new EnterWorktree tool
func NewEnterWorktreeTool(config *EnterWorktreeConfig) *EnterWorktreeTool {
	if config == nil {
		config = DefaultEnterWorktreeConfig()
	}
	return &EnterWorktreeTool{
		config: config,
	}
}

// Definition returns the tool definition
func (t *EnterWorktreeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameEnterWorktree,
		DisplayName:        "enter_worktree",
		SearchHint:         SearchHintEnterWorktree,
		Description:        EnterWorktreePrompt,
		Category:           "worktree",
		IsReadOnly:         false,
		IsDestructive:      false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		Metadata: map[string]any{
			"is_stateful":      true,
			"surface_profiles": []string{"mono_run"},
		},
	}
}

// Call executes the tool
func (t *EnterWorktreeTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	// Check if already in a worktree
	if GetSession(input.SessionID) != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": "Already in a worktree session"},
			Content: "Error: Already in a worktree session. Use exit_worktree first.",
		}, nil
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = t.config.WorkingDir
	}

	// Parse input
	var parsedInput map[string]any
	if err := json.Unmarshal([]byte(input.Raw), &parsedInput); err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": fmt.Sprintf("Failed to parse input: %v", err)},
			Content: fmt.Sprintf("Failed to parse input: %v", err),
		}, nil
	}

	// Extract parameters
	slug := ""
	branch := ""

	if name, ok := parsedInput["name"].(string); ok {
		slug = name
	}
	if branchVal, ok := parsedInput["branch"].(string); ok {
		branch = branchVal
	}

	// Validate slug
	if slug != "" {
		if err := ValidateWorktreeSlug(slug); err != nil {
			return tool.CallResult{
				Data:    map[string]any{"error": err.Error()},
				Content: "Error: " + err.Error(),
			}, nil
		}
	}

	// Generate slug if not provided
	if slug == "" {
		slug = generateSlug()
	}

	// Create worktree
	session, err := t.config.Manager.CreateWorktree(ctx, slug, branch, cwd)
	if err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": err.Error()},
			Content: "Error: " + err.Error(),
		}, nil
	}

	// Register session scoped to this session ID
	SetSession(input.SessionID, session)

	// Change to worktree directory
	if err := os.Chdir(session.WorktreePath); err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": err.Error()},
			Content: "Error: " + err.Error(),
		}, nil
	}

	// Build result
	branchInfo := ""
	if session.WorktreeBranch != "" {
		branchInfo = " on branch " + session.WorktreeBranch
	}

	message := fmt.Sprintf("Created worktree at %s%s. The session is now working in the worktree. Use ExitWorktree to leave mid-session, or exit the session to be prompted.", session.WorktreePath, branchInfo)

	return tool.CallResult{
		Data: map[string]any{
			"worktreePath":   session.WorktreePath,
			"worktreeBranch": session.WorktreeBranch,
			"message":        message,
		},
		Content: message,
	}, nil
}

// ValidateInput validates tool input
func (t *EnterWorktreeTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Already validated in Call
	return input, nil
}

// CheckPermissions checks tool permissions
func (t *EnterWorktreeTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAsk}
}

// IsConcurrencySafe returns whether tool is concurrency safe
func (t *EnterWorktreeTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

// IsReadOnly returns whether tool is read-only
func (t *EnterWorktreeTool) IsReadOnly(input map[string]any) bool {
	return false
}

// IsEnabled returns whether tool is enabled
func (t *EnterWorktreeTool) IsEnabled() bool {
	return true
}

// FormatResult formats result
func (t *EnterWorktreeTool) FormatResult(data any) string {
	bytes, _ := json.Marshal(data)
	return string(bytes)
}

// BackfillInput enriches input
func (t *EnterWorktreeTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// Description returns description
func (t *EnterWorktreeTool) Description(ctx context.Context) (string, error) {
	return DescriptionEnterWorktree, nil
}

// generateSlug generates a random slug
func generateSlug() string {
	// Simple random slug based on timestamp
	return fmt.Sprintf("worktree-%d", os.Getpid())
}

// EnterWorktreePrompt is the system prompt for the EnterWorktree tool.
const EnterWorktreePrompt = `Creates an isolated git worktree and switches the session into it.

A worktree is an isolated git working directory that allows you to work on multiple
branches simultaneously without stashing or committing your current changes.

## How Worktrees Work

1. A new worktree is created as a subdirectory of the main repository
2. A new branch is created (or existing branch checked out)
3. The session is switched to work in the worktree
4. All file operations happen in the worktree
5. When done, use ExitWorktree to return to the original directory

## Usage

// Create worktree with auto-generated name
enter_worktree({})

// Create worktree with custom name
enter_worktree({ name: "feature/my-branch" })

// Create worktree on existing branch
enter_worktree({ name: "bugfix/fix-login", branch: "bugfix/fix-login" })

## Parameters

- name: Optional name for the worktree directory. Defaults to plan slug or random.
- branch: Optional branch name. If not provided, creates a new branch.

## Examples

// For a plan "my-plan", creates .worktree-my-plan/
enter_worktree({})

// Custom name with subdirectory
enter_worktree({ name: "hotfix/critical-bug" })

// Specify branch
enter_worktree({ name: "experiment", branch: "experiment-new-api" })

## Notes

- Worktrees are isolated - changes in one don't affect others
- ExitWorktree with action "keep" preserves the worktree
- ExitWorktree with action "remove" deletes the worktree
- You can only have one active worktree session at a time`
