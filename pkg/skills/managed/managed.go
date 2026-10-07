package managed

import internalmanaged "github.com/KPO-Tech/seshat/internal/tools/system/skills/managed"

// EnsureExtracted extracts builtin skills to destDir if they have not yet been
// extracted at the current Version.  It is safe to call on every boot — it is
// a no-op when the version stamp already matches.
func EnsureExtracted(destDir string) error {
	return internalmanaged.EnsureExtracted(destDir)
}
