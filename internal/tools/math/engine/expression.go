package engine

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Knetic/govaluate"
)

const (
	maxExpArgument       = 700.0
	maxFactorialArgument = 20
)

// ExpressionCalculator evaluates mathematical expressions with variable substitution.
type ExpressionCalculator struct{}

// NewExpressionCalculator creates a new ExpressionCalculator.
func NewExpressionCalculator() *ExpressionCalculator {
	return &ExpressionCalculator{}
}

// Evaluate parses and evaluates a mathematical expression.
func (ec *ExpressionCalculator) Evaluate(req ExpressionRequest) (CalculationResult, error) {
	if strings.TrimSpace(req.Expression) == "" {
		return CalculationResult{}, fmt.Errorf("expression cannot be empty")
	}

	expr, err := govaluate.NewEvaluableExpressionWithFunctions(req.Expression, ec.mathFunctions())
	if err != nil {
		return CalculationResult{}, fmt.Errorf("invalid expression: %v", err)
	}

	parameters := map[string]interface{}{
		"pi": math.Pi,
		"e":  math.E,
		"PI": math.Pi,
		"E":  math.E,
	}

	if req.Variables != nil {
		for key, value := range req.Variables {
			if !ec.isValidVariableName(key) {
				return CalculationResult{}, fmt.Errorf("invalid variable name: %s", key)
			}
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return CalculationResult{}, fmt.Errorf("invalid variable value for %s: %f", key, value)
			}
			parameters[key] = value
		}
	}

	result, err := expr.Evaluate(parameters)
	if err != nil {
		return CalculationResult{}, fmt.Errorf("evaluation error: %v", err)
	}

	var floatResult float64
	switch v := result.(type) {
	case float64:
		floatResult = v
	case int:
		floatResult = float64(v)
	case int64:
		floatResult = float64(v)
	default:
		return CalculationResult{}, fmt.Errorf("unexpected result type: %T", result)
	}

	if math.IsNaN(floatResult) {
		return CalculationResult{}, fmt.Errorf("expression resulted in NaN")
	}
	if math.IsInf(floatResult, 0) {
		return CalculationResult{}, fmt.Errorf("expression resulted in infinity")
	}

	return CalculationResult{Result: floatResult}, nil
}

// ExtractVariables returns user-defined variable names found in the expression.
func (ec *ExpressionCalculator) ExtractVariables(expression string) ([]string, error) {
	if strings.TrimSpace(expression) == "" {
		return []string{}, nil
	}

	builtIn := map[string]bool{
		"pi": true, "PI": true, "e": true, "E": true,
		"sin": true, "cos": true, "tan": true, "asin": true, "acos": true, "atan": true,
		"log": true, "ln": true, "abs": true, "sqrt": true, "pow": true, "exp": true, "factorial": true,
	}

	pattern := regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\b`)
	matches := pattern.FindAllString(expression, -1)

	seen := make(map[string]bool)
	var vars []string
	for _, m := range matches {
		if builtIn[m] || seen[m] {
			continue
		}
		if _, err := strconv.ParseFloat(m, 64); err == nil {
			continue
		}
		seen[m] = true
		vars = append(vars, m)
	}
	sort.Strings(vars)
	return vars, nil
}

func (ec *ExpressionCalculator) isValidVariableName(name string) bool {
	if len(name) == 0 {
		return false
	}
	c := name[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return false
	}
	for i := 1; i < len(name); i++ {
		ch := name[i]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	reserved := []string{"pi", "e", "PI", "E", "sin", "cos", "tan", "asin", "acos", "atan", "log", "ln", "abs", "pow", "exp", "sqrt", "factorial"}
	nameLower := strings.ToLower(name)
	for _, r := range reserved {
		if nameLower == strings.ToLower(r) {
			return false
		}
	}
	return true
}

func (ec *ExpressionCalculator) mathFunctions() map[string]govaluate.ExpressionFunction {
	fn1 := func(name string, f func(float64) float64) govaluate.ExpressionFunction {
		return func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("%s expects 1 argument", name)
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("%s expects numeric argument", name)
			}
			return f(v), nil
		}
	}

	fns := map[string]govaluate.ExpressionFunction{
		"sin":  fn1("sin", math.Sin),
		"cos":  fn1("cos", math.Cos),
		"tan":  fn1("tan", math.Tan),
		"atan": fn1("atan", math.Atan),
		"abs":  fn1("abs", math.Abs),
		"asin": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("asin expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("asin expects numeric argument")
			}
			if v < -1 || v > 1 {
				return nil, fmt.Errorf("asin domain error: argument must be between -1 and 1")
			}
			return math.Asin(v), nil
		},
		"acos": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("acos expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("acos expects numeric argument")
			}
			if v < -1 || v > 1 {
				return nil, fmt.Errorf("acos domain error: argument must be between -1 and 1")
			}
			return math.Acos(v), nil
		},
		"log": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("log expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("log expects numeric argument")
			}
			if v <= 0 {
				return nil, fmt.Errorf("log domain error: argument must be positive")
			}
			return math.Log10(v), nil
		},
		"ln": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("ln expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("ln expects numeric argument")
			}
			if v <= 0 {
				return nil, fmt.Errorf("ln domain error: argument must be positive")
			}
			return math.Log(v), nil
		},
		"sqrt": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("sqrt expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("sqrt expects numeric argument")
			}
			if v < 0 {
				return nil, fmt.Errorf("sqrt domain error: argument must be non-negative")
			}
			return math.Sqrt(v), nil
		},
		"exp": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("exp expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("exp expects numeric argument")
			}
			if v > maxExpArgument {
				return nil, fmt.Errorf("exp overflow: value too large")
			}
			return math.Exp(v), nil
		},
		"pow": func(args ...interface{}) (interface{}, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("pow expects 2 arguments")
			}
			base, ok1 := args[0].(float64)
			exp, ok2 := args[1].(float64)
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("pow expects numeric arguments")
			}
			if base == 0 && exp < 0 {
				return nil, fmt.Errorf("pow: 0 raised to negative power")
			}
			r := math.Pow(base, exp)
			if math.IsNaN(r) || math.IsInf(r, 0) {
				return nil, fmt.Errorf("pow resulted in invalid value")
			}
			return r, nil
		},
		"factorial": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("factorial expects 1 argument")
			}
			v, ok := args[0].(float64)
			if !ok {
				return nil, fmt.Errorf("factorial expects numeric argument")
			}
			if v < 0 {
				return nil, fmt.Errorf("factorial domain error: argument must be non-negative")
			}
			intVal := int(v)
			if v != float64(intVal) {
				return nil, fmt.Errorf("factorial domain error: argument must be an integer")
			}
			if intVal > maxFactorialArgument {
				return nil, fmt.Errorf("factorial overflow: argument must be ≤ %d", maxFactorialArgument)
			}
			result := 1.0
			for i := 2; i <= intVal; i++ {
				result *= float64(i)
			}
			return result, nil
		},
	}
	return fns
}
