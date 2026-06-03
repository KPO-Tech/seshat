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
