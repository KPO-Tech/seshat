package sdk

import "testing"

// These guard M3 in TODO.md: a host that builds ClientConfig directly
// (bypassing DefaultClientConfig) got no session-turn limit at all -
// engine.Config.MaxTurns stayed 0, and Session.enforceMaxTurns treats <= 0
// as "no limit" - instead of the same 100 every other default path uses
// (engine.DefaultConfig, DefaultLoopConfig, DefaultClientConfig).

func TestBuildEngineConfig_DefaultsMaxTurnsWhenUnset(t *testing.T) {
	engineConfig := buildEngineConfig(&ClientConfig{})
	if engineConfig.MaxTurns != 100 {
		t.Errorf("expected MaxTurns to default to 100 when unset, got %d", engineConfig.MaxTurns)
	}
}

func TestBuildEngineConfig_PreservesExplicitMaxTurns(t *testing.T) {
	engineConfig := buildEngineConfig(&ClientConfig{MaxTurns: 42})
	if engineConfig.MaxTurns != 42 {
		t.Errorf("expected explicit MaxTurns to be preserved, got %d", engineConfig.MaxTurns)
	}
}
