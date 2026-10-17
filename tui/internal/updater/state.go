package updater

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// CheckInterval is how long a result of the release check is kept before asking GitHub again.
	CheckInterval = 24 * time.Hour
	// retryAfterFailure is the wait before asking again when the last check could not reach GitHub.
	retryAfterFailure = time.Hour
	// SnoozeFor is how long "not now" silences the offer.
	SnoozeFor = 24 * time.Hour
)

// State is what the updater remembers between two runs of seshat, so that a launch does not wait for the network
// and a user who said no is not asked at every start.
type State struct {
	CheckedAt   time.Time `json:"checked_at"`
	Latest      string    `json:"latest"`
	SkippedTag  string    `json:"skipped_tag,omitempty"`
	SnoozeUntil time.Time `json:"snooze_until,omitempty"`
}

// StatePath is where the state is kept inside the runtime root.
func StatePath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "update-check.json")
}

// LoadState reads the state; a missing or damaged file is an empty state.
func LoadState(path string) State {
	var st State
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

// Save writes the state. A failure is returned but is never worth stopping the program for.
func (st State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Due reports whether the release check is old enough to be done again.
func (st State) Due(now time.Time) bool {
	return now.Sub(st.CheckedAt) >= CheckInterval
}

// Refresh asks GitHub for the latest release when the check is due and records the answer. When GitHub cannot be
// reached the check is tried again in an hour, not at every start.
func (st *State) Refresh(ctx context.Context, c *Client, now time.Time) {
	if !st.Due(now) {
		return
	}
	release, err := c.Latest(ctx)
	if err != nil {
		st.CheckedAt = now.Add(retryAfterFailure - CheckInterval)
		return
	}
	st.CheckedAt = now
	st.Latest = release.Tag
}

// Offer returns the tag to propose to the user: a release newer than current that they did not skip, outside a
// snooze.
func (st State) Offer(current string, now time.Time) (string, bool) {
	if st.Latest == "" || !Newer(current, st.Latest) || st.SkippedTag == st.Latest || now.Before(st.SnoozeUntil) {
		return "", false
	}
	return st.Latest, true
}
