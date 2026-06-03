// Package auto - NEXUS.md integration for classifier.
//
// This module handles integration with the user's NEXUS.md configuration file,
// which contains user-defined instructions for the agent. These instructions
// are treated as part of the user's intent when evaluating tool actions.
//
// This is the Nexus equivalent of OpenClaude's CLAUDE.md integration.
// The content is wrapped in <user_nexus_md> tags and prepended to classifier prompts.
//
// Usage:
//
//	auto.SetNexusMdProvider(func() string {
//	    return readFile("NEXUS.md")
//	})
package auto

import (
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// NexusMdProvider is a function type that returns the NEXUS.md content.
// This allows flexible integration with different file reading strategies.
type NexusMdProvider func() string

// nexusMdProvider is the global provider set by the application.
// Used by BuildNexusMdMessage() to retrieve user configuration.
var nexusMdProvider NexusMdProvider

// SetNexusMdProvider sets the global NEXUS.md content provider.
// Should be called during application initialization.
func SetNexusMdProvider(provider NexusMdProvider) {
	nexusMdProvider = provider
}

func GetNexusMdContent() string {
	if nexusMdProvider != nil {
		return nexusMdProvider()
	}
	return ""
}

func BuildNexusMdMessage() *types.Message {
	content := GetNexusMdContent()
	if content == "" {
		return nil
	}

	wrappedContent := `The following is the user's NEXUS.md configuration. These are ` +
		`instructions the user provided to the agent and should be treated ` +
		`as part of the user's intent when evaluating actions.\n\n` +
		`<user_nexus_md>\n` + content + "\n</user_nexus_md>"

	return &types.Message{
		ID:      types.MessageID("nexus-md-" + generateID()),
		Role:    types.RoleUser,
		Content: []types.ContentBlock{types.TextContent{Text: wrappedContent}},
	}
}

func BuildSystemPromptWithNexusMd() string {
	nexusMdMsg := BuildNexusMdMessage()
	systemPrompt := BuildSystemPrompt()

	if nexusMdMsg != nil {
		textContent := nexusMdMsg.Content[0].(types.TextContent)
		return textContent.Text + "\n\n" + systemPrompt
	}

	return systemPrompt
}

func HasNexusMdContent() bool {
	return GetNexusMdContent() != ""
}

const NexusMdDelimiter = "<user_nexus_md>"
const NexusMdDelimiterEnd = "</user_nexus_md>"

func ExtractNexusMdRules(content string) string {
	start := strings.Index(content, NexusMdDelimiter)
	if start == -1 {
		return ""
	}
	start += len(NexusMdDelimiter)

	end := strings.Index(content, NexusMdDelimiterEnd)
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(content[start:end])
}
