package automation

import "time"

// Options configure a single workflow execution.
// Workflow-specific parameters (e.g. topics, target path) belong in the
// workflow struct itself; Options covers cross-cutting execution concerns.
type Options struct {
	// Model overrides the Runner's default model for this execution only.
	// Format: "provider:model" (e.g. "anthropic:claude-sonnet-5-5").
	Model string

	// DryRun skips actual LLM execution and returns an empty Result.
	// Useful for validating scheduler configuration without incurring cost.
	DryRun bool

	// Timeout caps the total wall-clock time for the workflow.
	// Zero means no per-execution timeout (context deadline still applies).
	Timeout time.Duration

	// MaxRetries is the number of additional attempts after a failure.
	// Zero means no retry. Used by WithRetry middleware.
	MaxRetries int

	// RetryBackoff is the initial wait between retries (doubles each attempt).
	// Defaults to 5s when zero and MaxRetries > 0.
	RetryBackoff time.Duration

	// Extra holds workflow-specific key-value options that are not part of
	// the standard contract. Workflows may inspect this map for opt-in features.
	Extra map[string]any
}

// DefaultOptions returns sensible defaults for interactive or scheduled runs.
func DefaultOptions() Options {
	return Options{
		Timeout:      30 * time.Minute,
		MaxRetries:   0,
		RetryBackoff: 5 * time.Second,
	}
}

// WithModel returns a copy of o with the model overridden.
func (o Options) WithModel(model string) Options {
	o.Model = model
	return o
}

// WithTimeout returns a copy of o with the timeout set.
func (o Options) WithTimeout(d time.Duration) Options {
	o.Timeout = d
	return o
}

// WithRetry returns a copy of o with retry configured.
func (o Options) WithRetry(maxRetries int, backoff time.Duration) Options {
	o.MaxRetries = maxRetries
	o.RetryBackoff = backoff
	return o
}

// WithDryRun returns a copy of o with DryRun enabled.
func (o Options) WithDryRun() Options {
	o.DryRun = true
	return o
}

// Set stores a key-value pair in Extra, initialising the map if needed.
func (o *Options) Set(key string, value any) {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra[key] = value
}

// Get retrieves a value from Extra. Returns nil if the key is absent.
func (o Options) Get(key string) any {
	return o.Extra[key]
}
