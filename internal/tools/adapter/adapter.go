package adapter

import (
	"context"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

// RegistryToToolMap converts a registry to a tool map compatible with execution orchestrator.
func RegistryToToolMap(reg *tool.Registry) map[string]tool.Tool {
	builder := tool.NewSurfaceBuilder(reg)
	result, err := builder.BuildToolMap(context.Background(), tool.SurfaceBuildRequest{
		IncludeReadOnly:    true,
		IncludeDestructive: true,
	})
	if err != nil {
		return map[string]tool.Tool{}
	}
	return result
}

// ToolMapToRegistry converts a tool map to a registry
func ToolMapToRegistry(tools map[string]tool.Tool) *tool.Registry {
	reg := tool.NewRegistry()

	for _, t := range tools {
		_ = reg.Register(t)
	}

	return reg
}

// GetToolDefinitions returns all tool definitions from a registry
func GetToolDefinitions(reg *tool.Registry) []tool.Definition {
	builder := tool.NewSurfaceBuilder(reg)
	surface, err := builder.Build(context.Background(), tool.SurfaceBuildRequest{
		IncludeReadOnly:    true,
		IncludeDestructive: true,
	})
	if err != nil {
		return []tool.Definition{}
	}

	definitions := make([]tool.Definition, 0, len(surface.Tools))

	for _, entry := range surface.Tools {
		resolved, ok := reg.Resolve(entry.Name)
		if !ok {
			continue
		}
		definitions = append(definitions, resolved.Definition())
	}

	return definitions
}

// FilterToolsByName filters tools by name
func FilterToolsByName(reg *tool.Registry, names []string) map[string]tool.Tool {
	result := make(map[string]tool.Tool)

	for _, name := range names {
		if resolved, exists := reg.Resolve(name); exists {
			result[name] = resolved
		}
	}

	return result
}

// FilterToolsByCategory filters tools by category
func FilterToolsByCategory(reg *tool.Registry, category string) map[string]tool.Tool {
	tools := reg.ListByCategory(category)
	result := make(map[string]tool.Tool)

	for _, t := range tools {
		def := t.Definition()
		result[def.Name] = t
	}

	return result
}
