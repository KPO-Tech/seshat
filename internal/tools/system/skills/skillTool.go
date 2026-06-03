package skills

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type SkillInput struct {
	Skill string `json:"skill"`
	Args  string `json:"args,omitempty"`
}

type SkillOutput struct {
	Skill   string `json:"skill"`
	Args    string `json:"args,omitempty"`
	Output  string `json:"output"`
	Success bool   `json:"success"`
}

type SkillTool struct {
	loader SkillLoader
	cwd    string
	userID string
}

func NewSkillTool(loader SkillLoader) *SkillTool {
	if loader == nil {
		loader = NewFileSkillLoader("")
	}
	cwd, _ := os.Getwd()
	return &SkillTool{loader: loader, cwd: cwd}
}

func (t *SkillTool) SetCwd(cwd string) {
	t.cwd = cwd
}

func (t *SkillTool) SetUserID(userID string) {
	t.userID = userID
}

func (t *SkillTool) Definition() contract.Definition {
	skills, _ := t.loader.GetSkillDirCommands(t.cwd)
	skillList := FormatSkillsList(skills)

	return contract.Definition{
		Name:        SkillToolName,
		Description: GetPromptWithSkills(skillList),
		SearchHint:  "execute skill command plugin slash command",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill": map[string]any{
					"type":        "string",
					"description": "Name of the skill to execute",
				},
				"args": map[string]any{
					"type":        "string",
					"description": "Optional arguments to pass to the skill",
				},
			},
			"required": []string{"skill"},
		}),
		IsReadOnly:        false,
		IsConcurrencySafe: false,
		ShouldDefer:       true,
	}
}

func (t *SkillTool) Call(ctx context.Context, input contract.CallInput, permissionCheck types.CanUseToolFn) (contract.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		return contract.NewErrorResult(fmt.Errorf("no input parsed")), nil
	}

	skillName, _ := parsed["skill"].(string)
	if skillName == "" {
		return contract.NewErrorResult(fmt.Errorf("skill name is required")), nil
	}

	args, _ := parsed["args"].(string)

	skill, err := t.lookupSkill(skillName)
	if err != nil {
		return contract.NewErrorResult(fmt.Errorf("failed to load skill: %v", err)), nil
	}

	if skill == nil {
		all, _ := GetAllSkillsForUser(t.cwd, t.userID)
		var available []string
		for _, s := range all {
			available = append(available, s.Name)
		}
		return contract.NewErrorResult(fmt.Errorf("skill '%s' not found. Available skills: %s", skillName, strings.Join(available, ", "))), nil
	}

	// Execute the skill
	var promptResult []ContentBlock
	if skill.GetPromptForCommand != nil {
		promptResult, err = skill.GetPromptForCommand(args, ctx)
	} else {
		promptResult, err = ExecuteSkillPrompt(*skill, args, ctx)
	}

	if err != nil {
		return contract.NewErrorResult(fmt.Errorf("skill execution failed: %v", err)), nil
	}

	var promptText string
	for _, block := range promptResult {
		promptText += block.Text + "\n"
	}

	return contract.NewTextResult(fmt.Sprintf("Skill '%s' invoked with args: '%s'\n\n%s", skillName, args, promptText)), nil
}

// lookupSkill resolves a skill by name using the canonical priority order
// (managed > project > user > builtin > repos > bundled). All callers go through
// GetAllSkillsForUser so the deduplication logic is consistent with every other
// resolution path (HTTP /skills, nexus_list_skills, resolveSkillPrompt).
func (t *SkillTool) lookupSkill(name string) (*Skill, error) {
	all, err := GetAllSkillsForUser(t.cwd, t.userID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name || all[i].DisplayName == name {
			return &all[i], nil
		}
	}
	return nil, nil
}

func (t *SkillTool) Description(ctx context.Context) (string, error) {
	skills, _ := GetAllSkills(t.cwd)
	return GetPromptWithSkills(FormatSkillsList(skills)), nil
}

func (t *SkillTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (t *SkillTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx contract.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAsk}
}

func (t *SkillTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

func (t *SkillTool) IsReadOnly(input map[string]any) bool {
	return false
}

func (t *SkillTool) IsEnabled() bool {
	return true
}

func (t *SkillTool) FormatResult(data any) string {
	if output, ok := data.(SkillOutput); ok {
		if output.Success {
			return fmt.Sprintf("Skill '%s' executed with args: '%s'\n\n%s", output.Skill, output.Args, output.Output)
		}
		return fmt.Sprintf("Skill failed: %s", output.Output)
	}
	return fmt.Sprintf("%v", data)
}

func (t *SkillTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

func GetPrompt() string {
	return `Execute a skill within the main conversation

When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge.

When users reference a "slash command" or "/<something>" (e.g., "/commit", "/review-pr"), they are referring to a skill. Use this tool to invoke it.

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "pdf" - invoke the pdf skill
  - skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
  - skill: "review-pr", args: "123" - invoke with arguments

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, invoke the relevant Skill tool BEFORE generating any other response about the task
- NEVER mention a skill without actually calling this tool
- Do not invoke a skill that is already running`
}

func FormatSkillsList(skills []Skill) string {
	if len(skills) == 0 {
		return "No skills available"
	}
	var result string
	for _, skill := range skills {
		if skill.IsHidden {
			continue
		}
		line := "- " + skill.Name + ": " + skill.Description
		if len(skill.Triggers) > 0 {
			line += " [auto-activates when user says: " + strings.Join(skill.Triggers, " / ") + "]"
		}
		result += line + "\n"
	}
	if result == "" {
		return "No skills available"
	}
	return result
}

func GetPromptWithSkills(skillsList string) string {
	return `Execute a skill within the main conversation

When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge.

When users reference a "slash command" or "/<something>" (e.g., "/commit", "/review-pr"), they are referring to a skill. Use this tool to invoke it.

Available skills:
` + skillsList + `

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "pdf" - invoke the pdf skill
  - skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
  - skill: "review-pr", args: "123" - invoke with arguments
  - skill: "namespace:skill-name" - invoke namespaced skill

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, invoke the relevant Skill tool BEFORE generating any other response about the task
- NEVER mention a skill without actually calling this tool
- Do not invoke a skill that is already running
- Skill prompts support ${NEXUS_SKILL_DIR} and ${NEXUS_SESSION_ID} substitution`
}
