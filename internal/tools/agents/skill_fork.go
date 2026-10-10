package agents

import (
	"context"
	"fmt"
	"time"

	coreagent "github.com/KPO-Tech/seshat/internal/agent"
	"github.com/KPO-Tech/seshat/internal/engine"
	"github.com/KPO-Tech/seshat/internal/types"
)

// SkillForkRunner implements skills.ForkRunner: it runs a `context: fork`
// skill's rendered prompt as an isolated sub-agent, reusing the same
// spawn-depth guard, wall-clock timeout, and bypass permission mode every
// other sub-agent path in this package already uses, rather than inventing
// a second, divergent execution policy for skills specifically.
type SkillForkRunner struct {
	engine *engine.Engine
}

// NewSkillForkRunner builds a SkillForkRunner bound to eng. Wire the result
// into the registered skill tool via SkillTool.SetForkRunner once eng
// exists (the tool itself is constructed and registered before the engine
// is, so this is a post-construction injection - see pkg/sdk/client.go).
func NewSkillForkRunner(eng *engine.Engine) *SkillForkRunner {
	return &SkillForkRunner{engine: eng}
}

// RunForkedSkill runs task (the skill's already-rendered prompt, arguments
// already substituted) as a sub-agent scoped to allowedTools and model -
// both straight from the skill's own frontmatter (AllowedTools/Model),
// which have no effect on the default, non-forked execution path. Returns
// the sub-agent's final text output.
func (r *SkillForkRunner) RunForkedSkill(ctx context.Context, skillName, task string, allowedTools []string, model string) (string, error) {
	if r == nil || r.engine == nil {
		return "", fmt.Errorf("skill fork unavailable: no engine configured")
	}

	depth := subAgentDepth(ctx)
	maxDepth := effectiveMaxDepth(ctx)
	if depth >= maxDepth {
		return "", fmt.Errorf(
			"skill fork rejected: maximum sub-agent depth %d reached (current depth %d)",
			maxDepth, depth,
		)
	}

	toolPatterns := allowedTools
	if len(toolPatterns) == 0 {
		toolPatterns = []string{"*"}
	}

	// A fresh, call-scoped registry: this agent type exists only for this
	// one fork, named after the skill so logs/errors are legible, and never
	// shared with any other call.
	agentType := "skill-fork:" + skillName
	tempRegistry := coreagent.NewAgentRegistry()
	tempRegistry.Register(&coreagent.AgentDefinition{
		AgentType: agentType,
		Tools:     toolPatterns,
		Model:     model,
	})

	timeout := time.Duration(coreagent.DefaultSubAgentTimeout) * time.Second
	subCtx, cancel := context.WithTimeout(withSubAgentDepth(ctx), timeout)
	defer cancel()

	result, err := coreagent.RunAgent(&coreagent.RunConfig{
		AgentType:      agentType,
		Task:           task,
		Engine:         r.engine,
		Context:        subCtx,
		MaxTurns:       coreagent.DefaultSubAgentMaxTurns,
		Registry:       tempRegistry,
		PermissionMode: types.PermissionModeBypass,
	})
	if err != nil {
		return "", err
	}
	if !result.Success {
		return "", fmt.Errorf("skill fork failed: %s", result.Error)
	}
	return result.Output, nil
}
