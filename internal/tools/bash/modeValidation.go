package bash

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/shellparse"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/files/shared"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// AcceptEditsWriteCommands are commands auto-allowed in AcceptEdits mode, with
// the options listed in acceptEditsWriteOptions. sed is not one of them: with an
// editing script it writes where the script says (w, e), so a script has to be
// read to know what it does; the edit_file tool is the way to edit a file.
var AcceptEditsWriteCommands = []string{
	"mkdir", "touch", "rm", "rmdir", "mv", "cp",
}

// AcceptEditsReadOnlyCommands are read-only commands auto-allowed in AcceptEdits
// mode. uniq is not one of them: its second argument is a file it writes.
var AcceptEditsReadOnlyCommands = []string{
	"grep", "cat", "ls", "find", "head", "tail", "echo",
	"pwd", "wc", "sort", "diff",
}

// acceptEditsWriteOptions are the options a write command may carry and still be
// approved by the mode. Anything else (cp -t /etc, mv -T, rm --no-preserve-root)
// changes where the command writes or what it is allowed to touch, and asks.
var acceptEditsWriteOptions = map[string]map[string]bool{
	"mkdir": setOf("-p", "-v"),
	"touch": setOf("-a", "-m", "-c"),
	"rm":    setOf("-r", "-R", "-f", "-v", "-d", "-rf", "-fr", "-rv", "-vr", "--recursive", "--force"),
	"rmdir": setOf("-p", "-v"),
	"mv":    setOf("-v", "-n", "-f", "-i"),
	"cp":    setOf("-r", "-R", "-v", "-n", "-p", "-f", "-a", "--recursive"),
}

// findWriteOptions are the options of find that delete, run a command or write a
// file: a find that carries one is not a read.
var findWriteOptions = setOf("-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls")

func setOf(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
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
		return checkAcceptEditsMode(command, workingDir)
	default:
		return types.Passthrough(nil)
	}
}

// checkAcceptEditsMode implements AcceptEdits-mode permission logic.
//
// The mode approves one simple command at a time. It used to look at the first
// word of the line only, so "ls\nrm -rf .", "cat x | bash" and "ls | tee
// /etc/foo" were approved as if they were "ls" and "cat".
func checkAcceptEditsMode(command, workingDir string) types.PermissionResult {
	analysis := shellparse.Analyze(command)
	if analysis.Unsafe || len(analysis.Segments) != 1 {
		return types.Ask("a line with several commands, a substitution or a redirection requires approval in AcceptEdits mode")
	}
	segment := analysis.Segments[0]
	baseCmd := extractBaseCmd(segment)
	if isAcceptEditsWriteCmd(baseCmd) {
		return checkAcceptEditsWriteCommand(segment, baseCmd, workingDir)
	}
	if isAcceptEditsReadOnlyCmd(baseCmd) {
		return checkAcceptEditsReadOnlyCommand(segment, baseCmd)
	}
	return types.Ask(fmt.Sprintf("command '%s' requires approval in AcceptEdits mode", baseCmd))
}

// isSingleSimpleCommand reports whether a command line is one simple command with
// nothing that runs something else or writes somewhere.
func isSingleSimpleCommand(command string) bool {
	analysis := shellparse.Analyze(command)
	return !analysis.Unsafe && len(analysis.Segments) == 1
}

// writeCommandNeedsApproval reports why a write command cannot be approved by the
// mode from its text alone, or "".
func writeCommandNeedsApproval(command, baseCmd string) string {
	// The paths of the command are read from its words. A quote or a backslash
	// means a word is not what it looks like.
	if strings.ContainsAny(command, "\"'\\") {
		return "a quoted or escaped argument"
	}
	allowed := acceptEditsWriteOptions[baseCmd]
	endOfOptions := false
	for _, arg := range extractArgs(command) {
		switch {
		case endOfOptions:
		case arg == "--":
			endOfOptions = true
			continue
		case strings.HasPrefix(arg, "-") && arg != "-":
			if !allowed[arg] {
				return fmt.Sprintf("the option %s", arg)
			}
			continue
		}
		// A path that the shell expands cannot be placed inside the working
		// directory by reading it: $HOME/x, ~/x, `x`, {/etc/x,y}.
		if strings.ContainsAny(arg, "$~`{") {
			return fmt.Sprintf("the path %s", arg)
		}
	}
	return ""
}

func checkAcceptEditsWriteCommand(command, baseCmd, workingDir string) types.PermissionResult {
	if reason := writeCommandNeedsApproval(command, baseCmd); reason != "" {
		return types.AskWithDecisionReason(
			fmt.Sprintf("%s requires approval in AcceptEdits mode: %s", baseCmd, reason),
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonSafetyCheck,
				Source: "acceptEdits",
				Reason: reason,
			},
		)
	}
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

// readOnlyOptionWrites reports the option that makes a command of the read-only
// list write or run something, or "": find -delete / -exec, sort -o /
// --compress-program.
func readOnlyOptionWrites(command, baseCmd string) string {
	for _, arg := range extractArgs(command) {
		switch baseCmd {
		case "find":
			if findWriteOptions[arg] {
				return arg
			}
		case "sort":
			if arg == "-o" || arg == "--output" || strings.HasPrefix(arg, "--output=") ||
				strings.HasPrefix(arg, "--compress-program") ||
				(strings.HasPrefix(arg, "-o") && !strings.HasPrefix(arg, "--")) {
				return arg
			}
		}
	}
	return ""
}

func checkAcceptEditsReadOnlyCommand(command, baseCmd string) types.PermissionResult {
	if option := readOnlyOptionWrites(command, baseCmd); option != "" {
		return types.AskWithDecisionReason(
			fmt.Sprintf("%s %s writes or runs something and requires approval", baseCmd, option),
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonSafetyCheck,
				Source: "acceptEdits",
				Reason: fmt.Sprintf("the option %s", option),
			})
	}
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
		// Every word is a path to check, whatever it contains: an argument with an
		// "=" in it used to be skipped, so "touch /etc/x=y" was never looked at.
		if isAbsolutePathWord(arg) {
			paths = append(paths, arg)
		} else {
			paths = append(paths, filepath.Join(workingDir, arg))
		}
	}
	return paths
}

// isAbsolutePathWord reports whether a word of a shell command names a path from
// the root. The commands run in a POSIX shell (Git Bash on Windows), so "/etc/x"
// is absolute there even though filepath.IsAbs says no on Windows, where it was
// joined to the working directory and read as a path inside it.
func isAbsolutePathWord(arg string) bool {
	if filepath.IsAbs(arg) || strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, `\`) {
		return true
	}
	return len(arg) >= 2 && arg[1] == ':' && (arg[0] >= 'a' && arg[0] <= 'z' || arg[0] >= 'A' && arg[0] <= 'Z')
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
