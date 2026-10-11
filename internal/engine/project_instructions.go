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
	"SESHAT.md",
	"AGENTS.md",
	filepath.Join(".seshat", "instructions.md"),
}

// readProjectInstructions reads project-level instructions from the working
// directory. It checks SESHAT.md, AGENTS.md, and .seshat/instructions.md in
// order and returns the first non-empty content found. Returns "" if no file
// is found or all are empty.
//
// This does not check project trust - see readTrustedProjectInstructions,
// which every real caller (Session.buildAPIRequestForRuntimeTools) must use
// instead. This unchecked version stays only so the existing file-finding/
// truncation tests (TestReadProjectInstructions*) keep exercising that logic
// directly.
func readProjectInstructions(workdir string) string {
	_, _, content := findProjectInstructions(workdir)
	return content
}

// findProjectInstructions is readProjectInstructions split out to also report
// which file (if any) the content came from and its raw bytes, so
// readTrustedProjectInstructions can hash-check it without reading the file
// twice.
func findProjectInstructions(workdir string) (path string, rawData []byte, content string) {
	for _, name := range candidateInstructionFiles {
		p := filepath.Join(workdir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" {
			continue
		}
		if len(data) > projectInstructionsMaxBytes {
			// Truncate at last newline within the limit to avoid cutting mid-line.
			truncated := data[:projectInstructionsMaxBytes]
			if idx := strings.LastIndexByte(string(truncated), '\n'); idx > 0 {
				truncated = truncated[:idx]
			}
			trimmed = strings.TrimSpace(string(truncated))
		}
		return p, data, trimmed
	}
	return "", nil, ""
}

// readTrustedProjectInstructions is readProjectInstructions gated behind
// project trust (see docs/project-trust.md): a repository that was just
// cloned can carry a SESHAT.md, AGENTS.md or .seshat/instructions.md to put
// its own instructions directly into the model's system prompt. trustFn
// decides, by exact path and file content (hash-based, like the project's
// other configuration files - see internal/seshattui/config/trust.go),
// whether to use it. A nil trustFn, or one that returns false, fails closed:
// the file is found but never returned, exactly like an untrusted project's
// restricted configuration sections.
func readTrustedProjectInstructions(workdir string, trustFn func(path string, content []byte) bool) string {
	path, rawData, content := findProjectInstructions(workdir)
	if path == "" || trustFn == nil || !trustFn(path, rawData) {
		return ""
	}
	return content
}
