package toolsearch

import (
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
)

// mockDef creates a contract.Definition for testing.
func mockDef(name, displayName, category, searchHint, description string) contract.Definition {
	return contract.Definition{
		Name:        name,
		DisplayName: displayName,
		Category:    category,
		SearchHint:  searchHint,
		Description: description,
		InputSchema: schema.FromMap(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
	}
}

// ─── tokenize ─────────────────────────────────────────────────────────────────

func TestTokenize_basic(t *testing.T) {
	tokens := tokenize("Read a file from disk")
	// "a" and "from" are stop words; "read", "file", "disk" remain
	want := map[string]bool{"read": true, "file": true, "disk": true}
	for _, tok := range tokens {
		if !want[tok] {
			t.Errorf("unexpected token %q", tok)
		}
	}
	for w := range want {
		found := false
		for _, tok := range tokens {
			if tok == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected token %q not found in %v", w, tokens)
		}
	}
}

func TestTokenize_underscores(t *testing.T) {
	tokens := tokenize("read_file write_stdin")
	// underscores split into separate tokens
	found := map[string]bool{}
	for _, tok := range tokens {
		found[tok] = true
	}
	for _, want := range []string{"read", "file", "write", "stdin"} {
		if !found[want] {
			t.Errorf("expected token %q not found, got %v", want, tokens)
		}
	}
}

func TestTokenize_stopWords(t *testing.T) {
	tokens := tokenize("the a an and or with")
	if len(tokens) != 0 {
		t.Errorf("expected all stop words filtered, got %v", tokens)
	}
}

func TestTokenize_short(t *testing.T) {
	tokens := tokenize("ab") // length 2 — accepted (≥2)
	if len(tokens) == 0 {
		t.Error("2-char tokens should be accepted")
	}
	tokens = tokenize("a") // length 1 — rejected
	for _, tok := range tokens {
		if tok == "a" {
			t.Error("single-char token should be rejected")
		}
	}
}

// ─── BM25Engine ───────────────────────────────────────────────────────────────

func TestBM25Engine_empty(t *testing.T) {
	e := NewBM25Engine(nil)
	results := e.Search("anything", 5)
	if len(results) != 0 {
		t.Errorf("empty engine should return no results, got %d", len(results))
	}
}

func TestBM25Engine_singleDoc(t *testing.T) {
	e := NewBM25Engine([]string{"read file from disk"})
	results := e.Search("read", 5)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].ID != 0 {
		t.Errorf("single doc should have ID=0, got %d", results[0].ID)
	}
	if results[0].Score <= 0 {
		t.Error("matching doc should have positive score")
	}
}

func TestBM25Engine_ranking(t *testing.T) {
	// Doc 0 mentions "git" once; doc 1 mentions "git" three times → doc 1 scores higher.
	e := NewBM25Engine([]string{
		"git status list files",
		"git commit git push git status for version control",
	})
	results := e.Search("git", 5)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != 1 {
		t.Errorf("doc 1 (more 'git' mentions) should rank first, got ID=%d", results[0].ID)
	}
}

func TestBM25Engine_limit(t *testing.T) {
	texts := []string{
		"bash shell command execute",
		"shell bash script run",
		"execute bash terminal command",
		"command line shell",
		"bash run script execute command",
	}
	e := NewBM25Engine(texts)
	results := e.Search("bash", 3)
	if len(results) > 3 {
		t.Errorf("limit=3 but got %d results", len(results))
	}
}

func TestBM25Engine_noMatch(t *testing.T) {
	e := NewBM25Engine([]string{"read file", "write file", "delete file"})
	results := e.Search("calendar", 5)
	if len(results) != 0 {
		t.Errorf("unrelated query should return no results, got %d", len(results))
	}
}

func TestBM25Engine_descending(t *testing.T) {
	e := NewBM25Engine([]string{
		"bash command execution shell",
		"bash bash bash bash bash — strong signal",
		"something else entirely",
	})
	results := e.Search("bash", 5)
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted descending at idx %d: %.4f > %.4f",
				i, results[i].Score, results[i-1].Score)
		}
	}
}

func TestBM25Engine_multiTermQuery(t *testing.T) {
	e := NewBM25Engine([]string{
		"search the web for information using search engine",
		"read a file from disk",
		"web browser navigation fetch url",
	})
	// "web search" should match docs 0 and 2 (not 1).
	results := e.Search("web search", 5)
	ids := make(map[int]bool)
	for _, r := range results {
		ids[r.ID] = true
	}
	if ids[1] {
		t.Error("doc 1 (read file) should not match 'web search'")
	}
}

// ─── buildSearchText ──────────────────────────────────────────────────────────

func TestBuildSearchText_includesAllFields(t *testing.T) {
	def := mockDef("apply_patch", "Apply Patch", "filesystem", "apply structured patch to files",
		"Apply a structured patch to multiple files atomically.")

	text := buildSearchText(def)

	for _, want := range []string{
		"apply_patch",
		"apply patch", // underscore → space
		"filesystem",
		"apply structured patch",
		"Apply a structured patch",
	} {
		if !contains(text, want) {
			t.Errorf("buildSearchText missing %q in: %q", want, text[:min(len(text), 200)])
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle ||
			len(haystack) > 0 && containsStr(haystack, needle))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── toolNamespace ────────────────────────────────────────────────────────────

func TestToolNamespace_mcp(t *testing.T) {
	def := mockDef("calendar_create", "", "", "", "")
	def.IsMCP = true
	if ns := toolNamespace(def); ns != "mcp" {
		t.Errorf("expected mcp, got %q", ns)
	}
}

func TestToolNamespace_deferred(t *testing.T) {
	def := mockDef("some_tool", "", "", "", "")
	def.ShouldDefer = true
	if ns := toolNamespace(def); ns != "deferred" {
		t.Errorf("expected deferred, got %q", ns)
	}
}

func TestToolNamespace_category(t *testing.T) {
	def := mockDef("git_diff", "", "vcs", "", "")
	if ns := toolNamespace(def); ns != "vcs" {
		t.Errorf("expected vcs, got %q", ns)
	}
}

func TestToolNamespace_builtin(t *testing.T) {
	def := mockDef("bash", "", "", "", "")
	if ns := toolNamespace(def); ns != "builtin" {
		t.Errorf("expected builtin, got %q", ns)
	}
}
