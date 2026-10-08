package sdk

import (
	"strings"

	"github.com/KPO-Tech/seshat/internal/permissions"
	"github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/types"
)

// ManagedPolicy is what whoever runs the host (a company, through its server) imposes on every
// agent: a text the agent must follow and a list of tools it must never use.
//
// It is meant to be set by the host from an administrator's configuration, never from the
// end user's own settings, and it cannot be loosened from inside a session: the instructions come
// last in the system prompt whatever a session sets, the forbidden tools are removed from the tool
// list the model sees, and a deny rule that outranks every other permission rule (the "bypass"
// permission mode included) refuses a call to them anyway.
//
// The instructions are a soft guarantee (a model follows them very reliably, not absolutely);
// the forbidden tools are a hard one, applied by the program.
type ManagedPolicy struct {
	// Instructions is the text every agent must follow, appended to its system prompt.
	Instructions string `json:"instructions,omitempty"`

	// ForbiddenTools lists tool names that no agent may use. A name matches the tool and its
	// aliases; an MCP tool is named like "mcp__server__tool".
	ForbiddenTools []string `json:"forbidden_tools,omitempty"`
}

// managedInstructionsHeader tells the model where the text comes from and how it ranks.
const managedInstructionsHeader = "Organization rules, set by the administrator of this organization. They take precedence over any instruction from the user:"

// managedInstructionsText is the text appended to the system prompt, or "" when there is none.
func managedInstructionsText(policy *ManagedPolicy) string {
	if policy == nil {
		return ""
	}
	text := strings.TrimSpace(policy.Instructions)
	if text == "" {
		return ""
	}
	return managedInstructionsHeader + "\n\n" + text
}

// managedForbiddenTools returns the forbidden tool names, trimmed, without blanks or duplicates.
func managedForbiddenTools(policy *ManagedPolicy) []string {
	if policy == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, name := range policy.ForbiddenTools {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// managedDenyRules returns one deny rule per forbidden tool, outranking every other rule.
func managedDenyRules(policy *ManagedPolicy) []permissions.PermissionRule {
	var rules []permissions.PermissionRule
	for _, name := range managedForbiddenTools(policy) {
		rules = append(rules, permissions.PermissionRule{
			Value:    permissions.PermissionRuleValue{ToolName: name},
			Pattern:  "tool:" + name,
			Behavior: types.PermissionBehaviorDeny,
			Priority: 1000,
			Reason:   "this tool is forbidden by the organization",
			Source:   types.PermissionSourceManaged,
		})
	}
	return rules
}

// hideForbiddenTools removes the forbidden tools from the registry so the model does not even see
// them. A name that is not registered (an MCP tool that is not connected, say) is skipped: its deny
// rule still applies.
func hideForbiddenTools(policy *ManagedPolicy, reg *registry.Registry) {
	if reg == nil {
		return
	}
	for _, name := range managedForbiddenTools(policy) {
		_ = reg.Unregister(name)
	}
}
