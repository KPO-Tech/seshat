// Package utils provides common utility functions used across the codebase.
//
// This package contains shared helper functions that are used by multiple
// packages to avoid code duplication. Functions here are generally simple,
// stateless utilities that don't fit into more specific packages.
package utils

// CloneInput creates a shallow copy of a map[string]any.
// This is used to prevent mutation of original input during processing.
// Returns nil if input is nil.
func CloneInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for k, v := range input {
		cloned[k] = v
	}
	return cloned
}

// CloneMap creates a shallow copy of a generic map.
// Returns nil if input is nil.
func CloneMap[K comparable, V any](input map[K]V) map[K]V {
	if input == nil {
		return nil
	}
	cloned := make(map[K]V, len(input))
	for k, v := range input {
		cloned[k] = v
	}
	return cloned
}

// CloneSlice creates a shallow copy of a slice.
// Returns nil if input is nil.
func CloneSlice[T any](input []T) []T {
	if input == nil {
		return nil
	}
	cloned := make([]T, len(input))
	copy(cloned, input)
	return cloned
}
