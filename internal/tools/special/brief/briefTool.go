package brief

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Tool name
const ToolNameBrief = "brief"

const Description = `Send a message to the user - your primary visible output channel.

Usage:
- message: The message text (supports markdown formatting)
- attachments: Optional file paths to attach (images, logs, diffs)
- status: 'normal' (replying to user) or 'proactive' (unsolicited)

Examples:
- Simple message: {"message": "Task completed successfully"}
- With status: {"message": "Found the bug in line 42", "status": "proactive"}
- With attachments: {"message": "Here is the screenshot", "attachments": ["/path/to/screenshot.png"]}

Status meaning:
- normal: You're replying to something the user just said
- proactive: You're initiating - task completed, blocker hit, status update

Notes:
- Use this tool for all messages to the user
- Text outside this tool may not be visible
- Be concise and factual
- Use markdown for formatting if needed`

// BriefTool sends messages to the user
type BriefTool struct {
	// workingDir is the working directory
	workingDir string
}

// Status enum for message urgency
type Status string

const (
	StatusNormal    Status = "normal"
	StatusProactive Status = "proactive"
)

// Config for the tool
type BriefToolConfig struct {
	// WorkingDir is the working directory
	WorkingDir string
}

// DefaultConfig returns default configuration
func DefaultBriefToolConfig() *BriefToolConfig {
	return &BriefToolConfig{
		WorkingDir: ".",
	}
}

// NewBriefTool creates a new brief tool
func NewBriefTool(config *BriefToolConfig) *BriefTool {
	if config == nil {
		config = DefaultBriefToolConfig()
	}

	return &BriefTool{
		workingDir: config.WorkingDir,
	}
}

// Definition returns the tool definition
func (t *BriefTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolNameBrief,
		DisplayName:        "Brief",
		SearchHint:         "send a message to the user",
		Description:        Description,
		Category:           "communication",
		IsReadOnly:         true,
		IsDestructive:      false,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
		Metadata: map[string]any{
			"is_stateful": false,
			"user_facing": true,
		},
	}
}

// Input represents the tool input
type Input struct {
	// Message is the message for the user
	Message string `json:"message"`

	// Attachments is optional list of file paths to attach
	Attachments []string `json:"attachments,omitempty"`

	// Status indicates message urgency
	Status Status `json:"status,omitempty"`
}

// Output represents the tool output
type Output struct {
	// Message is the message
	Message string `json:"message"`

	// Attachments is resolved attachment metadata
	Attachments []Attachment `json:"attachments,omitempty"`

	// SentAt is the ISO timestamp when the message was sent
	SentAt string `json:"sentAt"`
}

// Attachment represents a resolved attachment
type Attachment struct {
	// Path is the file path
	Path string `json:"path"`

	// Size is the file size in bytes
	Size int64 `json:"size"`

	// IsImage indicates if the file is an image
	IsImage bool `json:"isImage"`

	// FileUUID is the file identifier
	FileUUID string `json:"file_uuid,omitempty"`
}

// Call executes the tool
func (t *BriefTool) Call(ctx context.Context, input tool.CallInput, toolCtx tool.ToolUseContext) (tool.CallResult, error) {
	// Parse input
	var parsed Input
	if err := json.Unmarshal([]byte(input.Raw), &parsed); err != nil {
		return tool.NewErrorResult(fmt.Errorf("Failed to parse input: %w", err)), nil
	}

	// Validate required fields
	if parsed.Message == "" {
		return tool.NewErrorResult(fmt.Errorf("message is required")), nil
	}

	// Validate status
	if parsed.Status == "" {
		parsed.Status = StatusNormal
	}
	if parsed.Status != StatusNormal && parsed.Status != StatusProactive {
		return tool.NewErrorResult(fmt.Errorf("status must be 'normal' or 'proactive'")), nil
	}

	// Capture timestamp
	sentAt := time.Now().UTC().Format(time.RFC3339)

	// Build output
	output := Output{
		Message: parsed.Message,
		SentAt:  sentAt,
	}

	// Handle attachments (basic validation - could be extended)
	if len(parsed.Attachments) > 0 {
		output.Attachments = make([]Attachment, len(parsed.Attachments))
		for i, path := range parsed.Attachments {
			output.Attachments[i] = Attachment{
				Path:    path,
				IsImage: isImageFile(path),
			}
		}
	}

	// Format content for display
	content := parsed.Message
	if len(output.Attachments) > 0 {
		attachmentInfo := fmt.Sprintf(" (%d attachment%s included)",
			len(output.Attachments),
			suffix(len(output.Attachments), "", "s"))
		content += attachmentInfo
	}

	return tool.CallResult{
		Data: map[string]any{
			"message":     output.Message,
			"attachments": output.Attachments,
			"sentAt":      output.SentAt,
		},
		Content: content,
	}, nil
}

// ValidateInput validates tool input
func (t *BriefTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Check required fields
	if input["message"] == nil || input["message"] == "" {
		return map[string]any{
			"result":  false,
			"message": "message is required",
		}, nil
	}

	return input, nil
}

// CheckPermissions checks permissions
func (t *BriefTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	// Brief is always allowed - it's the primary output channel
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether tool is concurrency safe
func (t *BriefTool) IsConcurrencySafe() bool {
	return true
}

// IsReadOnly returns whether tool is read-only
func (t *BriefTool) IsReadOnly(input tool.CallInput) bool {
	// Brief sends messages, not modifying anything
	return true
}

// Helper functions

// isImageFile checks if a path points to an image file
func isImageFile(path string) bool {
	imageExtensions := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}
	for _, ext := range imageExtensions {
		if len(path) >= len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}
	return false
}

// suffix returns singular or plural based on n
func suffix(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
