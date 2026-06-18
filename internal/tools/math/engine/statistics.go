package engine

import (
	"fmt"
	"math"
	"sort"

	"gonum.org/v1/gonum/stat"
)

// StatisticsCalculator performs statistical analysis on datasets.
type StatisticsCalculator struct{}

// NewStatisticsCalculator creates a new StatisticsCalculator.
func NewStatisticsCalculator() *StatisticsCalculator {
	return &StatisticsCalculator{}
}

// Calculate executes the requested statistical operation.
func (sc *StatisticsCalculator) Calculate(req StatisticsRequest) (StatisticsResult, error) {
	if len(req.Data) == 0 {
		return StatisticsResult{}, fmt.Errorf("data set cannot be empty")
	}
	if err := sc.validateData(req.Data); err != nil {
		return StatisticsResult{}, err
	}

	var result interface{}
	var err error

	switch req.Operation {
	case "mean":
		result = sc.mean(req.Data)
	case "median":
		result = sc.median(req.Data)
	case "mode":
		result, err = sc.mode(req.Data)
		if err != nil {
			return StatisticsResult{}, err
		}
	case "std_dev":
		result = sc.standardDeviation(req.Data)
	case "variance":
		result = sc.variance(req.Data)
	case "percentile":
		result = sc.percentiles(req.Data, []float64{25, 50, 75, 90, 95, 99})
	default:
		return StatisticsResult{}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	return StatisticsResult{Result: result, Count: len(req.Data)}, nil
}

// CalculatePercentile computes a specific percentile of the dataset.
func (sc *StatisticsCalculator) CalculatePercentile(data []float64, percentile float64) (float64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("data set cannot be empty")
	}
	if percentile < 0 || percentile > 100 {
		return 0, fmt.Errorf("percentile must be between 0 and 100")
	}
	if err := sc.validateData(data); err != nil {
		return 0, err
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	return stat.Quantile(percentile/100.0, stat.Empirical, sorted, nil), nil
}

// Summary returns a comprehensive statistical summary of the dataset.
func (sc *StatisticsCalculator) Summary(data []float64) (map[string]interface{}, error) {
	if err := sc.validateData(data); err != nil {
		return nil, err
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	dataRange, _ := sc.Range(data)
	summary := map[string]interface{}{
		"count":       len(data),
		"mean":        sc.mean(data),
		"median":      sc.median(data),
		"std_dev":     sc.standardDeviation(data),
		"variance":    sc.variance(data),
		"range":       dataRange,
		"min":         sorted[0],
		"max":         sorted[len(sorted)-1],
		"percentiles": sc.percentiles(data, []float64{25, 50, 75}),
	}
	return summary, nil
}

// Range returns max - min of the dataset.
func (sc *StatisticsCalculator) Range(data []float64) (float64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("data set cannot be empty")
	}
	if err := sc.validateData(data); err != nil {
		return 0, err
	}
	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return max - min, nil
}

func (sc *StatisticsCalculator) mean(data []float64) float64 {
	return stat.Mean(data, nil)
}

func (sc *StatisticsCalculator) median(data []float64) float64 {
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	return stat.Quantile(0.5, stat.Empirical, sorted, nil)
}

func (sc *StatisticsCalculator) mode(data []float64) (interface{}, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot calculate mode of empty data set")
	}
	freq := make(map[float64]int)
	for _, v := range data {
		freq[v]++
	}
	maxFreq := 0
	for _, f := range freq {
		if f > maxFreq {
			maxFreq = f
		}
	}
	if maxFreq == 1 {
		return "No mode (all values appear once)", nil
	}
	var modes []float64
	for v, f := range freq {
		if f == maxFreq {
			modes = append(modes, v)
		}
	}
	sort.Float64s(modes)
	if len(modes) == 1 {
		return modes[0], nil
	}
	return map[string]interface{}{"modes": modes, "frequency": maxFreq, "type": "multimodal"}, nil
}

func (sc *StatisticsCalculator) standardDeviation(data []float64) float64 {
	return stat.StdDev(data, nil)
}

func (sc *StatisticsCalculator) variance(data []float64) float64 {
	return stat.Variance(data, nil)
}

func (sc *StatisticsCalculator) percentiles(data []float64, ps []float64) map[string]float64 {
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	result := make(map[string]float64, len(ps))
	for _, p := range ps {
		result[fmt.Sprintf("P%.0f", p)] = stat.Quantile(p/100.0, stat.Empirical, sorted, nil)
	}
	return result
}

func (sc *StatisticsCalculator) validateData(data []float64) error {
	for i, v := range data {
		if math.IsNaN(v) {
			return fmt.Errorf("data point %d is NaN", i)
		}
		if math.IsInf(v, 0) {
			return fmt.Errorf("data point %d is infinite", i)
		}
	}
	return nil
}
