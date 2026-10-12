package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/tools/contract"
)

func registerTwoTestSkills(t *testing.T) {
	t.Cleanup(ClearBundledSkills)
	RegisterBundledSkill(BundledSkillDefinition{
		Name:          "allowed-skill",
		Description:   "the one the restriction should let through",
		UserInvocable: true,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: "ran allowed-skill"}}, nil
		},
	})
	RegisterBundledSkill(BundledSkillDefinition{
		Name:          "blocked-skill",
		Description:   "the one the restriction should hide",
		UserInvocable: true,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: "ran blocked-skill"}}, nil
		},
	})
}

func TestSkillTool_SetAllowedSkills_CallRestrictsExecution(t *testing.T) {
	registerTwoTestSkills(t)

	tool := NewSkillTool(nil)
	tool.SetAllowedSkills([]string{"allowed-skill"})

	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "allowed-skill"},
	}, nil)
	if err != nil {
		t.Fatalf("Call(allowed-skill) returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call(allowed-skill) returned result error: %v", result.Error)
	}
	if !strings.Contains(result.Content, "ran allowed-skill") {
		t.Fatalf("expected output to contain the allowed skill's output, got: %q", result.Content)
	}

	blocked, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "blocked-skill"},
	}, nil)
	if err != nil {
		t.Fatalf("Call(blocked-skill) returned error: %v", err)
	}
	if blocked.Error == nil {
		t.Fatalf("expected a not-found error for a skill outside the allow-list, got success: %q", blocked.Content)
	}
	if _, hint, found := strings.Cut(blocked.Error.Error(), "Available skills:"); found && strings.Contains(hint, "blocked-skill") {
		t.Fatalf("the 'available skills' hint must not list a skill outside the allow-list: %v", blocked.Error)
	}
}

func TestSkillTool_SetAllowedSkills_DefinitionOnlyListsAllowed(t *testing.T) {
	registerTwoTestSkills(t)

	tool := NewSkillTool(nil)
	tool.SetAllowedSkills([]string{"allowed-skill"})

	def := tool.Definition()
	if !strings.Contains(def.Description, "allowed-skill") {
		t.Fatalf("expected the tool definition to mention the allowed skill, got: %q", def.Description)
	}
	if strings.Contains(def.Description, "blocked-skill") {
		t.Fatalf("expected the tool definition to omit the blocked skill, got: %q", def.Description)
	}
}

func TestSkillTool_SetAllowedSkills_DescriptionOnlyListsAllowed(t *testing.T) {
	registerTwoTestSkills(t)

	tool := NewSkillTool(nil)
	tool.SetAllowedSkills([]string{"allowed-skill"})

	desc, err := tool.Description(context.Background())
	if err != nil {
		t.Fatalf("Description returned error: %v", err)
	}
	if !strings.Contains(desc, "allowed-skill") || strings.Contains(desc, "blocked-skill") {
		t.Fatalf("expected Description to list only the allowed skill, got: %q", desc)
	}
}

func TestSkillTool_NoRestrictionSeesEverything(t *testing.T) {
	registerTwoTestSkills(t)

	tool := NewSkillTool(nil)
	// SetAllowedSkills is never called - the default, unrestricted behavior.

	def := tool.Definition()
	if !strings.Contains(def.Description, "allowed-skill") || !strings.Contains(def.Description, "blocked-skill") {
		t.Fatalf("expected an unrestricted tool to list both skills, got: %q", def.Description)
	}

	for _, name := range []string{"allowed-skill", "blocked-skill"} {
		result, err := tool.Call(context.Background(), contract.CallInput{
			Parsed: map[string]any{"skill": name},
		}, nil)
		if err != nil {
			t.Fatalf("Call(%s) returned error: %v", name, err)
		}
		if result.Error != nil {
			t.Fatalf("Call(%s) returned result error: %v", name, result.Error)
		}
	}
}
