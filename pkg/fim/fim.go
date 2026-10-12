// Package fim is the public Fill-in-the-Middle code completion interface -
// a standalone client library separate from the code_complete agent tool.
// FIM is explicitly suited to IDE integrations and in-editor ghost text: a
// host application (e.g. an editor UI) can call completion directly for
// each keystroke/pause without an LLM tool-calling loop deciding whether to
// call it - that decision doesn't make sense for this use case, the human
// (or editor) already triggered it deterministically.
package fim

import internalfim "github.com/KPO-Tech/seshat-runtime/internal/fim"

type (
	// Completer is the interface for FIM providers.
	Completer = internalfim.Completer

	// Request holds the parameters for a FIM completion call.
	Request = internalfim.Request

	// Response holds the result of a completed FIM call.
	Response = internalfim.Response

	// Event is a single event in a streaming FIM response.
	Event = internalfim.Event

	// EventType identifies the type of a streaming FIM event.
	EventType = internalfim.EventType

	// FinishReason indicates why the model stopped generating.
	FinishReason = internalfim.FinishReason

	// Usage tracks token consumption for a FIM completion.
	Usage = internalfim.Usage
)

const (
	EventContentDelta = internalfim.EventContentDelta
	EventComplete     = internalfim.EventComplete
	EventError        = internalfim.EventError

	FinishReasonStop    = internalfim.FinishReasonStop
	FinishReasonLength  = internalfim.FinishReasonLength
	FinishReasonUnknown = internalfim.FinishReasonUnknown
)
