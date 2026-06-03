package skills

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
)

var mcpManagerInstance *mcp.MCPClientManager

func SetMCPManager(manager *mcp.MCPClientManager) {
	mcpManagerInstance = manager
}

type MCPSkillBuilder struct {
	CreateSkillCommand     func() interface{}
	ParseFrontmatterFields func(interface{}, string, string) interface{}
}

var mcpSkillBuilders []MCPSkillBuilder

func RegisterMCPSkillBuilder(builder MCPSkillBuilder) {
	mcpSkillBuilders = append(mcpSkillBuilders, builder)
}

func LoadMCPSkills(serverName string) ([]Skill, error) {
	if mcpManagerInstance == nil {
		return []Skill{}, fmt.Errorf("MCP manager not initialized")
	}

	client, ok := mcpManagerInstance.GetClient(serverName)
	if !ok {
		return []Skill{}, fmt.Errorf("server '%s' not connected", serverName)
	}

	ctx := context.Background()
	mcpTools, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools from %s: %w", serverName, err)
	}

	skills := make([]Skill, 0, len(mcpTools))
	for _, tool := range mcpTools {
		skill := Skill{
			Name:                        fmt.Sprintf("mcp:%s:%s", serverName, tool.Name),
			DisplayName:                 tool.Name,
			Description:                 tool.Description,
			HasUserSpecifiedDescription: false,
			AllowedTools:                []string{},
			ArgumentHint:                "",
			ArgNames:                    []string{},
			WhenToUse:                   fmt.Sprintf("Use MCP tool '%s' from server '%s'", tool.Name, serverName),
			Version:                     "",
			Model:                       "",
			DisableModelInvocation:      false,
			UserInvocable:               true,
			Context:                     "",
			Agent:                       "",
			Effort:                      "",
			Paths:                       nil,
			ContentLength:               0,
			IsHidden:                    false,
			ProgressMessage:             "running",
			Source:                      SourceMCP,
			LoadedFrom:                  LoadedFromMCP,
			SkillRoot:                   serverName,
			Hooks:                       nil,
			Shell:                       nil,
			GetPromptForCommand: func(args string, ctx context.Context) ([]ContentBlock, error) {
				return executeMCPSkill(serverName, tool.Name, args, ctx)
			},
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

func DiscoverMCPSkills(ctx context.Context) ([]Skill, error) {
	if mcpManagerInstance == nil {
		return []Skill{}, nil
	}

	servers := mcpManagerInstance.GetConnectedServers()
	var allMCPSkills []Skill

	for _, serverName := range servers {
		skills, err := LoadMCPSkills(serverName)
		if err != nil {
			slog.Warn("MCP skills: failed to load skills", "server", serverName, "err", err)
			continue
		}
		allMCPSkills = append(allMCPSkills, skills...)
	}

	return allMCPSkills, nil
}

func executeMCPSkill(serverName string, toolName string, args string, ctx context.Context) ([]ContentBlock, error) {
	if mcpManagerInstance == nil {
		return nil, fmt.Errorf("MCP manager not initialized")
	}

	client, ok := mcpManagerInstance.GetClient(serverName)
	if !ok {
		return nil, fmt.Errorf("server '%s' not connected", serverName)
	}

	arguments := parseToolArguments(args)
	result, err := client.CanonicalToolCall(ctx, toolName, arguments)
	if err != nil {
		return []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}}, nil
	}

	if result.IsError {
		return []ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %s", result.Text)}}, nil
	}

	return []ContentBlock{{Type: "text", Text: result.Text}}, nil
}

func parseToolArguments(args string) map[string]any {
	if args == "" {
		return nil
	}

	arguments := make(map[string]any)
	pairs := strings.Fields(args)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			arguments[parts[0]] = parts[1]
		} else {
			arguments[fmt.Sprintf("arg%d", len(arguments))] = pair
		}
	}
	return arguments
}

func IsMCPEnabled() bool {
	if mcpManagerInstance == nil {
		return false
	}
	return len(mcpManagerInstance.GetConnectedServers()) > 0
}

func GetMCPServers() []string {
	if mcpManagerInstance == nil {
		return []string{}
	}
	return mcpManagerInstance.GetConnectedServers()
}

func GetMCPHealth() map[string]string {
	if mcpManagerInstance == nil {
		return map[string]string{"status": "not initialized"}
	}

	health := make(map[string]string)
	servers := mcpManagerInstance.GetConnectedServers()
	health["connected_servers"] = fmt.Sprintf("%d", len(servers))
	health["servers"] = strings.Join(servers, ", ")

	return health
}

// --- Skill Change Detection ---

type SkillChangeCallback func(added []string, removed []string)

var skillChangeCallbacks []SkillChangeCallback

func OnSkillChange(callback SkillChangeCallback) {
	skillChangeCallbacks = append(skillChangeCallbacks, callback)
}

func NotifySkillChange(added []string, removed []string) {
	for _, callback := range skillChangeCallbacks {
		callback(added, removed)
	}
}

// --- Skill Path Validation ---

func ValidateSkillPath(skillPath string) error {
	if strings.Contains(skillPath, "..") {
		return fmt.Errorf("invalid skill path: path traversal not allowed")
	}
	if strings.Contains(skillPath, "/") && !strings.HasPrefix(skillPath, ".") {
		return fmt.Errorf("invalid skill path: must be relative to skills directory")
	}
	return nil
}

// --- Skill Aliases ---

type SkillAlias struct {
	Alias  string
	Target string
}

var skillAliases []SkillAlias

func RegisterSkillAlias(alias string, target string) {
	skillAliases = append(skillAliases, SkillAlias{Alias: alias, Target: target})
}

func ResolveSkillAlias(name string) string {
	for _, a := range skillAliases {
		if a.Alias == name {
			return a.Target
		}
	}
	return name
}

func GetSkillAliases() []SkillAlias {
	return skillAliases
}
