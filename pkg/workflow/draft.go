package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DraftFormatYAML = "yaml"
	DraftFormatJSON = "json"
)

// DraftOptions asks for a workflow definition to be normalised, validated and rendered. Format is "yaml" (the default, also "yml") or "json".
type DraftOptions struct {
	Definition Definition
	Format     string
}

// DraftResult is a normalised definition and its rendering in Format. Diagnostics holds the reason when the definition is invalid or the format is not supported.
type DraftResult struct {
	Definition  Definition `json:"definition"`
	Format      string     `json:"format"`
	Content     string     `json:"content"`
	Diagnostics []string   `json:"diagnostics,omitempty"`
}

// Draft normalises and validates a definition, then renders it as YAML or JSON. When it fails, it returns the error together with a result whose Diagnostics holds its message.
func Draft(options DraftOptions) (DraftResult, error) {
	format, err := normalizeDraftFormat(options.Format)
	if err != nil {
		return DraftResult{
			Definition:  options.Definition,
			Format:      strings.TrimSpace(options.Format),
			Diagnostics: []string{err.Error()},
		}, err
	}

	def := NormalizeDefinition(options.Definition)
	if err := Validate(def); err != nil {
		return DraftResult{
			Definition:  def,
			Format:      format,
			Diagnostics: []string{err.Error()},
		}, err
	}

	content, err := renderDefinition(def, format)
	if err != nil {
		return DraftResult{
			Definition:  def,
			Format:      format,
			Diagnostics: []string{err.Error()},
		}, err
	}

	return DraftResult{
		Definition: def,
		Format:     format,
		Content:    content,
	}, nil
}

// NormalizeDefinition trims the names, ids, kinds, agents, prompts and output formats of a definition, lower-cases the kinds, replaces a negative MaxTurns by 0, and cleans the lists of needs and routes.
func NormalizeDefinition(def Definition) Definition {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	for i := range def.Nodes {
		def.Nodes[i].ID = strings.TrimSpace(def.Nodes[i].ID)
		def.Nodes[i].Kind = normalizeNodeKind(def.Nodes[i].Kind)
		def.Nodes[i].Agent = strings.TrimSpace(def.Nodes[i].Agent)
		def.Nodes[i].Prompt = strings.TrimSpace(def.Nodes[i].Prompt)
		def.Nodes[i].OutputFormat = strings.TrimSpace(def.Nodes[i].OutputFormat)
		if def.Nodes[i].MaxTurns < 0 {
			def.Nodes[i].MaxTurns = 0
		}
		needs := make([]string, 0, len(def.Nodes[i].Needs))
		for _, need := range def.Nodes[i].Needs {
			need = strings.TrimSpace(need)
			if need != "" {
				needs = append(needs, need)
			}
		}
		def.Nodes[i].Needs = needs
		routes := make([]string, 0, len(def.Nodes[i].Routes))
		for _, route := range def.Nodes[i].Routes {
			route = strings.TrimSpace(route)
			if route != "" {
				routes = append(routes, route)
			}
		}
		def.Nodes[i].Routes = routes
	}
	return def
}

func normalizeDraftFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", DraftFormatYAML, "yml":
		return DraftFormatYAML, nil
	case DraftFormatJSON:
		return DraftFormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported workflow draft format %q; use yaml or json", format)
	}
}

func renderDefinition(def Definition, format string) (string, error) {
	switch format {
	case DraftFormatYAML:
		data, err := yaml.Marshal(def)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)) + "\n", nil
	case DraftFormatJSON:
		data, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data) + "\n", nil
	default:
		return "", errors.New("unsupported workflow draft format")
	}
}
