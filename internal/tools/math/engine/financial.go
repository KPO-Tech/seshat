package engine

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// FinancialCalculator performs financial calculations.
type FinancialCalculator struct{}

// NewFinancialCalculator creates a new FinancialCalculator.
func NewFinancialCalculator() *FinancialCalculator {
	return &FinancialCalculator{}
}

// Calculate executes the requested financial operation.
func (fc *FinancialCalculator) Calculate(req FinancialRequest) (FinancialResult, error) {
	if err := fc.validateRequest(req); err != nil {
		return FinancialResult{}, err
	}

	var result float64
	var breakdown map[string]interface{}
	var description string
	var err error

	switch req.Operation {
	case "compound_interest":
		result, breakdown, err = fc.compoundInterest(req)
		description = "Compound interest calculation"
	case "simple_interest":
		result, breakdown, err = fc.simpleInterest(req)
		description = "Simple interest calculation"
	case "loan_payment":
		result, breakdown, err = fc.loanPayment(req)
		description = "Monthly loan payment calculation"
	case "roi":
		result, breakdown, err = fc.returnOnInvestment(req)
		description = "Return on investment calculation"
	case "present_value":
		result, breakdown, err = fc.presentValue(req)
		description = "Present value calculation"
	case "future_value":
		result, breakdown, err = fc.futureValue(req)
		description = "Future value calculation"
	default:
		return FinancialResult{}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	if err != nil {
		return FinancialResult{}, err
	}
	return FinancialResult{Result: result, Breakdown: breakdown, Description: description}, nil
}

// NetPresentValue computes NPV from a series of cash flows.
func (fc *FinancialCalculator) NetPresentValue(cashFlows []float64, discountRate float64) (float64, error) {
	if len(cashFlows) == 0 {
		return 0, fmt.Errorf("cash flows cannot be empty")
	}
	if discountRate <= 0 {
		return 0, fmt.Errorf("discount rate must be positive")
	}
	rate := discountRate / 100
	npv := 0.0
	for i, cf := range cashFlows {
		npv += cf / math.Pow(1+rate, float64(i))
	}
	return npv, nil
}

// InternalRateOfReturn computes IRR using Newton-Raphson iteration.
func (fc *FinancialCalculator) InternalRateOfReturn(cashFlows []float64) (float64, error) {
	if len(cashFlows) < 2 {
		return 0, fmt.Errorf("at least 2 cash flows required")
	}
	rate := 0.1
	tolerance := 0.000001
	for i := 0; i < 100; i++ {
		npv, npvDeriv := 0.0, 0.0
		for t, cf := range cashFlows {
			factor := math.Pow(1+rate, float64(t))
			npv += cf / factor
			if t > 0 {
				npvDeriv += -float64(t) * cf / math.Pow(1+rate, float64(t+1))
			}
		}
		if math.Abs(npv) < tolerance {
			return rate * 100, nil
		}
		if npvDeriv == 0 {
			return 0, fmt.Errorf("cannot converge to IRR")
		}
		rate -= npv / npvDeriv
	}
	return 0, fmt.Errorf("IRR calculation did not converge")
}

func (fc *FinancialCalculator) compoundInterest(req FinancialRequest) (float64, map[string]interface{}, error) {
	if req.Principal <= 0 {
		return 0, nil, fmt.Errorf("principal must be positive")
	}
	if req.Rate < 0 {
		return 0, nil, fmt.Errorf("rate cannot be negative")
	}
	if req.Time <= 0 {
		return 0, nil, fmt.Errorf("time must be positive")
	}
	periods := req.Periods
	if periods == 0 {
		periods = 1
	}

	principal := decimal.NewFromFloat(req.Principal)
	rate := decimal.NewFromFloat(req.Rate / 100)
	n := decimal.NewFromInt(int64(periods))

	ratePerPeriod := rate.Div(n)
	onePlusRate := decimal.NewFromInt(1).Add(ratePerPeriod)

	exponent := n.Mul(decimal.NewFromFloat(req.Time))
	exponentF, _ := exponent.Float64()
	onePlusRateF, _ := onePlusRate.Float64()
	compoundFactor := math.Pow(onePlusRateF, exponentF)

	finalAmount, _ := principal.Mul(decimal.NewFromFloat(compoundFactor)).Float64()
	interestEarned := finalAmount - req.Principal

	breakdown := map[string]interface{}{
		"principal":          req.Principal,
		"rate_percent":       req.Rate,
		"time_years":         req.Time,
		"compounds_per_year": periods,
		"final_amount":       finalAmount,
		"interest_earned":    interestEarned,
		"effective_rate":     (finalAmount/req.Principal - 1) / req.Time * 100,
	}
	return finalAmount, breakdown, nil
}

func (fc *FinancialCalculator) simpleInterest(req FinancialRequest) (float64, map[string]interface{}, error) {
	if req.Principal <= 0 {
		return 0, nil, fmt.Errorf("principal must be positive")
	}
	if req.Rate < 0 {
		return 0, nil, fmt.Errorf("rate cannot be negative")
	}
	if req.Time <= 0 {
		return 0, nil, fmt.Errorf("time must be positive")
	}
	interest := req.Principal * (req.Rate / 100) * req.Time
	finalAmount := req.Principal + interest
	breakdown := map[string]interface{}{
		"principal":    req.Principal,
		"rate_percent": req.Rate,
		"time_years":   req.Time,
		"interest":     interest,
		"final_amount": finalAmount,
	}
	return interest, breakdown, nil
}

func (fc *FinancialCalculator) loanPayment(req FinancialRequest) (float64, map[string]interface{}, error) {
	if req.Principal <= 0 {
		return 0, nil, fmt.Errorf("principal must be positive")
	}
	if req.Rate <= 0 {
		return 0, nil, fmt.Errorf("rate must be positive")
	}
	if req.Time <= 0 {
		return 0, nil, fmt.Errorf("time must be positive")
	}
	periods := req.Periods
	if periods == 0 {
		periods = 12
	}
	periodRate := (req.Rate / 100) / float64(periods)
	totalPayments := req.Time * float64(periods)

	if periodRate == 0 {
		payment := req.Principal / totalPayments
		return payment, map[string]interface{}{
			"loan_amount": req.Principal, "rate_percent": req.Rate,
			"term_years": req.Time, "payments_per_year": periods,
			"monthly_payment": payment, "total_paid": payment * totalPayments, "total_interest": 0.0,
		}, nil
	}

	factor := math.Pow(1+periodRate, totalPayments)
	payment := req.Principal * (periodRate * factor) / (factor - 1)
	totalPaid := payment * totalPayments
	totalInterest := totalPaid - req.Principal

	breakdown := map[string]interface{}{
		"loan_amount": req.Principal, "rate_percent": req.Rate,
		"term_years": req.Time, "payments_per_year": periods,
		"monthly_payment": payment, "total_paid": totalPaid,
		"total_interest": totalInterest, "interest_percentage": (totalInterest / req.Principal) * 100,
	}
	return payment, breakdown, nil
}

func (fc *FinancialCalculator) returnOnInvestment(req FinancialRequest) (float64, map[string]interface{}, error) {
	if req.Principal <= 0 {
		return 0, nil, fmt.Errorf("initial investment must be positive")
	}
	if req.FutureValue <= 0 {
		return 0, nil, fmt.Errorf("final value must be positive")
	}
	roi := ((req.FutureValue - req.Principal) / req.Principal) * 100
	breakdown := map[string]interface{}{
		"initial_investment": req.Principal,
		"final_value":        req.FutureValue,
		"gain_loss":          req.FutureValue - req.Principal,
		"roi_percent":        roi,
	}
	if req.Time > 0 {
		breakdown["annualized_roi_percent"] = (math.Pow(req.FutureValue/req.Principal, 1/req.Time) - 1) * 100
		breakdown["time_years"] = req.Time
	}
	return roi, breakdown, nil
}

func (fc *FinancialCalculator) presentValue(req FinancialRequest) (float64, map[string]interface{}, error) {
	if req.FutureValue <= 0 {
		return 0, nil, fmt.Errorf("future value must be positive")
	}
	if req.Rate <= 0 {
		return 0, nil, fmt.Errorf("discount rate must be positive")
	}
	if req.Time <= 0 {
		return 0, nil, fmt.Errorf("time must be positive")
	}
	periods := req.Periods
	if periods == 0 {
		periods = 1
	}
	periodRate := (req.Rate / 100) / float64(periods)
	totalPeriods := req.Time * float64(periods)
	discountFactor := math.Pow(1+periodRate, totalPeriods)
	pv := req.FutureValue / discountFactor

	breakdown := map[string]interface{}{
		"future_value": req.FutureValue, "discount_rate": req.Rate,
		"time_years": req.Time, "compounds_per_year": periods,
		"present_value": pv, "discount_amount": req.FutureValue - pv, "discount_factor": discountFactor,
	}
	return pv, breakdown, nil
}

func (fc *FinancialCalculator) futureValue(req FinancialRequest) (float64, map[string]interface{}, error) {
	result, breakdown, err := fc.compoundInterest(req)
	if err != nil {
		return 0, nil, err
	}
	breakdown["present_value"] = req.Principal
	delete(breakdown, "principal")
	breakdown["growth"] = result - req.Principal
	return result, breakdown, nil
}

func (fc *FinancialCalculator) validateRequest(req FinancialRequest) error {
	if req.Operation == "" {
		return fmt.Errorf("operation cannot be empty")
	}
	fields := map[string]float64{
		"principal": req.Principal, "rate": req.Rate,
		"time": req.Time, "futureValue": req.FutureValue,
	}
	for name, v := range fields {
		if math.IsNaN(v) {
			return fmt.Errorf("%s cannot be NaN", name)
		}
		if math.IsInf(v, 0) {
			return fmt.Errorf("%s cannot be infinite", name)
		}
	}
	if req.Periods < 0 {
		return fmt.Errorf("periods cannot be negative")
	}
	return nil
}
