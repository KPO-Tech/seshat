// Package expr evaluates JS boolean/value expressions (n8n's "{{ $json.x }}"
// convention, already used by seshat-backend's inbox event filter) for
// dataflow nodes that need per-item logic (filter/if/set/switch) — unlike
// event_filter.go's one-VM-per-call approach (fine for a single check per
// inbound message), a dataflow node may evaluate the same expression against
// many items in one run, so this pool reuses goja runtimes and caches
// compiled bytecode across evaluations.
package expr

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// Pool evaluates expressions against a bounded set of reused goja runtimes,
// caching compiled bytecode by source string. A goja.Runtime cannot run two
// scripts concurrently, so runtimes are checked out/in like a connection
// pool; Compile is the expensive, reusable part (bytecode has no per-run
// state), so it's cached independently of runtime checkout.
type Pool struct {
	runtimes chan *goja.Runtime
	maxSize  int

	mu       sync.RWMutex
	programs map[string]*goja.Program
}

// DefaultTimeout bounds a single evaluation — an expression authored by an
// LLM (the Inbox Agent) is untrusted input in the same sense event_filter.go
// already treats it: it must fail loudly rather than hang the graph.
const DefaultTimeout = 2 * time.Second

// NewPool creates a pool that keeps up to maxRuntimes JavaScript runtimes ready to evaluate expressions (8 when it is 0 or less) and caches the compiled programs.
func NewPool(maxRuntimes int) *Pool {
	if maxRuntimes <= 0 {
		maxRuntimes = 8
	}
	return &Pool{
		runtimes: make(chan *goja.Runtime, maxRuntimes),
		maxSize:  maxRuntimes,
		programs: make(map[string]*goja.Program),
	}
}

func (p *Pool) checkout() *goja.Runtime {
	select {
	case rt := <-p.runtimes:
		return rt
	default:
		return goja.New()
	}
}

func (p *Pool) checkin(rt *goja.Runtime) {
	rt.ClearInterrupt()
	select {
	case p.runtimes <- rt:
	default:
		// Pool is at capacity (more concurrent evaluations than maxSize
		// have been in flight) — drop the extra runtime rather than block
		// the caller or grow the pool unbounded.
	}
}

func (p *Pool) compile(source string) (*goja.Program, error) {
	p.mu.RLock()
	program, ok := p.programs[source]
	p.mu.RUnlock()
	if ok {
		return program, nil
	}

	program, err := goja.Compile("expr", source, false)
	if err != nil {
		return nil, fmt.Errorf("compile expression: %w", err)
	}
	p.mu.Lock()
	p.programs[source] = program
	p.mu.Unlock()
	return program, nil
}

// Eval evaluates source with bindings set as global variables (e.g.
// bindings["$json"] = the current Item) and returns the raw goja value's
// exported Go form.
func (p *Pool) Eval(ctx context.Context, source string, bindings map[string]any) (any, error) {
	program, err := p.compile(source)
	if err != nil {
		return nil, err
	}

	rt := p.checkout()
	defer p.checkin(rt)

	for name, value := range bindings {
		if err := rt.Set(name, value); err != nil {
			return nil, fmt.Errorf("bind %s: %w", name, err)
		}
	}

	deadline := DefaultTimeout
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining > 0 && remaining < deadline {
			deadline = remaining
		}
	}
	timer := time.AfterFunc(deadline, func() { rt.Interrupt("expression evaluation timed out") })
	defer timer.Stop()

	result, err := rt.RunProgram(program)
	if err != nil {
		return nil, fmt.Errorf("evaluate expression: %w", err)
	}
	return result.Export(), nil
}

// EvalBool evaluates source and requires a boolean result — the shape
// filter/if nodes need, matching event_filter.go's own "must be a boolean,
// no silent coercion" rule.
func (p *Pool) EvalBool(ctx context.Context, source string, bindings map[string]any) (bool, error) {
	value, err := p.Eval(ctx, source, bindings)
	if err != nil {
		return false, err
	}
	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("expression must evaluate to a boolean, got %T", value)
	}
	return b, nil
}
