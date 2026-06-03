package skills

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

const (
	DefaultSkillDirectory    = ".nexus/skills"
	DefaultCommandsDirectory = ".nexus/commands"
	BundledSkillExtractDir   = ".nexus/bundled-skills"
)

var sessionID string

func init() {
	sessionID = os.Getenv("NEXUS_SESSION_ID")
	if sessionID == "" {
		sessionID = "default"
	}
}

// GetManagedSkillsPath returns the directory for admin-managed (policy) skills.
// Respects NEXUS_RUNTIME_ROOT so the path moves with the deployment.
func GetManagedSkillsPath() string {
	return filepath.Join(runtimepath.SkillsDir(""), "managed")
}

// GetUserSkillsPath returns the default single-user skill path (legacy CLI mode).
func GetUserSkillsPath() string {
	return filepath.Join(runtimepath.SkillsDir(""), "user")
}

func GetUserSkillsPathForUser(userID string) string {
	if strings.TrimSpace(userID) == "" {
		return GetUserSkillsPath()
	}
	return filepath.Join(runtimepath.SkillsDir(""), "users", userID)
}

func GetProjectSkillsPath(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "skills")
}

func GetProjectCommandsPath(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "commands")
}

func GetBundledSkillsRoot() string {
	return BundledSkillExtractDir
}

type FileSkillLoader struct {
	Directory string
	Source    SettingSource
	// CollectionRoot, when non-empty, is the repo root that owns this skill
	// collection. It is injected into skill prompts so the AI can resolve
	// paths relative to the repo root (e.g. data/, scripts/).
	CollectionRoot string
}

func NewFileSkillLoader(directory string) *FileSkillLoader {
	if directory == "" {
		directory = DefaultSkillDirectory
	}
	return &FileSkillLoader{Directory: directory, Source: SettingSourceProjectSettings}
}

func NewFileSkillLoaderWithSource(directory string, source SettingSource) *FileSkillLoader {
	return &FileSkillLoader{Directory: directory, Source: source}
}

// NewCollectionSkillLoader creates a loader for a skill repo whose skills
// reference files relative to the repo root (e.g. data/, scripts/).
// directory must be the repo root — each subdirectory containing a SKILL.md
// becomes one skill, and the repo root is injected as the base path.
func NewCollectionSkillLoader(repoRoot string, source SettingSource) *FileSkillLoader {
	return &FileSkillLoader{Directory: repoRoot, Source: source, CollectionRoot: repoRoot}
}

func (l *FileSkillLoader) LoadSkills() ([]Skill, error) {
	if l.Directory == "" {
		return []Skill{}, nil
	}

	skillFiles, err := findSkillMarkdownFiles(l.Directory)
	if err != nil {
		return nil, err
	}

	var skills []Skill

	for _, skillFile := range skillFiles {
		skill, err := l.loadSkillFile(skillFile, l.Source)
		if err != nil {
			slog.Warn("skills: error loading skill file", "file", skillFile, "err", err)
			continue
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

func (l *FileSkillLoader) GetSkill(name string) (*Skill, error) {
	skills, err := l.LoadSkills()
	if err != nil {
		return nil, err
	}

	for _, skill := range skills {
		if skill.Name == name || skill.DisplayName == name {
			return &skill, nil
		}
	}

	return nil, nil
}

func (l *FileSkillLoader) GetSkillDirCommands(cwd string) ([]Skill, error) {
	return l.GetSkillDirCommandsForUser(cwd, "")
}

func (l *FileSkillLoader) GetSkillDirCommandsForUser(cwd string, userID string) ([]Skill, error) {
	var allSkills []Skill

	managedSkills := NewFileSkillLoaderWithSource(GetManagedSkillsPath(), SettingSourcePolicySettings)
	userSkills := NewFileSkillLoaderWithSource(GetUserSkillsPathForUser(userID), SettingSourceUserSettings)
	projectSkills := NewFileSkillLoaderWithSource(GetProjectSkillsPath(cwd), SettingSourceProjectSettings)
	builtinSkills := NewFileSkillLoaderWithSource(GetBuiltinSkillsPath(), SettingSourceProjectSettings)
	legacyCommands := NewFileSkillLoaderWithSource(filepath.Join(cwd, DefaultCommandsDirectory), SettingSourceProjectSettings)

	sources := []SkillLoader{projectSkills, managedSkills, userSkills, builtinSkills, legacyCommands}

	// Add any cloned skill repo collections (e.g. paperasse).
	// Each repo root is a skill collection: every subdirectory containing a SKILL.md
	// is loaded as a skill, and the repo root is used as the file path base.
	reposDir := GetSkillReposPath()
	if entries, err := os.ReadDir(reposDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			repoRoot := filepath.Join(reposDir, e.Name())
			sources = append(sources, NewCollectionSkillLoader(repoRoot, SettingSourceProjectSettings))
		}
	}

	for _, source := range sources {
		loaded, err := source.LoadSkills()
		if err != nil {
			slog.Warn("skills: error loading source", "err", err)
			continue
		}
		allSkills = append(allSkills, loaded...)
	}

	allSkills = deduplicateSkills(allSkills)

	return allSkills, nil
}

func deduplicateSkills(skills []Skill) []Skill {
	seen := make(map[string]bool)
	var result []Skill

	for _, skill := range skills {
		key := skill.Name
		if !seen[key] {
			seen[key] = true
			result = append(result, skill)
		}
	}

	return result
}

func isSkillFile(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))
	return base == "skill.md"
}

func findSkillMarkdownFiles(basePath string) ([]string, error) {
	var skillFiles []string
	var topLevelDirs []string

	entries, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(basePath, entry.Name())

		if entry.IsDir() {
			topLevelDirs = append(topLevelDirs, entryPath)
			continue
		}

		if isSymlink, err := isSymlinkDir(entryPath); err == nil && isSymlink {
			topLevelDirs = append(topLevelDirs, entryPath)
			continue
		}

		// Single-skill repos (e.g. code-review-skill) place their SKILL.md
		// directly at the repo root rather than inside a named subdirectory.
		if isSkillFile(entryPath) {
			skillFiles = append(skillFiles, entryPath)
		}
	}

	for _, dir := range topLevelDirs {
		files, err := walkSkillDir(dir)
		if err != nil {
			continue
		}
		skillFiles = append(skillFiles, files...)
	}

	sort.Strings(skillFiles)
	return skillFiles, nil
}

func walkSkillDir(dir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			subFiles, err := walkSkillDir(entryPath)
			if err != nil {
				continue
			}
			files = append(files, subFiles...)
			continue
		}

		if isSymlink, err := isSymlinkDir(entryPath); err == nil && isSymlink {
			subFiles, err := walkSkillDir(entryPath)
			if err != nil {
				continue
			}
			files = append(files, subFiles...)
			continue
		}

		if isSkillFile(entryPath) {
			files = append(files, entryPath)
		}
	}

	return files, nil
}

func (l *FileSkillLoader) loadSkillFile(path string, source SettingSource) (Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	frontmatter, markdownContent := ParseFrontmatter(string(content), path)

	skillName := getSkillCommandName(path, l.Directory)

	// For collection loaders the AI needs the repo root to resolve relative
	// paths (data/, scripts/, …).  Pass it as the effective base dir so it
	// becomes the value of ${NEXUS_SKILL_DIR} and is injected as a header.
	baseDir := filepath.Dir(path)
	if l.CollectionRoot != "" {
		baseDir = l.CollectionRoot
	}

	skill, err := CreateSkillFromFrontmatter(frontmatter, markdownContent, skillName, baseDir, string(source), source)
	return skill, err
}

func getSkillCommandName(filePath string, baseDir string) string {
	skillDirPath := filepath.Dir(filePath)
	commandBaseName := filepath.Base(skillDirPath)

	if !strings.HasPrefix(skillDirPath, baseDir) {
		return commandBaseName
	}

	relativePath := strings.TrimPrefix(skillDirPath, baseDir)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))

	if relativePath == "" {
		return commandBaseName
	}

	// relativePath already contains the skill's own directory name, so convert
	// path separators to colons to form the full namespaced identifier.
	// e.g. "my-skill" → "my-skill", "ns/foo" → "ns:foo"
	return strings.ReplaceAll(relativePath, string(filepath.Separator), ":")
}

func isSymlinkDir(path string) (bool, error) {
	var st syscall.Stat_t
	err := syscall.Lstat(path, &st)
	if err != nil {
		return false, err
	}
	return (st.Mode & syscall.S_IFMT) == syscall.S_IFLNK, nil
}

func CreateSkillFromFrontmatter(frontmatter FrontmatterData, markdownContent string, skillName string, baseDir string, loadedFrom string, source SettingSource) (Skill, error) {
	description := ""
	if frontmatter.Description != nil {
		if desc, ok := frontmatter.Description.(string); ok {
			description = desc
		}
	}

	if description == "" {
		description = ExtractDescriptionFromMarkdown(markdownContent, "Skill")
	}

	userInvocable := true
	if frontmatter.UserInvocable != nil {
		userInvocable = ParseBooleanFrontmatter(frontmatter.UserInvocable)
	}

	disableModelInvocation := false
	if frontmatter.DisableModelInvocation != nil {
		disableModelInvocation = ParseBooleanFrontmatter(frontmatter.DisableModelInvocation)
	}

	argumentNames := ParseArgumentNames(frontmatter.Arguments)
	allowedTools := ParseStringList(frontmatter.AllowedTools)

	var model string
	if frontmatter.Model != "" && frontmatter.Model != "inherit" {
		model = frontmatter.Model
	}

	var effort string
	if frontmatter.Effort != "" {
		effort = ParseEffortValue(frontmatter.Effort)
	}

	var execContext ExecutionContext
	if frontmatter.Context == "fork" {
		execContext = ExecutionContextFork
	} else if frontmatter.Context == "inline" {
		execContext = ExecutionContextInline
	}

	hooks := ParseHooksFromFrontmatter(frontmatter.Hooks)
	shell := ParseShellFrontmatter(frontmatter.Shell, skillName)

	var displayName string
	if frontmatter.Name != nil {
		if dn, ok := frontmatter.Name.(string); ok {
			displayName = dn
		}
	}

	var version string
	if frontmatter.Version != "" {
		version = frontmatter.Version
	}

	var agent string
	if frontmatter.Agent != "" {
		agent = frontmatter.Agent
	}

	paths := SplitPathInFrontmatter(frontmatter.Paths)
	triggers := ParseStringList(frontmatter.Triggers)
	preambleTier := ParsePreambleTier(frontmatter.PreambleTier)
	requires := ParseRequires(frontmatter.Requires)

	scope := scopeForSource(source)

	capturedContent := markdownContent
	capturedArgNames := argumentNames
	capturedBaseDir := baseDir
	capturedTier := preambleTier
	capturedRequires := requires

	return Skill{
		Name:                        skillName,
		DisplayName:                 displayName,
		Description:                 description,
		HasUserSpecifiedDescription: description != "" && frontmatter.Description != nil,
		AllowedTools:                allowedTools,
		ArgumentHint:                frontmatter.ArgumentHint,
		ArgNames:                    argumentNames,
		WhenToUse:                   frontmatter.WhenToUse,
		Version:                     version,
		Model:                       model,
		DisableModelInvocation:      disableModelInvocation,
		UserInvocable:               userInvocable,
		Context:                     execContext,
		Agent:                       agent,
		Effort:                      effort,
		Paths:                       paths,
		ContentLength:               len(markdownContent),
		IsHidden:                    !userInvocable,
		ProgressMessage:             "running",
		Source:                      SkillSource(source),
		LoadedFrom:                  LoadedFrom(loadedFrom),
		SkillRoot:                   baseDir,
		Hooks:                       hooks,
		Shell:                       shell,
		Triggers:                    triggers,
		PreambleTier:                preambleTier,
		Scope:                       scope,
		Requires:                    requires,
		GetPromptForCommand: func(cmdArgs string, _ context.Context) ([]ContentBlock, error) {
			expanded := SubstituteArguments(capturedContent, cmdArgs, capturedArgNames)
			expanded = SubstituteNexusVariables(expanded, capturedBaseDir, sessionID)
			expanded = applyPreamble(expanded, capturedBaseDir, capturedTier)
			if preflight := buildPreflightSection(capturedRequires, capturedBaseDir); preflight != "" {
				expanded = preflight + "\n\n---\n\n" + expanded
			}
			return []ContentBlock{{Type: "text", Text: expanded}}, nil
		},
	}, nil
}

// buildPreflightSection generates a pre-flight dependency check block to prepend
// to the skill prompt when the skill declares requirements. The agent runs each
// check command and, if it fails, requests user permission before installing.
func buildPreflightSection(reqs []SkillRequirement, skillDir string) string {
	if len(reqs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Pre-flight: Dependency Check\n\n")
	b.WriteString("Before running this skill, verify each dependency below using Bash.\n")
	b.WriteString("If a check fails:\n")
	b.WriteString("1. Tell the user which dependency is missing and why it is needed.\n")
	b.WriteString("2. Ask for explicit permission before running the install command.\n")
	b.WriteString("3. Only proceed with the skill once all required deps are satisfied.\n\n")

	for i, r := range reqs {
		label := r.Type
		if label == "" {
			label = fmt.Sprintf("dependency %d", i+1)
		}
		optional := ""
		if r.Optional {
			optional = " *(optional — skill runs in degraded mode if missing)*"
		}
		b.WriteString(fmt.Sprintf("### %s%s\n", label, optional))
		if len(r.Packages) > 0 {
			b.WriteString(fmt.Sprintf("- **Packages**: %s\n", strings.Join(r.Packages, ", ")))
		}
		if r.Check != "" {
			b.WriteString(fmt.Sprintf("- **Check**: `%s`\n", r.Check))
		}
		if r.InstallCmd != "" {
			installCmd := strings.ReplaceAll(r.InstallCmd, "${NEXUS_SKILL_DIR}", skillDir)
			b.WriteString(fmt.Sprintf("- **Install** (only with user permission): `%s`\n", installCmd))
		}
	}
	return b.String()
}

// applyPreamble injects context headers into the skill prompt based on the tier.
//
//	0 (default): inject base dir comment — same as the previous behaviour.
//	1 (basic):   no extra context — skill content only.
//	2 (standard): inject base dir comment.
//	3 (full):    inject base dir + full-context marker (reserved for user prefs).
func applyPreamble(content, baseDir string, tier int) string {
	switch tier {
	case 1:
		return content
	case 3:
		if baseDir != "" {
			return fmt.Sprintf("<!-- skill base directory: %s -->\n<!-- context-level: full -->\n\n", baseDir) + content
		}
		return content
	default: // 0 and 2 both inject the base dir (backward compatible)
		if baseDir != "" {
			return fmt.Sprintf("<!-- skill base directory: %s -->\n\n", baseDir) + content
		}
		return content
	}
}

// scopeForSource maps a SettingSource to the corresponding SkillScope.
func scopeForSource(source SettingSource) SkillScope {
	switch source {
	case SettingSourcePolicySettings:
		return ScopeAdmin
	case SettingSourceUserSettings:
		return ScopeUser
	case SettingSourcePlugin:
		return ScopeSystem
	default: // SettingSourceProjectSettings and anything unknown
		return ScopeRepo
	}
}

func DiscoverSkillDirsForPaths(filePaths []string, cwd string) []string {
	var newDirs []string
	visitedDirs := make(map[string]bool)

	resolvedCwd := cwd
	if strings.HasSuffix(resolvedCwd, string(filepath.Separator)) {
		resolvedCwd = resolvedCwd[:len(resolvedCwd)-1]
	}

	for _, filePath := range filePaths {
		currentDir := filepath.Dir(filePath)

		for len(currentDir) > len(resolvedCwd) && strings.HasPrefix(currentDir, resolvedCwd+string(filepath.Separator)) {
			skillDir := filepath.Join(currentDir, ".claude", "skills")

			if !visitedDirs[skillDir] {
				visitedDirs[skillDir] = true

				if _, err := os.Stat(skillDir); err == nil {
					newDirs = append(newDirs, skillDir)
				}
			}

			parent := filepath.Dir(currentDir)
			if parent == currentDir {
				break
			}
			currentDir = parent
		}
	}

	sort.Slice(newDirs, func(i, j int) bool {
		return strings.Count(newDirs[j], string(filepath.Separator)) > strings.Count(newDirs[i], string(filepath.Separator))
	})

	return newDirs
}

func SubstituteArguments(content string, args string, argumentNames []string) string {
	if args == "" || len(argumentNames) == 0 {
		return content
	}

	result := content

	for _, argName := range argumentNames {
		placeholder := "${argument:" + argName + "}"
		result = strings.ReplaceAll(result, placeholder, args)
		result = strings.ReplaceAll(result, "${"+argName+"}", args)
	}

	return result
}

func SubstituteNexusVariables(content string, skillDir string, sessionID string) string {
	result := content
	result = strings.ReplaceAll(result, "${NEXUS_SKILL_DIR}", skillDir)
	result = strings.ReplaceAll(result, "${NEXUS_SESSION_ID}", sessionID)
	return result
}

func ExecuteSkillPrompt(skill Skill, args string, ctx context.Context) ([]ContentBlock, error) {
	// Default implementation - should be overridden by actual skill prompt function
	prompt := ""

	if skill.SkillRoot != "" {
		prompt += "Base directory for this skill: " + skill.SkillRoot + "\n\n"
	}

	prompt += SubstituteArguments(prompt, args, skill.ArgNames)
	prompt = SubstituteNexusVariables(prompt, skill.SkillRoot, sessionID)

	return []ContentBlock{{Type: "text", Text: prompt}}, nil
}

// --- Bundled Skills Registry ---

var bundledSkillsRegistry []Skill

func RegisterBundledSkill(def BundledSkillDefinition) {
	skill := Skill{
		Name:                        def.Name,
		DisplayName:                 def.Name,
		Description:                 def.Description,
		HasUserSpecifiedDescription: true,
		AllowedTools:                def.AllowedTools,
		ArgumentHint:                def.ArgumentHint,
		ArgNames:                    nil,
		WhenToUse:                   def.WhenToUse,
		Version:                     "",
		Model:                       def.Model,
		DisableModelInvocation:      def.DisableModelInvocation,
		UserInvocable:               def.UserInvocable,
		Context:                     def.Context,
		Agent:                       def.Agent,
		Effort:                      def.Effort,
		Paths:                       nil,
		ContentLength:               0,
		IsHidden:                    !def.UserInvocable,
		ProgressMessage:             "running",
		Source:                      SourceBundled,
		LoadedFrom:                  LoadedFromBundled,
		SkillRoot:                   "",
		Hooks:                       def.Hooks,
		Shell:                       nil,
		Triggers:                    def.Triggers,
		PreambleTier:                0,
		Scope:                       ScopeSystem,
		GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
			return def.GetPromptForCommand(args, ctx)
		},
	}

	bundledSkillsRegistry = append(bundledSkillsRegistry, skill)
}

func GetBundledSkills() []Skill {
	result := make([]Skill, len(bundledSkillsRegistry))
	copy(result, bundledSkillsRegistry)
	return result
}

func ClearBundledSkills() {
	bundledSkillsRegistry = nil
}

// ReadSkillEnabled reads the user-invocable value from a skill.md file on disk.
// Returns true if the file cannot be read or the field is absent.
func ReadSkillEnabled(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	fm, _ := ParseFrontmatter(string(data), path)
	if fm.UserInvocable == nil {
		return true
	}
	return ParseBooleanFrontmatter(fm.UserInvocable)
}

func GetSkillsRootPath() string {
	return runtimepath.SkillsDir("")
}

func GetBuiltinSkillsPath() string {
	return filepath.Join(runtimepath.SkillsDir(""), "builtin")
}

func GetSkillReposPath() string {
	return filepath.Join(runtimepath.SkillsDir(""), "repos")
}

func GetAllSkills(cwd string) ([]Skill, error) {
	return GetAllSkillsForUser(cwd, "")
}

func GetAllSkillsForUser(cwd string, userID string) ([]Skill, error) {
	var allSkills []Skill

	// File-based skills first (project > managed > user > builtin > legacy) so they override bundled.
	loader := NewFileSkillLoader("")
	fileSkills, err := loader.GetSkillDirCommandsForUser(cwd, userID)
	if err != nil {
		return nil, err
	}
	allSkills = append(allSkills, fileSkills...)

	// Bundled skills last — lowest priority.
	allSkills = append(allSkills, GetBundledSkills()...)

	return deduplicateSkills(allSkills), nil
}

// GetSkillsForContext returns skills that are active for the given file context.
// Skills with no Paths restriction are always included. Skills with Paths are
// included only when at least one of the provided filePaths matches a pattern.
// When filePaths is empty, all skills are returned (no filtering).
func GetSkillsForContext(cwd string, filePaths []string, userID string) ([]Skill, error) {
	all, err := GetAllSkillsForUser(cwd, userID)
	if err != nil {
		return nil, err
	}
	if len(filePaths) == 0 {
		return all, nil
	}
	result := make([]Skill, 0, len(all))
	for _, sk := range all {
		if len(sk.Paths) == 0 {
			result = append(result, sk)
			continue
		}
		if skillMatchesAnyPath(sk, filePaths, cwd) {
			result = append(result, sk)
		}
	}
	return result, nil
}

func skillMatchesAnyPath(sk Skill, filePaths []string, cwd string) bool {
	for _, fp := range filePaths {
		for _, pattern := range sk.Paths {
			if matchesPathPattern(fp, pattern, cwd) {
				return true
			}
		}
	}
	return false
}

// MatchTrigger returns the first skill whose trigger phrases match userInput,
// or nil if no skill matches. Matching is case-insensitive substring search.
func MatchTrigger(userInput string, skills []Skill) *Skill {
	lower := strings.ToLower(userInput)
	for i := range skills {
		for _, t := range skills[i].Triggers {
			if t != "" && strings.Contains(lower, strings.ToLower(strings.TrimSpace(t))) {
				return &skills[i]
			}
		}
	}
	return nil
}

// Skill change detection state
var (
	skillCacheValid   bool = true
	conditionalSkills map[string]Skill
	dynamicSkills     map[string]Skill
)

func init() {
	conditionalSkills = make(map[string]Skill)
	dynamicSkills = make(map[string]Skill)
}

func InvalidateSkillCache() {
	skillCacheValid = false
}

func ClearSkillCaches() {
	skillCacheValid = false
	conditionalSkills = make(map[string]Skill)
	dynamicSkills = make(map[string]Skill)
}

func ActivateConditionalSkillsForPaths(filePaths []string, cwd string) []string {
	var activated []string

	for name, skill := range conditionalSkills {
		if len(skill.Paths) == 0 {
			continue
		}

		for _, filePath := range filePaths {
			for _, pattern := range skill.Paths {
				if matchesPathPattern(filePath, pattern, cwd) {
					dynamicSkills[name] = skill
					activated = append(activated, name)
					delete(conditionalSkills, name)
					break
				}
			}
		}
	}

	if len(activated) > 0 {
		skillCacheValid = false
	}

	return activated
}

func matchesPathPattern(filePath string, pattern string, cwd string) bool {
	// Simple pattern matching - just check if the file matches the pattern
	// In a real implementation, this would use the ignore library
	relPath := filePath
	if !isAbs(pattern) && len(cwd) > 0 {
		if len(filePath) > len(cwd) && filePath[:len(cwd)] == cwd {
			relPath = filePath[len(cwd):]
		}
	}

	// Simple wildcard matching
	if pattern == "**" {
		return true
	}

	if strings.HasPrefix(pattern, "**/") {
		pattern = pattern[3:]
		return strings.HasSuffix(relPath, pattern)
	}

	if strings.HasSuffix(pattern, "/**") {
		prefix := pattern[:len(pattern)-3]
		return strings.HasPrefix(relPath, prefix)
	}

	return relPath == pattern || strings.Contains(relPath, pattern)
}

func isAbs(path string) bool {
	return len(path) > 0 && (path[0] == '/' || path[0] == '\\')
}
