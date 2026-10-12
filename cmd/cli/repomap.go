package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/KPO-Tech/seshat-runtime/pkg/repomap"
)

func runRepoMap(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tokens := fs.Int("tokens", repomap.DefaultTokenBudget, "approximate token budget for rendered output")
	cwd := fs.String("cwd", "", "repository root or working directory")
	var focusFiles multiFlag
	var focusSymbols multiFlag
	fs.Var(&focusFiles, "focus", "path fragment to boost in ranking; repeatable")
	fs.Var(&focusSymbols, "focus-symbol", "symbol name to boost in ranking; repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	m, err := repomap.Build(ctx, repomap.Options{
		Root:         *cwd,
		TokenBudget:  *tokens,
		FocusFiles:   focusFiles,
		FocusSymbols: focusSymbols,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, repomap.Render(m, *tokens))
	return nil
}

type multiFlag []string

func (m *multiFlag) String() string {
	if m == nil {
		return ""
	}
	return fmt.Sprint([]string(*m))
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}
