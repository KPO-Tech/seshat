package nexusskill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	skills "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type baseTool struct{}

type listedSkill struct {
	Skill      skills.Skill
	Collection string
	SkillDir   string
}

func (baseTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}

func (baseTool) IsConcurrencySafe(_ map[string]any) bool {
	return true
}

func (baseTool) IsReadOnly(_ map[string]any) bool {
	return true
}

func (baseTool) IsEnabled() bool {
	return true
}

func (baseTool) FormatResult(data any) string {
	return fmt.Sprintf("%v", data)
}

func (baseTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

func workingDirFromInput(input tool.CallInput) string {
	if input.ToolContext != nil && strings.TrimSpace(input.ToolContext.WorkingDirectory) != "" {
		return input.ToolContext.WorkingDirectory
	}
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		return "."
	}
	return cwd
}

func listRuntimeSkills(cwd string) ([]listedSkill, error) {
	allSkills, err := skills.GetAllSkills(cwd)
	if err != nil {
		return nil, err
	}
	listed := make([]listedSkill, 0, len(allSkills))
	for _, sk := range allSkills {
		listed = append(listed, listedSkill{
			Skill:      sk,
			Collection: inferCollection(sk),
			SkillDir:   resolveSkillDir(sk),
		})
	}
	sort.Slice(listed, func(i, j int) bool {
		if listed[i].Collection != listed[j].Collection {
			return listed[i].Collection < listed[j].Collection
		}
		return listed[i].Skill.Name < listed[j].Skill.Name
	})
	return listed, nil
}

func inferCollection(sk skills.Skill) string {
	if sk.Source == skills.SourceMCP {
		return "mcp"
	}
	if sk.Source == skills.SourceBundled {
		return "builtin"
	}

	reposDir := skills.GetSkillReposPath()
	if strings.HasPrefix(sk.SkillRoot, reposDir+string(filepath.Separator)) {
		rel := strings.TrimPrefix(sk.SkillRoot, reposDir+string(filepath.Separator))
		parts := strings.SplitN(rel, string(filepath.Separator), 2)
		return parts[0]
	}

	managedDir := skills.GetManagedSkillsPath()
	if strings.HasPrefix(sk.SkillRoot, managedDir+string(filepath.Separator)) || sk.SkillRoot == managedDir {
		return "managed"
	}

	builtinDir := skills.GetBuiltinSkillsPath()
	if strings.HasPrefix(sk.SkillRoot, builtinDir+string(filepath.Separator)) || sk.SkillRoot == builtinDir {
		return "builtin"
	}

	skillsRoot := skills.GetSkillsRootPath()
	if strings.HasPrefix(sk.SkillRoot, filepath.Join(skillsRoot, "users")+string(filepath.Separator)) ||
		strings.HasPrefix(sk.SkillRoot, filepath.Join(skillsRoot, "user")+string(filepath.Separator)) {
		return "user"
	}

	return "project"
}

func matchesCollection(filter, collection string) bool {
	if filter == "" || filter == "all" {
		return true
	}
	if filter == "repo" {
		switch collection {
		case "builtin", "managed", "user", "project", "mcp":
			return false
		default:
			return true
		}
	}
	return collection == filter
}

func resolveSkillDir(sk skills.Skill) string {
	if sk.SkillRoot == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(sk.SkillRoot, "skill.md")); err == nil {
		return sk.SkillRoot
	}

	relativeName := strings.ReplaceAll(sk.Name, ":", string(filepath.Separator))
	if relativeName != "" {
		candidate := filepath.Join(sk.SkillRoot, relativeName)
		if _, err := os.Stat(filepath.Join(candidate, "skill.md")); err == nil {
			return candidate
		}
	}

	return sk.SkillRoot
}

func appendOverrideUserSkills(listed []listedSkill, userSkillsDir string) []listedSkill {
	if strings.TrimSpace(userSkillsDir) == "" {
		return listed
	}

	loader := skills.NewFileSkillLoaderWithSource(userSkillsDir, skills.SettingSourceUserSettings)
	overrideSkills, err := loader.LoadSkills()
	if err != nil {
		return listed
	}

	seen := make(map[string]struct{}, len(listed))
	for _, item := range listed {
		seen[item.Skill.Name] = struct{}{}
	}
	for _, sk := range overrideSkills {
		if _, ok := seen[sk.Name]; ok {
			continue
		}
		listed = append(listed, listedSkill{
			Skill:      sk,
			Collection: "user",
			SkillDir:   resolveSkillDir(sk),
		})
	}
	return listed
}

func findListedSkill(name string, listed []listedSkill) *listedSkill {
	for i := range listed {
		if strings.EqualFold(listed[i].Skill.Name, name) || strings.EqualFold(listed[i].Skill.DisplayName, name) {
			return &listed[i]
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "..."
}
