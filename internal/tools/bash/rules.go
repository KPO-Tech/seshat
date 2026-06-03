package bash

import (
	"regexp"
	"strings"
)

// permissionRuleContentMatches checks whether a permission rule's content
// matches the given command. Used by PreparePermissionMatcher so the global
// permission engine can match stored rule patterns against bash commands.
func permissionRuleContentMatches(ruleContent string, command string) bool {
	ruleContent = strings.TrimSpace(ruleContent)
	command = strings.TrimSpace(command)
	if ruleContent == "" || command == "" {
		return false
	}
	if prefix, ok := legacyPrefixRule(ruleContent); ok {
		return command == prefix || strings.HasPrefix(command, prefix+" ")
	}
	if hasRuleWildcard(ruleContent) {
		return matchRuleWildcard(ruleContent, command)
	}
	return command == ruleContent
}

func legacyPrefixRule(ruleContent string) (string, bool) {
	if strings.HasSuffix(ruleContent, ":*") && len(ruleContent) > 2 {
		return strings.TrimSuffix(ruleContent, ":*"), true
	}
	return "", false
}

func hasRuleWildcard(pattern string) bool {
	escaped := false
	for _, r := range pattern {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '*' {
			return true
		}
	}
	return false
}

func matchRuleWildcard(pattern string, command string) bool {
	pattern = strings.TrimSpace(pattern)
	command = strings.TrimSpace(command)

	var b strings.Builder
	b.WriteString("^")
	escaped := false
	for _, r := range pattern {
		switch {
		case escaped:
			b.WriteString(regexp.QuoteMeta(string(r)))
			escaped = false
		case r == '\\':
			escaped = true
		case r == '*':
			b.WriteString(".*")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if escaped {
		b.WriteString(regexp.QuoteMeta("\\"))
	}
	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(command)
}
