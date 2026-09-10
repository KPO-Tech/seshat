package automation

import (
	"context"
	"fmt"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
	"github.com/KPO-Tech/seshat/pkg/sdk"
	"github.com/KPO-Tech/seshat/pkg/types"
	"github.com/KPO-Tech/seshat/pkg/workflow"
)

// messageSubmitter is the slice of *sdk.Session this file depends on —
// narrow enough that a test can satisfy it with a fake, matching this
// package's existing preference for unit-testing execution logic without a
// real LLM call (see runner.go's buildClientConfig).
type messageSubmitter interface {
	SubmitMessage(ctx context.Context, content string) (*sdk.SessionResponse, error)
}

// sessionAgentCaller adapts a single session to dataflow.AgentCaller so a
// graph's "agent" nodes reuse the job's own session (and its conversation
// so far) rather than each spinning up an unrelated one — unlike
// sdk.Client.RunWorkflow's default executor (pkg/sdk/workflow.go), which
// calls Client.Ask per node and so starts a fresh session every time.
// agentSlug is accepted for interface symmetry with a future multi-agent
// dispatch but unused today — jobWorkflow's session is already the one
// job.Agent resolved to (unchanged from before dataflow.AgentCaller grew
// its tools parameter - not tightening this now, since jobs already rely
// on a non-empty, effectively-ignored slug here, per
// TestSessionAgentCallerReturnsAssistantText).
//
// tools is new and has no such existing contract to preserve. Actually
// scoping a node's tools needs a way to resolve a tool name to a
// registrable sdk.Tool (internal/tools/registry.Registry.Get is type-
// compatible - both alias contract.Tool - but nothing here holds a
// Registry to call it on yet). Rather than silently ignoring a tools
// override a workflow author explicitly set (which would run their node
// with every tool the job's agent already has instead of the requested
// subset, with no indication anything was wrong), it fails loudly instead
// until that resolution path exists.
type sessionAgentCaller struct{ session messageSubmitter }

func (a sessionAgentCaller) Ask(ctx context.Context, _ string, prompt string, tools []string) (string, error) {
	if len(tools) > 0 {
		return "", fmt.Errorf("dataflow: agent node requested tools=%v, but per-node tool scoping is not implemented yet - omit it to use the workflow's own agent's tools", tools)
	}
	resp, err := a.session.SubmitMessage(ctx, prompt)
	if err != nil {
		return "", err
	}
	return lastAssistantText(resp.Messages), nil
}

// sessionSubworkflowRunner adapts the same session to dataflow.SubworkflowRunner
// by giving pkg/workflow.Run an Executor that calls session.SubmitMessage per
// node — the same pattern sdk.Client.RunWorkflow's default executor uses
// (BuildNodePrompt + one call per node), just against this session instead
// of a fresh one per node.
type sessionSubworkflowRunner struct{ session messageSubmitter }

func (r sessionSubworkflowRunner) Run(ctx context.Context, def workflow.Definition) (workflow.Result, error) {
	executor := workflow.ExecutorFunc(func(execCtx context.Context, node workflow.Node, inputs map[string]workflow.NodeResult) (string, error) {
		resp, err := r.session.SubmitMessage(execCtx, workflow.BuildNodePrompt(node, inputs))
		if err != nil {
			return "", err
		}
		return lastAssistantText(resp.Messages), nil
	})
	return workflow.Run(ctx, def, executor, workflow.Options{})
}

// lastAssistantText extracts the text of the most recent assistant message
// — the same extraction pkg/sdk's own Ask/SubmitMessage helpers do
// internally (unexported there), reimplemented here against the public
// pkg/types shapes rather than reaching into pkg/sdk internals.
func lastAssistantText(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != types.RoleAssistant {
			continue
		}
		var text string
		for _, block := range messages[i].Content {
			if tc, ok := block.(types.TextContent); ok {
				text += tc.Text
			}
		}
		return text
	}
	return ""
}

// runGraph executes job.Graph against session — the Job.Graph path of
// jobWorkflow.Run. registry must cover every node type the graph
// references (see RunnerConfig.NodeRegistry); secrets may be nil if the
// graph uses no credential-needing node type.
func runGraph(ctx context.Context, graph *dataflow.Definition, registry *dataflow.Registry, secrets dataflow.SecretResolver, session messageSubmitter) error {
	if registry == nil {
		return fmt.Errorf("job has a Graph but this Runner has no NodeRegistry configured")
	}
	rt := &dataflow.Runtime{
		Secrets:     secrets,
		Agent:       sessionAgentCaller{session: session},
		Subworkflow: sessionSubworkflowRunner{session: session},
	}
	result, err := dataflow.Run(ctx, *graph, registry, rt, nil, dataflow.Options{})
	if err != nil {
		return fmt.Errorf("run graph: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("graph %q failed: %s", graph.Name, firstNodeError(result))
	}
	return nil
}

func firstNodeError(result dataflow.Result) string {
	for _, id := range result.Order {
		if r := result.Results[id]; !r.Success && r.Error != "" {
			return fmt.Sprintf("node %q: %s", id, r.Error)
		}
	}
	return "unknown error"
}
