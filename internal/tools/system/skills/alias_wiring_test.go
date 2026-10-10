package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/tools/contract"
)

// resetSkillAliases restores the package-level alias registry after a test
// that registers aliases, so it doesn't leak into other tests in this
// package (there is no public reset - the registry is meant to be
// populated once at process startup).
func resetSkillAliases(t *testing.T) {
	t.Cleanup(func() {
		skillAliases = nil
	})
}

func TestSkillTool_ResolvesAliasToTargetSkill(t *testing.T) {
	resetSkillAliases(t)
	t.Cleanup(ClearBundledSkills)

	const targetName = "alias-target-skill"
	const wantOutput = "ran the target skill"

	RegisterBundledSkill(BundledSkillDefinition{
		Name:          targetName,
		Description:   "test target",
		UserInvocable: true,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: wantOutput}}, nil
		},
	})
	RegisterSkillAlias("alias-name", targetName)

	tool := NewSkillTool(nil)
	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "alias-name"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned result error: %v", result.Error)
	}
	if !strings.Contains(result.Content, wantOutput) {
		t.Fatalf("expected output to contain %q, got: %q", wantOutput, result.Content)
	}
}

func TestSkillTool_UnknownNameStillFails(t *testing.T) {
	resetSkillAliases(t)

	tool := NewSkillTool(nil)
	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "no-such-skill-and-no-alias"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.Error == nil {
		t.Fatalf("expected an error result for an unresolvable skill name")
	}
}

func TestResolveSkillAlias(t *testing.T) {
	resetSkillAliases(t)

	if got := ResolveSkillAlias("not-registered"); got != "not-registered" {
		t.Fatalf("expected ResolveSkillAlias to pass through an unregistered name, got %q", got)
	}

	RegisterSkillAlias("short", "long-target-name")
	if got := ResolveSkillAlias("short"); got != "long-target-name" {
		t.Fatalf("expected ResolveSkillAlias(%q) = %q, got %q", "short", "long-target-name", got)
	}
}
