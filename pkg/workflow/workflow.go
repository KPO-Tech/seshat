package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Definition is a workflow: a named set of nodes, each one a prompt for an agent, linked by the nodes they need. It is read from YAML or JSON (see LoadFile).
type Definition struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Nodes       []Node `json:"nodes" yaml:"nodes"`
}

// Node is one step of a workflow: a prompt for an agent, run once the nodes listed in Needs have succeeded, and whose output is given to the nodes that need it.\nKind gives the node a role: "agent" (or empty) for a plain step, "verifier", "critic", or "router", a node that chooses among the nodes listed in Routes (at least one is required). MaxTurns is the number of turns the executor should allow the agent of the node, 0 leaving it to the executor, and OutputFormat describes the form the output should take.
type Node struct {
	ID           string   `json:"id" yaml:"id"`
	Kind         string   `json:"kind,omitempty" yaml:"kind,omitempty"`
	Agent        string   `json:"agent,omitempty" yaml:"agent,omitempty"`
	Prompt       string   `json:"prompt" yaml:"prompt"`
	Needs        []string `json:"needs,omitempty" yaml:"needs,omitempty"`
	Routes       []string `json:"routes,omitempty" yaml:"routes,omitempty"`
	MaxTurns     int      `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	OutputFormat string   `json:"output_format,omitempty" yaml:"output_format,omitempty"`
}

// NodeResult is the outcome of one node: whether it succeeded, its output or its error, and when it ran.
type NodeResult struct {
	ID        string        `json:"id"`
	Agent     string        `json:"agent,omitempty"`
	Success   bool          `json:"success"`
	Output    string        `json:"output,omitempty"`
	Error     string        `json:"error,omitempty"`
	StartedAt time.Time     `json:"started_at"`
	EndedAt   time.Time     `json:"ended_at"`
	Duration  time.Duration `json:"duration"`
}

// Result is the outcome of a workflow run: whether every node succeeded, the result of each node by id, and the order in which the nodes that ran finished. A node that did not run because a dependency failed has a result with Success false and no entry in Order.
type Result struct {
	Name      string                `json:"name"`
	Success   bool                  `json:"success"`
	StartedAt time.Time             `json:"started_at"`
	EndedAt   time.Time             `json:"ended_at"`
	Duration  time.Duration         `json:"duration"`
	Results   map[string]NodeResult `json:"results"`
	Order     []string              `json:"order"`
}

// Executor runs one node of a workflow. inputs holds the results of the nodes it needs; it returns the output of the node, or an error that makes the node fail.
type Executor interface {
	ExecuteNode(ctx context.Context, node Node, inputs map[string]NodeResult) (string, error)
}

// ExecutorFunc is a function used as an Executor.
type ExecutorFunc func(ctx context.Context, node Node, inputs map[string]NodeResult) (string, error)

// ExecuteNode calls f.
func (f ExecutorFunc) ExecuteNode(ctx context.Context, node Node, inputs map[string]NodeResult) (string, error) {
	return f(ctx, node, inputs)
}

// Options tunes a workflow run. MaxParallel bounds how many nodes of the same level run at once; a value of 0 or less means 4.
type Options struct {
	MaxParallel int
}

// LoadFile reads a workflow definition from a file: JSON when the extension is .json, YAML otherwise.
func LoadFile(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	var def Definition
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(data, &def)
	default:
		err = yaml.Unmarshal(data, &def)
	}
	if err != nil {
		return Definition{}, err
	}
	return def, nil
}

// Validate checks a definition: it has at least one node, every node has a unique id and a prompt and a supported kind, a router declares routes, the nodes it needs and routes to exist (a router cannot route to itself), and the dependencies contain no cycle.
func Validate(def Definition) error {
	if len(def.Nodes) == 0 {
		return errors.New("workflow must contain at least one node")
	}
	seen := map[string]bool{}
	for _, node := range def.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return errors.New("workflow node id is required")
		}
		if seen[node.ID] {
			return fmt.Errorf("duplicate workflow node id %q", node.ID)
		}
		seen[node.ID] = true
		if strings.TrimSpace(node.Prompt) == "" {
			return fmt.Errorf("workflow node %q prompt is required", node.ID)
		}
		if err := validateNodeKind(node); err != nil {
			return err
		}
	}
	for _, node := range def.Nodes {
		for _, dep := range node.Needs {
			if !seen[dep] {
				return fmt.Errorf("workflow node %q depends on unknown node %q", node.ID, dep)
			}
		}
		for _, route := range node.Routes {
			if !seen[route] {
				return fmt.Errorf("workflow router node %q routes to unknown node %q", node.ID, route)
			}
			if route == node.ID {
				return fmt.Errorf("workflow router node %q cannot route to itself", node.ID)
			}
		}
	}
	if _, err := topologicalLevels(def); err != nil {
		return err
	}
	return nil
}

// Run executes the workflow with executor: the nodes are grouped in levels by their dependencies, the levels run one after the other and the nodes of a level in parallel, at most Options.MaxParallel at a time. A node whose dependency failed is not run. The returned error reports an invalid definition or a missing executor; a failing node shows in the Result, not in the error.
func Run(ctx context.Context, def Definition, executor Executor, options Options) (Result, error) {
	if executor == nil {
		return Result{}, errors.New("workflow executor is required")
	}
	if err := Validate(def); err != nil {
		return Result{}, err
	}
	if options.MaxParallel <= 0 {
		options.MaxParallel = 4
	}
	levels, err := topologicalLevels(def)
	if err != nil {
		return Result{}, err
	}

	started := time.Now()
	result := Result{
		Name:      def.Name,
		Success:   true,
		StartedAt: started,
		Results:   map[string]NodeResult{},
	}
	nodes := map[string]Node{}
	for _, node := range def.Nodes {
		nodes[node.ID] = node
	}

	for _, level := range levels {
		sem := make(chan struct{}, options.MaxParallel)
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, id := range level {
			node := nodes[id]
			inputs := make(map[string]NodeResult, len(node.Needs))
			skip := false
			for _, dep := range node.Needs {
				depResult := result.Results[dep]
				if !depResult.Success {
					now := time.Now()
					result.Results[node.ID] = NodeResult{
						ID:        node.ID,
						Agent:     node.Agent,
						Success:   false,
						Error:     fmt.Sprintf("dependency %q failed", dep),
						StartedAt: now,
						EndedAt:   now,
					}
					result.Success = false
					skip = true
					break
				}
				inputs[dep] = depResult
			}
			if skip {
				continue
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(node Node, inputs map[string]NodeResult) {
				defer wg.Done()
				defer func() { <-sem }()
				nodeStarted := time.Now()
				output, execErr := executor.ExecuteNode(ctx, node, inputs)
				nodeEnded := time.Now()
				nodeResult := NodeResult{
					ID:        node.ID,
					Agent:     node.Agent,
					Success:   execErr == nil,
					Output:    output,
					StartedAt: nodeStarted,
					EndedAt:   nodeEnded,
					Duration:  nodeEnded.Sub(nodeStarted),
				}
				if execErr != nil {
					nodeResult.Error = execErr.Error()
				}
				mu.Lock()
				result.Results[node.ID] = nodeResult
				result.Order = append(result.Order, node.ID)
				if execErr != nil {
					result.Success = false
				}
				mu.Unlock()
			}(node, inputs)
		}
		wg.Wait()
	}
	result.EndedAt = time.Now()
	result.Duration = result.EndedAt.Sub(result.StartedAt)
	return result, nil
}

// BuildNodePrompt builds the prompt of a node for its agent: the role of its kind (verifier, critic, router), the outputs of the nodes it needs, the prompt of the node and its output format.
func BuildNodePrompt(node Node, inputs map[string]NodeResult) string {
	var b strings.Builder
	writeNodeRolePrompt(&b, node)
	if len(inputs) > 0 {
		keys := make([]string, 0, len(inputs))
		for id := range inputs {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		b.WriteString("Dependency outputs:\n")
		for _, id := range keys {
			b.WriteString("\n[")
			b.WriteString(id)
			b.WriteString("]\n")
			b.WriteString(strings.TrimSpace(inputs[id].Output))
			b.WriteString("\n")
		}
		b.WriteString("\nTask:\n")
	}
	b.WriteString(strings.TrimSpace(node.Prompt))
	if node.OutputFormat != "" {
		b.WriteString("\n\nExpected output format:\n")
		b.WriteString(strings.TrimSpace(node.OutputFormat))
	}
	return b.String()
}

func validateNodeKind(node Node) error {
	switch normalizeNodeKind(node.Kind) {
	case "", "agent", "verifier", "critic":
		return nil
	case "router":
		if len(node.Routes) == 0 {
			return fmt.Errorf("workflow router node %q must declare at least one route", node.ID)
		}
		return nil
	default:
		return fmt.Errorf("workflow node %q has unsupported kind %q", node.ID, node.Kind)
	}
}

func normalizeNodeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func writeNodeRolePrompt(b *strings.Builder, node Node) {
	switch normalizeNodeKind(node.Kind) {
	case "verifier":
		b.WriteString("Role: verifier.\n")
		b.WriteString("Check dependency outputs and the task result criteria. Return PASS with concise evidence, or FAIL with exact issues and fixes.\n\n")
	case "critic":
		b.WriteString("Role: critic.\n")
		b.WriteString("Find weaknesses, missing context, risky assumptions, and concrete improvements. Be direct and prioritize actionable issues.\n\n")
	case "router":
		b.WriteString("Role: router.\n")
		b.WriteString("Choose the best next route(s) from the allowed route ids. Explain the decision briefly and return the chosen route ids first.\n")
		if len(node.Routes) > 0 {
			b.WriteString("Allowed routes: ")
			b.WriteString(strings.Join(node.Routes, ", "))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
}

func topologicalLevels(def Definition) ([][]string, error) {
	deps := map[string]map[string]bool{}
	reverse := map[string][]string{}
	remaining := map[string]bool{}
	for _, node := range def.Nodes {
		remaining[node.ID] = true
		deps[node.ID] = map[string]bool{}
		for _, dep := range node.Needs {
			deps[node.ID][dep] = true
			reverse[dep] = append(reverse[dep], node.ID)
		}
	}
	var levels [][]string
	for len(remaining) > 0 {
		var ready []string
		for id := range remaining {
			if len(deps[id]) == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, errors.New("workflow graph contains a cycle")
		}
		sort.Strings(ready)
		levels = append(levels, ready)
		for _, id := range ready {
			delete(remaining, id)
			for _, child := range reverse[id] {
				delete(deps[child], id)
			}
		}
	}
	return levels, nil
}
