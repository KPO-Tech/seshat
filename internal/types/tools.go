package types

import (
	"bytes"
	"io"
)

// ToolProgress represents progress updates for a tool execution
type ToolProgress struct {
	// ToolName is the name of the tool
	ToolName string `json:"tool_name,omitempty"`

	// ToolUseID identifies the concrete tool call instance when available.
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Stage indicates the current stage
	Stage ToolProgressStage `json:"stage"`

	// Message is a human-readable progress message
	Message string `json:"message,omitempty"`

	// PercentComplete is 0-100
	PercentComplete float64 `json:"percent_complete,omitempty"`

	// Metadata contains additional progress information
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolProgressStage represents the stage of tool execution
type ToolProgressStage string

const (
	ToolProgressStagePending   ToolProgressStage = "pending"
	ToolProgressStageRunning   ToolProgressStage = "running"
	ToolProgressStageCompleted ToolProgressStage = "completed"
	ToolProgressStageFailed    ToolProgressStage = "failed"
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	// Data contains the result data
	// The specific type depends on the tool
	Data any `json:"data"`

	// Error contains any error that occurred
	Error error `json:"error,omitempty"`

	// Metadata contains additional result information
	Metadata *ToolResultMetadata `json:"metadata,omitempty"`
}

// ToolResultMetadata contains metadata about a tool result
type ToolResultMetadata struct {
	// ContentReplacement indicates the result was too large and replaced
	ContentReplacement *ContentReplacementState `json:"content_replacement,omitempty"`

	// ExecutionDuration is how long the tool took to run
	ExecutionDuration int64 `json:"execution_duration_ms,omitempty"`

	// Additional metadata
	Additional map[string]any `json:"additional,omitempty"`
}

// ContentReplacementState represents when content was replaced due to size
type ContentReplacementState struct {
	// OriginalSize is the original size in bytes
	OriginalSize int64 `json:"original_size"`

	// ReplacedSize is the size after replacement
	ReplacedSize int64 `json:"replaced_size"`

	// ReplacementType indicates what type of replacement occurred
	ReplacementType ContentReplacementType `json:"replacement_type"`

	// Preview is a preview of the original content
	Preview string `json:"preview,omitempty"`

	// FilePath is where the content was stored (if applicable)
	FilePath string `json:"file_path,omitempty"`
}

// ContentReplacementType represents the type of content replacement
type ContentReplacementType string

const (
	// ContentReplacementTypeDisk means content was written to disk
	ContentReplacementTypeDisk ContentReplacementType = "disk"

	// ContentReplacementTypePreview means content was replaced with a preview
	ContentReplacementTypePreview ContentReplacementType = "preview"

	// ContentReplacementTypeTruncated means content was truncated
	ContentReplacementTypeTruncated ContentReplacementType = "truncated"
)

// ProgressStream is a stream of progress updates
type ProgressStream <-chan ToolProgress

// ToolOutput represents the output of a tool that can be streamed
type ToolOutput interface {
	// Bytes returns the output as bytes
	Bytes() []byte

	// String returns the output as a string
	String() string

	// Reader returns a reader for the output
	Reader() io.Reader
}

// StreamToolOutput is a streaming implementation of ToolOutput
type StreamToolOutput struct {
	data []byte
}

// NewStreamToolOutput creates a new StreamToolOutput
func NewStreamToolOutput(data []byte) StreamToolOutput {
	return StreamToolOutput{data: data}
}

// Bytes implements ToolOutput
func (s StreamToolOutput) Bytes() []byte {
	return s.data
}

// String implements ToolOutput
func (s StreamToolOutput) String() string {
	return string(s.data)
}

// Reader implements ToolOutput
func (s StreamToolOutput) Reader() io.Reader {
	return bytes.NewReader(s.data)
}
