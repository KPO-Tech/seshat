package permissions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	automode "github.com/EngineerProjects/nexus-engine/internal/permissions/auto"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/internal/utils"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// Integrator integrates permission checking with tool execution.
// The Orchestrator is the primary execution path and has its own safety checks
// via SafetyChecker. The Integrator provides a PermissionResolver for contexts
// that need standalone permission resolution (e.g. the query loop).
type Integrator struct {
	engine *Engine

	promptFn types.PromptFn

	mu           sync.RWMutex
	sessionTools map[types.SessionID]map[string]bool
}

// NewIntegrator creates a new permission integrator.
func NewIntegrator(engine *Engine) *Integrator {
	return &Integrator{
		engine:       engine,
		sessionTools: make(map[types.SessionID]map[string]bool),
	}
}

// SetPromptFn sets the prompt function for asking users.
func (i *Integrator) SetPromptFn(fn types.PromptFn) {
	i.promptFn = fn
}

// Resolver creates a typed PermissionResolver that integrates with the permission engine.
func (i *Integrator) Resolver(sessionID types.SessionID, turnID types.TurnID, mode types.PermissionMode) types.PermissionResolver {
	return i.ResolverWithContext(sessionID, turnID, &types.PermissionContext{Mode: mode}, nil)
}

// ResolverWithContext creates a typed PermissionResolver that carries the live
// session permission context and transcript into the permission engine.
func (i *Integrator) ResolverWithContext(
	sessionID types.SessionID,
	turnID types.TurnID,
	permissionContext *types.PermissionContext,
	transcript []types.Message,
) types.PermissionResolver {
	return types.CanUseToolFn(func(ctx context.Context, request types.ToolPermissionRequest) types.PermissionResult {
		toolName := request.ToolName
		toolInput := request.ToolInput
		activePermissionContext := cloneSessionPermissionContext(permissionContext)
		activePermissionContext.NormalizeLegacyPlanMode()
		requestMode := request.PermissionMode
		if requestMode == "" {
			requestMode = activePermissionContext.Mode
		}
		if requestMode == "" {
			requestMode = types.PermissionModeOnRequest
		}
		requestSessionID := request.SessionID
		if requestSessionID == "" {
			requestSessionID = sessionID
		}

		if requestSessionID != "" {
			// 1. Fast path: check in-memory map
			i.mu.RLock()
			hasSession := i.sessionTools != nil && i.sessionTools[requestSessionID] != nil
			var allowed bool
			if hasSession {
				allowed = i.sessionTools[requestSessionID][toolName]
			}
			i.mu.RUnlock()

			// 2. Slow path: if session is not in memory, try to load from disk
			if !hasSession {
				i.mu.Lock()
				// Double-check inside lock
				if i.sessionTools == nil {
					i.sessionTools = make(map[types.SessionID]map[string]bool)
				}
				if i.sessionTools[requestSessionID] == nil {
					sessionDir := runtimepath.SessionDir("", string(requestSessionID))
					filePath := filepath.Join(sessionDir, "permissions.json")
					loadedMap := make(map[string]bool)
					if data, err := os.ReadFile(filePath); err == nil {
						_ = json.Unmarshal(data, &loadedMap)
					}
					i.sessionTools[requestSessionID] = loadedMap
				}
				allowed = i.sessionTools[requestSessionID][toolName]
				i.mu.Unlock()
			}

			if allowed {
				return types.AllowWithInputAndDecisionReason("auto-approved for session", utils.CloneInput(toolInput), &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "session",
					Reason: "auto-approved for session",
				})
			}
		}
		requestTurnID := request.TurnID
		if requestTurnID == "" {
			requestTurnID = turnID
		}

		metadata := clonePermissionMetadata(request.Metadata)
		if metadata == nil {
			metadata = make(map[string]any)
		}
		if request.WorkingDirectory != "" {
			metadata["working_directory"] = request.WorkingDirectory
		}
		if len(transcript) > 0 {
			metadata["transcript_messages"] = append([]types.Message(nil), transcript...)
		}

		pctx := &PermissionContext{
			Mode:                             requestMode,
			ExecutionMode:                    activePermissionContext.ExecutionMode,
			ToolName:                         toolName,
			ToolInput:                        toolInput,
			SessionID:                        requestSessionID,
			TurnID:                           requestTurnID,
			Stage:                            request.Stage,
			Intent:                           request.Intent,
			IsConcurrent:                     false,
			ToolUseID:                        request.ToolUseID,
			IsBypassPermissionsModeAvailable: activePermissionContext.IsBypassPermissionsModeAvailable,
			Additional:                       metadata,
		}
		if resolvedTool := tool.ToolFromMetadata(metadata); resolvedTool != nil {
			pctx.Tool = resolvedTool
		}
		// Read ShouldAvoidPermissionPrompts from metadata.
		if metadata != nil {
			if v, ok := metadata["should_avoid_permission_prompts"].(bool); ok {
				pctx.ShouldAvoidPermissionPrompts = v
			}
		}

		result, err := i.engine.CheckPermission(ctx, pctx)
		if result.UpdatedInput == nil && toolInput != nil {
			result.UpdatedInput = toolInput
		}
		if err != nil {
			return types.AskWithDecisionReason(fmt.Sprintf("permission check failed: %v", err), &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonOther,
				Source: "integrator",
				Reason: err.Error(),
			})
		}

		// Non-ask results (allow, deny, passthrough) are returned directly.
		if result.Behavior != types.PermissionBehaviorAsk {
			return result
		}

		// dontAsk mode: transform ask → deny at the integrator level too,
		// for cases where the engine returned ask (e.g., from hooks or
		// tool-level checks that the engine passed through).
		if requestMode == types.PermissionModeNever {
			return types.DenyWithDecisionReason(
				fmt.Sprintf("permission to use %s denied: running in dontAsk mode", toolName),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonMode,
					Source: "dontAsk",
					Reason: "dontAsk mode enabled",
				},
			)
		}

		// Headless auto-deny: when permission prompts should be avoided.
		if pctx.ShouldAvoidPermissionPrompts {
			return types.DenyWithDecisionReason(
				fmt.Sprintf("permission to use %s denied: permission prompts not available in this context", toolName),
				&types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonAsyncAgent,
					Source: "headless",
					Reason: "permission prompts are not available in this context",
				},
			)
		}

		if i.promptFn == nil {
			return result
		}

		promptReq := types.PromptRequest{
			Type:    types.PromptTypeConfirm,
			Message: fmt.Sprintf("Allow tool '%s'?", toolName),
			Metadata: map[string]any{
				"tool_name":         toolName,
				"tool_input":        toolInput,
				"tool_use_id":       request.ToolUseID,
				"working_directory": request.WorkingDirectory,
			},
		}

		response, err := i.promptFn(ctx, promptReq)
		if err != nil {
			return types.DenyWithDecisionReason(fmt.Sprintf("prompt failed: %v", err), &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonPrompt,
				Source: "prompt",
				Reason: err.Error(),
			})
		}

		if response.Cancelled {
			return types.DenyWithDecisionReason("user cancelled", &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonPrompt,
				Source: "prompt",
				Reason: "user cancelled",
			})
		}

		var approved bool
		var always bool
		if b, ok := response.Value.(bool); ok {
			approved = b
		} else if s, ok := response.Value.(string); ok {
			if s == "always" {
				approved = true
				always = true
			}
		}

		if approved {
			reason := "user approved"
			if always && requestSessionID != "" {
				i.mu.Lock()
				if i.sessionTools == nil {
					i.sessionTools = make(map[types.SessionID]map[string]bool)
				}
				if i.sessionTools[requestSessionID] == nil {
					i.sessionTools[requestSessionID] = make(map[string]bool)
				}
				i.sessionTools[requestSessionID][toolName] = true

				// Save to disk
				sessionDir := runtimepath.SessionDir("", string(requestSessionID))
				filePath := filepath.Join(sessionDir, "permissions.json")
				if err := os.MkdirAll(sessionDir, 0700); err == nil {
					if data, err := json.Marshal(i.sessionTools[requestSessionID]); err == nil {
						_ = os.WriteFile(filePath, data, 0600)
					}
				}
				i.mu.Unlock()
				reason = "always approved for session"
			}
			return types.AllowWithInputAndDecisionReason(reason, result.UpdatedInput, &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonPrompt,
				Source: "prompt",
				Reason: reason,
			})
		}

		return types.DenyWithDecisionReason("user denied", &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonPrompt,
			Source: "prompt",
			Reason: "user denied",
		})
	})
}

// CanUseTool creates a CanUseToolFn that integrates with the permission engine.
func (i *Integrator) CanUseTool(sessionID types.SessionID, turnID types.TurnID, mode types.PermissionMode) types.CanUseToolFn {
	return types.CanUseToolFunc(i.Resolver(sessionID, turnID, mode))
}

// AutoModeAvailable reports whether the underlying permission engine has an
// operational auto-mode classifier configured.
func (i *Integrator) AutoModeAvailable() bool {
	if i == nil || i.engine == nil {
		return false
	}
	return i.engine.IsAutoModeAvailable()
}

// SetAutoModeProviderClient wires the auto-mode classifier to the given provider client.
func (i *Integrator) SetAutoModeProviderClient(apiClient *providers.Client, model types.ModelIdentifier) {
	if i == nil || i.engine == nil || apiClient == nil {
		return
	}
	classifierConfig := automode.DefaultTwoStageConfig()
	classifierConfig.Model = model.ProviderModelName()
	autoClassifier := automode.NewTwoStageClassifierWithAPI(classifierConfig, automode.NewClassifierAPIClient(apiClient))
	i.engine.SetClassifier(providerBackedAutoModeClassifier{classifier: autoClassifier})
	i.engine.SetAdvancedClassifier(autoClassifier)
}

// CheckToolUse checks permissions for a specific tool use.
// This is a convenience method that builds the request from tool use content.
func (i *Integrator) CheckToolUse(
	ctx context.Context,
	toolUse types.ToolUseContent,
	toolDef tool.Definition,
	sessionID types.SessionID,
	turnID types.TurnID,
	mode types.PermissionMode,
) (types.PermissionResult, error) {
	resolver := i.Resolver(sessionID, turnID, mode)
	result := resolver.ResolvePermission(ctx, types.GlobalToolPermissionRequest(
		toolUse.Name,
		toolUse.Input,
		toolUse.ID,
		sessionID,
		turnID,
		mode,
		"",
		nil,
	))
	if result.IsDenied() {
		return result, nil
	}

	// Build a richer context for the engine's second pass (with ToolDefinition).
	pctx := &PermissionContext{
		Mode:           mode,
		ToolName:       toolUse.Name,
		ToolInput:      toolUse.Input,
		SessionID:      sessionID,
		TurnID:         turnID,
		Stage:          types.ToolPermissionStageGlobal,
		Intent:         types.ToolPermissionIntentCheck,
		IsConcurrent:   toolDef.IsConcurrencySafe,
		ToolDefinition: &toolDef,
	}

	result, err := i.engine.CheckPermission(ctx, pctx)
	if err != nil {
		return types.PermissionResult{}, fmt.Errorf("permission check failed: %w", err)
	}
	if result.IsPassthrough() {
		return types.AllowWithInput(result.Reason, result.UpdatedInput), nil
	}

	return result, nil
}

func cloneSessionPermissionContext(ctx *types.PermissionContext) *types.PermissionContext {
	if ctx == nil {
		return &types.PermissionContext{Mode: types.PermissionModeOnRequest}
	}
	cloned := *ctx
	if ctx.StrippedDangerousRules != nil {
		cloned.StrippedDangerousRules = make(map[string][]string, len(ctx.StrippedDangerousRules))
		for key, values := range ctx.StrippedDangerousRules {
			cloned.StrippedDangerousRules[key] = append([]string(nil), values...)
		}
	}
	return &cloned
}

func clonePermissionMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

type providerBackedAutoModeClassifier struct {
	classifier *automode.TwoStageClassifier
}

func (a providerBackedAutoModeClassifier) Classify(ctx context.Context, toolName string, input map[string]any) (Classification, error) {
	result, err := a.classifier.Classify(ctx, toolName, input)
	if err != nil {
		return Classification{}, err
	}
	return Classification{
		Allowed:    result.Allowed,
		Confidence: result.Confidence,
		Reason:     result.Reason,
	}, nil
}

// BatchCheckToolUses checks permissions for multiple tool uses.
func (i *Integrator) BatchCheckToolUses(
	ctx context.Context,
	toolUses []types.ToolUseContent,
	tools map[string]tool.Tool,
	sessionID types.SessionID,
	turnID types.TurnID,
	mode types.PermissionMode,
) ([]types.PermissionResult, error) {
	results := make([]types.PermissionResult, len(toolUses))

	for idx, toolUse := range toolUses {
		t, ok := tools[toolUse.Name]
		if !ok {
			results[idx] = types.Deny(fmt.Sprintf("tool not found: %s", toolUse.Name))
			continue
		}

		result, err := i.CheckToolUse(ctx, toolUse, t.Definition(), sessionID, turnID, mode)
		if err != nil {
			return nil, fmt.Errorf("failed to check tool '%s': %w", toolUse.Name, err)
		}

		results[idx] = result
	}

	return results, nil
}

// FilterAllowedToolUses filters tool uses to only those allowed.
func (i *Integrator) FilterAllowedToolUses(
	ctx context.Context,
	toolUses []types.ToolUseContent,
	tools map[string]tool.Tool,
	sessionID types.SessionID,
	turnID types.TurnID,
	mode types.PermissionMode,
) ([]types.ToolUseContent, []types.PermissionResult, error) {
	results, err := i.BatchCheckToolUses(ctx, toolUses, tools, sessionID, turnID, mode)
	if err != nil {
		return nil, nil, err
	}

	allowed := make([]types.ToolUseContent, 0)
	for idx, result := range results {
		if result.IsAllowed() {
			allowed = append(allowed, toolUses[idx])
		}
	}

	return allowed, results, nil
}

// PermissionMiddleware creates a middleware that checks permissions before tool execution.
func (i *Integrator) PermissionMiddleware(
	sessionID types.SessionID,
	turnID types.TurnID,
	mode types.PermissionMode,
) func(ctx context.Context, toolName string, toolInput map[string]any) error {
	return func(ctx context.Context, toolName string, toolInput map[string]any) error {
		resolver := i.Resolver(sessionID, turnID, mode)
		result := resolver.ResolvePermission(ctx, types.GlobalToolPermissionRequest(
			toolName,
			toolInput,
			"",
			sessionID,
			turnID,
			mode,
			"",
			nil,
		))

		if result.IsDenied() {
			return &PermissionDeniedError{
				ToolName: toolName,
				Reason:   result.Reason,
			}
		}

		if result.IsPassthrough() {
			return nil
		}

		if result.IsAsk() && i.promptFn == nil {
			return &PermissionDeniedError{
				ToolName: toolName,
				Reason:   "permission required but no prompt function available",
			}
		}

		return nil
	}
}

// PermissionDeniedError represents a permission denied error.
type PermissionDeniedError struct {
	ToolName string
	Reason   string
}

func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf("permission denied for tool '%s': %s", e.ToolName, e.Reason)
}

// IsPermissionDenied returns true if an error is a permission denied error.
func IsPermissionDenied(err error) bool {
	_, ok := err.(*PermissionDeniedError)
	return ok
}
