package schema

import (
	"encoding/json"
	"fmt"
)

// Validator validates tool input against JSON Schema.
type Validator struct {
	// schemas maps tool names to their JSON schemas.
	schemas map[string]JSONSchema
}

// JSONSchema represents a JSON Schema for tool input.
type JSONSchema struct {
	Type                 string                `json:"type,omitempty"`
	Description          string                `json:"description,omitempty"`
	Properties           map[string]JSONSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	Items                *JSONSchema           `json:"items,omitempty"`
	Default              any                   `json:"default,omitempty"`
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
}

// NewValidator creates a new schema validator.
func NewValidator() *Validator {
	return &Validator{
		schemas: make(map[string]JSONSchema),
	}
}

// RegisterSchema registers a JSON schema for a tool.
func (v *Validator) RegisterSchema(toolName string, schema JSONSchema) {
	v.schemas[toolName] = schema
}

// RegisterSchemaFromMap registers a schema from a map (useful for simple definitions).
func (v *Validator) RegisterSchemaFromMap(toolName string, schemaMap map[string]any) error {
	v.schemas[toolName] = FromMap(schemaMap)
	return nil
}

// Validate validates input against the registered schema for the tool.
// Returns a normalized input map if validation succeeds.
func (v *Validator) Validate(toolName string, input map[string]any) (map[string]any, error) {
	schema, ok := v.schemas[toolName]
	if !ok {
		// No schema registered for this tool - pass through
		return input, nil
	}

	// Check required fields
	for _, required := range schema.Required {
		if _, ok := input[required]; !ok {
			return nil, fmt.Errorf("missing required field: %s", required)
		}
	}

	// Type checking for known fields
	for key, value := range input {
		propDef, ok := schema.Properties[key]
		if !ok {
			// Unknown field - could be ignored or rejected depending on policy
			// For now, we allow extra fields
			continue
		}

		if err := validateValue(key, value, propDef); err != nil {
			return nil, err
		}
	}

	return input, nil
}

// validateValue checks a single value against its property definition.
func validateValue(key string, value any, propDef JSONSchema) error {
	if value == nil {
		return nil
	}

	switch propDef.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %s: expected string, got %T", key, value)
		}
	case "integer", "number":
		switch value.(type) {
		case int, int64, float32, float64, json.Number:
			// Valid numeric types
		default:
			return fmt.Errorf("field %s: expected number, got %T", key, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field %s: expected boolean, got %T", key, value)
		}
	case "array":
		values, ok := value.([]any)
		if !ok {
			return fmt.Errorf("field %s: expected array, got %T", key, value)
		}
		if propDef.Items != nil {
			for idx, item := range values {
				if err := validateValue(fmt.Sprintf("%s[%d]", key, idx), item, *propDef.Items); err != nil {
					return err
				}
			}
		}
	case "object":
		objectValue, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("field %s: expected object, got %T", key, value)
		}
		for _, required := range propDef.Required {
			if _, ok := objectValue[required]; !ok {
				return fmt.Errorf("field %s: missing required field %s", key, required)
			}
		}
		for nestedKey, nestedValue := range objectValue {
			nestedSchema, ok := propDef.Properties[nestedKey]
			if !ok {
				continue
			}
			if err := validateValue(fmt.Sprintf("%s.%s", key, nestedKey), nestedValue, nestedSchema); err != nil {
				return err
			}
		}
	}

	// Check enum constraints if present
	if len(propDef.Enum) > 0 {
		strValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %s: enum values must be strings, got %T", key, value)
		}
		valid := false
		for _, allowed := range propDef.Enum {
			if strValue == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("field %s: value %q not in enum %v", key, strValue, propDef.Enum)
		}
	}

	return nil
}

// FromMap converts a permissive map representation into the structured schema
// model used by the contract types.
func FromMap(schemaMap map[string]any) JSONSchema {
	if len(schemaMap) == 0 {
		return JSONSchema{}
	}

	schema := JSONSchema{}
	if value, ok := schemaMap["type"].(string); ok {
		schema.Type = value
	}
	if value, ok := schemaMap["description"].(string); ok {
		schema.Description = value
	}
	if value, ok := schemaMap["default"]; ok {
		schema.Default = value
	}
	if value, ok := schemaMap["additionalProperties"]; ok {
		schema.AdditionalProperties = value
	}
	if rawRequired, ok := schemaMap["required"]; ok {
		schema.Required = stringSlice(rawRequired)
	}
	if rawEnum, ok := schemaMap["enum"]; ok {
		schema.Enum = stringSlice(rawEnum)
	}
	if rawItems, ok := schemaMap["items"].(map[string]any); ok {
		items := FromMap(rawItems)
		schema.Items = &items
	}
	if rawProperties, ok := schemaMap["properties"].(map[string]any); ok {
		properties := make(map[string]JSONSchema, len(rawProperties))
		for key, rawProperty := range rawProperties {
			propertyMap, ok := rawProperty.(map[string]any)
			if !ok {
				continue
			}
			properties[key] = FromMap(propertyMap)
		}
		schema.Properties = properties
	}

	return schema
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}
