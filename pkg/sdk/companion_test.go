package sdk

import (
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/companion"
)

func TestBuildEngineConfigAppendsCompanionPrompt(t *testing.T) {
	appendPrompt := "Existing append."
	profile := companion.DefaultProfile()
	profile.Name = "Nora"
	config := DefaultClientConfig()
	config.PromptConfig = &PromptConfig{AppendSystemPrompt: &appendPrompt}
	config.Companion = &profile

	engineConfig := buildEngineConfig(config)
	for _, want := range []string{"Existing append.", "<companion>", "Nora"} {
		if !strings.Contains(engineConfig.AppendSystemPrompt, want) {
			t.Fatalf("expected append prompt to contain %q, got:\n%s", want, engineConfig.AppendSystemPrompt)
		}
	}
}

func TestBuildEngineConfigSkipsDisabledCompanion(t *testing.T) {
	profile := companion.DefaultProfile()
	profile.Enabled = false
	config := DefaultClientConfig()
	config.Companion = &profile

	engineConfig := buildEngineConfig(config)
	if strings.Contains(engineConfig.AppendSystemPrompt, "<companion>") {
		t.Fatalf("expected disabled companion to be skipped, got:\n%s", engineConfig.AppendSystemPrompt)
	}
}
