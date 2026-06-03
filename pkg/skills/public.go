package skills

import internalskills "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills"

type (
	FrontmatterData = internalskills.FrontmatterData
	SkillSource     = internalskills.SkillSource
)

const (
	SourceBundled  = internalskills.SourceBundled
	SourceCommands = internalskills.SourceCommands
	SourcePlugin   = internalskills.SourcePlugin
	SourceManaged  = internalskills.SourceManaged
	SourceMCP      = internalskills.SourceMCP
)

func GetSkillsRootPath() string {
	return internalskills.GetSkillsRootPath()
}

func GetBuiltinSkillsPath() string {
	return internalskills.GetBuiltinSkillsPath()
}

func GetManagedSkillsPath() string {
	return internalskills.GetManagedSkillsPath()
}

func GetSkillReposPath() string {
	return internalskills.GetSkillReposPath()
}

func GetUserSkillsPath() string {
	return internalskills.GetUserSkillsPath()
}

func ParseFrontmatter(content string, filePath string) (FrontmatterData, string) {
	return internalskills.ParseFrontmatter(content, filePath)
}

func ParseBooleanFrontmatter(value interface{}) bool {
	return internalskills.ParseBooleanFrontmatter(value)
}

func GetUserSkillsPathForUser(userID string) string {
	return internalskills.GetUserSkillsPathForUser(userID)
}
