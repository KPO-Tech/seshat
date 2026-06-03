package main

import (
	"strings"
	"testing"

	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

func TestResolveModelKeepsExplicitProviderPrefix(t *testing.T) {
	model := resolveModel(engineconfig.Config{Model: "openai:gpt-5.5"})
	if model.Provider != sdk.APIProviderOpenAI {
		t.Fatalf("unexpected provider: got %q", model.Provider)
	}
	if model.Model != "gpt-5.5" {
		t.Fatalf("unexpected model: got %q", model.Model)
	}
}

func TestResolveModelInfersOllamaFromRawModelID(t *testing.T) {
	model := resolveModel(engineconfig.Config{Model: "qwen2.5-coder:7b"})
	if model.Provider != sdk.APIProviderOllama {
		t.Fatalf("unexpected provider: got %q", model.Provider)
	}
	if model.Model != "qwen2.5-coder:7b" {
		t.Fatalf("unexpected model: got %q", model.Model)
	}
}

func TestParsePermissionModeRejectsPlan(t *testing.T) {
	_, err := parsePermissionMode("plan")
	if err == nil {
		t.Fatal("expected plan permission mode to be rejected")
	}
	if !strings.Contains(err.Error(), "execution mode") {
		t.Fatalf("expected execution mode guidance, got %q", err)
	}
}
