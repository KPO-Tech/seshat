// Package auto - Cache control support for classifier API calls.
//
// This module provides cache control header support for the classifier API,
// enabling prompt caching to reduce latency and costs. Cache control allows
// the API to cache the system prompt and transcript prefix across calls.
//
// TTL Values:
//   - ephemeral-1-hour: 1 hour cache TTL (default for auto mode)
//   - ephemeral-5-minutes: 5 minute cache TTL (shorter)
package auto

// Cache control TTL constants
const (
	CacheControlTTL1Hour = "ephemeral-1-hour"
	CacheControlTTL5Min  = "ephemeral-5-minutes"
)

// CacheControl represents cache control metadata for API requests.
// Used to enable prompt caching on classifier calls.
type CacheControl struct {
	Type string // Cache type (typically "ephemeral")
	TTL  string // Cache TTL (e.g., "ephemeral-1-hour")
}

func GetCacheControl(querySource string) *CacheControl {
	flags := GetFeatureFlags()
	if !flags.CacheControl {
		return nil
	}

	return &CacheControl{
		Type: "ephemeral",
		TTL:  CacheControlTTL1Hour,
	}
}

func (c *CacheControl) ToAPIString() string {
	if c == nil {
		return ""
	}
	if c.TTL != "" {
		return c.Type + "-" + c.TTL
	}
	return c.Type
}
