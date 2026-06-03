package bash

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/files/shared"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// AcceptEditsWriteCommands are commands auto-allowed in AcceptEdits mode.
var AcceptEditsWriteCommands = []string{
	"mkdir", "touch", "rm", "rmdir", "mv", "cp", "sed",
}

// AcceptEditsReadOnlyCommands are read-only commands auto-allowed in AcceptEdits mode.
var AcceptEditsReadOnlyCommands = []string{
	"grep", "cat", "ls", "find", "head", "tail", "echo",
	"pwd", "wc", "sort", "uniq", "diff",
}

// checkPermissionsMode applies mode-specific permission logic before the
// global approval pipeline. It returns:
//   - types.Allow  → the command may execute without further approval
//   - types.Deny   → the command is rejected immediately
//   - types.Passthrough → no mode-specific opinion; continue normal flow
func checkPermissionsMode(command string, mode types.PermissionMode, workingDir string) types.PermissionResult {
	baseCmd := extractBaseCmd(command)
	if baseCmd == "" {
		return types.Ask("empty command")
	}
	switch mode {
	case types.PermissionModeAcceptEdits:
		return checkAcceptEditsMode(command, baseCmd, workingDir)
	default:
		return types.Passthrough(nil)
	}
}

// checkAcceptEditsMode implements AcceptEdits-mode permission logic.
func checkAcceptEditsMode(command, baseCmd, workingDir string) types.PermissionResult {
	if isAcceptEditsWriteCmd(baseCmd) {
		return checkAcceptEditsWriteCommand(command, baseCmd, workingDir)
	}
	if isAcceptEditsReadOnlyCmd(baseCmd) {
		return checkAcceptEditsReadOnlyCommand(command)
	}
	return types.Ask(fmt.Sprintf("command '%s' requires approval in AcceptEdits mode", baseCmd))
}

func checkAcceptEditsWriteCommand(command, baseCmd, workingDir string) types.PermissionResult {
	if baseCmd == "rm" || baseCmd == "rmdir" {
		for _, arg := range extractArgs(command) {
			if err := shared.CheckDangerousRemovalPath(arg, workingDir); err != nil {
				return types.DenyWithDecisionReason(err.Error(), &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonSafetyCheck,
					Source: "acceptEdits",
					Reason: err.Error(),
				})
			}
		}
	}

	for _, path := range extractPathsFrom(command, workingDir) {
		if shared.IsDangerousFile(path) {
			return types.AskWithDecisionReason(
				fmt.Sprintf("cannot auto-edit dangerous file: %s", path),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonSafetyCheck,
					Source: "acceptEdits",
					Reason: fmt.Sprintf("file is protected: %s", path),
				},
			)
		}
		if shared.IsDangerousDirectory(path) {
			return types.AskWithDecisionReason(
				fmt.Sprintf("cannot auto-edit dangerous directory: %s", path),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonSafetyCheck,
					Source: "acceptEdits",
					Reason: fmt.Sprintf("directory is protected: %s", path),
				},
			)
		}
		if shared.HasSuspiciousPattern(path) {
			return types.AskWithDecisionReason(
				fmt.Sprintf("suspicious path pattern: %s", path),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonSafetyCheck,
					Source: "acceptEdits",
					Reason: fmt.Sprintf("path has suspicious pattern: %s", path),
				},
			)
		}
		if !shared.IsInWorkingDirectory(path, workingDir) {
			return types.AskWithDecisionReason(
				fmt.Sprintf("path outside working directory requires approval: %s", path),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "acceptEdits",
					Reason: fmt.Sprintf("path is outside working directory: %s", path),
				},
			)
		}
	}

	return types.AllowWithDecisionReason("command is safe in AcceptEdits mode", &types.PermissionDecisionReason{
		Type:   types.PermissionDecisionReasonMode,
		Source: "acceptEdits",
		Reason: "command is in working directory and passes safety checks",
	})
}

func checkAcceptEditsReadOnlyCommand(command string) types.PermissionResult {
	if hasRedirection(command) {
		return types.AskWithDecisionReason("read-only command with shell redirection requires approval",
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonOther,
				Source: "acceptEdits",
				Reason: "shell redirection detected",
			})
	}
	if hasDangerousSyntax(command) {
		return types.AskWithDecisionReason("command with complex shell syntax requires approval",
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonSafetyCheck,
				Source: "acceptEdits",
				Reason: "command contains shell syntax requiring approval",
			})
	}
	return types.AllowWithDecisionReason("read-only command is safe", &types.PermissionDecisionReason{
		Type:   types.PermissionDecisionReasonMode,
		Source: "acceptEdits",
		Reason: "read-only command passes AcceptEdits checks",
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func extractBaseCmd(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if idx := strings.LastIndexAny(name, "/\\"); idx != -1 {
		name = name[idx+1:]
	}
	return strings.ToLower(name)
}

func extractArgs(command string) []string {
	fields := strings.Fields(command)
	if len(fields) <= 1 {
		return nil
	}
	return fields[1:]
}

func extractPathsFrom(command, workingDir string) []string {
	var paths []string
	for _, arg := range extractArgs(command) {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.HasPrefix(arg, "\"") || strings.HasPrefix(arg, "'") {
			continue
		}
		if strings.ContainsAny(arg, "=|&") {
			continue
		}
		if filepath.IsAbs(arg) {
			paths = append(paths, arg)
		} else {
			paths = append(paths, filepath.Join(workingDir, arg))
		}
	}
	return paths
}

func isAcceptEditsWriteCmd(cmd string) bool {
	for _, c := range AcceptEditsWriteCommands {
		if cmd == c {
			return true
		}
	}
	return false
}

func isAcceptEditsReadOnlyCmd(cmd string) bool {
	for _, c := range AcceptEditsReadOnlyCommands {
		if cmd == c {
			return true
		}
	}
	return false
}

func hasRedirection(command string) bool {
	for _, op := range []string{">>", ">", "<", "2>", "&>"} {
		if strings.Contains(command, op) {
			return true
		}
	}
	return false
}

func hasDangerousSyntax(command string) bool {
	return strings.Contains(command, "$(") ||
		strings.Contains(command, "`") ||
		strings.Contains(command, "$((") ||
		strings.Contains(command, "eval ") ||
		strings.HasSuffix(command, "eval") ||
		strings.Contains(command, "<(") ||
		strings.Contains(command, ">(")
}
