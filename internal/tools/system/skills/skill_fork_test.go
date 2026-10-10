package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/tools/contract"
)

type mockForkRunner struct {
	gotSkillName    string
	gotTask         string
	gotAllowedTools []string
	gotModel        string
	output          string
	err             error
}

func (m *mockForkRunner) RunForkedSkill(ctx context.Context, skillName, task string, allowedTools []string, model string) (string, error) {
	m.gotSkillName = skillName
	m.gotTask = task
	m.gotAllowedTools = allowedTools
	m.gotModel = model
	if m.err != nil {
		return "", m.err
	}
	return m.output, nil
}

func TestSkillTool_ForkSkill_UsesForkRunnerWithFrontmatterFields(t *testing.T) {
	t.Cleanup(ClearBundledSkills)
	RegisterBundledSkill(BundledSkillDefinition{
		Name:          "forked-skill",
		Description:   "runs as a sub-agent",
		UserInvocable: true,
		AllowedTools:  []string{"read_file", "grep"},
		Model:         "anthropic:claude-sonnet-4-20250514",
		Context:       ExecutionContextFork,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: "do the forked task: " + args}}, nil
		},
	})

	tool := NewSkillTool(nil)
	runner := &mockForkRunner{output: "sub-agent finished the task"}
	tool.SetForkRunner(runner)

	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "forked-skill", "args": "do-it"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned result error: %v", result.Error)
	}

	if runner.gotSkillName != "forked-skill" {
		t.Errorf("expected forkRunner to receive skill name %q, got %q", "forked-skill", runner.gotSkillName)
	}
	if !strings.Contains(runner.gotTask, "do the forked task: do-it") {
		t.Errorf("expected forkRunner to receive the rendered prompt, got %q", runner.gotTask)
	}
	if len(runner.gotAllowedTools) != 2 || runner.gotAllowedTools[0] != "read_file" || runner.gotAllowedTools[1] != "grep" {
		t.Errorf("expected forkRunner to receive the skill's AllowedTools, got %v", runner.gotAllowedTools)
	}
	if runner.gotModel != "anthropic:claude-sonnet-4-20250514" {
		t.Errorf("expected forkRunner to receive the skill's Model, got %q", runner.gotModel)
	}
	if !strings.Contains(result.Content, "sub-agent finished the task") {
		t.Fatalf("expected the fork runner's output in the tool result, got: %q", result.Content)
	}
}

func TestSkillTool_ForkSkill_NoRunnerConfigured_ReturnsError(t *testing.T) {
	t.Cleanup(ClearBundledSkills)
	RegisterBundledSkill(BundledSkillDefinition{
		Name:          "forked-but-unconfigured",
		Description:   "declares fork but the host wired no runner",
		UserInvocable: true,
		Context:       ExecutionContextFork,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: "body"}}, nil
		},
	})

	tool := NewSkillTool(nil)
	// SetForkRunner is never called.

	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "forked-but-unconfigured"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.Error == nil {
		t.Fatalf("expected an error result when context: fork has no runner configured, got success: %q", result.Content)
	}
	if !strings.Contains(result.Error.Error(), "no fork runner is configured") {
		t.Fatalf("expected the error to explain the missing fork runner, got: %v", result.Error)
	}
}

func TestSkillTool_ForkSkill_RunnerErrorSurfacesAsResultError(t *testing.T) {
	t.Cleanup(ClearBundledSkills)
	RegisterBundledSkill(BundledSkillDefinition{
		Name:          "forked-and-failing",
		Description:   "the sub-agent run itself fails",
		UserInvocable: true,
		Context:       ExecutionContextFork,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: "body"}}, nil
		},
	})

	tool := NewSkillTool(nil)
	tool.SetForkRunner(&mockForkRunner{err: errSentinelForkFailure})

	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "forked-and-failing"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.Error == nil {
		t.Fatalf("expected the fork runner's error to surface as a result error, got success: %q", result.Content)
	}
}

func TestSkillTool_InlineSkill_DoesNotRequireForkRunner(t *testing.T) {
	t.Cleanup(ClearBundledSkills)
	RegisterBundledSkill(BundledSkillDefinition{
		Name:          "inline-only-skill",
		Description:   "no fork runner needed",
		UserInvocable: true,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return []ContentBlock{{Type: "text", Text: "inline body"}}, nil
		},
	})

	tool := NewSkillTool(nil)
	// SetForkRunner is never called - the default, non-fork path.

	result, err := tool.Call(context.Background(), contract.CallInput{
		Parsed: map[string]any{"skill": "inline-only-skill"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("a non-fork skill must not require a fork runner, got error: %v", result.Error)
	}
	if !strings.Contains(result.Content, "inline body") {
		t.Fatalf("expected the inline body in the result, got: %q", result.Content)
	}
}

var errSentinelForkFailure = &forkTestError{"sub-agent exploded"}

type forkTestError struct{ msg string }

func (e *forkTestError) Error() string { return e.msg }
