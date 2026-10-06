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

// undocumentedExported is, for each package of pkg/ (the public API), how many exported functions, methods of exported types and
// types had no doc comment when this test was written. AGENTS.md asks for a doc comment on every public type, and a public API
// without them cannot be used without reading the code. The numbers only go down: a new exported symbol without a comment raises the
// count and fails the test, and writing a comment lowers it and fails the test until the number here is lowered too.
var undocumentedExported = map[string]int{
	"pkg/agent":                   6,
	"pkg/auth/oauth":              8,
	"pkg/automation":              58,
	"pkg/companion":               6,
	"pkg/config":                  18,
	"pkg/connectors":              15,
	"pkg/contract":                1,
	"pkg/dataflow":                9,
	"pkg/dataflow/expr":           1,
	"pkg/dataflow/nodes":          35,
	"pkg/dataflow/nodes/database": 15,
	"pkg/doctor":                  8,
	"pkg/documentreader":          7,
	"pkg/grpc/seshat":             169,
	"pkg/mcp":                     16,
	"pkg/memory/longterm":         12,
	"pkg/model":                   5,
	"pkg/monitoring":              4,
	"pkg/msgraph":                 7,
	"pkg/pdfsmart":                7,
	"pkg/providers":               16,
	"pkg/rag":                     39,
	"pkg/rag/embedder":            5,
	"pkg/rag/reranker":            1,
	"pkg/repomap":                 6,
	"pkg/runtimepath":             19,
	"pkg/sdk":                     37,
	"pkg/skills":                  20,
	"pkg/skills/managed":          1,
	"pkg/skills/skillrepos":       4,
	"pkg/storage":                 21,
	"pkg/tools":                   6,
	"pkg/types":                   35,
	"pkg/vector":                  9,
	"pkg/web":                     5,
	"pkg/web/search":              10,
	"pkg/web/search/providers":    19,
	"pkg/workflow":                16,
	"pkg/workspace":               4,
}

func TestPublicAPIDocCommentsDoNotRegress(t *testing.T) {
	root := filepath.Join("..", "..")
	got := map[string]int{}
	err := filepath.WalkDir(filepath.Join(root, "pkg"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if perr != nil {
			return perr
		}
		rel, rerr := filepath.Rel(root, filepath.Dir(path))
		if rerr != nil {
			return rerr
		}
		pkg := filepath.ToSlash(rel)
		for _, decl := range file.Decls {
			switch x := decl.(type) {
			case *ast.FuncDecl:
				if !x.Name.IsExported() || x.Doc != nil {
					continue
				}
				if x.Recv != nil && !exportedReceiver(x.Recv) {
					continue
				}
				got[pkg]++
			case *ast.GenDecl:
				for _, spec := range x.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() && x.Doc == nil && ts.Doc == nil {
						got[pkg]++
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	pkgs := map[string]bool{}
	for p := range got {
		pkgs[p] = true
	}
	for p := range undocumentedExported {
		pkgs[p] = true
	}
	names := make([]string, 0, len(pkgs))
	for p := range pkgs {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		switch have, allowed := got[p], undocumentedExported[p]; {
		case have > allowed:
			t.Errorf("%s has %d exported symbols without a doc comment, %d are known: document the new ones (AGENTS.md)", p, have, allowed)
		case have < allowed:
			t.Errorf("%s has %d exported symbols without a doc comment, the list says %d: lower undocumentedExported[%q] to %d", p, have, allowed, p, have)
		}
	}
}

func exportedReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	typ := recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if idx, ok := typ.(*ast.IndexExpr); ok {
		typ = idx.X
	}
	id, ok := typ.(*ast.Ident)
	return ok && id.IsExported()
}
