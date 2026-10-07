package repomap

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultTokenBudget = 2048

// Options configures a repository map: the directory to map (the working directory when empty), the budget in tokens of the rendered map, and the files and symbols that the ranking should favour.
type Options struct {
	Root         string
	TokenBudget  int
	FocusFiles   []string
	FocusSymbols []string
}

// Map is a ranked outline of a repository: how many files were scanned and how many were mapped, and the entries, best ranked first.
type Map struct {
	Root          string
	FilesScanned  int
	FilesIncluded int
	Entries       []Entry
}

// Entry is one file of the map: its path, language and package, what it imports, the symbols it declares, and the rank score that orders it.
type Entry struct {
	Path      string
	Language  string
	Package   string
	Imports   []string
	Symbols   []Symbol
	RankScore float64
}

// Symbol is a declaration found in a file: its kind, name and signature, whether it is exported, and its line.
type Symbol struct {
	Kind      string
	Name      string
	Signature string
	Exported  bool
	Line      int
}

// Build scans the repository under opts.Root, keeps the Go source files (not the tests nor the generated files) that declare or import something, ranks them, favouring opts.FocusFiles and opts.FocusSymbols, and returns the map. Cancelling ctx stops the scan.
func Build(ctx context.Context, opts Options) (*Map, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	files, err := enumerateFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(files))
	for _, rel := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if filepath.Ext(rel) != ".go" || strings.HasSuffix(rel, "_test.go") || isGeneratedGoFile(rel) {
			continue
		}
		entry, err := parseGoFile(root, rel)
		if err != nil {
			continue
		}
		if len(entry.Symbols) == 0 && len(entry.Imports) == 0 {
			continue
		}
		entries = append(entries, entry)
	}
	rankEntries(entries, opts.FocusFiles, opts.FocusSymbols)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].RankScore == entries[j].RankScore {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].RankScore > entries[j].RankScore
	})
	return &Map{Root: root, FilesScanned: len(files), FilesIncluded: len(entries), Entries: entries}, nil
}

// Render writes a map as text for a prompt: a header, then the entries in rank order until the budget of tokenBudget tokens (about four characters each, DefaultTokenBudget when it is 0 or less) is reached. A nil map renders as an empty string.
func Render(m *Map, tokenBudget int) string {
	if m == nil {
		return ""
	}
	if tokenBudget <= 0 {
		tokenBudget = DefaultTokenBudget
	}
	charBudget := tokenBudget * 4
	var b strings.Builder
	fmt.Fprintf(&b, "Repo map for %s\n", m.Root)
	fmt.Fprintf(&b, "Files scanned: %d; Go files mapped: %d\n\n", m.FilesScanned, m.FilesIncluded)
	wroteEntries := 0
	for _, entry := range m.Entries {
		before := b.Len()
		fmt.Fprintf(&b, "%s", filepath.ToSlash(entry.Path))
		if entry.Package != "" {
			fmt.Fprintf(&b, " package %s", entry.Package)
		}
		fmt.Fprintln(&b)
		for _, sym := range entry.Symbols {
			if b.Len() > charBudget && wroteEntries > 0 {
				out := b.String()[:before]
				return strings.TrimRight(out, "\n") + "\n\n... repo map truncated to budget"
			}
			line := "  " + sym.Signature
			if sym.Line > 0 {
				line = fmt.Sprintf("  L%d %s", sym.Line, sym.Signature)
			}
			fmt.Fprintln(&b, line)
		}
		if len(entry.Imports) > 0 {
			fmt.Fprintf(&b, "  imports: %s\n", strings.Join(entry.Imports, ", "))
		}
		fmt.Fprintln(&b)
		if b.Len() > charBudget {
			if wroteEntries > 0 {
				out := b.String()[:before]
				return strings.TrimRight(out, "\n") + "\n\n... repo map truncated to budget"
			}
			return strings.TrimRight(b.String(), "\n") + "\n\n... repo map truncated to budget"
		}
		wroteEntries++
	}
	return strings.TrimRight(b.String(), "\n")
}

func enumerateFiles(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err == nil {
		return splitGitFiles(out), nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if d.IsDir() && shouldSkipDir(rel) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

func splitGitFiles(out []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			files = append(files, filepath.FromSlash(trimmed))
		}
	}
	return files
}

func shouldSkipDir(rel string) bool {
	first := strings.Split(filepath.ToSlash(rel), "/")[0]
	switch first {
	case ".git", ".seshat", "node_modules", "vendor", "dist", "build", "coverage", ".next", ".cache":
		return true
	default:
		return false
	}
}

func isGeneratedGoFile(rel string) bool {
	base := filepath.Base(rel)
	return strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, "_generated.go") ||
		strings.HasSuffix(base, ".gen.go") ||
		strings.HasSuffix(base, "_gen.go")
}

func parseGoFile(root, rel string) (Entry, error) {
	path := filepath.Join(root, rel)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{Path: rel, Language: "go", Package: file.Name.Name}
	for _, imp := range file.Imports {
		entry.Imports = append(entry.Imports, strings.Trim(imp.Path.Value, `"`))
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			entry.Symbols = append(entry.Symbols, Symbol{
				Kind:      "func",
				Name:      d.Name.Name,
				Signature: renderFuncSignature(d),
				Exported:  d.Name.IsExported(),
				Line:      fset.Position(d.Pos()).Line,
			})
		case *ast.GenDecl:
			entry.Symbols = append(entry.Symbols, renderGenDeclSymbols(fset, d)...)
		}
	}
	return entry, nil
}

func renderFuncSignature(fn *ast.FuncDecl) string {
	var b bytes.Buffer
	b.WriteString("func ")
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(exprString(fn.Recv.List[0].Type))
		b.WriteString(") ")
	}
	b.WriteString(fn.Name.Name)
	b.WriteString(fieldListString(fn.Type.Params, true))
	if fn.Type.Results != nil {
		b.WriteString(" ")
		b.WriteString(fieldListString(fn.Type.Results, false))
	}
	return b.String()
}

func renderGenDeclSymbols(fset *token.FileSet, decl *ast.GenDecl) []Symbol {
	var symbols []Symbol
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			kind := "type"
			switch s.Type.(type) {
			case *ast.StructType:
				kind = "struct"
			case *ast.InterfaceType:
				kind = "interface"
			}
			symbols = append(symbols, Symbol{Kind: kind, Name: s.Name.Name, Signature: kind + " " + s.Name.Name, Exported: s.Name.IsExported(), Line: fset.Position(s.Pos()).Line})
		case *ast.ValueSpec:
			for _, name := range s.Names {
				if name.Name == "_" {
					continue
				}
				kind := strings.ToLower(decl.Tok.String())
				symbols = append(symbols, Symbol{Kind: kind, Name: name.Name, Signature: kind + " " + name.Name, Exported: name.IsExported(), Line: fset.Position(name.Pos()).Line})
			}
		}
	}
	return symbols
}

func fieldListString(fields *ast.FieldList, includeNames bool) string {
	if fields == nil || len(fields.List) == 0 {
		return "()"
	}
	parts := []string{}
	for _, field := range fields.List {
		typ := exprString(field.Type)
		if includeNames && len(field.Names) > 0 {
			names := make([]string, 0, len(field.Names))
			for _, name := range field.Names {
				names = append(names, name.Name)
			}
			parts = append(parts, strings.Join(names, ", ")+" "+typ)
		} else {
			parts = append(parts, typ)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func exprString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var b bytes.Buffer
	_ = printer.Fprint(&b, token.NewFileSet(), expr)
	out := b.String()
	out = strings.ReplaceAll(out, "\n", " ")
	out = strings.Join(strings.Fields(out), " ")
	return out
}

func rankEntries(entries []Entry, focusFiles, focusSymbols []string) {
	importCounts := map[string]int{}
	for _, entry := range entries {
		for _, imp := range entry.Imports {
			importCounts[imp]++
		}
	}
	for i := range entries {
		score := float64(len(entries[i].Symbols))
		for _, sym := range entries[i].Symbols {
			if sym.Exported {
				score += 2
			}
			if containsFold(focusSymbols, sym.Name) {
				score += 25
			}
		}
		for imp, count := range importCounts {
			if strings.Contains(imp, strings.TrimSuffix(filepath.ToSlash(entries[i].Path), ".go")) {
				score += float64(count * 3)
			}
		}
		if pathFocused(entries[i].Path, focusFiles) {
			score += 1000
		}
		entries[i].RankScore = score
	}
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func pathFocused(path string, focuses []string) bool {
	path = filepath.ToSlash(path)
	for _, focus := range focuses {
		focus = filepath.ToSlash(strings.TrimSpace(focus))
		if focus != "" && strings.Contains(path, focus) {
			return true
		}
	}
	return false
}
