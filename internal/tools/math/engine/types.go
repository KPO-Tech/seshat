package engine

// BasicMathRequest is the input for basic arithmetic operations.
type BasicMathRequest struct {
	Operation string    `json:"operation"`
	Operands  []float64 `json:"operands"`
	Precision int       `json:"precision,omitempty"`
}

// AdvancedMathRequest is the input for advanced mathematical functions.
type AdvancedMathRequest struct {
	Function string  `json:"function"`
	Value    float64 `json:"value"`
	Exponent float64 `json:"exponent,omitempty"`
	Unit     string  `json:"unit,omitempty"`
}

// ExpressionRequest is the input for expression evaluation.
type ExpressionRequest struct {
	Expression string             `json:"expression"`
	Variables  map[string]float64 `json:"variables,omitempty"`
}

// StatisticsRequest is the input for statistical operations.
type StatisticsRequest struct {
	Data      []float64 `json:"data"`
	Operation string    `json:"operation"`
}

// UnitConversionRequest is the input for unit conversions.
type UnitConversionRequest struct {
	Value    float64 `json:"value"`
	FromUnit string  `json:"fromUnit"`
	ToUnit   string  `json:"toUnit"`
	Category string  `json:"category"`
}

// FinancialRequest is the input for financial calculations.
type FinancialRequest struct {
	Operation   string  `json:"operation"`
	Principal   float64 `json:"principal,omitempty"`
	Rate        float64 `json:"rate,omitempty"`
	Time        float64 `json:"time,omitempty"`
	Periods     int     `json:"periods,omitempty"`
	FutureValue float64 `json:"futureValue,omitempty"`
}

// CalculationResult is the result of a numeric calculation.
type CalculationResult struct {
	Result float64 `json:"result"`
	Unit   string  `json:"unit,omitempty"`
}

// StatisticsResult is the result of a statistical operation.
type StatisticsResult struct {
	Result interface{} `json:"result"`
	Count  int         `json:"count"`
}

// FinancialResult is the result of a financial calculation.
type FinancialResult struct {
	Result      float64                `json:"result"`
	Breakdown   map[string]interface{} `json:"breakdown,omitempty"`
	Description string                 `json:"description,omitempty"`
}
