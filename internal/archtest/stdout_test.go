package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// interactiveStdout are the files of internal/ that talk to a person on the standard output on purpose: a login prompt, a question to
// answer on the console, a console sink. Everything else in internal/ is a library of an engine that can be embedded (gRPC, MCP over
// stdio, the SDK), where text on the standard output corrupts the host's own protocol: it reports through log/slog or returns the
// information. internal/seshattui is the terminal interface itself and is not looked at.
var interactiveStdout = map[string]bool{
	"internal/tools/special/ask_user/askUserQuestionTool.go": true,
	"internal/providers/auth.go":                             true,
	"internal/automation/sink.go":                            true,
}

func TestInternalLibraryCodeDoesNotPrintToStdout(t *testing.T) {
	root := filepath.Join("..", "..")
	var problems []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "internal/seshattui" || rel == "internal/archtest" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || interactiveStdout[rel] {
			return nil
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && pkg.Name == "fmt" && (sel.Sel.Name == "Print" || sel.Sel.Name == "Printf" || sel.Sel.Name == "Println") {
				problems = append(problems, rel+": fmt."+sel.Sel.Name+" writes to the standard output of the host; use log/slog, or add the file to interactiveStdout if it is a prompt")
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}
