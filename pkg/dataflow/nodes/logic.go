package nodes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
	"github.com/KPO-Tech/seshat/pkg/dataflow/expr"
)

// logicBindings builds the bindings map for a per-item expression in
// filter/if/switch - these nodes evaluate their whole parameter field as JS
// (not the "=" convention ResolveValue looks for), so they don't call
// ResolveValue themselves, but still get the same enriched bindings
// (Tier 2.1/3.2) it would have offered: $itemIndex (the loop already has
// it), $now/$today, $node and $vars (both nil-safe: a nil rt yields a
// working always-empty accessor/map, matching how these nodes already
// tolerate a nil Runtime today).
func logicBindings(rt *dataflow.Runtime, item dataflow.Item, itemIndex int) map[string]any {
	now := time.Now()
	return map[string]any{
		"$json":      map[string]any(item),
		"$itemIndex": itemIndex,
		"$now":       now,
		"$today":     now.Truncate(24 * time.Hour),
		"$node":      rt.NodeAccessor(),
		"$vars":      rt.VarsMap(),
	}
}

// Filter is the "filter" node type: keeps only items where expression
// evaluates true, evaluated per item with $json bound to that item — same
// goja/"$"-prefixed convention as internal/inbox/event_filter.go's $event,
// so an LLM authoring a graph reuses one expression language across the SDK
// instead of two.
type Filter struct{ pool *expr.Pool }

func NewFilter(pool *expr.Pool) *Filter { return &Filter{pool: pool} }

func (n *Filter) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "filter", Name: "Filter", Category: "Logic",
		Description: "Keeps only items matching a boolean expression, dropping the rest — all kept items continue on the \"main\" port. " +
			"Parameters: expression (string, required) — a JS boolean expression with the current item bound as $json, e.g. \"$json.subject.includes('invoice')\".",
		Properties: []dataflow.NodeProperty{
			{Name: "expression", DisplayName: "Expression", Type: dataflow.PropText, Required: true,
				Placeholder: "$json.subject.includes('invoice')",
				Description: "JS boolean expression, current item bound as $json. Same convention as a \"=\"-prefixed value in a Set node's fields."},
		}}
}

func (n *Filter) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "expression", "") == "" {
		return errors.New("expression is required")
	}
	return nil
}

func (n *Filter) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	expression := dataflow.StringParam(params, "expression", "")
	kept := make([]dataflow.Item, 0, len(input))
	for i, item := range input {
		ok, err := n.pool.EvalBool(ctx, expression, logicBindings(rt, item, i))
		if err != nil {
			return dataflow.Output{}, fmt.Errorf("filter: %w", err)
		}
		if ok {
			kept = append(kept, item)
		}
	}
	return dataflow.Main(kept), nil
}

// If is the "if" node type: splits items into "true"/"false" output ports
// by expression — unlike Filter (which drops non-matching items), both
// branches are wired to different downstream nodes.
type If struct{ pool *expr.Pool }

func NewIf(pool *expr.Pool) *If { return &If{pool: pool} }

func (n *If) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "if", Name: "If", Category: "Logic",
		Description: "Routes each item to the \"true\" or \"false\" output port by a boolean expression — wire connections[\"true\"] and/or connections[\"false\"] to different downstream node ids to actually branch. " +
			"Parameters: expression (string, required) — a JS boolean expression with the current item bound as $json.",
		Properties: []dataflow.NodeProperty{
			{Name: "expression", DisplayName: "Expression", Type: dataflow.PropText, Required: true,
				Placeholder: "$json.status === 'ok'",
				Description: "JS boolean expression, current item bound as $json. Same convention as a \"=\"-prefixed value in a Set node's fields."},
		}}
}

func (n *If) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "expression", "") == "" {
		return errors.New("expression is required")
	}
	return nil
}

func (n *If) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	expression := dataflow.StringParam(params, "expression", "")
	var trueItems, falseItems []dataflow.Item
	for i, item := range input {
		ok, err := n.pool.EvalBool(ctx, expression, logicBindings(rt, item, i))
		if err != nil {
			return dataflow.Output{}, fmt.Errorf("if: %w", err)
		}
		if ok {
			trueItems = append(trueItems, item)
		} else {
			falseItems = append(falseItems, item)
		}
	}
	return dataflow.Output{Ports: map[string][]dataflow.Item{"true": trueItems, "false": falseItems}}, nil
}

// Switch is the "switch" node type: like If but with more than two
// branches. Parameters: cases (map[string]string, port name -> boolean
// expression), evaluated in the order given by casesOrder ([]string, since
// map iteration order isn't stable and case order matters — first match
// wins). An item matching no case routes to the "default" port.
type Switch struct{ pool *expr.Pool }

func NewSwitch(pool *expr.Pool) *Switch { return &Switch{pool: pool} }

func (n *Switch) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "switch", Name: "Switch", Category: "Logic",
		Description: "Routes each item to the port of the first matching case (evaluated in casesOrder), or to the \"default\" port if none match — wire connections[<case name>] and connections[\"default\"] to downstream node ids. " +
			"Parameters: casesOrder (array of string, required) — case names, checked in this order. cases (object of string->string, required) — case name -> JS boolean expression, current item bound as $json.",
		// casesOrder/cases are a paired, repeatable structure this flat
		// property system doesn't model well as separate fields (case
		// count/names aren't known ahead of time) - declared as two JSON
		// fields rather than forced into single-value form controls, still
		// a real improvement over the whole node being one JSON blob.
		Properties: []dataflow.NodeProperty{
			{Name: "casesOrder", DisplayName: "Case order", Type: dataflow.PropJSON, Required: true,
				Description: "Array of case names, checked in this order, e.g. [\"urgent\", \"spam\"]."},
			{Name: "cases", DisplayName: "Cases", Type: dataflow.PropJSON, Required: true,
				Description: "Object of case name -> JS boolean expression, current item bound as $json."},
		}}
}

func (n *Switch) ValidateParameters(params map[string]any) error {
	order, ok := params["casesOrder"].([]any)
	if !ok || len(order) == 0 {
		return errors.New("casesOrder (non-empty list of case names) is required")
	}
	cases, ok := params["cases"].(map[string]any)
	if !ok {
		return errors.New("cases (map of case name to expression) is required")
	}
	for _, name := range order {
		key, _ := name.(string)
		if _, ok := cases[key]; !ok {
			return fmt.Errorf("casesOrder references unknown case %q", key)
		}
	}
	return nil
}

func (n *Switch) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	orderRaw, _ := params["casesOrder"].([]any)
	cases, _ := params["cases"].(map[string]any)
	order := make([]string, 0, len(orderRaw))
	for _, v := range orderRaw {
		if s, ok := v.(string); ok {
			order = append(order, s)
		}
	}

	ports := map[string][]dataflow.Item{}
	for i, item := range input {
		matched := ""
		for _, caseName := range order {
			expression, _ := cases[caseName].(string)
			ok, err := n.pool.EvalBool(ctx, expression, logicBindings(rt, item, i))
			if err != nil {
				return dataflow.Output{}, fmt.Errorf("switch case %q: %w", caseName, err)
			}
			if ok {
				matched = caseName
				break
			}
		}
		if matched == "" {
			matched = "default"
		}
		ports[matched] = append(ports[matched], item)
	}
	return dataflow.Output{Ports: ports}, nil
}
