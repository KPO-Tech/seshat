package skills

import (
	"fmt"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseFrontmatter(content string, filePath string) (FrontmatterData, string) {
	content = strings.TrimSpace(content)

	if !strings.HasPrefix(content, "---") {
		return FrontmatterData{}, content
	}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return FrontmatterData{}, content
	}

	frontmatterYAML := strings.TrimSpace(parts[1])
	markdownContent := strings.TrimSpace(parts[2])

	var frontmatter FrontmatterData
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		slog.Warn("skills: failed to parse frontmatter", "file", filePath, "err", err)
		return FrontmatterData{}, content
	}

	return frontmatter, markdownContent
}

func ParseBooleanFrontmatter(value any) bool {
	if value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true" || v == "1"
	case int:
		return v != 0
	default:
		return false
	}
}

func ParseStringList(value any) []string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		if len(v) == 0 {
			return nil
		}
		return v
	case string:
		if v == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	default:
		return nil
	}
}

// ParsePreambleTier converts a frontmatter preamble-tier value to an int.
//
//	0 = unset (backward-compatible default: inject base dir)
//	1 = basic    — skill content only, no extra context
//	2 = standard — + working directory header
//	3 = full     — + working directory + full context hint
func ParsePreambleTier(value any) int {
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		if v >= 1 && v <= 3 {
			return v
		}
	case float64:
		iv := int(v)
		if iv >= 1 && iv <= 3 {
			return iv
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "basic":
			return 1
		case "2", "standard":
			return 2
		case "3", "full":
			return 3
		}
	}
	return 0
}

func ParseArgumentNames(value any) []string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			switch entry := item.(type) {
			case string:
				result = append(result, entry)
			case map[string]any:
				// The documented structured form: {name, description, default}
				// (docs/skills.md). Only the name feeds argument substitution
				// today - description is for human/model documentation, and
				// there is nowhere yet to apply a default.
				if name, ok := entry["name"].(string); ok && name != "" {
					result = append(result, name)
				}
			}
		}
		return result
	case string:
		if v == "" {
			return nil
		}
		// Parse formats like: "--message, -m" or just "message"
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			// Remove leading dashes
			part = strings.TrimLeft(part, "-")
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	default:
		return nil
	}
}

func ParseEffortValue(value string) string {
	validEfforts := []string{"minimal", "low", "medium", "high", "maximum"}
	value = strings.ToLower(strings.TrimSpace(value))

	for _, eff := range validEfforts {
		if value == eff {
			return value
		}
	}

	// Try parsing as integer
	// If it's a valid integer, it's allowed (for custom effort levels)

	return value
}

func ParseShellFrontmatter(value any, skillName string) *FrontmatterShell {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]any:
		shell := &FrontmatterShell{}
		if before, ok := v["before"].([]any); ok {
			shell.Before = interfaceToStringList(before)
		}
		if after, ok := v["after"].([]any); ok {
			shell.After = interfaceToStringList(after)
		}
		if onError, ok := v["on_error"].([]any); ok {
			shell.OnError = interfaceToStringList(onError)
		}
		if onComplete, ok := v["on_complete"].([]any); ok {
			shell.OnComplete = interfaceToStringList(onComplete)
		}
		return shell
	default:
		return nil
	}
}

func ParseHooksFromFrontmatter(value any) *HooksSettings {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]any:
		hooks := &HooksSettings{}
		if beforeTool, ok := v["before_tool"].([]any); ok {
			hooks.BeforeTool = interfaceToStringList(beforeTool)
		}
		if afterTool, ok := v["after_tool"].([]any); ok {
			hooks.AfterTool = interfaceToStringList(afterTool)
		}
		if before, ok := v["before"].([]any); ok {
			hooks.Before = interfaceToStringList(before)
		}
		if after, ok := v["after"].([]any); ok {
			hooks.After = interfaceToStringList(after)
		}
		if onError, ok := v["on_error"].([]any); ok {
			hooks.OnError = interfaceToStringList(onError)
		}
		if onCancel, ok := v["on_cancel"].([]any); ok {
			hooks.OnCancel = interfaceToStringList(onCancel)
		}
		if onComplete, ok := v["on_complete"].([]any); ok {
			hooks.OnComplete = interfaceToStringList(onComplete)
		}
		if toolAllowed, ok := v["tool_allowed"].([]any); ok {
			hooks.ToolAllowed = interfaceToStringList(toolAllowed)
		}
		if toolDenied, ok := v["tool_denied"].([]any); ok {
			hooks.ToolDenied = interfaceToStringList(toolDenied)
		}
		return hooks
	default:
		return nil
	}
}

func interfaceToStringList(arr []any) []string {
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func SplitPathInFrontmatter(paths any) []string {
	if paths == nil {
		return nil
	}

	switch v := paths.(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		if v == "" {
			return nil
		}
		// Support both newline and comma separated
		result := strings.FieldsFunc(v, func(r rune) bool {
			return r == '\n' || r == ',' || r == ';'
		})
		for i := range result {
			result[i] = strings.TrimSpace(result[i])
		}
		return result
	default:
		return nil
	}
}

func ExtractDescriptionFromMarkdown(content string, fallbackLabel string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "##") {
			continue
		}
		if line != "" {
			if len(line) > 200 {
				line = line[:197] + "..."
			}
			return line
		}
	}
	return fallbackLabel + " skill"
}

// ParseRequires converts a frontmatter requires value into a slice of
// SkillRequirement. Accepts either a YAML list of maps or a single map.
func ParseRequires(value any) []SkillRequirement {
	if value == nil {
		return nil
	}

	toReq := func(m map[string]any) (SkillRequirement, bool) {
		r := SkillRequirement{}
		if v, ok := m["type"].(string); ok {
			r.Type = v
		}
		if v, ok := m["check"].(string); ok {
			r.Check = v
		}
		if v, ok := m["install-cmd"].(string); ok {
			r.InstallCmd = v
		}
		if v, ok := m["optional"].(bool); ok {
			r.Optional = v
		}
		if pkgs, ok := m["packages"].([]any); ok {
			for _, p := range pkgs {
				if s, ok := p.(string); ok {
					r.Packages = append(r.Packages, s)
				}
			}
		}
		return r, r.Type != "" || r.Check != ""
	}

	switch v := value.(type) {
	case []any:
		var result []SkillRequirement
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if r, ok := toReq(m); ok {
					result = append(result, r)
				}
			}
		}
		return result
	case map[string]any:
		if r, ok := toReq(v); ok {
			return []SkillRequirement{r}
		}
	}
	return nil
}

func CoerceDescriptionToString(value any, skillName string) *string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return &v
	case int, float64:
		s := fmt.Sprintf("%v", v)
		return &s
	default:
		return nil
	}
}
