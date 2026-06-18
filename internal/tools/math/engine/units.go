package engine

import (
	"fmt"
	"math"
)

// UnitConverter converts values between different units of measurement.
type UnitConverter struct {
	toBase map[string]map[string]float64 // category → unit → factor to base
}

// NewUnitConverter creates a UnitConverter with all supported conversions.
func NewUnitConverter() *UnitConverter {
	uc := &UnitConverter{}
	uc.init()
	return uc
}

// Convert converts a value from one unit to another within the same category.
func (uc *UnitConverter) Convert(req UnitConversionRequest) (CalculationResult, error) {
	if err := uc.validateRequest(req); err != nil {
		return CalculationResult{}, err
	}

	var result float64
	var err error

	switch req.Category {
	case "temperature":
		result, err = uc.convertTemperature(req.Value, req.FromUnit, req.ToUnit)
	default:
		result, err = uc.convertGeneric(req.Value, req.FromUnit, req.ToUnit, req.Category)
	}
	if err != nil {
		return CalculationResult{}, err
	}
	return CalculationResult{Result: result, Unit: req.ToUnit}, nil
}

// ConvertMultiple converts a slice of values between units.
func (uc *UnitConverter) ConvertMultiple(values []float64, fromUnit, toUnit, category string) ([]float64, error) {
	results := make([]float64, len(values))
	for i, v := range values {
		r, err := uc.Convert(UnitConversionRequest{Value: v, FromUnit: fromUnit, ToUnit: toUnit, Category: category})
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		results[i] = r.Result
	}
	return results, nil
}

// SupportedUnits returns the unit names for a given category.
func (uc *UnitConverter) SupportedUnits(category string) ([]string, error) {
	m, ok := uc.toBase[category]
	if !ok {
		return nil, fmt.Errorf("unsupported category: %s", category)
	}
	if category == "temperature" {
		return []string{"C", "F", "K", "R"}, nil
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names, nil
}

func (uc *UnitConverter) convertGeneric(value float64, fromUnit, toUnit, category string) (float64, error) {
	if fromUnit == toUnit {
		return value, nil
	}
	m, ok := uc.toBase[category]
	if !ok {
		return 0, fmt.Errorf("unsupported category: %s", category)
	}
	fromFactor, ok1 := m[fromUnit]
	toFactor, ok2 := m[toUnit]
	if !ok1 {
		return 0, fmt.Errorf("unsupported unit %q for category %q", fromUnit, category)
	}
	if !ok2 {
		return 0, fmt.Errorf("unsupported unit %q for category %q", toUnit, category)
	}
	return value * fromFactor / toFactor, nil
}

func (uc *UnitConverter) convertTemperature(value float64, fromUnit, toUnit string) (float64, error) {
	if fromUnit == toUnit {
		return value, nil
	}
	// To Celsius first
	var celsius float64
	switch fromUnit {
	case "C":
		celsius = value
	case "F":
		celsius = (value - 32) * 5 / 9
	case "K":
		celsius = value - 273.15
	case "R":
		celsius = (value - 491.67) * 5 / 9
	default:
		return 0, fmt.Errorf("unsupported temperature unit: %s", fromUnit)
	}
	// From Celsius to target
	switch toUnit {
	case "C":
		return celsius, nil
	case "F":
		return celsius*9/5 + 32, nil
	case "K":
		if celsius < -273.15 {
			return 0, fmt.Errorf("temperature below absolute zero")
		}
		return celsius + 273.15, nil
	case "R":
		if celsius < -273.15 {
			return 0, fmt.Errorf("temperature below absolute zero")
		}
		return (celsius + 273.15) * 9 / 5, nil
	default:
		return 0, fmt.Errorf("unsupported temperature unit: %s", toUnit)
	}
}

func (uc *UnitConverter) validateRequest(req UnitConversionRequest) error {
	if math.IsNaN(req.Value) || math.IsInf(req.Value, 0) {
		return fmt.Errorf("value must be a finite number")
	}
	if req.FromUnit == "" {
		return fmt.Errorf("fromUnit cannot be empty")
	}
	if req.ToUnit == "" {
		return fmt.Errorf("toUnit cannot be empty")
	}
	if req.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}
	return nil
}

func (uc *UnitConverter) init() {
	uc.toBase = map[string]map[string]float64{
		// length → meters
		"length": {
			"nm": 1e-9, "μm": 1e-6, "mm": 0.001, "cm": 0.01, "m": 1,
			"km": 1000, "in": 0.0254, "ft": 0.3048, "yd": 0.9144,
			"mi": 1609.344, "mil": 2.54e-5,
		},
		// weight → grams
		"weight": {
			"mg": 0.001, "g": 1, "kg": 1000, "t": 1e6,
			"oz": 28.3495, "lb": 453.592, "st": 6350.29, "ton": 907185,
		},
		// volume → liters
		"volume": {
			"ml": 0.001, "cl": 0.01, "dl": 0.1, "l": 1, "kl": 1000,
			"fl_oz": 0.0295735, "cup": 0.236588, "pt": 0.473176,
			"qt": 0.946353, "gal": 3.78541,
			"tsp": 0.00492892, "tbsp": 0.0147868, "bbl": 158.987,
		},
		// area → square meters
		"area": {
			"mm2": 1e-6, "cm2": 1e-4, "m2": 1, "km2": 1e6,
			"in2": 6.4516e-4, "ft2": 0.092903, "yd2": 0.836127,
			"mi2": 2589988.11, "acre": 4046.86, "ha": 10000,
		},
		// temperature handled separately (non-linear)
		"temperature": {},
	}
}
