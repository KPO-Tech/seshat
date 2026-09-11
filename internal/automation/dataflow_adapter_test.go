package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
	"github.com/KPO-Tech/seshat/pkg/dataflow/nodes"
	"github.com/KPO-Tech/seshat/pkg/sdk"
	"github.com/KPO-Tech/seshat/pkg/types"
	"github.com/KPO-Tech/seshat/pkg/workflow"
)

// fakeSession is a messageSubmitter test double — no real LLM call, matching
// this package's existing preference for testing execution logic in
// isolation (see buildClientConfig in runner.go).
type fakeSession struct {
	responses    []string // one entry consumed per SubmitMessage call, in order
	calls        []string // prompts received, for assertions
	err          error
	registered   []string // tool names passed to RegisterTool, in order
	unregistered []string // tool names passed to UnregisterTool, in order
	registerErr  error
}

func (f *fakeSession) SubmitMessage(_ context.Context, content string) (*sdk.SessionResponse, error) {
	f.calls = append(f.calls, content)
	if f.err != nil {
		return nil, f.err
	}
	i := len(f.calls) - 1
	text := ""
	if i < len(f.responses) {
		text = f.responses[i]
	}
	return &sdk.SessionResponse{Messages: []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextContent{Text: text}}},
	}}, nil
}

func (f *fakeSession) RegisterTool(t sdk.Tool) error {
	if f.registerErr != nil {
		return f.registerErr
	}
	f.registered = append(f.registered, t.Definition().Name)
	return nil
}

func (f *fakeSession) UnregisterTool(name string) error {
	f.unregistered = append(f.unregistered, name)
	return nil
}

func TestLastAssistantTextExtractsTrailingAssistantMessage(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{types.TextContent{Text: "ignored"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextContent{Text: "first"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextContent{Text: "last"}}},
	}
	if got := lastAssistantText(messages); got != "last" {
		t.Fatalf("expected %q, got %q", "last", got)
	}
}

func TestLastAssistantTextEmptyForNoAssistantMessage(t *testing.T) {
	if got := lastAssistantText([]types.Message{{Role: types.RoleUser}}); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestSessionAgentCallerReturnsAssistantText(t *testing.T) {
	session := &fakeSession{responses: []string{"the answer"}}
	caller := sessionAgentCaller{session: session}
	got, err := caller.Ask(context.Background(), "inbox", "what is the answer?", nil, nil)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if got != "the answer" {
		t.Fatalf("expected %q, got %q", "the answer", got)
	}
	if len(session.calls) != 1 || session.calls[0] != "what is the answer?" {
		t.Fatalf("unexpected calls: %#v", session.calls)
	}
}

func TestSessionAgentCallerFailsLoudlyOnToolsOverride(t *testing.T) {
	session := &fakeSession{responses: []string{"should not be reached"}}
	caller := sessionAgentCaller{session: session}
	if _, err := caller.Ask(context.Background(), "", "x", []string{"weather_lookup"}, nil); err == nil {
		t.Fatal("expected an error - per-node tool scoping is not implemented yet")
	}
	if len(session.calls) != 0 {
		t.Fatalf("expected no SubmitMessage call when tools scoping is requested, got %#v", session.calls)
	}
}

func TestSessionAgentCallerRegistersAndUnregistersGraphTools(t *testing.T) {
	session := &fakeSession{responses: []string{"used the tool"}}
	caller := sessionAgentCaller{session: session}
	// The fake session's own SubmitMessage never actually calls a
	// registered tool's handler (no real LLM in this test) - this test
	// exercises the register-before/unregister-after lifecycle around that
	// call, not the handler itself (see TestToolSpecInvokeMergesArgsOverParams
	// in pkg/dataflow for that).
	spec := dataflow.ToolSpec{
		Name:        "gmail_send",
		Description: "sends an email",
		Schema:      map[string]any{"type": "object"},
		Invoke: func(_ context.Context, args map[string]any) (string, error) {
			return "sent", nil
		},
	}
	got, err := caller.Ask(context.Background(), "", "send the email", nil, []dataflow.ToolSpec{spec})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if got != "used the tool" {
		t.Fatalf("expected %q, got %q", "used the tool", got)
	}
	if len(session.registered) != 1 || session.registered[0] != "gmail_send" {
		t.Fatalf("expected gmail_send to be registered, got %#v", session.registered)
	}
	if len(session.unregistered) != 1 || session.unregistered[0] != "gmail_send" {
		t.Fatalf("expected gmail_send to be unregistered after the call, got %#v", session.unregistered)
	}
}

func TestSessionAgentCallerPropagatesError(t *testing.T) {
	session := &fakeSession{err: errors.New("boom")}
	caller := sessionAgentCaller{session: session}
	if _, err := caller.Ask(context.Background(), "", "x", nil, nil); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestSessionSubworkflowRunnerExecutesEachNode(t *testing.T) {
	session := &fakeSession{responses: []string{"step one done", "step two done"}}
	runner := sessionSubworkflowRunner{session: session}
	def := workflow.Definition{Name: "chain", Nodes: []workflow.Node{
		{ID: "a", Prompt: "do step one"},
		{ID: "b", Prompt: "do step two", Needs: []string{"a"}},
	}}
	result, err := runner.Run(context.Background(), def)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, results=%#v", result.Results)
	}
	if result.Results["a"].Output != "step one done" {
		t.Fatalf("unexpected node a output: %#v", result.Results["a"])
	}
	if len(session.calls) != 2 {
		t.Fatalf("expected 2 SubmitMessage calls, got %d", len(session.calls))
	}
}

func TestRunGraphExecutesQueryNode(t *testing.T) {
	registry := dataflow.NewRegistry()
	dataflow.RegisterBuiltins(registry)
	session := &fakeSession{responses: []string{"drafted"}}

	graph := &dataflow.Definition{Name: "g", Nodes: []dataflow.Node{
		{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
		{ID: "draft", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "draft a reply"}},
	}}
	if err := runGraph(context.Background(), graph, registry, nil, session); err != nil {
		t.Fatalf("run graph: %v", err)
	}
}

func TestRunGraphExecutesDeterministicNode(t *testing.T) {
	registry := dataflow.NewRegistry()
	nodes.Register(registry, nil)

	graph := &dataflow.Definition{Name: "g", Nodes: []dataflow.Node{
		{ID: "wait", Type: "wait", Parameters: map[string]any{"seconds": 1}},
	}}
	if err := runGraph(context.Background(), graph, registry, nil, &fakeSession{}); err != nil {
		t.Fatalf("run graph: %v", err)
	}
}

func TestRunGraphFailsClearlyWithoutRegistry(t *testing.T) {
	graph := &dataflow.Definition{Name: "g", Nodes: []dataflow.Node{{ID: "a", Type: "agent"}}}
	err := runGraph(context.Background(), graph, nil, nil, &fakeSession{})
	if err == nil {
		t.Fatal("expected error when NodeRegistry is nil")
	}
}

func TestRunGraphSurfacesNodeFailure(t *testing.T) {
	registry := dataflow.NewRegistry()
	dataflow.RegisterBuiltins(registry)
	// "query" node with no prompt fails ValidateParameters.
	graph := &dataflow.Definition{Name: "g", Nodes: []dataflow.Node{
		{ID: "id", Type: "agent"},
		{ID: "bad", Type: "query", Agent: "id"},
	}}
	err := runGraph(context.Background(), graph, registry, nil, &fakeSession{})
	if err == nil {
		t.Fatal("expected error for invalid query node parameters")
	}
}

func TestJobWorkflowRunUsesGraphWhenSet(t *testing.T) {
	registry := dataflow.NewRegistry()
	dataflow.RegisterBuiltins(registry)
	session := &fakeSession{responses: []string{"ok"}}

	job := &Job{
		ID: "job-1",
		Graph: &dataflow.Definition{Name: "g", Nodes: []dataflow.Node{
			{ID: "id", Type: "agent", Parameters: map[string]any{"agent": "inbox"}},
			{ID: "a", Type: "query", Agent: "id", Parameters: map[string]any{"prompt": "go"}},
		}},
		Task: "this should be ignored since Graph is set",
	}
	wf := &jobWorkflow{job: job, registry: registry}
	if err := wf.run(context.Background(), session); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(session.calls) != 1 || session.calls[0] != "go" {
		t.Fatalf("expected the graph's query node prompt to be submitted, got %#v", session.calls)
	}
}

func TestJobWorkflowRunFallsBackToTaskWhenNoGraph(t *testing.T) {
	session := &fakeSession{responses: []string{"ok"}}
	job := &Job{ID: "job-1", Task: "plain task"}
	wf := &jobWorkflow{job: job}
	if err := wf.run(context.Background(), session); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(session.calls) != 1 || session.calls[0] != "plain task" {
		t.Fatalf("expected the flat Task to be submitted, got %#v", session.calls)
	}
}
