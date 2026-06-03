package modes

import (
	"testing"
)

func TestExecutionModeConstants(t *testing.T) {
	tests := []struct {
		name  string
		mode  ExecutionMode
		value string
	}{
		{"Execute mode", ExecutionModeExecute, "execute"},
		{"Plan mode", ExecutionModePlan, "plan"},
		{"PairProgramming mode", ExecutionModePairProgramming, "pair_programming"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.value {
				t.Errorf("Expected %s, got %s", tt.value, tt.mode)
			}
		})
	}
}

func TestIsPlanMode(t *testing.T) {
	if !IsPlanMode(ExecutionModePlan) {
		t.Error("Expected true for Plan mode")
	}
	if IsPlanMode(ExecutionModeExecute) {
		t.Error("Expected false for Execute mode")
	}
}

func TestIsPairProgrammingMode(t *testing.T) {
	if !IsPairProgrammingMode(ExecutionModePairProgramming) {
		t.Error("Expected true for PairProgramming mode")
	}
	if IsPairProgrammingMode(ExecutionModeExecute) {
		t.Error("Expected false for Execute mode")
	}
}

func TestIsExecuteMode(t *testing.T) {
	if !IsExecuteMode(ExecutionModeExecute) {
		t.Error("Expected true for Execute mode")
	}
	if !IsExecuteMode("") {
		t.Error("Expected true for empty mode (defaults to execute)")
	}
	if IsExecuteMode(ExecutionModePlan) {
		t.Error("Expected false for Plan mode")
	}
}

func TestModeStringFunctions(t *testing.T) {
	tests := []struct {
		name     string
		modeStr  string
		expected bool
		check    func(string) bool
	}{
		{"Plan mode string", "plan", true, IsPlanModeString},
		{"PairProgramming mode string", "pair_programming", true, IsPairProgrammingModeString},
		{"Invalid mode string", "invalid", false, IsPlanModeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check(tt.modeStr) != tt.expected {
				t.Errorf("Expected %v for mode %s", tt.expected, tt.modeStr)
			}
		})
	}
}
