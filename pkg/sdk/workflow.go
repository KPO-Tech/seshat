package sdk

import (
	"context"

	"github.com/KPO-Tech/seshat/pkg/workflow"
)

// WorkflowDefinition is a workflow: a named set of nodes, each one a prompt for an agent, linked by the nodes they need. It is read from YAML or JSON (see LoadFile).
type WorkflowDefinition = workflow.Definition

// WorkflowNode is one step of a workflow: a prompt for an agent, run once the nodes listed in Needs have succeeded, and whose output is given to the nodes that need it.\nKind gives the node a role: "agent" (or empty) for a plain step, "verifier", "critic", or "router", a node that chooses among the nodes listed in Routes (at least one is required). MaxTurns is the number of turns the executor should allow the agent of the node, 0 leaving it to the executor, and OutputFormat describes the form the output should take.
type WorkflowNode = workflow.Node

// WorkflowResult is the outcome of a workflow run: whether every node succeeded, the result of each node by id, and the order in which the nodes that ran finished. A node that did not run because a dependency failed has a result with Success false and no entry in Order.
type WorkflowResult = workflow.Result

// WorkflowNodeResult is the outcome of one node: whether it succeeded, its output or its error, and when it ran.
type WorkflowNodeResult = workflow.NodeResult

// WorkflowDraftOptions asks for a workflow definition to be normalised, validated and rendered. Format is "yaml" (the default, also "yml") or "json".
type WorkflowDraftOptions = workflow.DraftOptions

// WorkflowDraftResult is a normalised definition and its rendering in Format. Diagnostics holds the reason when the definition is invalid or the format is not supported.
type WorkflowDraftResult = workflow.DraftResult

// WorkflowOptions tunes Client.RunWorkflow: how many nodes may run at once, the tools given to the default executor, and an Executor to use instead of the default one, which asks the model.
type WorkflowOptions struct {
	MaxParallel int
	Tools       []Tool
	Executor    workflow.Executor
}

// LoadWorkflowFile reads a workflow definition from a file: JSON when the extension is .json, YAML otherwise.
func LoadWorkflowFile(path string) (WorkflowDefinition, error) {
	return workflow.LoadFile(path)
}

// ValidateWorkflow checks a definition: it has at least one node, every node has a unique id and a prompt and a supported kind, a router declares routes, the nodes it needs and routes to exist (a router cannot route to itself), and the dependencies contain no cycle.
func ValidateWorkflow(def WorkflowDefinition) error {
	return workflow.Validate(def)
}

// DraftWorkflow normalises and validates a definition, then renders it as YAML or JSON. When it fails, it returns the error together with a result whose Diagnostics holds its message.
func DraftWorkflow(options WorkflowDraftOptions) (WorkflowDraftResult, error) {
	return workflow.Draft(options)
}

// BuildWorkflowNodePrompt builds the prompt of a node for its agent: the role of its kind (verifier, critic, router), the outputs of the nodes it needs, the prompt of the node and its output format.
func BuildWorkflowNodePrompt(node WorkflowNode, inputs map[string]WorkflowNodeResult) string {
	return workflow.BuildNodePrompt(node, inputs)
}

// RunWorkflow runs def. Each node is executed by options.Executor or, when none is given, by asking the model with the prompt of the node and the outputs of the nodes it needs (Ask, with options.Tools).
func (c *Client) RunWorkflow(ctx context.Context, def WorkflowDefinition, options WorkflowOptions) (WorkflowResult, error) {
	executor := options.Executor
	if executor == nil {
		executor = workflow.ExecutorFunc(func(execCtx context.Context, node workflow.Node, inputs map[string]workflow.NodeResult) (string, error) {
			response, err := c.Ask(execCtx, workflow.BuildNodePrompt(node, inputs), options.Tools)
			if err != nil {
				return "", err
			}
			return response.Content, nil
		})
	}
	return workflow.Run(ctx, def, executor, workflow.Options{MaxParallel: options.MaxParallel})
}
