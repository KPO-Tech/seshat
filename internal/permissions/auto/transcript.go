// Package auto - Transcript building for classifier.
//
// This module handles building the conversation transcript that gets sent
// to the classifier. It converts Seshat messages into a compact format that
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

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// Tool interface defines the contract for tool input encoding.
// Each tool implements ToAutoClassifierInput to provide a compact
// representation of its input for the classifier.
type Tool interface {
	Name() string                                      // Tool name (e.g., "read_file", "edit_file")
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
	NewDefaultTool("read_file", []string{"Read"}, func(input map[string]any) string {
		path := input["file_path"]
		if path == nil {
			path = input["path"]
		}
		return fmt.Sprintf("read_file %v", path)
	}),
	NewDefaultTool("edit_file", []string{"Edit"}, func(input map[string]any) string {
		oldString := input["old_string"]
		newString := input["new_string"]
		return fmt.Sprintf("edit_file: replace %v with %v", oldString, newString)
	}),
	NewDefaultTool("write_file", []string{"Write"}, func(input map[string]any) string {
		path := input["file_path"]
		if path == nil {
			path = input["path"]
		}
		content := input["content"]
		return fmt.Sprintf("write_file %v (length=%d)", path, len(fmt.Sprintf("%v", content)))
	}),
	NewDefaultTool("bash", []string{"Shell", "Command"}, func(input map[string]any) string {
		command := input["command"]
		return fmt.Sprintf("Bash %v", command)
	}),
	NewDefaultTool("glob", []string{"Glob"}, func(input map[string]any) string {
		pattern := input["pattern"]
		return fmt.Sprintf("glob %v", pattern)
	}),
	NewDefaultTool("grep", []string{"Grep"}, func(input map[string]any) string {
		pattern := input["pattern"]
		path := input["path"]
		return fmt.Sprintf("grep %v in %v", pattern, path)
	}),
	NewDefaultTool("list_directory", []string{"LS"}, func(input map[string]any) string {
		path := input["path"]
		return fmt.Sprintf("list_directory %v", path)
	}),
	NewDefaultTool("web_fetch", []string{"fetch"}, func(input map[string]any) string {
		url := input["url"]
		return fmt.Sprintf("WebFetch %v", url)
	}),
	NewDefaultTool("task_list", []string{"TodoRead"}, func(input map[string]any) string {
		return "task_list"
	}),
	NewDefaultTool("task_update", []string{"todo_write", "TodoWrite"}, func(input map[string]any) string {
		content := input["content"]
		if content == nil {
			content = input["tasks"]
		}
		return fmt.Sprintf("task_update %v", content)
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
