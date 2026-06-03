package config

import (
	"context"
	"encoding/json"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Tool name
const ToolNameConfig = "config"

const Description = `Get or set configuration settings for the AI assistant.

Usage:
- GET: Provide only the setting key to read its current value
- SET: Provide both setting and value keys to update

Examples:
- Get a setting: {"setting": "theme"}
- Set a setting: {"setting": "theme", "value": "light"}

Supported settings:
- theme: UI theme (dark, light)
- model: Default model to use
- maxTokens: Maximum tokens per response
- temperature: Model temperature (0.0 to 1.0)

Notes:
- Use GET to check current values
- Some settings may require restart to take effect
- Invalid values will be rejected with an error`

// ConfigTool provides get/set configuration functionality
type ConfigTool struct {
	// configStore handles the configuration storage
	configStore ConfigStore
}

// ConfigStore interface for configuration storage
type ConfigStore interface {
	Get(key string) (any, bool)
	Set(key string, value any) error
	Delete(key string) error
	GetAll() map[string]any
}

// DefaultConfigStore is a simple in-memory config store
type DefaultConfigStore struct {
	data map[string]any
}

func NewDefaultConfigStore() *DefaultConfigStore {
	return &DefaultConfigStore{
		data: make(map[string]any),
	}
}

func (s *DefaultConfigStore) Get(key string) (any, bool) {
	val, ok := s.data[key]
	return val, ok
}

func (s *DefaultConfigStore) Set(key string, value any) error {
	s.data[key] = value
	return nil
}

func (s *DefaultConfigStore) Delete(key string) error {
	delete(s.data, key)
	return nil
}

func (s *DefaultConfigStore) GetAll() map[string]any {
	result := make(map[string]any)
	for k, v := range s.data {
		result[k] = v
	}
	return result
}

// Config for the tool
type ConfigToolConfig struct {
	// WorkingDir is the working directory
	WorkingDir string

	// Store is the configuration store (optional)
	Store ConfigStore
}

// DefaultConfig returns default configuration
func DefaultConfigToolConfig() *ConfigToolConfig {
	return &ConfigToolConfig{
		WorkingDir: ".",
	}
}

// NewConfigTool creates a new config tool
func NewConfigTool(config *ConfigToolConfig) *ConfigTool {
	if config == nil {
		config = DefaultConfigToolConfig()
	}

	store := config.Store
	if store == nil {
		store = NewDefaultConfigStore()
	}

	return &ConfigTool{
		configStore: store,
	}
}

// Definition returns the tool definition
func (t *ConfigTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameConfig,
		DisplayName:        "Config",
		SearchHint:         "get or set configuration settings",
		Description:        Description,
		Category:           "configuration",
		IsReadOnly:         true,
		IsDestructive:      false,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
		Metadata: map[string]any{
			"is_stateful": false,
		},
	}
}

// Input represents the tool input
type Input struct {
	// Setting is the setting key
	Setting string `json:"setting"`

	// Value is the new value (omit for GET)
	Value any `json:"value,omitempty"`
}

// Output represents the tool output
type Output struct {
	// Success indicates if the operation was successful
	Success bool `json:"success"`

	// Operation is "get" or "set"
	Operation string `json:"operation,omitempty"`

	// Setting is the setting key
	Setting string `json:"setting,omitempty"`

	// Value is the current value
	Value any `json:"value,omitempty"`

	// PreviousValue is the previous value (for SET)
	PreviousValue any `json:"previousValue,omitempty"`

	// NewValue is the new value (for SET)
	NewValue any `json:"newValue,omitempty"`

	// Error is the error message if failed
	Error string `json:"error,omitempty"`
}

// Call executes the tool
func (t *ConfigTool) Call(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
	// Parse input
	var parsed Input
	if err := json.Unmarshal([]byte(input.Raw), &parsed); err != nil {
		return tool.NewErrorResult(fmt.Errorf("Failed to parse input: %w", err)), nil
	}

	// Validate required fields
	if parsed.Setting == "" {
		return tool.NewErrorResult(fmt.Errorf("setting is required")), nil
	}

	// Determine operation
	isGet := parsed.Value == nil

	// GET operation
	if isGet {
		value, exists := t.configStore.Get(parsed.Setting)
		if !exists {
			return tool.NewErrorResult(fmt.Errorf("Setting '%s' not found", parsed.Setting)), nil
		}

		return tool.CallResult{
			Data: map[string]any{
				"success":   true,
				"operation": "get",
				"setting":   parsed.Setting,
				"value":     value,
			},
			Content: fmt.Sprintf("%s = %v", parsed.Setting, value),
		}, nil
	}

	// SET operation
	previousValue, _ := t.configStore.Get(parsed.Setting)

	// Set the new value
	if err := t.configStore.Set(parsed.Setting, parsed.Value); err != nil {
		return tool.NewErrorResult(fmt.Errorf("Failed to set: %w", err)), nil
	}

	return tool.CallResult{
		Data: map[string]any{
			"success":       true,
			"operation":     "set",
			"setting":       parsed.Setting,
			"previousValue": previousValue,
			"newValue":      parsed.Value,
		},
		Content: fmt.Sprintf("Set %s to %v", parsed.Setting, parsed.Value),
	}, nil
}

// ValidateInput validates tool input
func (t *ConfigTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Check required fields
	if input["setting"] == nil || input["setting"] == "" {
		return map[string]any{
			"result":  false,
			"message": "setting is required",
		}, nil
	}

	return input, nil
}

// CheckPermissions checks permissions
func (t *ConfigTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	// Allow read operations, ask for write
	if input["value"] == nil {
		return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
	}
	return types.PermissionResult{Behavior: types.PermissionBehaviorAsk}
}

// IsConcurrencySafe returns whether tool is concurrency safe
func (t *ConfigTool) IsConcurrencySafe() bool {
	return true
}

// IsReadOnly returns whether tool is read-only
func (t *ConfigTool) IsReadOnly(input tool.CallInput) bool {
	var parsed Input
	if err := json.Unmarshal([]byte(input.Raw), &parsed); err != nil {
		return true
	}
	return parsed.Value == nil
}
