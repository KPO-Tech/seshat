package engine

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// BasicCalculator performs arithmetic operations with decimal precision.
type BasicCalculator struct{}

// NewBasicCalculator creates a new BasicCalculator.
func NewBasicCalculator() *BasicCalculator {
	return &BasicCalculator{}
}

// Calculate executes the requested arithmetic operation.
func (bc *BasicCalculator) Calculate(req BasicMathRequest) (CalculationResult, error) {
	if len(req.Operands) < 2 {
		return CalculationResult{}, fmt.Errorf("at least 2 operands required")
	}

	var result float64
	var err error

	switch req.Operation {
	case "add":
		result = bc.add(req.Operands)
	case "subtract":
		result = bc.subtract(req.Operands)
	case "multiply":
		result = bc.multiply(req.Operands)
	case "divide":
		result, err = bc.divide(req.Operands)
		if err != nil {
			return CalculationResult{}, err
		}
	default:
		return CalculationResult{}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	result = bc.roundToPrecision(result, req.Precision)
	return CalculationResult{Result: result}, nil
}

func (bc *BasicCalculator) add(operands []float64) float64 {
	result := decimal.NewFromFloat(operands[0])
	for i := 1; i < len(operands); i++ {
		result = result.Add(decimal.NewFromFloat(operands[i]))
	}
	f, _ := result.Float64()
	return f
}

func (bc *BasicCalculator) subtract(operands []float64) float64 {
	result := decimal.NewFromFloat(operands[0])
	for i := 1; i < len(operands); i++ {
		result = result.Sub(decimal.NewFromFloat(operands[i]))
	}
	f, _ := result.Float64()
	return f
}

func (bc *BasicCalculator) multiply(operands []float64) float64 {
	result := decimal.NewFromFloat(operands[0])
	for i := 1; i < len(operands); i++ {
		result = result.Mul(decimal.NewFromFloat(operands[i]))
	}
	f, _ := result.Float64()
	return f
}

func (bc *BasicCalculator) divide(operands []float64) (float64, error) {
	for i := 1; i < len(operands); i++ {
		if operands[i] == 0 {
			return 0, fmt.Errorf("division by zero")
		}
	}
	result := decimal.NewFromFloat(operands[0])
	for i := 1; i < len(operands); i++ {
		result = result.Div(decimal.NewFromFloat(operands[i]))
	}
	f, _ := result.Float64()
	return f, nil
}

func (bc *BasicCalculator) roundToPrecision(value float64, precision int) float64 {
	multiplier := math.Pow(10, float64(precision))
	return math.Round(value*multiplier) / multiplier
}
