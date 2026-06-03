package engine

import (
	"os"
	"path/filepath"
	"strings"
)

const projectInstructionsMaxBytes = 32 * 1024 // 32 KB cap

// candidateInstructionFiles lists the file names checked, in priority order.
// The first non-empty file found wins.
var candidateInstructionFiles = []string{
	"NEXUS.md",
	"AGENTS.md",
	filepath.Join(".nexus", "instructions.md"),
}

// readProjectInstructions reads project-level instructions from the working
// directory. It checks NEXUS.md, AGENTS.md, and .nexus/instructions.md in
// order and returns the first non-empty content found. Returns "" if no file
// is found or all are empty.
func readProjectInstructions(workdir string) string {
	for _, name := range candidateInstructionFiles {
		path := filepath.Join(workdir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		if len(data) > projectInstructionsMaxBytes {
			// Truncate at last newline within the limit to avoid cutting mid-line.
			truncated := data[:projectInstructionsMaxBytes]
			if idx := strings.LastIndexByte(string(truncated), '\n'); idx > 0 {
				truncated = truncated[:idx]
			}
			content = strings.TrimSpace(string(truncated))
		}
		return content
	}
	return ""
}
