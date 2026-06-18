package engine

import (
	"fmt"
	"math"
)

// AdvancedCalculator performs advanced mathematical functions.
type AdvancedCalculator struct{}

// NewAdvancedCalculator creates a new AdvancedCalculator.
func NewAdvancedCalculator() *AdvancedCalculator {
	return &AdvancedCalculator{}
}

// Calculate executes the requested mathematical function.
func (ac *AdvancedCalculator) Calculate(req AdvancedMathRequest) (CalculationResult, error) {
	var result float64
	var err error

	value := req.Value
	if ac.isTrigFunction(req.Function) && req.Unit == "degrees" {
		value = degreesToRadians(value)
	}

	switch req.Function {
	case "sin":
		result = math.Sin(value)
	case "cos":
		result = math.Cos(value)
	case "tan":
		result = math.Tan(value)
	case "asin":
		if value < -1 || value > 1 {
			return CalculationResult{}, fmt.Errorf("asin domain error: value must be between -1 and 1")
		}
		result = math.Asin(value)
		if req.Unit == "degrees" {
			result = radiansToDegrees(result)
		}
	case "acos":
		if value < -1 || value > 1 {
			return CalculationResult{}, fmt.Errorf("acos domain error: value must be between -1 and 1")
		}
		result = math.Acos(value)
		if req.Unit == "degrees" {
			result = radiansToDegrees(result)
		}
	case "atan":
		result = math.Atan(value)
		if req.Unit == "degrees" {
			result = radiansToDegrees(result)
		}
	case "log", "log10":
		if value <= 0 {
			return CalculationResult{}, fmt.Errorf("log domain error: value must be positive")
		}
		result = math.Log10(value)
	case "ln":
		if value <= 0 {
			return CalculationResult{}, fmt.Errorf("ln domain error: value must be positive")
		}
		result = math.Log(value)
	case "sqrt":
		if value < 0 {
			return CalculationResult{}, fmt.Errorf("sqrt domain error: value must be non-negative")
		}
		result = math.Sqrt(value)
	case "abs":
		result = math.Abs(value)
	case "factorial":
		if value < 0 {
			return CalculationResult{}, fmt.Errorf("factorial domain error: value must be non-negative")
		}
		if value != math.Floor(value) {
			return CalculationResult{}, fmt.Errorf("factorial domain error: value must be an integer")
		}
		if value > 170 {
			return CalculationResult{}, fmt.Errorf("factorial overflow: value too large (max 170)")
		}
		result, err = factorial(int(value))
		if err != nil {
			return CalculationResult{}, err
		}
	case "exp":
		if value > 700 {
			return CalculationResult{}, fmt.Errorf("exponential overflow: value too large")
		}
		result = math.Exp(value)
	case "pow":
		result, err = power(value, req.Exponent)
		if err != nil {
			return CalculationResult{}, err
		}
	default:
		return CalculationResult{}, fmt.Errorf("unsupported function: %s", req.Function)
	}

	if math.IsNaN(result) {
		return CalculationResult{}, fmt.Errorf("calculation resulted in NaN")
	}
	if math.IsInf(result, 0) {
		return CalculationResult{}, fmt.Errorf("calculation resulted in infinity")
	}

	return CalculationResult{Result: result}, nil
}

// Power computes base^exponent with domain checks.
func power(base, exponent float64) (float64, error) {
	if base == 0 && exponent < 0 {
		return 0, fmt.Errorf("division by zero: 0 raised to negative power")
	}
	if base < 0 && exponent != math.Floor(exponent) {
		return 0, fmt.Errorf("complex result: negative base with non-integer exponent")
	}
	result := math.Pow(base, exponent)
	if math.IsNaN(result) {
		return 0, fmt.Errorf("calculation resulted in NaN")
	}
	if math.IsInf(result, 0) {
		return 0, fmt.Errorf("calculation resulted in infinity")
	}
	return result, nil
}

func factorial(n int) (float64, error) {
	if n < 0 {
		return 0, fmt.Errorf("factorial of negative number")
	}
	result := 1.0
	for i := 2; i <= n; i++ {
		result *= float64(i)
	}
	return result, nil
}

func degreesToRadians(degrees float64) float64 { return degrees * (math.Pi / 180) }
func radiansToDegrees(radians float64) float64 { return radians * (180 / math.Pi) }

func (ac *AdvancedCalculator) isTrigFunction(fn string) bool {
	switch fn {
	case "sin", "cos", "tan":
		return true
	}
	return false
}
