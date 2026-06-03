// Package schema provides types and utilities for structured output constraints.
//
// StructuredOutputInfo defines a JSON Schema that can be passed to providers
// supporting native JSON schema output (OpenAI json_schema mode, Gemini
// responseSchema). Providers without native support receive an injected system
// message asking them to respond with conforming JSON.
//
// Usage with a Go struct:
//
//	type BugReport struct {
//	    Title    string `json:"title"    desc:"One-line summary of the bug"`
//	    Severity string `json:"severity" desc:"critical|high|medium|low" enum:"critical,high,medium,low"`
//	    Steps    []string `json:"steps"  desc:"Reproduction steps"`
//	}
//	s := schema.FromStruct("bug_report", "A structured bug report", BugReport{})
//
// Usage with a manual schema:
//
//	s := schema.New("result", "Task result", map[string]any{
//	    "properties": map[string]any{
//	        "done":    map[string]any{"type": "boolean"},
//	        "message": map[string]any{"type": "string"},
//	    },
//	}, []string{"done", "message"})
package schema

import (
	"reflect"
	"strings"
)

// StructuredOutputInfo defines a JSON Schema constraint for model output.
// Name is used as the schema identifier in the provider request.
type StructuredOutputInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Required    []string       `json:"required"`
}

// New creates a StructuredOutputInfo from an explicit parameters map.
func New(name, description string, parameters map[string]any, required []string) *StructuredOutputInfo {
	return &StructuredOutputInfo{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Required:    required,
	}
}

// FromStruct derives a StructuredOutputInfo by reflecting on a Go struct.
//
// Supported struct tags:
//   - json:"name" — overrides the JSON property name (required)
//   - desc:"…"    — adds a "description" field to the property schema
//   - enum:"a,b"  — restricts the value to the listed strings
//   - required:"false" — marks the field as nullable (pointer or omitempty also do this)
//
// All non-pointer, non-omitempty fields are included in `required` and OpenAI
// strict mode is satisfied because every property appears in `required`.
func FromStruct(name, description string, v any) *StructuredOutputInfo {
	props, req := generateSchema(v)
	return &StructuredOutputInfo{
		Name:        name,
		Description: description,
		Parameters:  props,
		Required:    req,
	}
}

// SystemPromptHint returns a compact instruction suitable for injection into a
// system prompt when the provider does not support native structured output.
// It renders the schema as inline JSON so the model knows the expected shape.
func (s *StructuredOutputInfo) SystemPromptHint() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("Respond ONLY with valid JSON that matches this schema — no prose, no markdown fences:\n")
	b.WriteString(`{"name":"`)
	b.WriteString(s.Name)
	b.WriteString(`","required":`)
	b.WriteString(jsonList(s.Required))
	b.WriteString(`,"properties":`)
	b.WriteString(jsonMapKeys(s.Parameters))
	b.WriteString("}")
	return b.String()
}

// OpenAIResponseFormat returns the `response_format` object for the OpenAI
// Messages API json_schema mode.
func (s *StructuredOutputInfo) OpenAIResponseFormat() map[string]any {
	jsonSchema := map[string]any{
		"name":   s.Name,
		"strict": true,
		"schema": map[string]any{
			"type":                 "object",
			"properties":           s.Parameters,
			"required":             s.Required,
			"additionalProperties": false,
		},
	}
	return map[string]any{
		"type":        "json_schema",
		"json_schema": jsonSchema,
	}
}

// GeminiResponseSchema returns the `responseSchema` object for the Gemini
// generateContent API.
func (s *StructuredOutputInfo) GeminiResponseSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": s.Parameters,
		"required":   s.Required,
	}
}

// ─── Schema generator (reflection) ───────────────────────────────────────────

func generateSchema(v any) (map[string]any, []string) {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, nil
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, nil
	}

	properties := make(map[string]any)
	var required []string

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("json")
		if name == "" {
			name = strings.ToLower(field.Name)
		} else {
			name = strings.Split(name, ",")[0]
			if name == "-" {
				continue
			}
		}

		optional := field.Type.Kind() == reflect.Pointer ||
			strings.Contains(field.Tag.Get("json"), "omitempty") ||
			field.Tag.Get("required") == "false"

		prop := buildPropertySchema(field, optional)
		properties[name] = prop
		required = append(required, name)
	}

	return properties, required
}

func buildPropertySchema(field reflect.StructField, optional bool) map[string]any {
	prop := make(map[string]any)

	ft := field.Type
	if ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}

	// Nested struct
	if ft.Kind() == reflect.Struct {
		nested, nestedReq := generateSchema(reflect.New(ft).Elem().Interface())
		if nested != nil {
			if optional {
				prop["type"] = []string{"object", "null"}
			} else {
				prop["type"] = "object"
			}
			prop["properties"] = nested
			prop["required"] = nestedReq
			prop["additionalProperties"] = false
			applyCommonTags(prop, field)
			return prop
		}
	}

	// Slice / array
	if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
		elemType := ft.Elem()
		items := map[string]any{"type": goTypeToJSONType(elemType)}
		if elemType.Kind() == reflect.Struct {
			nested, nestedReq := generateSchema(reflect.New(elemType).Elem().Interface())
			if nested != nil {
				items = map[string]any{
					"type":                 "object",
					"properties":           nested,
					"required":             nestedReq,
					"additionalProperties": false,
				}
			}
		}
		prop["type"] = "array"
		prop["items"] = items
		applyCommonTags(prop, field)
		return prop
	}

	// Scalar
	base := goTypeToJSONType(field.Type)
	if optional {
		prop["type"] = []string{base, "null"}
	} else {
		prop["type"] = base
	}
	applyCommonTags(prop, field)
	return prop
}

func applyCommonTags(prop map[string]any, field reflect.StructField) {
	if desc := field.Tag.Get("desc"); desc != "" {
		prop["description"] = desc
	}
	if enum := field.Tag.Get("enum"); enum != "" {
		prop["enum"] = strings.Split(enum, ",")
	}
}

func goTypeToJSONType(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return "string"
	}
}

// ─── Minimal JSON helpers (no full marshal to keep the hint readable) ─────────

func jsonList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(s)
		b.WriteByte('"')
	}
	b.WriteByte(']')
	return b.String()
}

func jsonMapKeys(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteByte('{')
	i := 0
	for k := range m {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(k)
		b.WriteString(`":"..."`)
		i++
	}
	b.WriteByte('}')
	return b.String()
}
