// Package auto - Transcript building for classifier.
//
// This module handles building the conversation transcript that gets sent
// to the classifier. It converts Nexus messages into a compact format that
// includes user messages and tool use blocks, while respecting character limits.
//
// Key Features:
// - Converts Message[] to TranscriptEntry[]
// - Supports JSONL and text format for transcript
// - Tool input encoding via Tool interface
// - Character limit truncation for context management
package auto

import (
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Tool interface defines the contract for tool input encoding.
// Each tool implements ToAutoClassifierInput to provide a compact
// representation of its input for the classifier.
type Tool interface {
	Name() string                                      // Tool name (e.g., "Read", "Edit")
	Aliases() []string                                 // Alternative names for the tool
	ToAutoClassifierInput(input map[string]any) string // Encode input for classifier
}

type ToolRegistry map[string]Tool

func BuildToolLookup(tools []Tool) ToolRegistry {
	lookup := make(ToolRegistry)
	for _, tool := range tools {
		lookup[tool.Name()] = tool
		for _, alias := range tool.Aliases() {
			lookup[alias] = tool
		}
	}
	return lookup
}

func (r ToolRegistry) Get(name string) Tool {
	return r[name]
}

type DefaultTool struct {
	name    string
	aliases []string
	inputFn func(map[string]any) string
}

func (t DefaultTool) Name() string {
	return t.name
}

func (t DefaultTool) Aliases() []string {
	return t.aliases
}

func (t DefaultTool) ToAutoClassifierInput(input map[string]any) string {
	if t.inputFn != nil {
		return t.inputFn(input)
	}
	return FormatToolUseCompact(t.name, input)
}

func NewDefaultTool(name string, aliases []string, inputFn func(map[string]any) string) Tool {
	return DefaultTool{
		name:    name,
		aliases: aliases,
		inputFn: inputFn,
	}
}

var DefaultToolRegistry = []Tool{
	NewDefaultTool("Read", []string{}, func(input map[string]any) string {
		path := input["path"]
		return fmt.Sprintf("Read %v", path)
	}),
	NewDefaultTool("Edit", []string{}, func(input map[string]any) string {
		oldString := input["old_string"]
		newString := input["new_string"]
		return fmt.Sprintf("Edit: replace %v with %v", oldString, newString)
	}),
	NewDefaultTool("Write", []string{}, func(input map[string]any) string {
		path := input["path"]
		content := input["content"]
		return fmt.Sprintf("Write %v (length=%d)", path, len(fmt.Sprintf("%v", content)))
	}),
	NewDefaultTool("bash", []string{"Shell", "Command"}, func(input map[string]any) string {
		command := input["command"]
		return fmt.Sprintf("Bash %v", command)
	}),
	NewDefaultTool("Glob", []string{}, func(input map[string]any) string {
		pattern := input["pattern"]
		return fmt.Sprintf("Glob %v", pattern)
	}),
	NewDefaultTool("Grep", []string{}, func(input map[string]any) string {
		pattern := input["pattern"]
		path := input["path"]
		return fmt.Sprintf("Grep %v in %v", pattern, path)
	}),
	NewDefaultTool("LS", []string{}, func(input map[string]any) string {
		path := input["path"]
		return fmt.Sprintf("LS %v", path)
	}),
	NewDefaultTool("web_fetch", []string{"fetch"}, func(input map[string]any) string {
		url := input["url"]
		return fmt.Sprintf("WebFetch %v", url)
	}),
	NewDefaultTool("TodoRead", []string{}, func(input map[string]any) string {
		return "TodoRead"
	}),
	NewDefaultTool("todo_write", []string{}, func(input map[string]any) string {
		content := input["content"]
		return fmt.Sprintf("TodoWrite %v", content)
	}),
}

type TranscriptBuilder struct {
	tools        []Tool
	maxChars     int
	jsonlEnabled bool
}

func NewTranscriptBuilder(tools []Tool, maxChars int, jsonlEnabled bool) *TranscriptBuilder {
	if tools == nil {
		tools = DefaultToolRegistry
	}
	if maxChars <= 0 {
		maxChars = MaxTranscriptChars
	}
	return &TranscriptBuilder{
		tools:        tools,
		maxChars:     maxChars,
		jsonlEnabled: jsonlEnabled,
	}
}

func (tb *TranscriptBuilder) Build(messages []types.Message, action *TranscriptEntry) string {
	toolLookup := BuildToolLookup(tb.tools)

	transcriptEntries := BuildTranscriptFromMessages(messages)

	var result strings.Builder
	totalChars := 0

	for i := len(transcriptEntries) - 1; i >= 0; i-- {
		entry := transcriptEntries[i]
		entryText := serializeEntryCompact(entry, toolLookup, tb.jsonlEnabled)

		if totalChars+len(entryText) > tb.maxChars {
			break
		}

		result.WriteString(entryText)
		totalChars += len(entryText)
	}

	if action != nil {
		actionText := serializeEntryCompact(*action, toolLookup, tb.jsonlEnabled)
		if totalChars+len(actionText) <= tb.maxChars {
			result.WriteString(actionText)
		}
	}

	return result.String()
}

func serializeEntryCompact(entry TranscriptEntry, lookup ToolRegistry, jsonl bool) string {
	var sb strings.Builder
	for _, block := range entry.Content {
		if block.Type == "tool_use" {
			tool := lookup.Get(block.Name)
			if tool == nil {
				continue
			}
			inputMap, ok := block.Input.(map[string]any)
			if !ok {
				inputMap = map[string]any{}
			}
			encoded := tool.ToAutoClassifierInput(inputMap)
			if encoded == "" {
				continue
			}
			if jsonl {
				sb.WriteString(fmt.Sprintf(`{"%s":"%s"}`+"\n", block.Name, truncateValue(encoded, MaxBlockValueChars)))
			} else {
				sb.WriteString(fmt.Sprintf("%s %s\n", block.Name, truncateValue(encoded, MaxBlockValueChars)))
			}
		} else if block.Type == "text" && entry.Role == "user" {
			if jsonl {
				sb.WriteString(fmt.Sprintf(`{"user":"%s"}`+"\n", truncateValue(block.Text, MaxBlockValueChars)))
			} else {
				sb.WriteString(fmt.Sprintf("User: %s\n", truncateValue(block.Text, MaxBlockValueChars)))
			}
		}
	}
	return sb.String()
}

func truncateValue(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "…"
}

func BuildTranscriptForClassifier(messages []types.Message, tools []Tool, maxChars int) string {
	tb := NewTranscriptBuilder(tools, maxChars, IsJSONLTranscriptEnabled())
	return tb.Build(messages, nil)
}
