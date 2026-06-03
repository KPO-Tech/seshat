package hooks

import "sort"

// Registry stores tool hooks and exposes stable pre/post stage views.
type Registry struct {
	hooks []ToolHook
}

func NewRegistry() *Registry {
	return &Registry{hooks: make([]ToolHook, 0)}
}

func (r *Registry) Add(hook ToolHook) {
	if r == nil {
		return
	}
	r.hooks = append(r.hooks, hook)
	sort.SliceStable(r.hooks, func(i, j int) bool {
		return r.hooks[i].Priority > r.hooks[j].Priority
	})
}

func (r *Registry) ByStage(stage ToolHookStage) []ToolHook {
	if r == nil || len(r.hooks) == 0 {
		return nil
	}
	out := make([]ToolHook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		if hook.Stage == stage {
			out = append(out, hook)
		}
	}
	return out
}

func (r *Registry) Pre() []ToolHook {
	return r.ByStage(ToolHookStagePre)
}

func (r *Registry) Post() []ToolHook {
	return r.ByStage(ToolHookStagePost)
}
