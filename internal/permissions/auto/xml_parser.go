// Package auto - XML Parser for classifier responses.
//
// This module provides XML parsing for the two-stage classifier output.
// The classifier returns responses in XML format with <block>, <reason>,
// and optional <thinking> tags. This parser handles extraction of these
// values from the raw response text.
//
// Response Format:
//   - <block>yes</block> or <block>no</block> for decision
//   - <reason>explanation</reason> for decision rationale
//   - <thinking>chain-of-thought</thinking> for reasoning (stage 2 only)
//
// Alignment: This aligns with OpenClaude's classifierShared.ts parsing.
package auto

import (
	"fmt"
	"regexp"
	"strings"
)

// XMLParser parses XML responses from the two-stage classifier.
// Aligned with OpenClaude's classifierShared.ts.
type XMLParser struct{}

// NewXMLParser creates a new XML parser instance.
// Returns a pointer to a fresh XMLParser ready for use.
func NewXMLParser() *XMLParser {
	return &XMLParser{}
}

// ParseBlock parses the <block> tag from classifier response.
// Returns: true (blocked), false (allowed), nil (parse error / not found).
func (p *XMLParser) ParseBlock(response string) *bool {
	block := p.extractTagContent(response, "block")
	if block == "" {
		return nil
	}
	block = strings.ToLower(strings.TrimSpace(block))
	switch block {
	case "true", "yes", "1":
		trueVal := true
		return &trueVal
	case "false", "no", "0":
		falseVal := false
		return &falseVal
	default:
		return nil
	}
}

// ParseReason extracts the <reason> tag from classifier response.
func (p *XMLParser) ParseReason(response string) string {
	reason := p.extractTagContent(response, "reason")
	return strings.TrimSpace(reason)
}

// ParseThinking extracts the <thinking> tag from classifier response (thinking mode).
func (p *XMLParser) ParseThinking(response string) string {
	return p.extractTagContent(response, "thinking")
}

// ParseBlockWithReason parses both block and reason from response.
// Returns the parsed values and whether parsing succeeded.
func (p *XMLParser) ParseBlockWithReason(response string) (blocked *bool, reason string) {
	blocked = p.ParseBlock(response)
	reason = p.ParseReason(response)
	return blocked, reason
}

// extractTagContent extracts content between opening and closing XML tags.
func (p *XMLParser) extractTagContent(response, tagName string) string {
	// Match <tagName>...</tagName> (case insensitive)
	pattern := regexp.MustCompile(`(?i)<` + tagName + `>(.*?)</` + tagName + `>`)
	matches := pattern.FindStringSubmatch(response)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// ParseFullResponse parses a complete classifier response including
// block, reason, and thinking tags.
func (p *XMLParser) ParseFullResponse(response string) (blocked *bool, reason, thinking string) {
	blocked = p.ParseBlock(response)
	reason = p.ParseReason(response)
	thinking = p.ParseThinking(response)
	return
}

// IsParseFailure checks if the response indicates a parsing failure
// (response too short, malformed XML, etc.).
func (p *XMLParser) IsParseFailure(response string) bool {
	response = strings.TrimSpace(response)
	// If response is very short or doesn't contain expected tags, might be a failure
	if len(response) < 10 {
		return true
	}
	// Check for common error patterns
	lowerResp := strings.ToLower(response)
	if strings.Contains(lowerResp, "error") || strings.Contains(lowerResp, "cannot") {
		return true
	}
	return false
}

// ExtractToolUse extracts the tool use block from the classifier prompt response.
// This is used to identify which tool use is being classified.
func (p *XMLParser) ExtractToolUse(toolUseID, toolName string, toolInput map[string]any) string {
	var sb strings.Builder
	sb.WriteString("Tool Use ID: ")
	sb.WriteString(toolUseID)
	sb.WriteString("\n")
	sb.WriteString("Tool Name: ")
	sb.WriteString(toolName)
	sb.WriteString("\n")
	sb.WriteString("Tool Input: ")
	for k, v := range toolInput {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(fmt.Sprintf("%v", v))
		sb.WriteString(", ")
	}
	return sb.String()
}

// FormatToolUseCompact creates a compact representation of a tool use
// for the classifier prompt.
func FormatToolUseCompact(toolName string, input map[string]any) string {
	if input == nil {
		return toolName + "()"
	}
	var sb strings.Builder
	sb.WriteString(toolName)
	sb.WriteString("(")
	first := true
	for k, v := range input {
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		// Truncate long values
		valStr := fmt.Sprintf("%v", v)
		if len(valStr) > 500 {
			valStr = valStr[:500] + "..."
		}
		sb.WriteString(valStr)
		first = false
	}
	sb.WriteString(")")
	return sb.String()
}

// BlockedByFastClassifier is the reason when fast stage blocks.
const BlockedByFastClassifier = "Blocked by fast classifier"

// NoReasonProvided is the default reason when classifier doesn't provide one.
const NoReasonProvided = "No reason provided"
