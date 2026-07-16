package managed

import internalmanaged "github.com/KPO-Tech/seshat/internal/tools/system/skills/managed"

func EnsureExtracted(destDir string) error {
	return internalmanaged.EnsureExtracted(destDir)
}
