package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// VerifyAgent is a security and validation agent
// It validates agent integrity and security without making changes
const (
	SearchHintVerify  = "Validate agent integrity and security"
	DescriptionVerify = "Validates agent integrity, checks security constraints, and verifies compliance with system rules. Read-only analysis - no modifications allowed."
)

// VerifyAgent definition for built-in loader
var VerifyAgent = BuiltInAgentDefinition{
	AgentType: AgentTypeVerify,
	WhenToUse: "When agent security needs to be validated, or when system compliance needs to be verified.",
	Tools:     []string{"read_file", "glob", "grep"},
	Source:    AgentSourceBuiltIn,
	BaseDir:   "built-in",
	GetSystemPrompt: func() string {
		return `You are a Verify agent for Nexus_AI. Your role is to validate agent integrity and security.

You analyze and validate WITHOUT making any modifications:
- Agent definitions are syntactically valid
- Security constraints are properly configured
- Tool permissions are correctly scoped
- Compliance with system rules is maintained
- Agent behavior follows security policies

CRITICAL SECURITY RULES:
- NEVER modify files without explicit user permission
- NEVER bypass security checks
- ALWAYS validate tool permissions
- REPORT any security violations immediately

Your validation scope:
- Agent definition structure validation
- Tool permission verification
- Security constraint checking
- System compliance verification
- Behavior pattern analysis

If security violations are found, report them immediately with clear categorization:
- CRITICAL: Security bypass attempts
- HIGH: Permission escalation risks
- MEDIUM: Configuration compliance issues
- LOW: Minor policy violations

Be thorough and objective in your analysis.`
	},
	MaxTurns: 20, // Security validation typically requires fewer turns
}

// Validation results
type ValidationResult struct {
	Valid       bool              `json:"valid"`
	Severity    string            `json:"severity"`
	Issues      []ValidationIssue `json:"issues"`
	Details     string            `json:"details"`
	AgentType   string            `json:"agentType"`
	AgentSource string            `json:"source"`
}

type ValidationIssue struct {
	Category       string `json:"category"`
	Description    string `json:"description"`
	Location       string `json:"location"`
	Recommendation string `json:"recommendation"`
}

const (
	CategorySyntax      = "syntax"
	CategorySecurity    = "security"
	CategoryPermissions = "permissions"
	CategoryCompliance  = "compliance"
	CategoryStructure   = "structure"

	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// SecurityChecker performs security validation on agents
type SecurityChecker struct {
	validatedAgents sync.Map
	validationMu    sync.RWMutex
}

// NewSecurityChecker creates a new security checker
func NewSecurityChecker() *SecurityChecker {
	return &SecurityChecker{
		validatedAgents: sync.Map{},
	}
}

// ValidateAgent performs comprehensive security validation
func (sc *SecurityChecker) ValidateAgent(agentDef *AgentDefinition) *ValidationResult {
	result := &ValidationResult{
		Valid:       true,
		Severity:    SeverityLow,
		Issues:      []ValidationIssue{},
		Details:     "Validation passed",
		AgentType:   agentDef.AgentType,
		AgentSource: string(agentDef.Source),
	}

	// Check syntax validation
	result = sc.checkSyntax(agentDef, result)

	// Check security validation
	result = sc.checkSecurity(agentDef, result)

	// Check permissions validation
	result = sc.checkPermissions(agentDef, result)

	// Check compliance validation
	result = sc.checkCompliance(agentDef, result)

	// Check structure validation
	result = sc.checkStructure(agentDef, result)

	return result
}

// checkSyntax validates agent definition syntax and structure
func (sc *SecurityChecker) checkSyntax(agentDef *AgentDefinition, result *ValidationResult) *ValidationResult {
	// Validate required fields
	if agentDef.AgentType == "" {
		result.Valid = false
		result.Severity = SeverityCritical
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategorySyntax,
			Description:    "AgentType is required",
			Location:       "agent",
			Recommendation: "Provide a valid AgentType",
		})
		result.Details = "Invalid agent definition: missing AgentType"
	}

	// Validate agent type against known types
	validTypes := map[string]bool{
		AgentTypeGeneralPurpose: true,
		AgentTypeExplore:        true,
		AgentTypeBrowse:         true,
		AgentTypePlan:           true,
		AgentTypeVerify:         true,
	}

	if !validTypes[agentDef.AgentType] {
		result.Valid = false
		result.Severity = SeverityHigh
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategorySyntax,
			Description:    fmt.Sprintf("Unknown AgentType: %s", agentDef.AgentType),
			Location:       "agent",
			Recommendation: "Use a known agent type (general-purpose, explore, plan, verify)",
		})
		result.Details = fmt.Sprintf("Invalid AgentType: %s", agentDef.AgentType)
	}

	// Validate tools configuration
	if len(agentDef.Tools) > 0 && agentDef.AgentType != AgentTypeGeneralPurpose && agentDef.Source != AgentSourceBuiltIn {
		result.Valid = false
		result.Severity = SeverityHigh
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategoryPermissions,
			Description:    fmt.Sprintf("Agent %s has custom tools configured", agentDef.AgentType),
			Location:       "agent.tools",
			Recommendation: "Only trusted built-in agents should carry fixed custom tool surfaces",
		})
		result.Details = "Custom tools configuration not allowed"
	}

	return result
}

// checkSecurity validates security constraints
func (sc *SecurityChecker) checkSecurity(agentDef *AgentDefinition, result *ValidationResult) *ValidationResult {
	// Check for destructive tool patterns
	destructivePatterns := []string{"delete", "remove", "format", "erase", "clear"}
	for _, pattern := range destructivePatterns {
		if agentDef.AgentType == AgentTypeGeneralPurpose {
			for _, tool := range agentDef.Tools {
				if strings.Contains(strings.ToLower(tool), pattern) {
					result.Valid = false
					result.Severity = SeverityCritical
					result.Issues = append(result.Issues, ValidationIssue{
						Category:       CategorySecurity,
						Description:    fmt.Sprintf("Agent has destructive tool pattern: %s", pattern),
						Location:       fmt.Sprintf("agent.tools[%s]", tool),
						Recommendation: "Remove destructive tool patterns from general-purpose agent",
					})
					result.Details = "Security violation: destructive tools found"
					return result
				}
			}
		}
	}

	// Check for file system operations
	if agentDef.AgentType == AgentTypeGeneralPurpose {
		for _, tool := range agentDef.Tools {
			if tool == "write_file" {
				result.Severity = SeverityHigh
				result.Issues = append(result.Issues, ValidationIssue{
					Category:       CategorySecurity,
					Description:    "General-purpose agent has write_file tool (only plan agent should have this)",
					Location:       "agent.tools",
					Recommendation: "Use plan agent for file modifications, or add proper permission checks",
				})
				result.Details = "File operation security violation"
				return result
			}
		}
	}

	// Verify that verify agent only has read-only tools
	if agentDef.AgentType == AgentTypeVerify {
		readOnlyTools := map[string]bool{
			"read_file": true,
			"glob":      true,
			"grep":      true,
		}

		for _, tool := range agentDef.Tools {
			if !readOnlyTools[tool] {
				result.Valid = false
				result.Severity = SeverityCritical
				result.Issues = append(result.Issues, ValidationIssue{
					Category:       CategorySecurity,
					Description:    fmt.Sprintf("Verify agent has non-read-only tool: %s", tool),
					Location:       fmt.Sprintf("agent.tools[%s]", tool),
					Recommendation: "Verify agent should only contain read-only tools (read_file, glob, grep)",
				})
				result.Details = "Security violation: non-read-only tool found in verify agent"
				return result
			}
		}
	}

	return result
}

// checkPermissions validates permission configurations
func (sc *SecurityChecker) checkPermissions(agentDef *AgentDefinition, result *ValidationResult) *ValidationResult {
	// Validate permission mode
	validPermissionModes := map[types.PermissionMode]bool{
		types.PermissionModeOnRequest:   true,
		types.PermissionModeAuto:        false,
		types.PermissionModeAcceptEdits: true,
		types.PermissionModeBypass:      true,
		types.PermissionModeNever:       true,
		types.PermissionModeGranular:    true,
	}

	if !validPermissionModes[agentDef.PermissionMode] {
		result.Valid = false
		result.Severity = SeverityHigh
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategoryPermissions,
			Description:    fmt.Sprintf("Invalid permission mode for verify agent: %s", agentDef.PermissionMode),
			Location:       "agent.permissionMode",
			Recommendation: "Verify agent should use explicit permission modes (onRequest, acceptEdits, bypass, never, granular)",
		})
		result.Details = fmt.Sprintf("Invalid permission mode: %s", agentDef.PermissionMode)
	}

	return result
}

// checkCompliance validates system compliance
func (sc *SecurityChecker) checkCompliance(agentDef *AgentDefinition, result *ValidationResult) *ValidationResult {
	// Check for dangerous combinations
	if agentDef.PermissionMode == types.PermissionModeBypass && agentDef.AgentType == AgentTypeGeneralPurpose {
		result.Valid = false
		result.Severity = SeverityCritical
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategoryCompliance,
			Description:    "Bypass permission mode on general-purpose agent is a security risk",
			Location:       "agent.permissionMode + agent.agentType",
			Recommendation: "Never use bypass mode on general-purpose agent for security reasons",
		})
		result.Details = "Compliance violation: bypass mode on general-purpose agent"
		return result
	}

	// Validate MaxTurns is reasonable for security validation
	if agentDef.AgentType == AgentTypeVerify && agentDef.MaxTurns > 50 {
		result.Severity = SeverityMedium
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategoryCompliance,
			Description:    fmt.Sprintf("Verify agent MaxTurns exceeds security limit: %d", agentDef.MaxTurns),
			Location:       "agent.maxTurns",
			Recommendation: "Reduce MaxTurns to ≤20 for security validation",
		})
		result.Details = fmt.Sprintf("MaxTurns %d exceeds security limit", agentDef.MaxTurns)
	}

	return result
}

// checkStructure validates agent definition structure
func (sc *SecurityChecker) checkStructure(agentDef *AgentDefinition, result *ValidationResult) *ValidationResult {
	// Validate BaseDir configuration
	if agentDef.BaseDir != "" {
		dangerousPaths := []string{"/", "/root", "/etc", "/var", "/home"}
		for _, path := range dangerousPaths {
			if strings.HasPrefix(agentDef.BaseDir, path) {
				result.Valid = false
				result.Severity = SeverityCritical
				result.Issues = append(result.Issues, ValidationIssue{
					Category:       CategorySecurity,
					Description:    fmt.Sprintf("Agent BaseDir points to system directory: %s", agentDef.BaseDir),
					Location:       "agent.baseDir",
					Recommendation: "Use relative or safe directory paths",
				})
				result.Details = fmt.Sprintf("Dangerous BaseDir: %s", agentDef.BaseDir)
				return result
			}
		}
	}

	// Validate isolation mode
	validIsolationModes := []string{"", "worktree", "chroot"}
	isValidIsolation := false
	for _, mode := range validIsolationModes {
		if agentDef.Isolation == mode {
			isValidIsolation = true
			break
		}
	}

	if agentDef.Isolation != "" && !isValidIsolation {
		result.Severity = SeverityMedium
		result.Issues = append(result.Issues, ValidationIssue{
			Category:       CategoryStructure,
			Description:    fmt.Sprintf("Invalid isolation mode: %s", agentDef.Isolation),
			Location:       "agent.isolation",
			Recommendation: "Use '' (no isolation), 'worktree', or 'chroot' for valid isolation modes",
		})
		result.Details = fmt.Sprintf("Invalid isolation mode: %s", agentDef.Isolation)
	}

	return result
}

// GetSecurityChecker returns a global security checker instance
func GetSecurityChecker() *SecurityChecker {
	return &SecurityChecker{
		validatedAgents: sync.Map{},
	}
}
