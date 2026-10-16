package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const tuiImportPath = "github.com/KPO-Tech/seshat-runtime/tui"

// The terminal interface lives in the tui module, apart from the engine: part of it comes from Crush and is under
// the Functional Source License (see NOTICE and docs/licensing.md). The engine, the gRPC server and the SDK must
// not depend on it, so that someone who embeds the engine does not take that code with them. Go already refuses a
// cycle when the dependency is real; this test also catches an import added in a package the tui does not use.
func TestTheEngineDoesNotImportTheTerminalInterface(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "bin", "tmp", "graphify-out", "tui":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, spec := range file.Imports {
			imported, uerr := strconv.Unquote(spec.Path.Value)
			if uerr == nil && (imported == tuiImportPath || strings.HasPrefix(imported, tuiImportPath+"/")) {
				offenders = append(offenders, rel+" imports "+imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(offenders)
	for _, offender := range offenders {
		t.Errorf("%s: the engine must not depend on the terminal interface (the tui module, under the Crush FSL licence, see docs/licensing.md)", offender)
	}
}
