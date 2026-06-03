package worktree

import (
	"sync"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	sid1 = types.SessionID("test-session-1")
	sid2 = types.SessionID("test-session-2")
)

// TestWorktreeSession_ConcurrentGetSet verifies that concurrent Get/Set calls
// on the same session ID are race-free.
func TestWorktreeSession_ConcurrentGetSet(t *testing.T) {
	SetSession(sid1, nil)

	session := &WorktreeSession{
		WorktreePath:   "/tmp/test-worktree",
		WorktreeBranch: "test-branch",
		OriginalCwd:    "/tmp",
	}

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetSession(sid1, session)
		}()
		go func() {
			defer wg.Done()
			_ = GetSession(sid1)
		}()
	}
	wg.Wait()

	SetSession(sid1, nil)
}

// TestWorktreeSession_ConcurrentNilSet verifies nil sets are race-free.
func TestWorktreeSession_ConcurrentNilSet(t *testing.T) {
	session := &WorktreeSession{WorktreePath: "/tmp/w"}

	const goroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			SetSession(sid1, session)
		}()
		go func() {
			defer wg.Done()
			SetSession(sid1, nil)
		}()
		go func() {
			defer wg.Done()
			_ = GetSession(sid1)
		}()
	}
	wg.Wait()
	SetSession(sid1, nil)
}

// TestWorktreeSession_SetGetConsistency verifies Get always returns the last
// set value (single-threaded sanity check).
func TestWorktreeSession_SetGetConsistency(t *testing.T) {
	SetSession(sid1, nil)
	if GetSession(sid1) != nil {
		t.Error("expected nil after SetSession(nil)")
	}

	s := &WorktreeSession{WorktreePath: "/foo"}
	SetSession(sid1, s)
	if GetSession(sid1) != s {
		t.Error("expected session returned after Set")
	}

	SetSession(sid1, nil)
	if GetSession(sid1) != nil {
		t.Error("expected nil after second SetSession(nil)")
	}
}

// TestWorktreeSession_SessionIsolation verifies two sessions never see each
// other's worktree state.
func TestWorktreeSession_SessionIsolation(t *testing.T) {
	SetSession(sid1, nil)
	SetSession(sid2, nil)

	s1 := &WorktreeSession{WorktreePath: "/tmp/s1"}
	s2 := &WorktreeSession{WorktreePath: "/tmp/s2"}

	SetSession(sid1, s1)
	SetSession(sid2, s2)

	if got := GetSession(sid1); got != s1 {
		t.Errorf("sid1: got %v, want %v", got, s1)
	}
	if got := GetSession(sid2); got != s2 {
		t.Errorf("sid2: got %v, want %v", got, s2)
	}

	// Clearing one session must not affect the other.
	SetSession(sid1, nil)
	if GetSession(sid1) != nil {
		t.Error("sid1: expected nil after clear")
	}
	if got := GetSession(sid2); got != s2 {
		t.Errorf("sid2 should be unaffected after sid1 clear, got %v", got)
	}

	SetSession(sid2, nil)
}

// TestWorktreeSession_ConcurrentDifferentSessions verifies concurrent access
// on distinct session IDs does not cause races.
func TestWorktreeSession_ConcurrentDifferentSessions(t *testing.T) {
	s1 := &WorktreeSession{WorktreePath: "/tmp/cs1"}
	s2 := &WorktreeSession{WorktreePath: "/tmp/cs2"}

	const goroutines = 30
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			SetSession(sid1, s1)
		}()
		go func() {
			defer wg.Done()
			_ = GetSession(sid1)
		}()
		go func() {
			defer wg.Done()
			SetSession(sid2, s2)
		}()
		go func() {
			defer wg.Done()
			_ = GetSession(sid2)
		}()
	}
	wg.Wait()

	SetSession(sid1, nil)
	SetSession(sid2, nil)
}
