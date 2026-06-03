package permissions

import (
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// RuleSourcePriority defines the priority order for rule sources.
// Higher priority sources override lower priority sources.
// Aligned with OpenClaude's SETTING_SOURCES order (permissions.ts:109-114).
var RuleSourcePriority = map[types.PermissionRuleSource]int{
	types.PermissionSourceCliArg:          100, // CLI args have highest priority
	types.PermissionSourceSession:         90,  // Session-specific rules
	types.PermissionSourceProjectSettings: 80,  // Project-level settings
	types.PermissionSourceUserSettings:    70,  // User-level settings
	types.PermissionSourceLocalSettings:   60,  // Local machine settings
	types.PermissionSourceStatic:          50,  // Static/default rules
	types.PermissionSourceDynamic:         40,  // Dynamic rules
	types.PermissionSourceClassifier:      30,  // Classifier-generated rules
	types.PermissionSourceHook:            20,  // Hook-generated rules
}

// GetRulesBySource returns all rules from a specific source.
func (e *Engine) GetRulesBySource(source types.PermissionRuleSource) []PermissionRule {
	var rules []PermissionRule
	for _, rule := range e.rules {
		if rule.Source == source {
			rules = append(rules, rule)
		}
	}
	return rules
}

// RemoveRulesBySource removes all rules from a specific source.
func (e *Engine) RemoveRulesBySource(source types.PermissionRuleSource) {
	var newRules []PermissionRule
	for _, rule := range e.rules {
		if rule.Source != source {
			newRules = append(newRules, rule)
		}
	}
	e.rules = newRules
	e.sortRules()
}

// UpdateRules applies permission updates from various sources.
// Aligned with OpenClaude's permission update system (permissions.ts:1420-1447).
func (e *Engine) UpdateRules(update types.PermissionUpdate) error {
	switch update.Type {
	case types.PermissionUpdateTypeAddRules:
		for _, ruleValue := range update.Rules {
			rule := PermissionRule{
				Value:    PermissionRuleValue{ToolName: ruleValue.ToolName, RuleContent: ruleValue.RuleContent},
				Behavior: update.Behavior,
				Source:   update.Destination,
				Priority: RuleSourcePriority[update.Destination],
				Reason:   fmt.Sprintf("Added from %s", update.Destination),
			}
			if err := e.AddRule(rule); err != nil {
				return fmt.Errorf("failed to add rule: %w", err)
			}
		}

	case types.PermissionUpdateTypeReplaceRules:
		// Remove existing rules from this source
		e.RemoveRulesBySource(update.Destination)
		// Add new rules
		for _, ruleValue := range update.Rules {
			rule := PermissionRule{
				Value:    PermissionRuleValue{ToolName: ruleValue.ToolName, RuleContent: ruleValue.RuleContent},
				Behavior: update.Behavior,
				Source:   update.Destination,
				Priority: RuleSourcePriority[update.Destination],
				Reason:   fmt.Sprintf("Replaced from %s", update.Destination),
			}
			if err := e.AddRule(rule); err != nil {
				return fmt.Errorf("failed to add rule: %w", err)
			}
		}

	case types.PermissionUpdateTypeDeleteRules:
		// Remove specific rules from this source
		var newRules []PermissionRule
		for _, rule := range e.rules {
			if rule.Source != update.Destination {
				newRules = append(newRules, rule)
				continue
			}
			// Check if this rule matches any of the rule values to delete
			shouldDelete := false
			for _, ruleValue := range update.Rules {
				if rule.Value.ToolName == ruleValue.ToolName &&
					rule.Value.RuleContent == ruleValue.RuleContent {
					shouldDelete = true
					break
				}
			}
			if !shouldDelete {
				newRules = append(newRules, rule)
			}
		}
		e.rules = newRules
		e.sortRules()

	default:
		return fmt.Errorf("unknown permission update type: %s", update.Type)
	}

	return nil
}

// GetEffectiveRules returns rules sorted by priority (highest first).
// This ensures that higher-priority sources (cliArg, session) take precedence.
func (e *Engine) GetEffectiveRules() []PermissionRule {
	// Rules are already sorted by priority in sortRules()
	return e.rules
}

// GetRuleSourcePriority returns the priority for a given source.
func (e *Engine) GetRuleSourcePriority(source types.PermissionRuleSource) int {
	if priority, ok := RuleSourcePriority[source]; ok {
		return priority
	}
	return 0 // Default priority
}

// MergeRulesFromSources merges rules from multiple sources, respecting priority.
// This is useful when loading rules from different configuration sources.
func (e *Engine) MergeRulesFromSources(sourceRules map[types.PermissionRuleSource][]PermissionRule) error {
	// Sort sources by priority (highest first)
	sources := make([]types.PermissionRuleSource, 0, len(sourceRules))
	for source := range sourceRules {
		sources = append(sources, source)
	}
	// Sort by priority descending
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			if RuleSourcePriority[sources[i]] < RuleSourcePriority[sources[j]] {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}

	// Clear existing rules
	e.rules = make([]PermissionRule, 0)

	// Add rules from each source in priority order
	for _, source := range sources {
		rules := sourceRules[source]
		for _, rule := range rules {
			rule.Source = source
			rule.Priority = RuleSourcePriority[source]
			if err := e.AddRule(rule); err != nil {
				return fmt.Errorf("failed to add rule from %s: %w", source, err)
			}
		}
	}

	return nil
}

// GetAllRuleSources returns all rule sources currently in use.
func (e *Engine) GetAllRuleSources() []types.PermissionRuleSource {
	sources := make(map[types.PermissionRuleSource]bool)
	for _, rule := range e.rules {
		sources[rule.Source] = true
	}

	result := make([]types.PermissionRuleSource, 0, len(sources))
	for source := range sources {
		result = append(result, source)
	}
	return result
}

// GetRuleCountBySource returns the count of rules for each source.
func (e *Engine) GetRuleCountBySource() map[types.PermissionRuleSource]int {
	counts := make(map[types.PermissionRuleSource]int)
	for _, rule := range e.rules {
		counts[rule.Source]++
	}
	return counts
}
