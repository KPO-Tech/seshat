package permissions

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	auto "github.com/EngineerProjects/nexus-engine/internal/permissions/auto"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/internal/utils"
)

// Engine checks permissions for tool usage.
type Engine struct {
	// rules are the permission rules.
	rules []PermissionRule

	// classifier is the optional classifier for auto-mode.
	classifier Classifier

	// advancedClassifier is the optional transcript-aware classifier for auto-mode.
	advancedClassifier auto.AdvancedClassifierInterface

	// autoMode is the auto mode handler (uses permissions/auto package)
	autoMode *auto.Mode

	// hooks are pre/post permission check hooks.
	hooks []Hook

	// FailClosed controls classifier unavailability behavior.
	// When true (default), classifier errors result in deny (fail-closed).
	// When false, classifier errors fall back to ask (fail-open).
	FailClosed bool
}

// PermissionRuleValue is the structured value of a permission rule.
type PermissionRuleValue struct {
	// ToolName identifies the tool this rule targets.
	ToolName string `json:"tool_name"`

	// RuleContent carries optional content-specific matching handled by the tool.
	RuleContent string `json:"rule_content,omitempty"`
}

// PermissionRule represents a permission rule.
type PermissionRule struct {
	// Value is the structured rule target.
	Value PermissionRuleValue `json:"value"`

	// Pattern is the legacy serialized form, kept for compatibility.
	Pattern string `json:"pattern,omitempty"`

	// Behavior is the allow/deny/ask decision.
	Behavior types.PermissionBehavior `json:"behavior"`

	// Priority determines rule precedence (higher = more important).
	Priority int `json:"priority"`

	// Reason explains why this rule exists.
	Reason string `json:"reason,omitempty"`

	// Source indicates where this rule came from.
	Source types.PermissionRuleSource `json:"source"`
}

// Classifier predicts permission decisions.
type Classifier interface {
	// Classify predicts if a tool use should be allowed.
	Classify(ctx context.Context, toolName string, input map[string]any) (Classification, error)
}

// Classification represents a classifier's prediction.
type Classification struct {
	// Allowed is true if the classifier predicts the action is safe.
	Allowed bool `json:"allowed"`

	// Confidence is 0-1.
	Confidence float64 `json:"confidence"`

	// Reason explains the classification.
	Reason string `json:"reason,omitempty"`
}

// Hook is called before or after permission checks.
type Hook struct {
	// Stage is when this hook is called.
	Stage types.HookStage `json:"stage"`

	// Handler is the hook function.
	Handler HookHandler `json:"-"`

	// Priority determines order (higher = earlier).
	Priority int `json:"priority"`

	// ID uniquely identifies this hook.
	ID string `json:"id"`
}

// HookHandler is a function that handles a hook.
type HookHandler func(ctx context.Context, pctx *PermissionContext) (types.PermissionResult, error)

// PermissionContext provides context for permission checking.
type PermissionContext struct {
	// Mode is the current permission mode.
	Mode types.PermissionMode `json:"mode"`

	// ExecutionMode is the current execution mode.
	ExecutionMode string `json:"execution_mode,omitempty"`

	// ToolName is the tool being called.
	ToolName string `json:"tool_name"`

	// ToolInput is the input to the tool.
	ToolInput map[string]any `json:"tool_input"`

	// SessionID identifies the session.
	SessionID types.SessionID `json:"session_id"`

	// TurnID identifies the turn.
	TurnID types.TurnID `json:"turn_id"`

	// ToolUseID identifies the specific tool use when available.
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Stage identifies which permission stage is being resolved.
	Stage types.ToolPermissionStage `json:"stage,omitempty"`

	// Intent identifies the decision kind requested for this stage.
	Intent types.ToolPermissionIntent `json:"intent,omitempty"`

	// IsConcurrent indicates if this tool will run concurrently.
	IsConcurrent bool `json:"is_concurrent"`

	// ToolDefinition is the tool's definition (if available).
	ToolDefinition *tool.Definition `json:"tool_definition,omitempty"`

	// Tool provides access to optional content-specific permission matching.
	Tool tool.Tool `json:"-"`

	// ShouldAvoidPermissionPrompts is true for headless/background agents
	// that cannot display UI prompts. When true, ask decisions are
	// automatically denied with an asyncAgent decision reason.
	ShouldAvoidPermissionPrompts bool `json:"should_avoid_permission_prompts,omitempty"`

	// IsBypassPermissionsModeAvailable indicates whether bypass mode was
	// available at the start of the session. Used by plan mode to determine
	// whether it should behave like bypass mode or ask for permissions.
	IsBypassPermissionsModeAvailable bool `json:"is_bypass_permissions_mode_available,omitempty"`

	// IsToolRunningInSandbox indicates whether the tool will run inside a sandbox.
	// This is used by canSandboxAutoAllow to determine if a command can be auto-approved.
	IsToolRunningInSandbox bool `json:"is_tool_running_in_sandbox,omitempty"`

	// Additional context.
	Additional map[string]any `json:"additional,omitempty"`
}

// NewEngine creates a new permission engine.
func NewEngine() *Engine {
	return &Engine{
		rules:      make([]PermissionRule, 0),
		hooks:      make([]Hook, 0),
		FailClosed: true,
	}
}

// SetClassifier sets the classifier and initializes autoMode.
func (e *Engine) SetClassifier(classifier Classifier) {
	e.classifier = classifier
	e.ensureAutoMode()
}

// SetAdvancedClassifier wires a transcript-aware classifier into auto mode.
func (e *Engine) SetAdvancedClassifier(classifier auto.AdvancedClassifierInterface) {
	e.advancedClassifier = classifier
	e.ensureAutoMode()
}

// IsAutoModeAvailable returns whether auto mode has a working classifier path configured.
func (e *Engine) IsAutoModeAvailable() bool {
	return e != nil && e.autoMode != nil && (e.classifier != nil || e.advancedClassifier != nil)
}

func (e *Engine) ensureAutoMode() {
	var autoClassifier auto.Classifier
	if e.classifier != nil {
		autoClassifier = &classifierAdapter{classifier: e.classifier}
	}
	if e.autoMode == nil {
		e.autoMode = auto.NewMode(autoClassifier, &auto.ModeConfig{
			FailClosed: e.FailClosed,
		})
	} else {
		e.autoMode.SetClassifier(autoClassifier)
	}
	if e.advancedClassifier != nil {
		e.autoMode.SetAdvancedClassifier(e.advancedClassifier)
	}
}

// classifierAdapter wraps permissions.Classifier to implement auto.Classifier
type classifierAdapter struct {
	classifier Classifier
}

func (a *classifierAdapter) Classify(ctx context.Context, toolName string, input map[string]any) (auto.Classification, error) {
	classification, err := a.classifier.Classify(ctx, toolName, input)
	if err != nil {
		return auto.Classification{}, err
	}
	return auto.Classification{
		Allowed:    classification.Allowed,
		Confidence: classification.Confidence,
		Reason:     classification.Reason,
	}, nil
}

// AddRule adds a permission rule.
func (e *Engine) AddRule(rule PermissionRule) error {
	rule = normalizePermissionRule(rule)
	if err := validatePermissionRule(rule); err != nil {
		return err
	}

	e.rules = append(e.rules, rule)
	e.sortRules()
	return nil
}

// AddRules adds multiple permission rules.
func (e *Engine) AddRules(rules []PermissionRule) error {
	for _, rule := range rules {
		if err := e.AddRule(rule); err != nil {
			return err
		}
	}
	return nil
}

// AddHook adds a permission hook.
func (e *Engine) AddHook(hook Hook) {
	e.hooks = append(e.hooks, hook)
	e.sortHooks()
}

// CheckPermission checks if a tool can be used.
func (e *Engine) CheckPermission(ctx context.Context, pctx *PermissionContext) (types.PermissionResult, error) {
	result, stop, err := e.runHooks(ctx, pctx, types.HookStagePre)
	if err != nil {
		return types.PermissionResult{}, err
	}
	if stop {
		return result, nil
	}

	switch pctx.Stage {
	case types.ToolPermissionStageWholeTool:
		return e.checkWholeToolPermission(pctx), nil
	case "", types.ToolPermissionStageGlobal:
		return e.checkGlobalPermission(ctx, pctx)
	default:
		return types.AskWithDecisionReason("unsupported permission stage", &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "engine",
			Reason: string(pctx.Stage),
		}), nil
	}
}

func (e *Engine) checkWholeToolPermission(pctx *PermissionContext) types.PermissionResult {
	// Support for explicit intents (used by the orchestrator)
	switch pctx.Intent {
	case types.ToolPermissionIntentDeny:
		if rule := e.findWholeToolRule(pctx, types.PermissionBehaviorDeny); rule != nil {
			return permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
		}
		return types.PassthroughWithDecisionReason(utils.CloneInput(pctx.ToolInput), &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "engine",
			Reason: "no whole-tool deny rule matched",
		})

	case types.ToolPermissionIntentAsk:
		// Check sandbox auto-allow first (aligned with OpenClaude step 1b)
		if !canSandboxAutoAllow(pctx) {
			if rule := e.findWholeToolRule(pctx, types.PermissionBehaviorAsk); rule != nil {
				return permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
			}
		}
		return types.PassthroughWithDecisionReason(utils.CloneInput(pctx.ToolInput), &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "engine",
			Reason: "no whole-tool ask rule matched",
		})

	case types.ToolPermissionIntentAllow:
		if rule := e.findWholeToolRule(pctx, types.PermissionBehaviorAllow); rule != nil {
			return permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
		}
		return types.PassthroughWithDecisionReason(utils.CloneInput(pctx.ToolInput), &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "engine",
			Reason: "no whole-tool allow rule matched",
		})

	default:
		return types.Passthrough(utils.CloneInput(pctx.ToolInput))
	}
}

func (e *Engine) checkGlobalPermission(ctx context.Context, pctx *PermissionContext) (types.PermissionResult, error) {
	// Step 2a: Bypass mode (including plan mode with available bypass)
	// Aligned with OpenClaude (permissions.ts:1268-1281)
	shouldBypass := pctx.Mode == types.PermissionModeBypass ||
		(pctx.ExecutionMode == "plan" && pctx.IsBypassPermissionsModeAvailable)

	if shouldBypass {
		// In bypass mode, we respect ONLY the rules (not the classifier)
		// Aligned with OpenClaude's checkRuleBasedPermissions() usage
		ruleResult, err := e.checkRuleBasedPermissions(ctx, pctx)
		if err != nil {
			return types.PermissionResult{}, err
		}

		// If no rule-based objection, allow
		if ruleResult.Behavior == types.PermissionBehaviorPassthrough {
			result := types.AllowWithInputAndDecisionReason("bypass mode", utils.CloneInput(pctx.ToolInput), &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonMode,
				Source: string(pctx.Mode),
				Reason: "bypass mode enabled",
			})
			// Run post-permission hooks
			if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
				return types.PermissionResult{}, err
			}
			return result, nil
		}

		// If rule-based objection, respect it
		if err := e.RunPostPermissionHooks(ctx, pctx, &ruleResult); err != nil {
			return types.PermissionResult{}, err
		}
		return ruleResult, nil
	}

	// Step 2b: Whole-tool always-allowed rule (check after deny rules)
	// Aligned with OpenClaude (permissions.ts:1287-1294)
	// NOTE: In OpenClaude, deny rules always take precedence over allow rules,
	// even whole-tool allow rules. So we check deny first, then whole-tool allow.

	// Step 3: Rule-based checks (deny → whole-tool-allow → allow → ask)
	// Aligned with OpenClaude (permissions.ts:1297-1313)
	if rule := e.findGlobalRule(ctx, pctx, types.PermissionBehaviorDeny); rule != nil {
		result := permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	// Whole-tool allow rules (only if no deny rule matched)
	if rule := e.findWholeToolRule(pctx, types.PermissionBehaviorAllow); rule != nil {
		result := permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	if rule := e.findGlobalRule(ctx, pctx, types.PermissionBehaviorAllow); rule != nil {
		result := permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}
	if rule := e.findGlobalRule(ctx, pctx, types.PermissionBehaviorAsk); rule != nil {
		result := permissionResultFromRule(*rule, utils.CloneInput(pctx.ToolInput))
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	// Step 3.5: Heuristic auto-allow for always-safe tools.
	// Read-only and network-read tools are auto-approved in all interactive modes
	// so that grep, web_search, tree, etc. never trigger a permission card.
	if pctx.Mode != types.PermissionModeNever && isAlwaysSafeTool(pctx.ToolName) {
		result := heuristicAllowResult(pctx.ToolName, "always-safe tool", pctx.ToolInput)
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	// Step 4: Auto mode classifier
	// Aligned with OpenClaude (permissions.ts:1316-1319)
	if pctx.Mode == types.PermissionModeAuto && e.autoMode != nil {
		autoCtx := &auto.ClassifierContext{
			ToolName:                     pctx.ToolName,
			ToolInput:                    pctx.ToolInput,
			Mode:                         pctx.Mode,
			SessionID:                    pctx.SessionID,
			TurnID:                       pctx.TurnID,
			ToolUseID:                    pctx.ToolUseID,
			ShouldAvoidPermissionPrompts: pctx.ShouldAvoidPermissionPrompts,
			Additional:                   pctx.Additional,
		}
		if dt, ok := pctx.Additional["denialTracking"].(*types.DenialTrackingState); ok {
			autoCtx.DenialTracking = dt
		}
		if messages, ok := pctx.Additional["transcript_messages"].([]types.Message); ok {
			autoCtx.Messages = append([]types.Message(nil), messages...)
		}
		result, err := e.autoMode.Classify(ctx, autoCtx)
		if err != nil {
			return types.PermissionResult{}, err
		}

		// Track classifier transcript for debug — not yet implemented.

		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	// Step 4.5: Auto mode fallback (no ML classifier configured).
	// Without a classifier, auto mode approves all tool calls — safe workflow
	// tools (agent, task_*, etc.) and all others including destructive tools.
	// This matches the user expectation: orange shield = "just run, don't ask".
	if pctx.Mode == types.PermissionModeAuto {
		result := heuristicAllowResult(pctx.ToolName, "auto mode (no classifier)", pctx.ToolInput)
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	// Step 4.6: onRequest mode — additionally auto-allow safe workflow tools
	// (agent, task management, plan mode) so they don't interrupt the flow.
	if pctx.Mode == types.PermissionModeOnRequest && isSafeWorkflowTool(pctx.ToolName) {
		result := heuristicAllowResult(pctx.ToolName, "safe workflow tool in onRequest mode", pctx.ToolInput)
		if err := e.RunPostPermissionHooks(ctx, pctx, &result); err != nil {
			return types.PermissionResult{}, err
		}
		return result, nil
	}

	// Step 5: Default ask
	// Aligned with OpenClaude (permissions.ts:1322-1332)
	result := types.AskWithDecisionReason("no matching permission rule", &types.PermissionDecisionReason{
		Type:   types.PermissionDecisionReasonOther,
		Source: "engine",
		Reason: "no matching permission rule",
	})

	// Step 6: Mode transformations (dontAsk, headless)
	transformed, err := e.applyModeTransformations(pctx, result)
	if err != nil {
		return types.PermissionResult{}, err
	}
	if err := e.RunPostPermissionHooks(ctx, pctx, &transformed); err != nil {
		return types.PermissionResult{}, err
	}
	return transformed, nil
}

// checkRuleBasedPermissions executes the rule-based permission pipeline ONLY
// (no classifier, no mode transformations). Used by bypass mode to respect
// user-authored rules while skipping the classifier.
// Aligned with OpenClaude's checkRuleBasedPermissions() (permissions.ts:1071-1156).
func (e *Engine) checkRuleBasedPermissions(
	ctx context.Context,
	pctx *PermissionContext,
) (types.PermissionResult, error) {
	// 1a. Whole-tool deny rule
	denyRule := e.findWholeToolRule(pctx, types.PermissionBehaviorDeny)
	if denyRule != nil {
		return permissionResultFromRule(*denyRule, utils.CloneInput(pctx.ToolInput)), nil
	}

	// 1b. Whole-tool ask rule (with sandbox auto-allow)
	askRule := e.findWholeToolRule(pctx, types.PermissionBehaviorAsk)
	if askRule != nil {
		if canSandboxAutoAllow(pctx) {
			// Fall through to tool-specific check
		} else {
			return permissionResultFromRule(*askRule, utils.CloneInput(pctx.ToolInput)), nil
		}
	}

	// 1c. Tool-specific permission check
	var toolPermissionResult types.PermissionResult
	if pctx.Tool != nil {
		workingDirectory, _ := pctx.Additional["working_directory"].(string)
		toolCtx := tool.ToolUseContext{
			SessionID:        pctx.SessionID,
			TurnID:           pctx.TurnID,
			ToolUseID:        pctx.ToolUseID,
			PermissionMode:   pctx.Mode,
			WorkingDirectory: workingDirectory,
			Metadata:         pctx.Additional,
		}
		toolPermissionResult = ensurePermissionResult(
			pctx.Tool.CheckPermissions(ctx, utils.CloneInput(pctx.ToolInput), toolCtx),
			"tool",
			utils.CloneInput(pctx.ToolInput),
		)
	} else {
		toolPermissionResult = types.Passthrough(utils.CloneInput(pctx.ToolInput))
	}

	// 1d. Tool implementation denied
	if toolPermissionResult.Behavior == types.PermissionBehaviorDeny {
		return toolPermissionResult, nil
	}

	// 1f. Content-specific ask rules from tool.checkPermissions
	// (type:'rule', ruleBehavior:'ask') - these are bypass-immune
	if toolPermissionResult.Behavior == types.PermissionBehaviorAsk &&
		toolPermissionResult.DecisionReason != nil &&
		toolPermissionResult.DecisionReason.Type == types.PermissionDecisionReasonRule &&
		toolPermissionResult.DecisionReason.RuleBehavior == types.PermissionBehaviorAsk {
		return toolPermissionResult, nil
	}

	// 1g. Safety checks (type:'safetyCheck', non-classifier-approvable) - bypass-immune
	if toolPermissionResult.Behavior == types.PermissionBehaviorAsk &&
		toolPermissionResult.DecisionReason != nil &&
		toolPermissionResult.DecisionReason.Type == types.PermissionDecisionReasonSafetyCheck &&
		!toolPermissionResult.DecisionReason.ClassifierApprovable {
		return toolPermissionResult, nil
	}

	// No rule-based objection
	return types.Passthrough(utils.CloneInput(pctx.ToolInput)), nil
}

func (e *Engine) findWholeToolRule(pctx *PermissionContext, behavior types.PermissionBehavior) *PermissionRule {
	for _, rule := range e.rules {
		if rule.Behavior != behavior {
			continue
		}
		if strings.TrimSpace(rule.Value.RuleContent) != "" {
			continue
		}
		if ruleMatchesToolName(rule, pctx.ToolName) {
			matched := rule
			return &matched
		}
	}
	return nil
}

func (e *Engine) findGlobalRule(ctx context.Context, pctx *PermissionContext, behavior types.PermissionBehavior) *PermissionRule {
	matcher, err := prepareRuleContentMatcher(ctx, pctx)
	if err != nil {
		return nil
	}

	for _, rule := range e.rules {
		if rule.Behavior != behavior {
			continue
		}
		if !ruleMatchesToolName(rule, pctx.ToolName) {
			continue
		}
		if strings.TrimSpace(rule.Value.RuleContent) == "" {
			matched := rule
			return &matched
		}
		if matcher != nil && matcher(rule.Value.RuleContent) {
			matched := rule
			return &matched
		}
	}
	return nil
}

func prepareRuleContentMatcher(ctx context.Context, pctx *PermissionContext) (func(ruleContent string) bool, error) {
	if pctx.Tool != nil {
		if matcherTool, ok := pctx.Tool.(tool.PermissionMatcherTool); ok {
			matcher, err := matcherTool.PreparePermissionMatcher(ctx, utils.CloneInput(pctx.ToolInput))
			if err != nil || matcher != nil {
				return matcher, err
			}
		}
	}
	if matcher := tool.PermissionMatcherFromMetadata(pctx.Additional); matcher != nil {
		return matcher, nil
	}
	return defaultPermissionMatcher(pctx.ToolInput), nil
}

// summarizePermissionInput extracts a summary string from tool input for permission matching.
func summarizePermissionInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "url", "pattern", "query"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultPermissionMatcher(input map[string]any) func(ruleContent string) bool {
	summary := summarizePermissionInput(input)
	if summary == "" {
		return nil
	}
	return func(ruleContent string) bool {
		return permissionRuleContentMatches(ruleContent, summary)
	}
}

func permissionRuleContentMatches(ruleContent string, candidate string) bool {
	ruleContent = strings.TrimSpace(ruleContent)
	candidate = strings.TrimSpace(candidate)
	if ruleContent == "" || candidate == "" {
		return false
	}

	if prefix, ok := legacyPrefixRule(ruleContent); ok {
		return candidate == prefix || strings.HasPrefix(candidate, prefix+" ")
	}
	if hasWildcard(ruleContent) {
		return matchWildcardPattern(ruleContent, candidate)
	}
	return candidate == ruleContent
}

func legacyPrefixRule(ruleContent string) (string, bool) {
	if strings.HasSuffix(ruleContent, ":*") && len(ruleContent) > 2 {
		return strings.TrimSuffix(ruleContent, ":*"), true
	}
	return "", false
}

func hasWildcard(pattern string) bool {
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

func matchWildcardPattern(pattern string, candidate string) bool {
	pattern = strings.TrimSpace(pattern)
	candidate = strings.TrimSpace(candidate)

	var builder strings.Builder
	builder.WriteString("^")
	escaped := false
	for _, r := range pattern {
		switch {
		case escaped:
			builder.WriteString(regexp.QuoteMeta(string(r)))
			escaped = false
		case r == '\\':
			escaped = true
		case r == '*':
			builder.WriteString(".*")
		default:
			builder.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if escaped {
		builder.WriteString(regexp.QuoteMeta("\\"))
	}
	builder.WriteString("$")

	re, err := regexp.Compile(builder.String())
	if err != nil {
		return false
	}
	return re.MatchString(candidate)
}

func ruleMatchesToolName(rule PermissionRule, toolName string) bool {
	return normalizeToolName(rule.Value.ToolName) == normalizeToolName(toolName)
}

func normalizeToolName(name string) string {
	return strings.TrimSpace(name)
}

func permissionResultFromRule(rule PermissionRule, input map[string]any) types.PermissionResult {
	decisionReason := &types.PermissionDecisionReason{
		Type:   types.PermissionDecisionReasonRule,
		Source: string(rule.Source),
		Reason: rule.Reason,
	}

	switch rule.Behavior {
	case types.PermissionBehaviorAllow:
		return types.AllowWithInputAndDecisionReason(rule.Reason, input, decisionReason)
	case types.PermissionBehaviorAsk:
		return types.AskWithDecisionReason(rule.Reason, decisionReason)
	case types.PermissionBehaviorDeny:
		return types.DenyWithDecisionReason(rule.Reason, decisionReason)
	default:
		return types.PassthroughWithDecisionReason(input, decisionReason)
	}
}

func normalizePermissionRule(rule PermissionRule) PermissionRule {
	if strings.TrimSpace(rule.Value.ToolName) == "" && strings.TrimSpace(rule.Pattern) != "" {
		rule.Value = parseLegacyPattern(rule.Pattern)
	}
	if strings.TrimSpace(rule.Pattern) == "" {
		rule.Pattern = legacyPatternFromValue(rule.Value)
	}
	return rule
}

func validatePermissionRule(rule PermissionRule) error {
	if strings.TrimSpace(rule.Value.ToolName) == "" {
		return fmt.Errorf("permission rule tool name cannot be empty")
	}
	if rule.Behavior == "" {
		return fmt.Errorf("permission rule behavior cannot be empty")
	}
	return nil
}

func parseLegacyPattern(pattern string) PermissionRuleValue {
	pattern = strings.TrimSpace(pattern)
	if !strings.HasPrefix(pattern, "tool:") {
		return PermissionRuleValue{ToolName: pattern}
	}

	body := strings.TrimPrefix(pattern, "tool:")
	parts := strings.SplitN(body, ":", 2)
	value := PermissionRuleValue{ToolName: strings.TrimSpace(parts[0])}
	if len(parts) == 2 {
		ruleContent := strings.TrimSpace(parts[1])
		if ruleContent != "" && ruleContent != "*" {
			value.RuleContent = ruleContent
		}
	}
	return value
}

func legacyPatternFromValue(value PermissionRuleValue) string {
	if strings.TrimSpace(value.RuleContent) == "" {
		return fmt.Sprintf("tool:%s", strings.TrimSpace(value.ToolName))
	}
	return fmt.Sprintf("tool:%s:%s", strings.TrimSpace(value.ToolName), strings.TrimSpace(value.RuleContent))
}

func clonePermissionResult(result types.PermissionResult) types.PermissionResult {
	cloned := result
	if result.UpdatedInput != nil {
		cloned.UpdatedInput = utils.CloneInput(result.UpdatedInput)
	}
	if result.Metadata != nil {
		cloned.Metadata = cloneMetadata(result.Metadata)
	}
	if result.DecisionReason != nil {
		decisionReason := *result.DecisionReason
		cloned.DecisionReason = &decisionReason
	}
	return cloned
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

// Auto mode classification is now handled by the permissions/auto package.
// This function is deprecated and kept for backward compatibility reference.
// See internal/permissions/auto/mode.go for the current implementation.

// runHooks runs hooks for a specific stage.
func (e *Engine) runHooks(ctx context.Context, pctx *PermissionContext, stage types.HookStage) (types.PermissionResult, bool, error) {
	for _, hook := range e.hooks {
		if hook.Stage != stage {
			continue
		}

		result, err := hook.Handler(ctx, pctx)
		if err != nil {
			return types.PermissionResult{}, false, err
		}

		if result.Behavior == types.PermissionBehaviorAllow || result.Behavior == types.PermissionBehaviorDeny {
			return result, true, nil
		}
	}

	return types.PermissionResult{}, false, nil
}

// sortRules sorts rules by priority (descending).
func (e *Engine) sortRules() {
	sort.SliceStable(e.rules, func(i, j int) bool {
		return e.rules[i].Priority > e.rules[j].Priority
	})
}

// sortHooks sorts hooks by priority (descending).
func (e *Engine) sortHooks() {
	sort.SliceStable(e.hooks, func(i, j int) bool {
		return e.hooks[i].Priority > e.hooks[j].Priority
	})
}

// canSandboxAutoAllow checks if a tool can be auto-allowed via sandbox.
// When sandbox is enabled and a command is running inside the sandbox, it can skip
// the "ask" permission rule and flow through to tool-specific checks.
// This is aligned with OpenClaude's sandbox auto-allow logic.
func canSandboxAutoAllow(pctx *PermissionContext) bool {
	// Only bash tool supports sandbox auto-allow
	// (other tools don't have sandbox execution context)
	if pctx.ToolName != "bash" && pctx.ToolName != "powershell" {
		return false
	}

	// Check if the command will run in sandbox
	// First check the direct field
	if pctx.IsToolRunningInSandbox {
		return true
	}

	// Then check Additional metadata (set by execution context)
	if pctx.Additional != nil {
		if isSandboxed, ok := pctx.Additional["is_tool_running_in_sandbox"].(bool); ok && isSandboxed {
			return true
		}
	}

	return false
}

// ensurePermissionResult ensures a PermissionResult has all required fields set.
// If behavior is allow/passthrough and UpdatedInput is nil, it's set to the input.
// If DecisionReason is nil, it's populated with appropriate defaults.
func ensurePermissionResult(result types.PermissionResult, source string, input map[string]any) types.PermissionResult {
	if result.Behavior == "" {
		result.Behavior = types.PermissionBehaviorAllow
	}
	if (result.Behavior == types.PermissionBehaviorAllow || result.Behavior == types.PermissionBehaviorPassthrough) && result.UpdatedInput == nil {
		result.UpdatedInput = utils.CloneInput(input)
	}
	if result.DecisionReason == nil {
		reasonType := types.PermissionDecisionReasonOther
		if source == "local" {
			reasonType = types.PermissionDecisionReasonTool
		}
		result.DecisionReason = &types.PermissionDecisionReason{
			Type:   reasonType,
			Source: source,
			Reason: result.Reason,
		}
	}
	return clonePermissionResult(result)
}

// applyModeTransformations applies mode-based transformations to permission results.
// Handles dontAsk mode, granular mode, and headless auto-deny.
func (e *Engine) applyModeTransformations(pctx *PermissionContext, result types.PermissionResult) (types.PermissionResult, error) {
	// dontAsk mode: transform ask → deny
	if result.Behavior == types.PermissionBehaviorAsk && pctx.Mode == types.PermissionModeNever {
		return types.DenyWithDecisionReason(
			fmt.Sprintf("permission to use %s denied: running in dontAsk mode", pctx.ToolName),
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonMode,
				Source: "dontAsk",
				Reason: "dontAsk mode enabled",
			},
		), nil
	}

	// Headless auto-deny: when permission prompts should be avoided
	if result.Behavior == types.PermissionBehaviorAsk && pctx.ShouldAvoidPermissionPrompts {
		return types.DenyWithDecisionReason(
			fmt.Sprintf("permission to use %s denied: permission prompts not available in this context", pctx.ToolName),
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonAsyncAgent,
				Source: "headless",
				Reason: "permission prompts are not available in this context",
			},
		), nil
	}

	// Granular mode: apply fine-grained approval controls
	if pctx.Mode == types.PermissionModeGranular {
		return e.applyGranularTransformations(pctx, result)
	}

	return result, nil
}

// applyGranularTransformations applies granular approval config to permission results.
// Each approval type can be individually enabled or disabled.
func (e *Engine) applyGranularTransformations(pctx *PermissionContext, result types.PermissionResult) (types.PermissionResult, error) {
	if result.Behavior != types.PermissionBehaviorAsk {
		return result, nil
	}

	// Get granular config from Additional or use default
	granularConfig := types.DefaultGranularConfig()
	if pctx.Additional != nil {
		if gc, ok := pctx.Additional["granular_config"].(types.GranularConfig); ok {
			granularConfig = gc
		}
	}

	// Check what type of approval this is and apply granular config
	decisionType := ""
	if result.DecisionReason != nil {
		decisionType = string(result.DecisionReason.Type)
	}

	// Sandbox approval (e.g., sandbox-based shell commands)
	if decisionType == "sandbox" || decisionType == "sandboxApproval" {
		if !granularConfig.AllowsSandboxApproval() {
			return types.DenyWithDecisionReason(
				fmt.Sprintf("permission to use %s denied: sandbox approval disabled in granular mode", pctx.ToolName),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "granular",
					Reason: "granular config: sandbox approval disabled",
				},
			), nil
		}
	}

	// Rules approval (exec-policy prompt rules)
	if decisionType == "rule" || decisionType == "rules" {
		if !granularConfig.AllowsRulesApproval() {
			return types.DenyWithDecisionReason(
				fmt.Sprintf("permission to use %s denied: rules approval disabled in granular mode", pctx.ToolName),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "granular",
					Reason: "granular config: rules approval disabled",
				},
			), nil
		}
	}

	// Skill approval
	if decisionType == "skill" || decisionType == "skillApproval" {
		if !granularConfig.AllowsSkillApproval() {
			return types.DenyWithDecisionReason(
				fmt.Sprintf("permission to use %s denied: skill approval disabled in granular mode", pctx.ToolName),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "granular",
					Reason: "granular config: skill approval disabled",
				},
			), nil
		}
	}

	// Request permissions tool approval
	if decisionType == "requestPermissions" || decisionType == "request_permissions" {
		if !granularConfig.AllowsRequestPermissionsApproval() {
			return types.DenyWithDecisionReason(
				fmt.Sprintf("permission to use %s denied: request_permissions approval disabled in granular mode", pctx.ToolName),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "granular",
					Reason: "granular config: request_permissions approval disabled",
				},
			), nil
		}
	}

	return result, nil
}

// NewDefaultRules returns default permission rules.
func NewDefaultRules() []PermissionRule {
	return []PermissionRule{
		{
			Value:    PermissionRuleValue{ToolName: "read_file"},
			Pattern:  "tool:read_file",
			Behavior: types.PermissionBehaviorAllow,
			Priority: 100,
			Reason:   "read-only tools are always safe",
			Source:   types.PermissionSourceStatic,
		},
		{
			Value:    PermissionRuleValue{ToolName: "bash", RuleContent: "git *"},
			Pattern:  "tool:bash:git *",
			Behavior: types.PermissionBehaviorAllow,
			Priority: 90,
			Reason:   "git commands are generally safe",
			Source:   types.PermissionSourceStatic,
		},
		{
			Value:    PermissionRuleValue{ToolName: "bash", RuleContent: "rm -rf *"},
			Pattern:  "tool:bash:rm -rf *",
			Behavior: types.PermissionBehaviorDeny,
			Priority: 100,
			Reason:   "destructive command blocked",
			Source:   types.PermissionSourceStatic,
		},
	}
}

// ── Safe-tool heuristics ──────────────────────────────────────────────────────

// isAlwaysSafeTool returns true for tools that are inherently read-only or
// purely network-read operations. These are auto-approved in all interactive
// modes so that grep, web_search, tree, read_file, etc. never pop a permission
// card regardless of whether the user is in onRequest or auto mode.
func isAlwaysSafeTool(name string) bool {
	switch name {
	case
		// File read-only
		"read_file", "grep", "glob", "tree",
		// Web read-only
		"web_search", "web_fetch", "web_crawl", "web_map",
		"scholarly_search", "wikipedia",
		// MCP read-only
		"mcp_list_resources", "mcp_read_resource",
		// Utility / introspection
		"tool_search", "lsp", "docx", "monitor", "skill":
		return true
	}
	return false
}

// isSafeWorkflowTool returns true for non-destructive workflow management tools
// that should not interrupt flow in onRequest mode (agent delegation, task
// tracking, plan mode transitions).
func isSafeWorkflowTool(name string) bool {
	switch name {
	case
		"agent",
		"task_create", "task_update", "task_get",
		"task_list", "task_output", "task_stop",
		"enter_plan_mode", "exit_plan_mode":
		return true
	}
	return false
}

// heuristicAllowResult builds an Allow result attributed to the safe-tool heuristic.
func heuristicAllowResult(toolName, reason string, input map[string]any) types.PermissionResult {
	return types.AllowWithInputAndDecisionReason(
		fmt.Sprintf("tool %q auto-approved: %s", toolName, reason),
		utils.CloneInput(input),
		&types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonMode,
			Source: "safe_heuristic",
			Reason: reason,
		},
	)
}
