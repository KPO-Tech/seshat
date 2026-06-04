package sdk

import (
	"context"
	"encoding/json"

	shellhooks "github.com/EngineerProjects/nexus-engine/internal/hooks"
	runtimehooks "github.com/EngineerProjects/nexus-engine/internal/runtime/hooks"
)

// registerShellPreToolHooks converts PreToolHookConfig entries to ToolHooks
// backed by our shell hooks runner and registers them on the orchestrator.
func (c *Client) registerShellPreToolHooks(cfgs []PreToolHookConfig, workDir string) {
	hookCfgs := make([]shellhooks.HookConfig, len(cfgs))
	for i, h := range cfgs {
		hookCfgs[i] = shellhooks.HookConfig{
			Matcher: h.Matcher,
			Command: h.Command,
			Timeout: h.Timeout,
		}
	}

	runner := shellhooks.NewRunner(hookCfgs, workDir, workDir)

	hook := runtimehooks.ToolHook{
		Stage:    runtimehooks.ToolHookStagePre,
		Priority: 10,
		ID:       "shell-pre-tool-hooks",
		Execute: func(ctx context.Context, input runtimehooks.ToolHookInput) runtimehooks.ToolHookResult {
			// Serialize tool input to JSON for the shell process.
			inputJSON, err := json.Marshal(input.Input)
			if err != nil {
				inputJSON = []byte("{}")
			}

			sessionID := ""
			if ctx != nil {
				// Best-effort: extract session ID from context if available.
				if v, ok := ctx.Value(contextKeySessionID{}).(string); ok {
					sessionID = v
				}
			}

			agg, runErr := runner.Run(ctx, sessionID, input.ToolName, string(inputJSON))
			if runErr != nil {
				return runtimehooks.ToolHookResult{} // non-blocking error
			}

			if agg.Halt || agg.Decision == shellhooks.DecisionDeny {
				reason := agg.Reason
				if reason == "" {
					if agg.Halt {
						reason = "turn halted by pre-tool hook"
					} else {
						reason = "tool call blocked by pre-tool hook"
					}
				}
				return runtimehooks.ToolHookResult{
					Stop: &runtimehooks.ToolHookStop{Content: reason, IsError: true},
				}
			}

			result := runtimehooks.ToolHookResult{}

			// Rewrite tool input if the hook returned updatedInput.
			if agg.UpdatedInput != "" {
				var updated map[string]any
				if err := json.Unmarshal([]byte(agg.UpdatedInput), &updated); err == nil {
					result.UpdatedInput = updated
				}
			}

			return result
		},
	}

	c.AddToolHook(hook)
}

// contextKeySessionID is the context key for the current session ID.
type contextKeySessionID struct{}
