package registry

import (
	"context"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type surfaceTestTool struct {
	def Definition
}

func (t surfaceTestTool) Definition() Definition { return t.def }
func (t surfaceTestTool) Call(ctx context.Context, input CallInput, permissionCheck types.CanUseToolFn) (CallResult, error) {
	return CallResult{Content: "ok", Data: "ok", ContentType: ContentTypeText}, nil
}
func (t surfaceTestTool) Description(ctx context.Context) (string, error) {
	return t.def.Description, nil
}
func (t surfaceTestTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t surfaceTestTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx ToolUseContext) types.PermissionResult {
	return types.AllowWithUpdatedInput(input)
}
func (t surfaceTestTool) IsConcurrencySafe(input map[string]any) bool { return t.def.IsConcurrencySafe }
func (t surfaceTestTool) IsReadOnly(input map[string]any) bool        { return t.def.IsReadOnly }
func (t surfaceTestTool) IsEnabled() bool                             { return true }
func (t surfaceTestTool) FormatResult(data any) string                { return "ok" }
func (t surfaceTestTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

func TestBuildSurfaceExcludesDeferredToolsWhenToolSearchEnabled(t *testing.T) {
	t.Setenv("ENABLE_TOOL_SEARCH", "true")

	reg := NewRegistry()
	if err := reg.Register(surfaceTestTool{def: Definition{Name: "Read", Description: "read", IsReadOnly: true}}); err != nil {
		t.Fatalf("failed to register non-deferred tool: %v", err)
	}
	if err := reg.Register(surfaceTestTool{def: Definition{Name: "Deploy", Description: "deploy", ShouldDefer: true}}); err != nil {
		t.Fatalf("failed to register deferred tool: %v", err)
	}

	surface, err := NewSurfaceBuilder(reg).Build(context.Background(), SurfaceBuildRequest{
		IncludeReadOnly:    true,
		IncludeDestructive: true,
	})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(surface.Tools) != 1 {
		t.Fatalf("expected only the non-deferred tool in the initial surface, got %#v", surface.Tools)
	}
	if surface.Tools[0].Name != "Read" {
		t.Fatalf("expected Read in the initial surface, got %q", surface.Tools[0].Name)
	}
}

func TestBuildSurfaceFiltersBySurfaceProfile(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(surfaceTestTool{def: Definition{
		Name:        "TodoWrite",
		Description: "todo",
		IsReadOnly:  true,
	}}); err != nil {
		t.Fatalf("failed to register TodoWrite tool: %v", err)
	}
	if err := reg.Register(surfaceTestTool{def: Definition{
		Name:        "TaskCreate",
		Description: "task create",
		IsReadOnly:  true,
		Metadata: map[string]any{
			toolSurfaceProfilesMetadataKey: []string{ToolSurfaceProfileSubagent},
		},
	}}); err != nil {
		t.Fatalf("failed to register TaskCreate tool: %v", err)
	}

	monoSurface, err := NewSurfaceBuilder(reg).Build(context.Background(), SurfaceBuildRequest{
		IncludeReadOnly: true,
		SurfaceProfile:  ToolSurfaceProfileMonoRun,
	})
	if err != nil {
		t.Fatalf("mono-run Build failed: %v", err)
	}
	if len(monoSurface.Tools) != 1 || monoSurface.Tools[0].Name != "TodoWrite" {
		t.Fatalf("expected mono-run surface to exclude subagent-only tools, got %#v", monoSurface.Tools)
	}

	subagentSurface, err := NewSurfaceBuilder(reg).Build(context.Background(), SurfaceBuildRequest{
		IncludeReadOnly: true,
		SurfaceProfile:  ToolSurfaceProfileSubagent,
	})
	if err != nil {
		t.Fatalf("subagent Build failed: %v", err)
	}
	if len(subagentSurface.Tools) != 2 {
		t.Fatalf("expected subagent surface to include both tools, got %#v", subagentSurface.Tools)
	}
}
