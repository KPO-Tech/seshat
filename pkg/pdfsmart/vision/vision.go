package vision

import internalvision "github.com/KPO-Tech/seshat/internal/pdfsmart/vision"

type (
	// LLMCaller is the minimal interface Transcriber needs to call the
	// provider API - normally satisfied by *providers.Client, but defined
	// as an interface so a caller can supply anything with a matching
	// CreateMessage method (or a fake, for tests).
	LLMCaller = internalvision.LLMCaller

	// Config configures a Transcriber: Model, an optional model.Registry
	// (defaults to model.Global), and Timeout.
	Config = internalvision.Config

	// Transcriber sends a rendered PDF page image to a vision-capable LLM
	// and returns its transcription - see internal/pdfsmart/vision's
	// package doc for the rationale and pdfsmart.VisionTranscriber for the
	// interface it implements.
	Transcriber = internalvision.Transcriber
)

// New creates a Transcriber. caller is normally the same *providers.Client
// already used elsewhere for chat completions.
func New(caller LLMCaller, cfg Config) *Transcriber {
	return internalvision.New(caller, cfg)
}
