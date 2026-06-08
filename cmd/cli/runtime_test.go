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

func TestParsePermissionModeCaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  sdk.PermissionMode
	}{
		{"onRequest", sdk.PermissionModeOnRequest},
		{"onrequest", sdk.PermissionModeOnRequest},
		{"ONREQUEST", sdk.PermissionModeOnRequest},
		{"auto", sdk.PermissionModeAuto},
		{"AUTO", sdk.PermissionModeAuto},
		{"acceptEdits", sdk.PermissionMode("acceptEdits")},
		{"acceptedits", sdk.PermissionMode("acceptEdits")},
		{"ACCEPTEDITS", sdk.PermissionMode("acceptEdits")},
		{"bypass", sdk.PermissionModeBypass},
		{"BYPASS", sdk.PermissionModeBypass},
		{"never", sdk.PermissionModeNever},
		{"NEVER", sdk.PermissionModeNever},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parsePermissionMode(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
