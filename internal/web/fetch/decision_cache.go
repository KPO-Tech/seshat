package fetch

import (
	"strings"
	"time"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

// DecisionCache stores stable auto-routing outcomes so repeated fetches avoid
// paying the HTTP-then-browser probe cost for hosts that are consistently
// browser-first or HTTP-friendly.
type DecisionCache = webcore.TTLCache[string]

func DefaultDecisionCache() *DecisionCache {
	return webcore.NewTTLCache[string](webcore.TTLCacheConfig{
		TTL:      30 * time.Minute,
		MaxItems: 256,
	})
}

func decisionCacheKey(rawURL string) string {
	return strings.TrimSpace(strings.ToLower(rawURL))
}
