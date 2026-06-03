package tools

import (
	"github.com/EngineerProjects/nexus-engine/internal/tools/builtin"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

// NewBuiltinRegistry is deprecated. Use internal/core/tools/builtin.NewBuiltinRegistry.
func NewBuiltinRegistry() (*tool.Registry, error) {
	return builtin.NewBuiltinRegistry()
}

// NewBuiltinTools is deprecated. Use builtin.NewBuiltinRegistry + List instead.
func NewBuiltinTools() ([]tool.Tool, error) {
	reg, err := NewBuiltinRegistry()
	if err != nil {
		return nil, err
	}
	return reg.List(), nil
}

// RegisterBuiltinTools is deprecated. Use internal/core/tools/builtin.RegisterBuiltinTools.
func RegisterBuiltinTools(registry *tool.Registry) error {
	return builtin.RegisterBuiltinTools(registry)
}
