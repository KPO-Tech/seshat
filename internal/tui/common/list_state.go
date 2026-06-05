package common

import "strings"

// State manages filter text, cursor movement, and filtered items for overlay lists.
type ListState[T any] struct {
	items    []T
	filtered []T
	filter   string
	cursor   int
	match    func(item T, needle string) bool
}

// NewListState constructs a filterable list state with the supplied matcher.
func NewListState[T any](match func(item T, needle string) bool) ListState[T] {
	return ListState[T]{match: match}
}

// SetItems replaces the backing items and resets the cursor to the first row.
func (s *ListState[T]) SetItems(items []T) {
	s.ResetItems(items, false)
}

// ResetItems replaces the backing items and optionally keeps the current cursor.
func (s *ListState[T]) ResetItems(items []T, preserveCursor bool) {
	s.items = clone(items)
	if !preserveCursor {
		s.cursor = 0
	}
	s.apply()
}

// SetFilter replaces the filter text and resets the cursor.
func (s *ListState[T]) SetFilter(filter string) {
	s.filter = filter
	s.cursor = 0
	s.apply()
}

// TypeFilter appends one character to the current filter.
func (s *ListState[T]) TypeFilter(ch string) {
	s.SetFilter(s.filter + ch)
}

// DeleteFilter removes the last character from the current filter.
func (s *ListState[T]) DeleteFilter() {
	if len(s.filter) == 0 {
		return
	}
	s.SetFilter(s.filter[:len(s.filter)-1])
}

// ClearFilter clears the current filter.
func (s *ListState[T]) ClearFilter() {
	s.SetFilter("")
}

// Refilter reapplies the current filter against the current items.
func (s *ListState[T]) Refilter() {
	s.apply()
}

// Up moves the cursor one row up.
func (s *ListState[T]) Up() {
	if s.cursor > 0 {
		s.cursor--
	}
}

// Down moves the cursor one row down.
func (s *ListState[T]) Down() {
	if s.cursor < len(s.filtered)-1 {
		s.cursor++
	}
}

// Selected returns the currently selected filtered item.
func (s *ListState[T]) Selected() (T, bool) {
	var zero T
	if s.cursor < 0 || s.cursor >= len(s.filtered) {
		return zero, false
	}
	return s.filtered[s.cursor], true
}

// FilteredItems returns the filtered items in display order.
func (s *ListState[T]) FilteredItems() []T {
	return s.filtered
}

// Filter returns the current filter text.
func (s *ListState[T]) Filter() string {
	return s.filter
}

// Cursor returns the current filtered cursor position.
func (s *ListState[T]) Cursor() int {
	return s.cursor
}

func (s *ListState[T]) apply() {
	if s.filter == "" {
		s.filtered = clone(s.items)
	} else {
		needle := strings.ToLower(s.filter)
		s.filtered = s.filtered[:0]
		for _, item := range s.items {
			if s.match == nil || s.match(item, needle) {
				s.filtered = append(s.filtered, item)
			}
		}
	}
	if s.cursor >= len(s.filtered) {
		s.cursor = max(0, len(s.filtered)-1)
	}
}

func clone[T any](items []T) []T {
	out := make([]T, len(items))
	copy(out, items)
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
