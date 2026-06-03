package patch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/files/shared"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ─── Change representation ────────────────────────────────────────────────────

// ChangeKind describes what happens to a file in a patch.
// Mirrors Codex's PatchChangeKind (add | delete | update | move).
type ChangeKind string

const (
	ChangeKindAdd    ChangeKind = "add"
	ChangeKindDelete ChangeKind = "delete"
	ChangeKindUpdate ChangeKind = "update"
	ChangeKindMove   ChangeKind = "move"
)

// PatchChange is a structured description of a single file change in a patch,
// computed BEFORE the patch is applied. Used for approval previews and progress events.
type PatchChange struct {
	Kind        ChangeKind `json:"kind"`
	Path        string     `json:"path"`                   // absolute resolved source path
	MovePath    string     `json:"move_path,omitempty"`    // non-empty for ChangeKindMove
	DiffPreview string     `json:"diff_preview,omitempty"` // compact diff for updates
}

// AnalyzeChanges returns a structured list of changes the patch will make,
// computed from hunk data without reading files from disk. Use this to build
// the approval description before calling Apply.
func (p *Patch) AnalyzeChanges(workingDir string) ([]PatchChange, error) {
	seen := make(map[string]struct{})
	var changes []PatchChange

	for _, h := range p.hunks {
		abs, err := resolvePatchPath(workingDir, h.path)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", h.path, err)
		}

		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}

		switch h.typ {
		case hunkAdd:
			changes = append(changes, PatchChange{
				Kind:        ChangeKindAdd,
				Path:        abs,
				DiffPreview: fmt.Sprintf("(new file, %d lines)", len(h.addLines)),
			})
		case hunkDelete:
			changes = append(changes, PatchChange{
				Kind: ChangeKindDelete,
				Path: abs,
			})
		case hunkUpdate:
			kind := ChangeKindUpdate
			var movePath string
			if h.moveTo != "" {
				kind = ChangeKindMove
				dst, err := resolvePatchPath(workingDir, h.moveTo)
				if err != nil {
					return nil, fmt.Errorf("resolve move target %q: %w", h.moveTo, err)
				}
				movePath = dst
				seen[dst] = struct{}{}
			}
			changes = append(changes, PatchChange{
				Kind:        kind,
				Path:        abs,
				MovePath:    movePath,
				DiffPreview: buildDiffPreview(h.blocks),
			})
		}
	}
	return changes, nil
}

// buildDiffPreview formats change blocks as a compact diff string for approval previews.
func buildDiffPreview(blocks []changeBlock) string {
	var sb strings.Builder
	for _, block := range blocks {
		if hint := strings.TrimSpace(block.hint); hint != "" {
			fmt.Fprintf(&sb, "@@ %s @@\n", hint)
		} else {
			sb.WriteString("@@\n")
		}
		for _, cl := range block.lines {
			sb.WriteByte(cl.op)
			sb.WriteString(cl.text)
			sb.WriteByte('\n')
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ─── Types ────────────────────────────────────────────────────────────────────

type hunkType int

const (
	hunkAdd    hunkType = iota // *** Add File
	hunkDelete                 // *** Delete File
	hunkUpdate                 // *** Update File
)

// changeLine is one line inside an update change block.
type changeLine struct {
	op   byte   // ' ' context, '+' add, '-' remove
	text string // content without the leading op character
}

// changeBlock is one @@ region within an update hunk.
type changeBlock struct {
	hint  string // text after "@@ " — informational only
	lines []changeLine
}

// hunk is a single file operation in the patch.
type hunk struct {
	typ      hunkType
	path     string        // source path (as written in patch)
	moveTo   string        // non-empty when "*** Move to:" is present
	addLines []string      // content lines for hunkAdd (already stripped of leading '+')
	blocks   []changeBlock // change blocks for hunkUpdate
}

// Patch is a parsed, ready-to-apply patch.
type Patch struct {
	hunks []hunk
}

// ApplySummary reports what was done.
type ApplySummary struct {
	Added   []string
	Deleted []string
	Updated []string
	Moved   []string // "src → dst"
}

// ─── Parser ───────────────────────────────────────────────────────────────────

// Parse parses a patch in the apply_patch format.
func Parse(text string) (*Patch, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	p := &Patch{}
	started := false
	var cur *hunk
	var curBlock *changeBlock

	finishBlock := func() {
		if curBlock != nil && cur != nil {
			cur.blocks = append(cur.blocks, *curBlock)
			curBlock = nil
		}
	}
	finishHunk := func() {
		finishBlock()
		if cur != nil {
			p.hunks = append(p.hunks, *cur)
			cur = nil
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")

		if !started {
			if line == "*** Begin Patch" {
				started = true
			}
			continue
		}

		if line == "*** End Patch" {
			finishHunk()
			break
		}

		// ── Hunk headers ────────────────────────────────────────────────────
		if strings.HasPrefix(line, "*** Add File: ") {
			finishHunk()
			cur = &hunk{typ: hunkAdd, path: strings.TrimPrefix(line, "*** Add File: ")}
			continue
		}
		if strings.HasPrefix(line, "*** Delete File: ") {
			finishHunk()
			p.hunks = append(p.hunks, hunk{typ: hunkDelete, path: strings.TrimPrefix(line, "*** Delete File: ")})
			continue
		}
		if strings.HasPrefix(line, "*** Update File: ") {
			finishHunk()
			cur = &hunk{typ: hunkUpdate, path: strings.TrimPrefix(line, "*** Update File: ")}
			continue
		}
		if strings.HasPrefix(line, "*** Move to: ") {
			if cur != nil && cur.typ == hunkUpdate {
				cur.moveTo = strings.TrimPrefix(line, "*** Move to: ")
			}
			continue
		}
		if line == "*** End of File" {
			finishBlock()
			continue
		}

		// ── Change block header ──────────────────────────────────────────────
		if strings.HasPrefix(line, "@@") {
			finishBlock()
			hint := strings.TrimSpace(strings.TrimPrefix(line, "@@"))
			curBlock = &changeBlock{hint: hint}
			continue
		}

		// ── Content lines ────────────────────────────────────────────────────
		if cur == nil {
			continue
		}
		switch cur.typ {
		case hunkAdd:
			if len(line) > 0 && line[0] == '+' {
				cur.addLines = append(cur.addLines, line[1:])
			}
		case hunkUpdate:
			if len(line) == 0 {
				continue // blank line between blocks — skip
			}
			switch line[0] {
			case ' ', '+', '-':
				if curBlock == nil {
					curBlock = &changeBlock{} // implicit block (no @@ header)
				}
				curBlock.lines = append(curBlock.lines, changeLine{op: line[0], text: line[1:]})
			}
		}
	}

	if !started {
		return nil, fmt.Errorf("patch must start with '*** Begin Patch'")
	}
	if len(p.hunks) == 0 {
		return nil, fmt.Errorf("patch contains no hunks")
	}
	return p, nil
}

// ─── Apply ────────────────────────────────────────────────────────────────────

// Apply applies the patch to files rooted at workingDir.
// All changes are computed in memory first; disk I/O only happens once all
// checks pass (fail-fast, not fully transactional but error-proof).
// If ctx carries a RuntimeEventEmitter, one ToolProgress event is emitted per
// file as it is committed to disk.
func (p *Patch) Apply(ctx context.Context, workingDir string, toolCtxWorkingDir string) (*ApplySummary, error) {
	emitProgress := func(message string, percent float64) {
		if emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent)); ok && emitter != nil {
			emitter(types.RuntimeEvent{
				Type: types.RuntimeEventTypeToolProgress,
				ToolProgress: &types.ToolProgress{
					ToolName:        "apply_patch",
					Stage:           types.ToolProgressStageRunning,
					Message:         message,
					PercentComplete: percent,
				},
			})
		}
	}
	base := workingDir
	if toolCtxWorkingDir != "" {
		base = toolCtxWorkingDir
	}

	resolvePath := func(raw string) (string, error) {
		if filepath.IsAbs(raw) {
			return shared.GetAbsolutePath(raw)
		}
		return shared.GetAbsolutePath(filepath.Join(base, raw))
	}

	// ── Build in-memory change set ───────────────────────────────────────────
	// writes: abs-path → new content (nil = delete)
	// cache: abs-path → working content (for multi-hunk edits on the same file)
	writes := make(map[string][]byte)
	cache := make(map[string][]byte)
	deletes := make(map[string]bool)
	summary := &ApplySummary{}

	for _, h := range p.hunks {
		absPath, err := resolvePath(h.path)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", h.path, err)
		}
		if err := validatePatchPath(absPath, base); err != nil {
			return nil, fmt.Errorf("%q: %w", h.path, err)
		}

		switch h.typ {

		// ── Add ──────────────────────────────────────────────────────────────
		case hunkAdd:
			if _, dup := writes[absPath]; dup {
				return nil, fmt.Errorf("duplicate add for %q", h.path)
			}
			var sb strings.Builder
			for _, l := range h.addLines {
				sb.WriteString(l)
				sb.WriteByte('\n')
			}
			writes[absPath] = []byte(sb.String())
			cache[absPath] = writes[absPath]
			summary.Added = append(summary.Added, h.path)

		// ── Delete ───────────────────────────────────────────────────────────
		case hunkDelete:
			deletes[absPath] = true
			summary.Deleted = append(summary.Deleted, h.path)

		// ── Update ───────────────────────────────────────────────────────────
		case hunkUpdate:
			// Load: prefer in-memory cache over disk.
			var content []byte
			if cached, ok := cache[absPath]; ok {
				content = cached
			} else {
				content, err = os.ReadFile(absPath)
				if err != nil {
					return nil, fmt.Errorf("update %q: cannot read file: %w", h.path, err)
				}
				cache[absPath] = content
			}

			fileLines := splitLines(string(content))

			for i, block := range h.blocks {
				search, replace := buildSearchReplace(block.lines)
				start, end, ok := findLinesMatch(fileLines, search)
				if !ok {
					preview := contextPreview(search)
					return nil, fmt.Errorf("update %q block %d: context not found in file\n  looking for: %s", h.path, i+1, preview)
				}
				// Replace [start:end] with replace lines.
				newLines := make([]string, 0, len(fileLines)+len(replace)-len(search))
				newLines = append(newLines, fileLines[:start]...)
				newLines = append(newLines, replace...)
				newLines = append(newLines, fileLines[end:]...)
				fileLines = newLines
			}

			newContent := joinLines(fileLines)
			targetPath := absPath

			if h.moveTo != "" {
				targetPath, err = resolvePath(h.moveTo)
				if err != nil {
					return nil, fmt.Errorf("resolve move target %q: %w", h.moveTo, err)
				}
				if err := validatePatchPath(targetPath, base); err != nil {
					return nil, fmt.Errorf("move target %q: %w", h.moveTo, err)
				}
				deletes[absPath] = true
				summary.Moved = append(summary.Moved, h.path+" → "+h.moveTo)
			} else {
				summary.Updated = append(summary.Updated, h.path)
			}

			writes[targetPath] = newContent
			cache[targetPath] = newContent
		}
	}

	// ── Commit to disk ───────────────────────────────────────────────────────
	total := len(writes) + len(deletes)
	done := 0
	for path, content := range writes {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create parent dirs for %q: %w", path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, fmt.Errorf("write %q: %w", path, err)
		}
		done++
		if total > 0 {
			emitProgress(fmt.Sprintf("wrote %s", filepath.Base(path)), float64(done)/float64(total)*100)
		}
	}
	for path := range deletes {
		if _, written := writes[path]; written {
			continue // file was moved/updated: don't delete the new version
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("delete %q: %w", path, err)
		}
		done++
		if total > 0 {
			emitProgress(fmt.Sprintf("deleted %s", filepath.Base(path)), float64(done)/float64(total)*100)
		}
	}

	return summary, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// buildSearchReplace derives the search sequence and replace sequence from a
// change block.  Context lines (' ') appear in both; '-' only in search; '+'
// only in replace.
func buildSearchReplace(lines []changeLine) (search, replace []string) {
	for _, cl := range lines {
		switch cl.op {
		case ' ':
			search = append(search, cl.text)
			replace = append(replace, cl.text)
		case '-':
			search = append(search, cl.text)
		case '+':
			replace = append(replace, cl.text)
		}
	}
	return
}

// findLinesMatch returns the first occurrence of searchLines inside fileLines.
func findLinesMatch(fileLines, searchLines []string) (start, end int, found bool) {
	if len(searchLines) == 0 {
		return 0, 0, true
	}
outer:
	for i := 0; i <= len(fileLines)-len(searchLines); i++ {
		for j, sl := range searchLines {
			if fileLines[i+j] != sl {
				continue outer
			}
		}
		return i, i + len(searchLines), true
	}
	return 0, 0, false
}

// splitLines splits content into a slice of lines, stripping the final newline
// so that joinLines(splitLines(s)) == s for any well-formed text file.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	// Remove exactly one trailing newline to avoid a phantom empty last element.
	content = strings.TrimSuffix(content, "\n")
	return strings.Split(content, "\n")
}

// joinLines is the inverse of splitLines: adds back the trailing newline.
func joinLines(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// validatePatchPath ensures the path is within workingDir and not a protected path.
func validatePatchPath(absPath, workingDir string) error {
	if shared.IsDangerousFile(absPath) || shared.IsDangerousDirectory(absPath) {
		return fmt.Errorf("refusing to modify protected path")
	}
	if workingDir != "" && !shared.IsInWorkingDirectory(absPath, workingDir) {
		return fmt.Errorf("path is outside working directory")
	}
	return nil
}

// contextPreview returns the first few search lines for error messages.
func contextPreview(lines []string) string {
	if len(lines) == 0 {
		return "(empty)"
	}
	n := 3
	if len(lines) < n {
		n = len(lines)
	}
	return strings.Join(lines[:n], " | ")
}
