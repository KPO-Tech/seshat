// Package permissions - Loader for permission rules from various sources.
//
// This module provides functionality for loading permission rules
// from disk (settings files). Aligned with OpenClaude's permissionsLoader.ts.
package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	RuleBehaviorAllow = "allow"
	RuleBehaviorDeny  = "deny"
	RuleBehaviorAsk   = "ask"
)

// SettingsSource represents where settings come from.
type SettingsSource string

const (
	SourceUserSettings    SettingsSource = "userSettings"
	SourceProjectSettings SettingsSource = "projectSettings"
	SourceLocalSettings   SettingsSource = "localSettings"
	SourceCliArg          SettingsSource = "cliArg"
	SourceManagedSettings SettingsSource = "policySettings"
)

// SettingsJSON represents the structure of a settings file.
type SettingsJSON struct {
	PermissionRules                 []StoredPermissionRule `json:"permission_rules,omitempty"`
	AllowManagedPermissionRulesOnly bool                   `json:"allow_managed_permission_rules_only,omitempty"`
}

// StoredPermissionRule represents a permission rule in settings.
type StoredPermissionRule struct {
	ToolName    string `json:"tool_name"`
	RuleContent string `json:"rule_content,omitempty"`
	Behavior    string `json:"behavior"`
	Pattern     string `json:"pattern"`
}

// LoadRulesFromDisk loads all permission rules from disk.
// Aligned with OpenClaude's loadAllPermissionRulesFromDisk.
func LoadRulesFromDisk() ([]PermissionRule, error) {
	var allRules []PermissionRule

	sources := []SettingsSource{
		SourceManagedSettings,
		SourceProjectSettings,
		SourceLocalSettings,
		SourceUserSettings,
	}

	for _, source := range sources {
		rules, err := loadRulesForSource(source)
		if err != nil {
			continue
		}
		allRules = append(allRules, rules...)
	}

	return allRules, nil
}

// loadRulesForSource loads rules for a specific source.
func loadRulesForSource(source SettingsSource) ([]PermissionRule, error) {
	settingsPath := getSettingsFilePathForSource(source)
	if settingsPath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var settings SettingsJSON
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	var rules []PermissionRule
	for _, rule := range settings.PermissionRules {
		permRule := PermissionRule{
			Value: PermissionRuleValue{
				ToolName:    rule.ToolName,
				RuleContent: rule.RuleContent,
			},
			Pattern: rule.Pattern,
			Source:  types.PermissionRuleSource(source),
		}
		rules = append(rules, permRule)
	}

	return rules, nil
}

// getSettingsFilePathForSource returns the path to settings file for a source.
func getSettingsFilePathForSource(source SettingsSource) string {
	switch source {
	case SourceUserSettings:
		return filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	case SourceProjectSettings:
		return ".claude-project-settings.json"
	case SourceLocalSettings:
		return ".claude-local-settings.json"
	case SourceManagedSettings:
		return ".claude-managed-settings.json"
	default:
		return ""
	}
}

// ShouldAllowManagedPermissionsOnly returns true if only managed rules should be used.
func ShouldAllowManagedPermissionsOnly() bool {
	path := getSettingsFilePathForSource(SourceManagedSettings)
	if path == "" {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var settings SettingsJSON
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}

	return settings.AllowManagedPermissionRulesOnly
}

// ShouldShowAlwaysAllowOptions returns true if "always allow" should be shown.
func ShouldShowAlwaysAllowOptions() bool {
	return !ShouldAllowManagedPermissionsOnly()
}

// DeletePermissionRuleFromSettings deletes a rule from settings.
func DeletePermissionRuleFromSettings(source SettingsSource, toolName string, ruleContent string) error {
	rules, err := loadRulesForSource(source)
	if err != nil {
		return err
	}

	var updated []StoredPermissionRule
	for _, rule := range rules {
		if rule.Value.ToolName != toolName || rule.Value.RuleContent != ruleContent {
			updated = append(updated, StoredPermissionRule{
				ToolName:    rule.Value.ToolName,
				RuleContent: rule.Value.RuleContent,
			})
		}
	}

	settings := SettingsJSON{
		PermissionRules: updated,
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	path := getSettingsFilePathForSource(source)
	if path == "" {
		return nil
	}

	return os.WriteFile(path, data, 0644)
}

// AddPermissionRulesToSettings adds rules to settings.
func AddPermissionRulesToSettings(source SettingsSource, rules []PermissionRule) error {
	existing, err := loadRulesForSource(source)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var stored []StoredPermissionRule
	for _, rule := range existing {
		stored = append(stored, StoredPermissionRule{
			ToolName:    rule.Value.ToolName,
			RuleContent: rule.Value.RuleContent,
			Behavior:    string(rule.Behavior),
			Pattern:     rule.Pattern,
		})
	}

	for _, rule := range rules {
		stored = append(stored, StoredPermissionRule{
			ToolName:    rule.Value.ToolName,
			RuleContent: rule.Value.RuleContent,
			Behavior:    string(rule.Behavior),
			Pattern:     rule.Pattern,
		})
	}

	settings := SettingsJSON{
		PermissionRules: stored,
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	path := getSettingsFilePathForSource(source)
	if path == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// IsValidRuleBehavior checks if a behavior is valid.
func IsValidRuleBehavior(behavior string) bool {
	return behavior == RuleBehaviorAllow ||
		behavior == RuleBehaviorDeny ||
		behavior == RuleBehaviorAsk
}

// ruleContentMatches checks if rule content matches input content.
func ruleContentMatches(ruleContent, candidate string) bool {
	if ruleContent == "" {
		return true
	}
	return strings.Contains(candidate, ruleContent)
}
