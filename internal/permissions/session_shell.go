package permissions

import (
	"context"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/shellparse"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// What "always allow" means for a shell tool.
//
// For most tools it approves the tool for the session. For a shell tool that
// would approve every command that will ever be typed, so it approves what the
// user was shown instead: the program, and the subcommand of the programs that
// have one ("npm test", "git status"), or the exact text of a line that cannot
// be reduced to that (one with a substitution or a redirection, or one that
// starts with a program that runs other programs, such as sh, sudo or xargs).

const (
	shellPrefixGrant = "bash:cmd:"
	shellExactGrant  = "bash:exact:"
)

// grantablePrograms are the programs whose use can be approved as a whole. The
// others (sh, eval, sudo, env, xargs, python, curl, rm...) run or reach for
// something that the program name does not describe, so only an exact line is
// approved for them.
var grantablePrograms = map[string]bool{
	"git": true, "npm": true, "pnpm": true, "yarn": true, "go": true, "cargo": true,
	"make": true, "pytest": true, "tsc": true,
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true,
	"grep": true, "rg": true, "pwd": true, "echo": true, "tree": true,
}

// subcommandPrograms need their subcommand to describe what is approved: "git
// status" is not "git push".
var subcommandPrograms = map[string]bool{
	"git": true, "npm": true, "pnpm": true, "yarn": true, "go": true, "cargo": true,
}

// shellSegmentGrant returns what approving a simple command approves, or "".
func shellSegmentGrant(segment string) string {
	fields := strings.Fields(segment)
	if len(fields) == 0 || !grantablePrograms[fields[0]] {
		return ""
	}
	if !subcommandPrograms[fields[0]] {
		return shellPrefixGrant + fields[0]
	}
	if len(fields) < 2 || strings.HasPrefix(fields[1], "-") {
		return "" // "git -c x=y ..." is not "git"
	}
	return shellPrefixGrant + fields[0] + " " + fields[1]
}

// shellGrantKeys returns the keys to remember when the user approves a shell
// command line for the session.
func shellGrantKeys(command string) []string {
	exact := []string{shellExactGrant + strings.TrimSpace(command)}
	analysis := shellparse.Analyze(command)
	if analysis.Unsafe || len(analysis.Segments) == 0 {
		return exact
	}
	keys := make([]string, 0, len(analysis.Segments))
	for _, segment := range analysis.Segments {
		grant := shellSegmentGrant(segment)
		if grant == "" {
			return exact
		}
		keys = append(keys, grant)
	}
	return keys
}

// sessionApproves reports whether the user already approved this call for the
// session. A shell tool is approved by what the user was shown (see above), any
// other tool as a whole, and a request_permissions call by the escalation it
// asked for.
func (i *Integrator) sessionApproves(sessionID types.SessionID, turnID types.TurnID, toolName string, toolInput map[string]any, grantKeys []string) bool {
	if command, ok := shellCommand(toolName, toolInput); ok {
		return i.shellSessionAllows(sessionID, turnID, command, grantKeys)
	}
	// A request_permissions call with scope=session also gets a second,
	// content-scoped lookup key (see requestPermissionsSessionKey): a session
	// grant for that tool must only cover the exact escalation that was
	// approved (same paths/access/targets), not every future request_permissions
	// call regardless of what it is asking for.
	lookupKeys := []string{toolName}
	if key := requestPermissionsSessionKey(toolName, toolInput); key != "" {
		lookupKeys = append(lookupKeys, key)
	}
	return i.sessionAllows(sessionID, turnID, lookupKeys, grantKeys)
}

// shellSessionAllows reports whether the grants of a session cover a shell
// command line: its exact text was approved, or every simple command in it
// belongs to an approved program or subcommand and the line holds nothing that
// runs something else or writes somewhere.
func (i *Integrator) shellSessionAllows(sessionID types.SessionID, turnID types.TurnID, command string, grantKeys []string) bool {
	if i.sessionAllows(sessionID, turnID, []string{shellExactGrant + strings.TrimSpace(command)}, grantKeys) {
		return true
	}
	analysis := shellparse.Analyze(command)
	if analysis.Unsafe || len(analysis.Segments) == 0 {
		return false
	}
	for _, segment := range analysis.Segments {
		grant := shellSegmentGrant(segment)
		if grant == "" || !i.sessionAllows(sessionID, turnID, []string{grant}, nil) {
			return false
		}
	}
	return true
}

// rememberApproval remembers what the user approved with "always allow": the
// tool for most tools, the program or the exact line for a shell tool.
func (i *Integrator) rememberApproval(sessionID types.SessionID, toolName string, toolInput map[string]any) {
	if command, ok := shellCommand(toolName, toolInput); ok {
		for _, key := range shellGrantKeys(command) {
			i.persistSessionApproval(sessionID, key)
		}
		return
	}
	i.persistSessionApproval(sessionID, toolName)
}

// hasDenyRule reports whether a deny rule applies to the call. A deny rule
// outranks what the user approved for the session: a rule written to forbid
// something must not be undone by an earlier click on "always allow". An ask
// rule does not: it is how a tool is made to ask in the first place, and
// "always allow" is the user's answer to it.
func (e *Engine) hasDenyRule(ctx context.Context, pctx *PermissionContext) bool {
	return e.findWholeToolRule(pctx, types.PermissionBehaviorDeny) != nil ||
		e.findGlobalRule(ctx, pctx, types.PermissionBehaviorDeny) != nil
}
