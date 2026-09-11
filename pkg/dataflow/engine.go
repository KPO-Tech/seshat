package dataflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const mainPort = "main"

// Validate checks graph structure only (unique node IDs, connections that
// target real nodes, no cycles, Tools/Agent references that are
// structurally sound) — it does not need a Registry, mirroring
// pkg/workflow.Validate. Per-node parameter validation, and the one Tools
// check that needs a node type's real metadata (a target can't be a Trigger
// or Logic node - see validateToolTargetTypes), happens in Run, once each
// node's executor is resolved.
func Validate(def Definition) error {
	if len(def.Nodes) == 0 {
		return errors.New("dataflow: graph must contain at least one node")
	}
	ids := make(map[string]bool, len(def.Nodes))
	for _, node := range def.Nodes {
		if node.ID == "" {
			return errors.New("dataflow: node id is required")
		}
		if ids[node.ID] {
			return fmt.Errorf("dataflow: duplicate node id %q", node.ID)
		}
		ids[node.ID] = true
		if node.Type == "" {
			return fmt.Errorf("dataflow: node %q is missing a type", node.ID)
		}
	}
	for _, node := range def.Nodes {
		for port, targets := range node.Connections {
			for _, target := range targets {
				if !ids[target] {
					return fmt.Errorf("dataflow: node %q port %q connects to unknown node %q", node.ID, port, target)
				}
			}
		}
	}
	byID := make(map[string]Node, len(def.Nodes))
	for _, node := range def.Nodes {
		byID[node.ID] = node
	}
	for _, node := range def.Nodes {
		if len(node.Tools) == 0 {
			continue
		}
		if node.Type != "query" && node.Type != "tools" {
			return fmt.Errorf("dataflow: node %q sets tools but is type %q, not \"query\" or \"tools\" - only those types may call tools", node.ID, node.Type)
		}
		for _, targetID := range node.Tools {
			target, ok := byID[targetID]
			if !ok {
				return fmt.Errorf("dataflow: node %q tools references unknown node %q", node.ID, targetID)
			}
			if targetID == node.ID {
				return fmt.Errorf("dataflow: node %q cannot list itself as a tool", node.ID)
			}
			if target.Type == "agent" {
				return fmt.Errorf("dataflow: node %q tools references %q, but an agent node cannot be used as a tool", node.ID, targetID)
			}
			if target.Type == "query" {
				return fmt.Errorf("dataflow: node %q tools references %q, but a query node cannot be used as a tool", node.ID, targetID)
			}
		}
	}
	for _, node := range def.Nodes {
		if node.Type != "query" {
			if node.Agent != "" {
				return fmt.Errorf("dataflow: node %q sets agent but is type %q, not \"query\" - only a query node references an agent", node.ID, node.Type)
			}
			continue
		}
		if node.Agent == "" {
			return fmt.Errorf("dataflow: query node %q requires an agent reference", node.ID)
		}
		target, ok := byID[node.Agent]
		if !ok {
			return fmt.Errorf("dataflow: query node %q references unknown agent %q", node.ID, node.Agent)
		}
		if target.Type != "agent" {
			return fmt.Errorf("dataflow: query node %q references %q, but it is type %q, not \"agent\"", node.ID, node.Agent, target.Type)
		}
	}
	for nodeID := range def.PinnedData {
		if !ids[nodeID] {
			return fmt.Errorf("dataflow: pinnedData references unknown node %q", nodeID)
		}
	}
	if _, err := topologicalLevels(def); err != nil {
		return err
	}
	return nil
}

// validateToolTargetTypes rejects a Tools reference to a Trigger or Logic
// node - the one Tools check that needs a node type's real metadata
// (NodeDescription.IsTrigger/Category), not just the raw Definition, so it
// runs here (once Run already has a Registry) rather than in the
// Registry-free Validate above. A Trigger is a graph's entry point, a Logic
// node (filter/if/switch, Category "Logic") is a branching structure -
// neither is a "capability" an agent's LLM should be able to invoke
// (automation-app-pages.md §37.9); Agent-typed targets are already rejected
// by Validate itself, no registry needed for that one.
func validateToolTargetTypes(def Definition, registry *Registry) error {
	byID := make(map[string]Node, len(def.Nodes))
	for _, node := range def.Nodes {
		byID[node.ID] = node
	}
	for _, node := range def.Nodes {
		for _, targetID := range node.Tools {
			target, ok := byID[targetID]
			if !ok {
				continue // already reported by Validate
			}
			executor, err := registry.Get(target.Type)
			if err != nil {
				return fmt.Errorf("dataflow: node %q tools references %q: %w", node.ID, targetID, err)
			}
			desc := executor.Description()
			if desc.IsTrigger {
				return fmt.Errorf("dataflow: node %q tools references %q, but a trigger node cannot be used as a tool", node.ID, targetID)
			}
			if desc.Category == "Logic" {
				return fmt.Errorf("dataflow: node %q tools references %q, but a logic node cannot be used as a tool", node.ID, targetID)
			}
		}
	}
	return nil
}

// predecessors returns, for every node, the set of node IDs whose output
// routes into it (derived by inverting every node's Connections).
func predecessors(def Definition) map[string]map[string]bool {
	preds := make(map[string]map[string]bool, len(def.Nodes))
	for _, node := range def.Nodes {
		preds[node.ID] = map[string]bool{}
	}
	for _, node := range def.Nodes {
		for _, targets := range node.Connections {
			for _, target := range targets {
				preds[target][node.ID] = true
			}
		}
	}
	return preds
}

// toolOnlyNodeIDs returns node IDs that exist purely as a query's callable
// tool - referenced (directly, or transitively through a "tools" hub node's
// own Tools list) in some node's Tools list, and structurally isolated from
// the rest of the graph (no incoming or outgoing Connections at all). These
// must never be scheduled by the normal topological walk (automation-app-
// pages.md §37.9) - only a query's own on-demand invocation (see
// ResolveTools) ever runs them. A node that's *also* wired into real
// Connections (dual-use) is deliberately excluded from this set - being
// additionally tool-callable never suppresses a node's normal place in the
// sequential flow, only pure orphans (nothing else in the graph reaches
// them) are held back. A "tools" hub node itself is included here too when
// it's isolated - it never does real work either (see toolsNode.Execute),
// so it shouldn't appear as "ran" in a trace any more than an isolated
// capability node should.
func toolOnlyNodeIDs(def Definition, preds map[string]map[string]bool) map[string]bool {
	byID := make(map[string]Node, len(def.Nodes))
	for _, node := range def.Nodes {
		byID[node.ID] = node
	}
	referenced := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		if referenced[id] {
			return // already found via another path, or a tools->tools cycle - either way, stop
		}
		referenced[id] = true
		node, ok := byID[id]
		if !ok || node.Type != "tools" {
			return
		}
		for _, nested := range node.Tools {
			walk(nested)
		}
	}
	for _, node := range def.Nodes {
		for _, id := range node.Tools {
			walk(id)
		}
	}
	if len(referenced) == 0 {
		return nil
	}
	hasOutgoing := map[string]bool{}
	for _, node := range def.Nodes {
		for _, targets := range node.Connections {
			if len(targets) > 0 {
				hasOutgoing[node.ID] = true
				break
			}
		}
	}
	toolOnly := map[string]bool{}
	for id := range referenced {
		if len(preds[id]) == 0 && !hasOutgoing[id] {
			toolOnly[id] = true
		}
	}
	return toolOnly
}

// topologicalLevels groups node IDs into execution batches (Kahn's
// algorithm): every node in a level depends only on nodes in earlier
// levels, so a level's nodes are safe to execute concurrently. Deterministic
// (sorted) ordering within a level, same approach as pkg/workflow's
// topologicalLevels. Nodes in toolOnlyNodeIDs never enter remaining at all -
// they get no level, ever, until/unless something (this function's own
// exclusion aside) later wires real Connections to them.
func topologicalLevels(def Definition) ([][]string, error) {
	preds := predecessors(def)
	toolOnly := toolOnlyNodeIDs(def, preds)
	remaining := make(map[string]map[string]bool, len(preds))
	for id, p := range preds {
		if toolOnly[id] {
			continue
		}
		cp := make(map[string]bool, len(p))
		for dep := range p {
			cp[dep] = true
		}
		remaining[id] = cp
	}

	var levels [][]string
	for len(remaining) > 0 {
		var ready []string
		for id, deps := range remaining {
			if len(deps) == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, errors.New("dataflow: graph contains a cycle")
		}
		sort.Strings(ready)
		levels = append(levels, ready)
		for _, id := range ready {
			delete(remaining, id)
		}
		for _, deps := range remaining {
			for _, id := range ready {
				delete(deps, id)
			}
		}
	}
	return levels, nil
}

// Options tunes Run's execution. MaxParallel caps how many nodes within one
// level run concurrently; 0 defaults to 4 (mirrors pkg/workflow.Options).
type Options struct {
	MaxParallel int
}

// Run executes def against registry, seeding every node with no
// predecessors with input, and returns once every reachable node has run
// (or been skipped because a predecessor failed).
func Run(ctx context.Context, def Definition, registry *Registry, rt *Runtime, input []Item, opts Options) (Result, error) {
	if registry == nil {
		return Result{}, errors.New("dataflow: registry is required")
	}
	if err := Validate(def); err != nil {
		return Result{}, err
	}
	if err := validateToolTargetTypes(def, registry); err != nil {
		return Result{}, err
	}
	if opts.MaxParallel <= 0 {
		opts.MaxParallel = 4
	}

	levels, err := topologicalLevels(def)
	if err != nil {
		return Result{}, err
	}
	preds := predecessors(def)
	nodes := make(map[string]Node, len(def.Nodes))
	for _, node := range def.Nodes {
		nodes[node.ID] = node
	}

	started := time.Now()
	result := Result{Name: def.Name, Success: true, StartedAt: started, Results: map[string]NodeResult{}}

	pendingInputs := map[string][]Item{}
	// pendingSources tracks, index-aligned with pendingInputs, where each
	// pending item came from - a zero-value ItemSource for the run's own
	// seed input, {Node, Port} of whichever upstream node produced it
	// otherwise. Written under the same mu.Lock() as pendingInputs below, so
	// the two always stay index-aligned even with multiple producer
	// goroutines writing to the same target.
	pendingSources := map[string][]ItemSource{}
	for id, p := range preds {
		if len(p) == 0 {
			pendingInputs[id] = append(pendingInputs[id], input...)
			for range input {
				pendingSources[id] = append(pendingSources[id], ItemSource{})
			}
		}
	}

	var mu sync.Mutex
	for _, level := range levels {
		sem := make(chan struct{}, opts.MaxParallel)
		var wg sync.WaitGroup
		for _, id := range level {
			node := nodes[id]

			// Every read/write of pendingInputs and result.Results for this
			// level happens under mu — level N+1's entries are still being
			// written concurrently by level N's goroutines below, and plain
			// Go maps aren't safe for concurrent read+write even on
			// disjoint keys.
			mu.Lock()
			failedDep := ""
			for dep := range preds[id] {
				// A dependency that failed but Continued (Tier 3.1's OnError
				// letting the run proceed anyway) is treated the same as a
				// real success here - it already produced whatever output
				// its OnError mode dictated (empty, or a synthesized error
				// item), and that's what this node should receive.
				if r := result.Results[dep]; !r.Success && !r.Continued {
					failedDep = dep
					break
				}
			}
			if failedDep != "" {
				now := time.Now()
				result.Results[id] = NodeResult{ID: id, Type: node.Type, Success: false, Skipped: true,
					Error: fmt.Sprintf("upstream node %q failed", failedDep), StartedAt: now, EndedAt: now}
				result.Order = append(result.Order, id)
				result.Success = false
				mu.Unlock()
				continue
			}
			nodeInput := pendingInputs[id]
			nodeSources := pendingSources[id]
			mu.Unlock()

			sem <- struct{}{}
			wg.Add(1)
			go func(node Node, nodeInput []Item, nodeSources []ItemSource) {
				defer wg.Done()
				defer func() { <-sem }()
				nodeCtx := withGraphContext(ctx, def, registry, node.ID)
				nodeResult, output := executeNode(nodeCtx, registry, rt, node, nodeInput, nodeSources, def.PinnedData)

				mu.Lock()
				result.Results[node.ID] = nodeResult
				result.Order = append(result.Order, node.ID)
				if !nodeResult.Success {
					result.Success = false
				}
				// A Continued failure (Tier 3.1's OnError) still routes its
				// output downstream and records for $node('Name'), same as
				// a real success - only a genuine stop-the-run failure
				// (neither Success nor Continued) withholds both.
				if nodeResult.Success || nodeResult.Continued {
					// Recorded here, not in executeNode - $node('Name')
					// (see ResolveValue) must only ever see a node that has
					// *finished*, matching n8n's own execution-order
					// requirement on $('Name').
					rt.recordNodeOutput(node.ID, nodeResult.Output)
					for port, targets := range node.Connections {
						items := output.Ports[port]
						for _, target := range targets {
							pendingInputs[target] = append(pendingInputs[target], items...)
							for range items {
								pendingSources[target] = append(pendingSources[target], ItemSource{Node: node.ID, Port: port})
							}
						}
					}
				}
				mu.Unlock()
			}(node, nodeInput, nodeSources)
		}
		wg.Wait()
	}

	result.EndedAt = time.Now()
	result.Duration = result.EndedAt.Sub(result.StartedAt)
	return result, nil
}

// executeNode runs one node and returns both its recorded NodeResult (for
// Result.Results — reports the "main" port only, or every port concatenated
// if the node used no "main" port at all, e.g. a pure if/switch, in Output;
// OutputByPort keeps the real per-port breakdown) and its raw Output (every
// port, used by Run's caller to route to Connections targets).
func executeNode(ctx context.Context, registry *Registry, rt *Runtime, node Node, input []Item, sources []ItemSource, pinnedData map[string][]Item) (NodeResult, Output) {
	start := time.Now()
	// A pinned node never touches its real executor at all - not even the
	// registry lookup - so a graph can be tested with a node whose real
	// config is incomplete, or whose type isn't implemented yet. Its
	// pinned items become its "main"-port output exactly as if a real
	// Execute had returned them.
	if items, ok := pinnedData[node.ID]; ok {
		now := time.Now()
		output := Main(items)
		return NodeResult{ID: node.ID, Type: node.Type, Success: true, Pinned: true, Input: input, InputSource: sources,
			Output: items, OutputByPort: output.Ports, StartedAt: start, EndedAt: now, Duration: now.Sub(start)}, output
	}
	executor, err := registry.Get(node.Type)
	if err != nil {
		return instantFailure(node, err, input, sources), Output{}
	}
	if err := executor.ValidateParameters(node.Parameters); err != nil {
		return instantFailure(node, fmt.Errorf("invalid parameters for node %q: %w", node.ID, err), input, sources), Output{}
	}
	output, err, attempts := executeWithRetry(ctx, executor, rt, node, input)
	end := time.Now()
	if err != nil {
		switch node.OnError {
		case "continueRegularOutput":
			// Proceed as if nothing happened - no output, but the run keeps
			// going (see engine.go's Run: Continued is treated like Success
			// for downstream-skip/routing purposes).
			return NodeResult{ID: node.ID, Type: node.Type, Success: false, Continued: true, Input: input, InputSource: sources,
				Error: err.Error(), Attempts: attempts, StartedAt: start, EndedAt: end, Duration: end.Sub(start)}, Main(nil)
		case "continueErrorOutput":
			errOutput := Output{Ports: map[string][]Item{"error": {{"error": err.Error()}}}}
			return NodeResult{ID: node.ID, Type: node.Type, Success: false, Continued: true, Input: input, InputSource: sources,
				Error: err.Error(), Attempts: attempts, OutputByPort: errOutput.Ports, StartedAt: start, EndedAt: end, Duration: end.Sub(start)}, errOutput
		default: // "" - today's existing behavior, stop this branch of the run
			return NodeResult{ID: node.ID, Type: node.Type, Success: false, Input: input, InputSource: sources,
				Error: err.Error(), Attempts: attempts, StartedAt: start, EndedAt: end, Duration: end.Sub(start)}, Output{}
		}
	}
	reported := output.Ports[mainPort]
	if reported == nil {
		for _, items := range output.Ports {
			reported = append(reported, items...)
		}
	}
	return NodeResult{ID: node.ID, Type: node.Type, Success: true, Input: input, InputSource: sources,
		Output: reported, OutputByPort: output.Ports, StartedAt: start, EndedAt: end, Duration: end.Sub(start), Attempts: attempts}, output
}

// executeWithRetry calls executor.Execute once, or up to node.MaxTries times
// (clamped 2-5) with node.WaitBetweenTriesMS (clamped 0-5000) between
// attempts if node.RetryOnFail is set (Tier 3.1) - a node that never opts in
// behaves exactly as before this feature existed: one call, no wait. Stops
// early on ctx cancellation, same as pkg/automation's own WithRetry
// middleware (a whole-workflow-level retry, the only other retry precedent
// in this SDK) already does. attempts is how many Execute calls were
// actually made, always >= 1.
func executeWithRetry(ctx context.Context, executor NodeExecutor, rt *Runtime, node Node, input []Item) (output Output, err error, attempts int) {
	maxTries := 1
	if node.RetryOnFail {
		maxTries = node.MaxTries
		if maxTries < 2 {
			maxTries = 2
		}
		if maxTries > 5 {
			maxTries = 5
		}
	}
	wait := time.Duration(node.WaitBetweenTriesMS) * time.Millisecond
	if wait < 0 {
		wait = 0
	}
	if wait > 5*time.Second {
		wait = 5 * time.Second
	}

retryLoop:
	for attempts = 1; attempts <= maxTries; attempts++ {
		output, err = executor.Execute(ctx, rt, input, node.Parameters)
		if err == nil || attempts == maxTries {
			break
		}
		if wait > 0 {
			select {
			case <-ctx.Done():
				err = ctx.Err()
				break retryLoop
			case <-time.After(wait):
			}
		}
	}
	return output, err, attempts
}

func instantFailure(node Node, err error, input []Item, sources []ItemSource) NodeResult {
	now := time.Now()
	return NodeResult{ID: node.ID, Type: node.Type, Success: false, Input: input, InputSource: sources,
		Error: err.Error(), StartedAt: now, EndedAt: now}
}
