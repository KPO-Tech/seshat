package browser

import (
	"context"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

var textOnlyBlockedPatterns = []string{
	"*.png*", "*.jpg*", "*.jpeg*", "*.gif*", "*.webp*", "*.avif*", "*.svg*",
	"*.woff*", "*.woff2*", "*.ttf*", "*.otf*",
	"*.mp3*", "*.m4a*", "*.aac*", "*.wav*", "*.ogg*",
	"*.mp4*", "*.webm*", "*.mov*", "*.avi*",
}

var aggressiveBlockedPatterns = append(append([]string{}, textOnlyBlockedPatterns...),
	"*google-analytics.com*", "*googletagmanager.com*", "*doubleclick.net*", "*hotjar.com*", "*segment.io*",
)

// GetNetworkPolicy returns the current session-scoped request policy.
func (m *RodManager) GetNetworkPolicy(ctx context.Context, sessionID types.SessionID) (NetworkPolicy, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return NetworkPolicy{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	session.lastActivity = time.Now().UTC()
	return cloneNetworkPolicy(session.networkPolicy), nil
}

// SetNetworkPolicy updates the current session-scoped request policy and applies it
// to every existing page immediately so future navigations follow the same rules.
func (m *RodManager) SetNetworkPolicy(ctx context.Context, sessionID types.SessionID, policy NetworkPolicy) (NetworkPolicy, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return NetworkPolicy{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()

	normalized := normalizeNetworkPolicy(policy)
	session.networkPolicy = normalized
	for _, pageID := range session.pageOrder {
		state := session.pages[pageID]
		if state == nil || state.page == nil {
			continue
		}
		if err := applyNetworkPolicyToPage(state, normalized); err != nil {
			return NetworkPolicy{}, err
		}
	}
	session.lastActivity = time.Now().UTC()
	return cloneNetworkPolicy(normalized), nil
}

func applyNetworkPolicyToPage(state *pageState, policy NetworkPolicy) error {
	if state == nil || state.page == nil {
		return nil
	}
	patterns := expandBlockedPatterns(policy)
	return withRod(func() error {
		return state.page.SetBlockedURLs(patterns)
	})
}

func expandBlockedPatterns(policy NetworkPolicy) []string {
	patterns := make([]string, 0, len(policy.BlockedURLs)+16)
	patterns = append(patterns, policy.BlockedURLs...)
	switch strings.TrimSpace(strings.ToLower(policy.ResourcePolicy)) {
	case "text_only":
		patterns = append(patterns, textOnlyBlockedPatterns...)
	case "aggressive":
		patterns = append(patterns, aggressiveBlockedPatterns...)
	}
	return dedupeStrings(patterns)
}

func normalizeNetworkPolicy(policy NetworkPolicy) NetworkPolicy {
	normalized := NetworkPolicy{
		ResourcePolicy: strings.TrimSpace(strings.ToLower(policy.ResourcePolicy)),
	}
	for _, pattern := range policy.BlockedURLs {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			normalized.BlockedURLs = append(normalized.BlockedURLs, pattern)
		}
	}
	normalized.BlockedURLs = dedupeStrings(normalized.BlockedURLs)
	switch normalized.ResourcePolicy {
	case "", "default", "text_only", "aggressive":
		if normalized.ResourcePolicy == "default" {
			normalized.ResourcePolicy = ""
		}
	default:
		normalized.ResourcePolicy = ""
	}
	return normalized
}

func cloneNetworkPolicy(policy NetworkPolicy) NetworkPolicy {
	return NetworkPolicy{
		BlockedURLs:    append([]string(nil), policy.BlockedURLs...),
		ResourcePolicy: policy.ResourcePolicy,
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
