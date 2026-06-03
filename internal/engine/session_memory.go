package engine

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/memory"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type learnedDirective struct {
	Scope memory.MemoryScope
	Kind  memory.MemoryType
	Key   string
	Value string
}

func (s *Session) rememberUserDirectives(content string) {
	if s == nil || s.engine == nil || s.engine.memoryService == nil {
		return
	}

	for _, directive := range extractPersistentDirectives(content) {
		var err error
		switch directive.Kind {
		case memory.MemoryTypeInstruction:
			err = s.engine.memoryService.LearnInstruction(directive.Scope, directive.Key, directive.Value, "runtime:user_message")
		default:
			err = s.engine.memoryService.LearnPreference(directive.Scope, directive.Key, directive.Value, "runtime:user_message")
		}
		if err != nil {
			slog.Warn("failed to learn directive", "key", directive.Key, "error", err)
		}
	}
}

func (s *Session) rememberToolUsage(toolUses []types.ToolUseContent, results []tool.CallResult) {
	if s == nil || s.engine == nil || s.engine.memoryService == nil {
		return
	}

	for i, toolUse := range toolUses {
		success := false
		var toolErr error
		if i < len(results) {
			success = results[i].IsSuccess()
			toolErr = results[i].Error
		} else {
			toolErr = fmt.Errorf("missing tool result for %s", toolUse.Name)
		}

		if err := s.engine.memoryService.LearnToolUsage(toolUse.Name, toolUse.Input, success, toolErr); err != nil {
			slog.Warn("failed to learn tool usage", "tool", toolUse.Name, "error", err)
		}
	}
}

func (s *Session) rememberSessionSummary() {
	if s == nil || s.engine == nil || s.engine.memoryService == nil || s.state == nil {
		return
	}

	summary := buildSessionSummary(s.state.Messages, s.state.TurnNumber)
	if summary == "" {
		return
	}

	if err := s.engine.memoryService.AddSessionSummary(
		string(s.state.SessionID),
		s.sessionProjectPath(),
		summary,
		collectSessionTools(s.state.Messages),
	); err != nil {
		slog.Warn("failed to store session summary", "session_id", s.state.SessionID, "error", err)
	}
}

func (s *Session) sessionProjectPath() string {
	if s != nil && s.state != nil && s.state.Metadata != nil && s.state.Metadata.RootPath != "" {
		return s.state.Metadata.RootPath
	}
	if s != nil {
		return s.workingDirectory()
	}
	return ""
}

func extractPersistentDirectives(content string) []learnedDirective {
	candidates := splitDirectiveCandidates(content)
	if len(candidates) == 0 {
		return nil
	}

	directivesByKey := make(map[string]learnedDirective)
	for _, candidate := range candidates {
		normalized := normalizeMemoryText(candidate)
		if normalized == "" {
			continue
		}
		lower := strings.ToLower(normalized)

		switch {
		// Explicit memory requests — highest confidence, user is directly asking.
		case containsMemoryPhrase(lower,
			"remember that", "remember:", "souviens-toi que", "retiens que", "n'oublie pas que",
			"keep in mind that", "keep in mind:", "note that", "don't forget that",
			"important:", "fyi:"):
			key := stableMemoryKey("instruction", normalized)
			directivesByKey[key] = learnedDirective{
				Scope: memory.MemoryScopeProject,
				Kind:  memory.MemoryTypeInstruction,
				Key:   key,
				Value: normalized,
			}

		// Language preference.
		case containsMemoryPhrase(lower,
			"answer in ", "respond in ", "reply in ", "write in ", "always write in ", "always respond in ",
			"reponds en ", "réponds en ", "en francais", "en français",
			"use english", "use french", "use spanish", "use german", "use portuguese"):
			directivesByKey["preference:response-language"] = learnedDirective{
				Scope: memory.MemoryScopeUser,
				Kind:  memory.MemoryTypePreference,
				Key:   "preference:response-language",
				Value: normalized,
			}

		// Response style (verbosity / depth).
		case containsMemoryPhrase(lower,
			"be concise", "stay concise", "sois concis", "soyez concis",
			"keep it brief", "keep it short", "be brief", "brief responses",
			"be detailed", "be verbose", "be thorough", "be comprehensive", "detailed responses"):
			directivesByKey["preference:response-style"] = learnedDirective{
				Scope: memory.MemoryScopeUser,
				Kind:  memory.MemoryTypePreference,
				Key:   "preference:response-style",
				Value: normalized,
			}

		// Tone preference.
		case containsMemoryPhrase(lower,
			"be formal", "be professional", "use formal", "formal tone",
			"be informal", "be casual", "casual tone", "speak casually"):
			directivesByKey["preference:response-tone"] = learnedDirective{
				Scope: memory.MemoryScopeUser,
				Kind:  memory.MemoryTypePreference,
				Key:   "preference:response-tone",
				Value: normalized,
			}

		// Output format preference.
		case containsMemoryPhrase(lower,
			"use markdown", "avoid markdown", "no markdown", "without markdown",
			"use bullet points", "use numbered lists", "plain text only", "use plain text",
			"format using", "format with"):
			directivesByKey["preference:response-format"] = learnedDirective{
				Scope: memory.MemoryScopeUser,
				Kind:  memory.MemoryTypePreference,
				Key:   "preference:response-format",
				Value: normalized,
			}

		// Emoji preference.
		case containsMemoryPhrase(lower, "do not use emoji", "don't use emoji", "without emoji", "sans emoji"):
			directivesByKey["instruction:response-no-emoji"] = learnedDirective{
				Scope: memory.MemoryScopeUser,
				Kind:  memory.MemoryTypeInstruction,
				Key:   "instruction:response-no-emoji",
				Value: normalized,
			}

		// Project / codebase conventions.
		case containsMemoryPhrase(lower,
			"for this project", "dans ce projet", "in this repo", "in this codebase",
			"always use ", "prefer ", "instead of ", "never use ", "do not use ", "don't use ",
			"n'utilise pas", "utilise toujours ", "toujours utilise ", "toujours utiliser ", "evite ", "évite "):
			kind := memory.MemoryTypeInstruction
			if containsMemoryPhrase(lower, "prefer ", "instead of ", "préfère ", "prefere ") {
				kind = memory.MemoryTypePreference
			}
			keyPrefix := "instruction"
			if kind == memory.MemoryTypePreference {
				keyPrefix = "preference"
			}
			key := stableMemoryKey(keyPrefix, normalized)
			directivesByKey[key] = learnedDirective{
				Scope: memory.MemoryScopeProject,
				Kind:  kind,
				Key:   key,
				Value: normalized,
			}
		}
	}

	if len(directivesByKey) == 0 {
		return nil
	}

	keys := make([]string, 0, len(directivesByKey))
	for key := range directivesByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	directives := make([]learnedDirective, 0, len(keys))
	for _, key := range keys {
		directives = append(directives, directivesByKey[key])
	}
	return directives
}

func splitDirectiveCandidates(content string) []string {
	replacer := strings.NewReplacer("\r", "\n", "\n", ".", ";", ".", "!", ".", "?", ".")
	parts := strings.Split(replacer.Replace(content), ".")
	candidates := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := normalizeMemoryText(part)
		if normalized == "" {
			continue
		}
		candidates = append(candidates, normalized)
	}
	return candidates
}

func buildSessionSummary(messages []types.Message, turns int) string {
	var parts []string
	if turns > 0 {
		parts = append(parts, fmt.Sprintf("Turns: %d.", turns))
	}

	if request := firstMessageTextByRole(messages, types.RoleUser); request != "" {
		parts = append(parts, fmt.Sprintf("Initial request: %q.", truncateMemoryText(request, 160)))
	}

	if response := lastMessageTextByRole(messages, types.RoleAssistant); response != "" {
		parts = append(parts, fmt.Sprintf("Final response: %q.", truncateMemoryText(response, 160)))
	}

	if tools := collectSessionTools(messages); len(tools) > 0 {
		parts = append(parts, fmt.Sprintf("Tools used: %s.", strings.Join(tools, ", ")))
	}

	return normalizeMemoryText(strings.Join(parts, " "))
}

func collectSessionTools(messages []types.Message) []string {
	seen := make(map[string]struct{})
	tools := make([]string, 0)
	for _, message := range messages {
		for _, block := range message.Content {
			toolUse, ok := block.(types.ToolUseContent)
			if !ok || toolUse.Name == "" {
				continue
			}
			if _, exists := seen[toolUse.Name]; exists {
				continue
			}
			seen[toolUse.Name] = struct{}{}
			tools = append(tools, toolUse.Name)
		}
	}
	sort.Strings(tools)
	return tools
}

func firstMessageTextByRole(messages []types.Message, role types.Role) string {
	for _, message := range messages {
		if message.Role != role {
			continue
		}
		if text := messageText(message); text != "" {
			return text
		}
	}
	return ""
}

func lastMessageTextByRole(messages []types.Message, role types.Role) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != role {
			continue
		}
		if text := messageText(messages[i]); text != "" {
			return text
		}
	}
	return ""
}

func messageText(message types.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, block := range message.Content {
		text, ok := block.(types.TextContent)
		if !ok {
			continue
		}
		normalized := normalizeMemoryText(text.Text)
		if normalized == "" {
			continue
		}
		parts = append(parts, normalized)
	}
	return strings.Join(parts, " ")
}

func stableMemoryKey(prefix, value string) string {
	var builder strings.Builder
	builder.WriteString(prefix)
	builder.WriteByte(':')

	dashPending := false
	for _, r := range strings.ToLower(value) {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			if dashPending && builder.Len() > len(prefix)+1 {
				builder.WriteByte('-')
			}
			builder.WriteRune(r)
			dashPending = false
			continue
		}
		dashPending = true
	}

	key := strings.Trim(builder.String(), "-:")
	if len(key) <= 96 {
		return key
	}
	return key[:96]
}

func truncateMemoryText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(normalizeMemoryText(text))
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func normalizeMemoryText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func containsMemoryPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
