package providers

import "errors"

// errStreamInterrupted marks a stream that ended - a clean EOF, or a
// provider's own [DONE]/completion sentinel - without ever delivering a
// proper completion signal (Anthropic's message_stop, or an
// OpenAI-compatible finish_reason) and without producing any visible output.
// Previously this was silently treated as an ordinary completion: the
// accumulated (possibly empty) text was returned as a successful response,
// so a dropped connection or a model that burned its whole output budget on
// reasoning with nothing visible to show for it looked identical to a real
// answer. Wrapping it as ErrCodeAPIResponse routes it through the engine's
// existing retry path (see Loop.isRecoverableError in internal/engine) so
// the turn is retried instead of silently accepted.
var errStreamInterrupted = errors.New("stream ended before a completion signal or any visible output was produced")

// errProviderStreamError marks an in-band error object a provider emitted
// inside an otherwise-200 SSE stream (e.g. an OpenRouter upstream failure
// surfaced mid-stream as `data: {"error": {...}}` instead of a non-200 HTTP
// status). The OpenAI-compatible chunk decoder had no field for this shape,
// so it silently unmarshalled into a no-op delta and the stream finished
// looking like an ordinary - possibly truncated - success.
var errProviderStreamError = errors.New("provider reported an error inside the stream")
