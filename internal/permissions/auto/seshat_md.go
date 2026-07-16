// Package auto - SESHAT.md integration for classifier.
//
// This module handles integration with the user's SESHAT.md configuration file,
// which contains user-defined instructions for the agent. These instructions
// are treated as part of the user's intent when evaluating tool actions.
//
// This is the Seshat equivalent of OpenClaude's CLAUDE.md integration.
// The content is wrapped in <user_seshat_md> tags and prepended to classifier prompts.
//
// Usage:
//
//	auto.SetSeshatMdProvider(func() string {
//	    return readFile("SESHAT.md")
//	})
package auto

import (
	"strings"

	"github.com/KPO-Tech/seshat/internal/types"
)

// SeshatMdProvider is a function type that returns the SESHAT.md content.
// This allows flexible integration with different file reading strategies.
type SeshatMdProvider func() string

// seshatMdProvider is the global provider set by the application.
// Used by BuildSeshatMdMessage() to retrieve user configuration.
var seshatMdProvider SeshatMdProvider

// SetSeshatMdProvider sets the global SESHAT.md content provider.
// Should be called during application initialization.
func SetSeshatMdProvider(provider SeshatMdProvider) {
	seshatMdProvider = provider
}

func GetSeshatMdContent() string {
	if seshatMdProvider != nil {
		return seshatMdProvider()
	}
	return ""
}

func BuildSeshatMdMessage() *types.Message {
	content := GetSeshatMdContent()
	if content == "" {
		return nil
	}

	wrappedContent := `The following is the user's SESHAT.md configuration. These are ` +
		`instructions the user provided to the agent and should be treated ` +
		`as part of the user's intent when evaluating actions.\n\n` +
		`<user_seshat_md>\n` + content + "\n</user_seshat_md>"

	return &types.Message{
		ID:      types.MessageID("seshat-md-" + generateID()),
		Role:    types.RoleUser,
		Content: []types.ContentBlock{types.TextContent{Text: wrappedContent}},
	}
}

func BuildSystemPromptWithSeshatMd() string {
	seshatMdMsg := BuildSeshatMdMessage()
	systemPrompt := BuildSystemPrompt()

	if seshatMdMsg != nil {
		textContent := seshatMdMsg.Content[0].(types.TextContent)
		return textContent.Text + "\n\n" + systemPrompt
	}

	return systemPrompt
}

func HasSeshatMdContent() bool {
	return GetSeshatMdContent() != ""
}

const SeshatMdDelimiter = "<user_seshat_md>"
const SeshatMdDelimiterEnd = "</user_seshat_md>"

func ExtractSeshatMdRules(content string) string {
	start := strings.Index(content, SeshatMdDelimiter)
	if start == -1 {
		return ""
	}
	start += len(SeshatMdDelimiter)

	end := strings.Index(content, SeshatMdDelimiterEnd)
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(content[start:end])
}
