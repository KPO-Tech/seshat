package managed

import internalmanaged "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills/managed"

func EnsureExtracted(destDir string) error {
	return internalmanaged.EnsureExtracted(destDir)
}
