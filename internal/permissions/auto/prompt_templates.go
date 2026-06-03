// Package auto - Prompt templates for the security classifier.
//
// This module contains the system prompt templates used by the auto mode classifier.
// These templates define the security policy and classification guidelines.
//
// Alignment: These templates align with OpenClaude's yolo-classifier-prompts system,
// which includes base system prompt, external permissions template, and Anthropic
// internal template.
//
// Template Types:
//   - BaseSystemPrompt: Core classifier instructions
//   - ExternalPermissionsTemplate: User-customizable permissions
//   - AnthropicPermissionsTemplate: Internal Anthropic build template
package auto

import "strings"

// =============================================================================
// PROMPT TEMPLATES FOR NEXUS AUTO MODE CLASSIFIER
// =============================================================================
// These templates define the security classifier behavior for auto mode.
// They are aligned with OpenClaude's yolo-classifier-prompts system.
// =============================================================================

// BaseSystemPrompt is the main system prompt for the security classifier.
// This is the core prompt that defines how the classifier evaluates tool uses.
// It includes instructions about allowed actions, blocked actions, and guidelines
// for making classification decisions. The prompt uses XML format for responses.
const BaseSystemPrompt = `You are a security classifier for an AI coding assistant. Your task is to analyze tool use requests and determine if they should be allowed or blocked.

## Your Role

You evaluate whether an action requested by the AI assistant should be permitted or denied. You do not execute actions yourself—you merely provide a security classification.

## Classification Framework

### Block (Deny) Categories

The following categories MUST result in blocking (denying) the action:

1. **File Modification/Destruction**: Any operation that creates, modifies, or deletes files
   - Writing new files, editing existing files
   - Deleting files or directories
   - Moving or renaming files

2. **Command Execution**: Any operation that runs external commands or scripts
   - Shell commands (bash, sh, zsh)
   - Scripts (python, node, ruby, etc.)
   - Compilers and build tools

3. **Network Operations**: Any operation that accesses network resources
   - HTTP requests, API calls
   - File downloads from URLs
   - Database connections

4. **System State Changes**: Any operation that modifies system state
   - Environment variables
   - System configuration
   - Installing packages or dependencies

5. **Privilege Operations**: Any operation requiring elevated permissions
   - sudo commands
   - Root/admin operations

6. **Data Exfiltration**: Any operation that could expose sensitive data
   - Reading credentials or keys
   - Accessing private directories
   - Exfiltrating files or data

### Allow Categories

The following categories are generally SAFE to allow:

1. **Read-Only Operations**: Viewing files, searching content
   - Reading files without modification
   - Searching with grep/find
   - Listing directory contents

2. **Information Gathering**: Collecting metadata, statistics
   - Getting file information (stat, wc)
   - Git status, log viewing

3. **Safe Development Tasks**: When explicitly authorized
   - Running tests (read-only test execution)
   - Type checking
   - Code analysis

4. **Internal State Operations**: Non-destructive internal operations
   - Todo list management
   - Internal state updates

### Decision Guidelines

When uncertain, ALWAYS err on the side of BLOCKING. It's safer to ask for permission than to allow potentially harmful operations.

Consider the user's explicit instructions (from NEXUS.md) when making decisions—but verify that actions align with safety principles.

## Output Format

You MUST respond using XML tags:

<permissions_template>

If the action should be BLOCKED:
<block>yes</block>
<reason>Brief explanation of why this is blocked</reason>

If the action should be ALLOWED:
<block>no</block>

Your response should be concise and focused on the security decision.
Do NOT include lengthy explanations or reasoning in your response.
`

// ExternalPermissionsTemplate is the external-facing permissions template.
// This template includes user-customizable sections wrapped in <user_*_to_replace> tags.
// Users can override these sections via settings.autoMode configuration.
const ExternalPermissionsTemplate = `## Security Policy

### Blocked Actions
The following actions are BLOCKED by default:

- Commands that modify or delete files (Write, Edit, Delete, Move, Rename)
- Commands that execute external code or scripts (Bash, Run, Exec)
- Operations that could cause data loss or corruption
- Actions requiring elevated privileges (sudo, root)
- Network operations that could expose data

<user_allow_rules_to_replace>
- Safe: Read-only operations (Glob, Grep, LS, cat without modification)
- Safe: Internal state management (TodoWrite, Memory operations)
- Safe: Code analysis and testing (when explicitly authorized by user)
</user_allow_rules_to_replace>

### Denied Actions (Soft Blocks)
These actions may be allowed with explicit user confirmation:

<user_deny_rules_to_replace>
- PowerShell commands (requires explicit permission)
- Commands modifying git repositories (commit, push, branch)
- Environment variable modifications
- Package installations
</user_deny_rules_to_replace>

### Environment Context
Consider the following when making decisions:

<user_environment_to_replace>
- Actions should align with user's explicit instructions
- Consider the project's best interests
- Prefer read-only operations when possible
</user_environment_to_replace>

## Decision Process

1. Identify the category of the requested action
2. Check against blocked actions list
3. Check against denied actions list
4. Consider environment context
5. Make decision: BLOCK if uncertain

Remember: When uncertain, err on the side of blocking.`

// AnthropicPermissionsTemplate is the internal Anthropic-specific template.
// This is used for internal builds with specific guidelines.
const AnthropicPermissionsTemplate = `## Security Policy - Internal Build

### Primary Rule: Block Destructive Operations

All operations that modify system state, files, or execute external code are BLOCKED by default.

### Allowed Operations

- Read-only file operations
- Code analysis and search
- Safe development tasks

### Special Handling

- PowerShell: Always requires explicit user permission
- Git operations: Require explicit authorization
- Network requests: Blocked by default

<user_allow_rules_to_replace>
- Read operations: Glob, Grep, LS, Read
- Analysis: TODO operations
</user_allow_rules_to_replace>

<user_deny_rules_to_replace>
- Write, Edit, Delete operations
- Bash and shell commands
- Network operations
</user_deny_rules_to_replace>

<user_environment_to_replace>
</user_environment_to_replace>

## Classification Process

1. Categorize the action
2. Check block list first
3. Then check soft-deny list
4. Apply environment rules
5. Default to BLOCK if uncertain`

// =============================================================================
// TEMPLATE CONFIGURATION
// =============================================================================

// TemplateConfig holds configuration for which template to use.
type TemplateConfig struct {
	UseExternalTemplate  bool     // Use external vs internal template
	UseAnthropicTemplate bool     // Use Anthropic internal template
	AllowRules           []string // User-defined allow rules
	DenyRules            []string // User-defined deny rules
	EnvironmentRules     []string // Environment-specific rules
}

// DefaultTemplateConfig returns the default template configuration.
// By default, uses the external template (user-customizable).
func DefaultTemplateConfig() *TemplateConfig {
	return &TemplateConfig{
		UseExternalTemplate:  true,
		UseAnthropicTemplate: false,
		AllowRules:           []string{},
		DenyRules:            []string{},
		EnvironmentRules:     []string{},
	}
}

// =============================================================================
// TEMPLATE BUILDING
// =============================================================================

// BuildSystemPrompt builds the complete system prompt using the template configuration.
// It combines the base prompt with the appropriate permissions template and applies
// any user-defined rules. The function returns a complete system prompt string ready
// for the classifier.
//
// Parameters:
//   - config: Template configuration containing rules and template selection
//
// Returns: Complete system prompt string for the classifier
func BuildSystemPromptWithConfig(config *TemplateConfig) string {
	if config == nil {
		config = DefaultTemplateConfig()
	}

	// Select the base permissions template
	var permissionsTemplate string
	switch {
	case config.UseAnthropicTemplate:
		permissionsTemplate = AnthropicPermissionsTemplate
	case config.UseExternalTemplate:
		permissionsTemplate = ExternalPermissionsTemplate
	default:
		permissionsTemplate = ExternalPermissionsTemplate
	}

	// Start with the base system prompt
	prompt := strings.Replace(
		BaseSystemPrompt,
		"<permissions_template>",
		permissionsTemplate,
		1,
	)

	// Apply user-defined allow rules if provided
	if len(config.AllowRules) > 0 {
		allowRulesStr := formatRulesAsBulletList(config.AllowRules)
		prompt = applyUserRule(prompt, "user_allow_rules_to_replace", allowRulesStr)
	}

	// Apply user-defined deny rules if provided
	if len(config.DenyRules) > 0 {
		denyRulesStr := formatRulesAsBulletList(config.DenyRules)
		prompt = applyUserRule(prompt, "user_deny_rules_to_replace", denyRulesStr)
	}

	// Apply environment rules if provided
	if len(config.EnvironmentRules) > 0 {
		envRulesStr := formatRulesAsBulletList(config.EnvironmentRules)
		prompt = applyUserRule(prompt, "user_environment_to_replace", envRulesStr)
	}

	return prompt
}

// formatRulesAsBulletList formats a slice of rules as bullet-point text.
// Each rule becomes a line prefixed with "- " (markdown bullet format).
//
// Parameters:
//   - rules: Slice of rule strings
//
// Returns: Formatted string with bullet points
func formatRulesAsBulletList(rules []string) string {
	if len(rules) == 0 {
		return ""
	}

	var lines []string
	for _, rule := range rules {
		lines = append(lines, "- "+rule)
	}
	return strings.Join(lines, "\n")
}

// applyUserRule replaces a tagged section in the template with user-defined rules.
// This function finds sections like <user_allow_rules_to_replace> and replaces
// the default content with user-provided rules.
//
// Parameters:
//   - prompt: The prompt template string
//   - tagName: The tag to replace (e.g., "user_allow_rules_to_replace")
//   - userContent: The user-defined content to insert
//
// Returns: Updated prompt with user rules applied
func applyUserRule(prompt, tagName, userContent string) string {
	if userContent == "" {
		return prompt
	}

	// Find the tag pattern
	startTag := "<" + tagName + ">"
	endTag := "</" + tagName + ">"

	startIdx := strings.Index(prompt, startTag)
	if startIdx == -1 {
		return prompt
	}

	endIdx := strings.Index(prompt, endTag)
	if endIdx == -1 {
		return prompt
	}

	// Replace the content between tags
	before := prompt[:startIdx]
	after := prompt[endIdx+len(endTag):]

	return before + startTag + userContent + after
}

// =============================================================================
// EXTRACT DEFAULT RULES
// =============================================================================

// ExtractDefaultAllowRules extracts the default allow rules from the external template.
// This is used to show users the default rules and allow them to customize.
//
// Returns: Slice of default allow rule strings
func ExtractDefaultAllowRules() []string {
	return extractTaggedBullets(ExternalPermissionsTemplate, "user_allow_rules_to_replace")
}

// ExtractDefaultDenyRules extracts the default deny rules from the external template.
//
// Returns: Slice of default deny rule strings
func ExtractDefaultDenyRules() []string {
	return extractTaggedBullets(ExternalPermissionsTemplate, "user_deny_rules_to_replace")
}

// ExtractDefaultEnvironmentRules extracts the default environment rules.
//
// Returns: Slice of default environment rule strings
func ExtractDefaultEnvironmentRules() []string {
	return extractTaggedBullets(ExternalPermissionsTemplate, "user_environment_to_replace")
}

// extractTaggedBullets extracts bullet-point rules from a tagged section.
// This parses the template and extracts lines starting with "- " within
// the specified tag.
//
// Parameters:
//   - template: The template string to parse
//   - tagName: The tag containing the rules
//
// Returns: Slice of extracted rule strings
func extractTaggedBullets(template, tagName string) []string {
	startTag := "<" + tagName + ">"
	endTag := "</" + tagName + ">"

	startIdx := strings.Index(template, startTag)
	if startIdx == -1 {
		return []string{}
	}

	startIdx += len(startTag)

	endIdx := strings.Index(template, endTag)
	if endIdx == -1 || endIdx <= startIdx {
		return []string{}
	}

	content := strings.TrimSpace(template[startIdx:endIdx])

	var rules []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			rules = append(rules, strings.TrimPrefix(line, "- "))
		}
	}

	return rules
}

// GetExternalTemplate returns the external permissions template string.
// This is used for tools like "nexus auto-mode defaults" to show default rules.
func GetExternalTemplate() string {
	return ExternalPermissionsTemplate
}

// GetAnthropicTemplate returns the Anthropic internal template string.
func GetAnthropicTemplate() string {
	return AnthropicPermissionsTemplate
}

// GetBasePrompt returns the base system prompt.
func GetBasePrompt() string {
	return BaseSystemPrompt
}
