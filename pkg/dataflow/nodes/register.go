// Package nodes provides the generic, deterministic node types every
// dataflow graph can use regardless of tenant/product (http_request,
// filter, if, switch, set, merge, wait) — no credentials, no multi-tenant
// concerns. Database and SaaS connector node types build on this package
// but live separately (see pkg/dataflow/nodes/database, Phase 2).
package nodes

import (
	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow"
	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow/expr"
)

// Register adds every node type in this package to registry, sharing one
// expr.Pool across the expression-evaluating node types (filter/if/switch/
// set) so their compiled-program cache and runtime pool are actually
// shared, not duplicated per node type.
func Register(registry *dataflow.Registry, pool *expr.Pool) {
	registry.Register("http_request", NewHTTPRequest())
	registry.Register("filter", NewFilter(pool))
	registry.Register("if", NewIf(pool))
	registry.Register("switch", NewSwitch(pool))
	registry.Register("set", NewSet(pool))
	registry.Register("merge", NewMerge())
	registry.Register("wait", NewWait())
	registry.Register("schedule_trigger", NewScheduleTrigger())
	registry.Register("webhook_trigger", NewWebhookTrigger())
}
