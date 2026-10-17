package workspace

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// memPlanStore is an in-process PlanStore implementation used by the TUI.
// Plans are transient — version tracking suffices for the review overlay;
// no disk persistence is required between restarts.
type memPlanStore struct {
	mu      sync.Mutex
	entries map[string]*planEntry
}

type planEntry struct {
	version int
	status  string
}

func newMemPlanStore() *memPlanStore {
	return &memPlanStore{entries: make(map[string]*planEntry)}
}

func (s *memPlanStore) CreateOrUpdate(_ context.Context, planID, _, _, _, _, _ string) (string, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if planID == "" {
		planID = uuid.New().String()
	}
	e, ok := s.entries[planID]
	if !ok {
		e = &planEntry{status: "pending"}
		s.entries[planID] = e
	}
	e.version++
	return planID, e.version, nil
}

func (s *memPlanStore) SetStatus(_ context.Context, planID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[planID]; ok {
		e.status = status
	}
	return nil
}
