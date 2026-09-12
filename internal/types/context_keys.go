package types

import "context"

// contextKeyRuntimeEventEmitter is the unexported context key for the runtime event emitter.
type contextKeyRuntimeEventEmitter struct{}

// RuntimeEventEmitterKey is used with context.WithValue to inject the parent session's
// event emitter into sub-agent calls, enabling streaming bridge from sub-agent to parent.
var RuntimeEventEmitterKey = contextKeyRuntimeEventEmitter{}

// contextKeySubAgentMaxDepth carries the per-request configured maximum sub-agent
// spawn depth. Injected by the API query handler from the user's preferences.
type contextKeySubAgentMaxDepth struct{}

// WithSubAgentMaxDepth returns a context carrying the user-configured sub-agent
// depth limit. The agent tool reads this to override the server-wide default.
// Pass 0 to clear any override and fall back to the server default.
func WithSubAgentMaxDepth(ctx context.Context, depth int) context.Context {
	if depth <= 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKeySubAgentMaxDepth{}, depth)
}

// SubAgentMaxDepthFromContext returns the configured depth limit from ctx,
// or 0 if none was set (caller should then use the server default constant).
func SubAgentMaxDepthFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	d, _ := ctx.Value(contextKeySubAgentMaxDepth{}).(int)
	return d
}

// contextKeyPromptFn carries the current turn's interactive-prompt bridge
// (consumed by ask_user_question and the permission integrator's auto-mode
// "ask" outcome). Resolved per-call via PromptFnFromContext instead of a
// value mutated on a shared client/tool-instance field (the older
// Client.SetPromptFn / Engine.SetPromptFn path, still supported as a
// fallback for embedders that never share a client across concurrent turns)
// - a host that reuses one *sdk.Client across multiple concurrent turns
// (e.g. a per-user/provider client cache) would otherwise have turn A's
// "reach the user" callback silently overwritten or cleared by turn B's,
// since that field lives on the shared client/tool, not on any one turn.
type contextKeyPromptFn struct{}

// WithPromptFn returns a context carrying this turn's prompt bridge.
func WithPromptFn(ctx context.Context, fn PromptFn) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKeyPromptFn{}, fn)
}

// PromptFnFromContext returns the current turn's prompt bridge, or nil if none was set.
func PromptFnFromContext(ctx context.Context) PromptFn {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(contextKeyPromptFn{}).(PromptFn)
	return fn
}

// contextKeyWebSearchRunner carries the current turn's web_search execution
// runner - same per-call-instead-of-shared-mutable-field reasoning as
// WithPromptFn above (see its doc comment). Stored as `any` rather than
// websearchtool.RunnerFn to avoid this package importing
// internal/tools/web/search (which already imports internal/types); the
// web_search tool asserts the concrete function type itself.
type contextKeyWebSearchRunner struct{}

// WithWebSearchRunner returns a context carrying this turn's web_search
// runner. fn should be a internal/tools/web/search.RunnerFn; typed as `any`
// here to avoid an import cycle.
func WithWebSearchRunner(ctx context.Context, fn any) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKeyWebSearchRunner{}, fn)
}

// WebSearchRunnerFromContext returns the current turn's web_search runner
// (as `any` - the caller type-asserts to websearchtool.RunnerFn), or nil if none was set.
func WebSearchRunnerFromContext(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(contextKeyWebSearchRunner{})
}

// contextKeyAgentUserID carries the authenticated user ID into the engine and tools
// so they can scope per-user operations (e.g. long-term memory) without an extra DB read.
type contextKeyAgentUserID struct{}

// WithAgentUserID returns a context carrying the authenticated user's ID.
func WithAgentUserID(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKeyAgentUserID{}, userID)
}

// AgentUserIDFromContext returns the user ID from ctx, or empty string if absent.
func AgentUserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(contextKeyAgentUserID{}).(string)
	return id
}
