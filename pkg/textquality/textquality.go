// Package textquality re-exports internal/textquality's broken-extraction
// detection for external consumers (e.g. seshat-ai).
package textquality

import internaltextquality "github.com/KPO-Tech/seshat-runtime/internal/textquality"

// IsGarbledText reports whether s looks like broken text extraction (e.g. a
// PDF's subsetted-font character mapping producing "(cid:NNN)" placeholders
// or Private Use Area code points instead of real characters) rather than
// genuine content. See internal/textquality.IsGarbledText for details.
func IsGarbledText(s string) bool {
	return internaltextquality.IsGarbledText(s)
}
