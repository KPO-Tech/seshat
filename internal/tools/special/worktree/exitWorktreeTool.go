package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Config for ExitWorktree tool
type ExitWorktreeConfig struct {
	// WorkingDir is the working directory
	WorkingDir string

	// Manager is the worktree manager
	Manager *WorktreeManager
}

// Default config
func DefaultExitWorktreeConfig() *ExitWorktreeConfig {
	return &ExitWorktreeConfig{
		WorkingDir: ".",
		Manager:    NewWorktreeManager(DefaultWorktreeConfig),
	}
}

// ExitWorktreeTool is the tool for exiting a worktree
type ExitWorktreeTool struct {
	config *ExitWorktreeConfig
}

// NewExitWorktreeTool creates a new ExitWorktree tool
func NewExitWorktreeTool(config *ExitWorktreeConfig) *ExitWorktreeTool {
	if config == nil {
		config = DefaultExitWorktreeConfig()
	}
	return &ExitWorktreeTool{
		config: config,
	}
}

// Definition returns the tool definition
func (t *ExitWorktreeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameExitWorktree,
		DisplayName:        "exit_worktree",
		SearchHint:         SearchHintExitWorktree,
		Description:        ExitWorktreePrompt,
		Category:           "worktree",
		IsReadOnly:         false,
		IsDestructive:      true,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		Metadata: map[string]any{
			"is_destructive":   true,
			"is_stateful":      true,
			"surface_profiles": []string{"mono_run"},
		},
	}
}

// Call executes the tool
func (t *ExitWorktreeTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	session := GetSession(input.SessionID)

	// Check for active session
	if session == nil {
		return tool.CallResult{
			Data:    map[string]any{"error": "No active worktree session to exit"},
			Content: "Error: No active worktree session to exit. This tool only operates on worktrees created by EnterWorktree in the current session.",
		}, nil
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
	action := "keep"
	discardChanges := false

	if actionVal, ok := parsedInput["action"].(string); ok {
		action = actionVal
	}
	if discardVal, ok := parsedInput["discard_changes"].(bool); ok {
		discardChanges = discardVal
	}

	// Validate action
	if action != "keep" && action != "remove" {
		return tool.CallResult{
			Data:    map[string]any{"error": "action must be 'keep' or 'remove'"},
			Content: "Error: action must be 'keep' or 'remove'",
		}, nil
	}

	// Get state before changes
	originalCwd := session.OriginalCwd
	worktreePath := session.WorktreePath
	worktreeBranch := session.WorktreeBranch

	// Count changes if removing
	var changedFiles, commits int
	if action == "remove" {
		changedFiles, commits, _ = t.config.Manager.CountWorktreeChanges(
			worktreePath, session.OriginalHeadCommit,
		)

		// Check for uncommitted changes
		if changedFiles > 0 || commits > 0 {
			if !discardChanges {
				parts := []string{}
				if changedFiles > 0 {
					parts = append(parts, fmt.Sprintf("%d uncommitted file(s)", changedFiles))
				}
				if commits > 0 {
					parts = append(parts, fmt.Sprintf("%d commit(s)", commits))
				}
				return tool.CallResult{
					Data: map[string]any{
						"error":     fmt.Sprintf("Worktree has %s. Removing will discard this work permanently.", joinParts(parts, " and ")),
						"errorCode": 2,
					},
					Content: fmt.Sprintf("Error: Worktree has %s. Removing will discard this work permanently. Confirm with discard_changes: true, or use action: 'keep' to preserve.", joinParts(parts, " and ")),
				}, nil
			}
		}
	}

	var message string

	if action == "keep" {
		// Keep the worktree on disk; clear only our registry entry.
		SetSession(input.SessionID, nil)
		message = fmt.Sprintf("Exited worktree. Your work is preserved at %s on branch %s. Session is now back in %s.",
			worktreePath, worktreeBranch, originalCwd)
		os.Chdir(originalCwd)

	} else {
		// Remove the worktree from disk.
		if err := t.config.Manager.RemoveWorktree(session, discardChanges); err != nil {
			return tool.CallResult{
				Data:    map[string]any{"error": err.Error()},
				Content: "Error: " + err.Error(),
			}, nil
		}
		SetSession(input.SessionID, nil)

		// Build message
		parts := []string{}
		if commits > 0 {
			parts = append(parts, fmt.Sprintf("Discarded %d commit(s)", commits))
		}
		if changedFiles > 0 {
			parts = append(parts, fmt.Sprintf("Discarded %d uncommitted file(s)", changedFiles))
		}

		discardNote := ""
		if len(parts) > 0 {
			discardNote = " " + joinParts(parts, " and ") + "."
		}

		message = fmt.Sprintf("Exited and removed worktree at %s.%s Session is now back in %s.",
			worktreePath, discardNote, originalCwd)
		os.Chdir(originalCwd)
	}

	return tool.CallResult{
		Data: map[string]any{
			"action":         action,
			"originalCwd":    originalCwd,
			"worktreePath":   worktreePath,
			"worktreeBranch": worktreeBranch,
			"message":        message,
		},
		Content: message,
	}, nil
}

// ValidateInput validates tool input.
// Session-presence checks require the session ID, which is only available in
// Call via CallInput; they are performed there instead.
func (t *ExitWorktreeTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	action, _ := input["action"].(string)
	if action != "" && action != "keep" && action != "remove" {
		return map[string]any{
			"result":    false,
			"message":   "action must be 'keep' or 'remove'",
			"errorCode": 3,
		}, nil
	}
	return input, nil
}

// CheckPermissions checks tool permissions
func (t *ExitWorktreeTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAsk}
}

// IsConcurrencySafe returns whether tool is concurrency safe
func (t *ExitWorktreeTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

// IsReadOnly returns whether tool is read-only
func (t *ExitWorktreeTool) IsReadOnly(input map[string]any) bool {
	action, _ := input["action"].(string)
	return action == "keep"
}

// IsDestructive returns whether tool is destructive
func (t *ExitWorktreeTool) IsDestructive(input map[string]any) bool {
	action, _ := input["action"].(string)
	return action == "remove"
}

// IsEnabled returns whether tool is enabled
func (t *ExitWorktreeTool) IsEnabled() bool {
	return true
}

// FormatResult formats result
func (t *ExitWorktreeTool) FormatResult(data any) string {
	bytes, _ := json.Marshal(data)
	return string(bytes)
}

// BackfillInput enriches input
func (t *ExitWorktreeTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// Description returns description
func (t *ExitWorktreeTool) Description(ctx context.Context) (string, error) {
	return DescriptionExitWorktree, nil
}

// ExitWorktreePrompt is the system prompt for the ExitWorktree tool.
const ExitWorktreePrompt = `Exits a worktree session and returns to the original working directory.

When you're done working in a worktree, use this tool to exit and optionally clean up.

## Parameters

- action: What to do with the worktree:
  - "keep": Leave the worktree and branch on disk for later use
  - "remove": Delete the worktree and branch permanently
- discard_changes: Required when action is "remove" and there are uncommitted changes.
  The tool will refuse to remove if there are changes unless you confirm.

## Usage

// Exit and keep the worktree for later
exit_worktree({ action: "keep" })

// Exit and remove the worktree (no changes)
exit_worktree({ action: "remove" })

// Exit and remove (with uncommitted changes - will fail)
exit_worktree({ action: "remove" })

// Exit and force remove (with uncommitted changes)
exit_worktree({ action: "remove", discard_changes: true })

## Safety

- The tool will refuse to remove a worktree with uncommitted changes
- Use action: "keep" if you want to preserve your work
- The original working directory is restored

## Notes

- Only works on worktrees created by EnterWorktree in this session
- Does not affect worktrees created manually or in previous sessions
- Tmux sessions are killed on remove, kept on exit`

// Helper functions
func joinParts(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		if i == len(parts)-1 {
			result += sep
		}
		result += parts[i]
	}
	return result
}
