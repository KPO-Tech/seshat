package skills

import (
	"context"

	internalskills "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills"
)

type (
	Alias = internalskills.SkillAlias
	Skill = internalskills.Skill
)

func All(cwd string) ([]Skill, error) {
	return internalskills.GetAllSkills(cwd)
}

func AllForUser(cwd string, userID string) ([]Skill, error) {
	return internalskills.GetAllSkillsForUser(cwd, userID)
}

// ForContext returns skills active for the given file paths. Skills with a
// paths restriction are included only when a file matches; all others are
// always returned. Equivalent to AllForUser when filePaths is empty.
func ForContext(cwd string, filePaths []string, userID string) ([]Skill, error) {
	return internalskills.GetSkillsForContext(cwd, filePaths, userID)
}

// MatchTrigger returns the first skill whose trigger phrases match userInput, or nil.
func MatchTrigger(userInput string, skills []Skill) *Skill {
	return internalskills.MatchTrigger(userInput, skills)
}

func UserPath(userID string) string {
	return internalskills.GetUserSkillsPathForUser(userID)
}

// ReadEnabled reads the user-invocable (enabled) status of a skill.md file.
// Returns true when the file cannot be read or the field is absent.
func ReadEnabled(path string) bool {
	return internalskills.ReadSkillEnabled(path)
}

func Bundled() []Skill {
	return internalskills.GetBundledSkills()
}

func DiscoverMCP(ctx context.Context) ([]Skill, error) {
	return internalskills.DiscoverMCPSkills(ctx)
}

func MCPServers() []string {
	return internalskills.GetMCPServers()
}

func MCPHealth() map[string]string {
	return internalskills.GetMCPHealth()
}

func Aliases() []Alias {
	return internalskills.GetSkillAliases()
}
