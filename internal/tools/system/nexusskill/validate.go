package nexusskill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"

	"gopkg.in/yaml.v3"
)

var skillDirNamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// ValidateTool implements nexus_validate_skill.
type ValidateTool struct{ baseTool }

func NewValidateTool() *ValidateTool { return &ValidateTool{} }

func (t *ValidateTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "nexus_validate_skill",
		DisplayName: "Validate Skill",
		Description: "Validate a Nexus skill directory against the runtime's expected structure and frontmatter fields. Returns blocking errors and non-blocking warnings.",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the skill directory to validate",
				},
			},
			"required": []string{"path"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

type validationResult struct {
	errors   []string
	warnings []string
}

func (v *validationResult) error(msg string) { v.errors = append(v.errors, "ERROR: "+msg) }
func (v *validationResult) warn(msg string)  { v.warnings = append(v.warnings, "WARN:  "+msg) }

func (t *ValidateTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	path, _ := input.Parsed["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		return tool.NewErrorResult(fmt.Errorf("path is required")), nil
	}

	res := &validationResult{}
	cleanPath := filepath.Clean(path)
	dirName := filepath.Base(cleanPath)
	if !skillDirNamePattern.MatchString(dirName) {
		res.error(fmt.Sprintf("directory name %q must use lowercase letters, digits, hyphens, or underscores", dirName))
	}
	if strings.HasPrefix(dirName, "-") || strings.HasSuffix(dirName, "-") || strings.Contains(dirName, "--") {
		res.error(fmt.Sprintf("directory name %q cannot start/end with hyphen or contain consecutive hyphens", dirName))
	}
	if len(dirName) > 64 {
		res.error(fmt.Sprintf("directory name too long: %d chars (max 64)", len(dirName)))
	}

	// 1. skill.md must exist
	skillMd := filepath.Join(cleanPath, "skill.md")
	content, err := os.ReadFile(skillMd)
	if err != nil {
		res.error("skill.md not found at " + skillMd)
		return tool.NewTextResult(formatValidationResult(cleanPath, res)), nil
	}

	// 2. Frontmatter must be present and parseable
	fm, body, fmErr := parseSkillFrontmatter(string(content))
	if fmErr != nil {
		res.error("Invalid frontmatter: " + fmErr.Error())
		return tool.NewTextResult(formatValidationResult(cleanPath, res)), nil
	}

	// 3. Required fields
	displayName := strings.TrimSpace(fmt.Sprintf("%v", fm["name"]))
	desc := strings.TrimSpace(fmt.Sprintf("%v", fm["description"]))

	if displayName == "" {
		res.error("Missing required field: name")
	} else {
		if len(displayName) > 120 {
			res.warn(fmt.Sprintf("display name is long (%d chars) — consider a shorter, scan-friendly title", len(displayName)))
		}
		if strings.Contains(displayName, "\n") {
			res.error("name must be a single line")
		}
	}

	if desc == "" {
		res.error("Missing required field: description")
	} else {
		if len(desc) > 1024 {
			res.error(fmt.Sprintf("description too long: %d chars (max 1024)", len(desc)))
		}
		if len(desc) < 30 {
			res.warn(fmt.Sprintf("description is very short (%d chars) — it should say WHEN to invoke AND what it does", len(desc)))
		}
		if strings.Contains(desc, "<") || strings.Contains(desc, ">") {
			res.error("description cannot contain angle brackets")
		}
		lowerDesc := strings.ToLower(desc)
		if !strings.Contains(lowerDesc, "when") && !strings.Contains(lowerDesc, "use") && !strings.Contains(lowerDesc, "for") {
			res.warn("description may not clearly describe when to invoke the skill — include trigger conditions")
		}
	}

	// 4. Unknown frontmatter fields
	known := map[string]bool{
		"name": true, "description": true, "when_to_use": true,
		"user-invocable": true, "argument-hint": true, "arguments": true,
		"allowed-tools": true, "model": true, "disable-model-invocation": true,
		"context": true, "agent": true, "effort": true,
		"version": true, "paths": true, "hooks": true, "shell": true,
	}
	for k := range fm {
		if !known[k] {
			res.warn(fmt.Sprintf("unknown frontmatter field: %q — may be ignored by the runtime", k))
		}
	}

	// 5. Body must not be empty
	if strings.TrimSpace(body) == "" {
		res.error("skill.md has no body content after the frontmatter")
	} else if len(strings.TrimSpace(body)) < 100 {
		res.warn("skill body is very short — consider adding workflow steps, examples, or reference pointers")
	}

	// 6. Directory size hint
	totalLines := 0
	_ = filepath.Walk(cleanPath, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(p), ".md") {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr == nil {
			totalLines += strings.Count(string(data), "\n")
		}
		return nil
	})
	if totalLines > 800 {
		res.warn(fmt.Sprintf("skill is large (~%d lines across .md files) — consider moving some content to references/", totalLines))
	}

	return tool.NewTextResult(formatValidationResult(cleanPath, res)), nil
}

func (t *ValidateTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	if p, ok := input["path"].(string); !ok || strings.TrimSpace(p) == "" {
		return nil, fmt.Errorf("path is required and must be a non-empty string")
	}
	return input, nil
}

func (t *ValidateTool) Description(_ context.Context) (string, error) {
	return "Validate a Nexus skill directory for structural and quality issues", nil
}

func formatValidationResult(path string, res *validationResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Validation: %s\n\n", filepath.Base(path)))

	if len(res.errors) == 0 && len(res.warnings) == 0 {
		sb.WriteString("OK: no issues found.\n")
		return sb.String()
	}

	all := append(res.errors, res.warnings...)
	sb.WriteString(fmt.Sprintf("%d error(s), %d warning(s):\n\n", len(res.errors), len(res.warnings)))
	for _, msg := range all {
		sb.WriteString("  " + msg + "\n")
	}

	if len(res.errors) > 0 {
		sb.WriteString("\nSkill has errors — fix before publishing.\n")
	} else {
		sb.WriteString("\nSkill is valid (warnings are non-blocking).\n")
	}
	return sb.String()
}

// parseSkillFrontmatter splits skill.md into frontmatter map and body.
func parseSkillFrontmatter(content string) (map[string]any, string, error) {
	if !strings.HasPrefix(content, "---") {
		return nil, content, fmt.Errorf("no frontmatter found (must start with ---)")
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, content, fmt.Errorf("frontmatter not closed (no closing ---)")
	}
	fmText := rest[:idx]
	body := rest[idx+4:]

	var fm map[string]any
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return nil, body, fmt.Errorf("YAML parse error: %w", err)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	return fm, body, nil
}
