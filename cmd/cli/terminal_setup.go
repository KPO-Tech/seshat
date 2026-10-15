package main

import (
	"flag"
	"fmt"
	"io"
	"runtime"

	"github.com/KPO-Tech/seshat-runtime/internal/termsetup"
)

// runTerminalSetup makes shift+Enter and ctrl+Enter insert a newline in the terminal
// interface. A terminal sends the same byte for Enter, shift+Enter and ctrl+Enter, so
// Windows Terminal and the VS Code terminal have to be told to send the sequences the
// interface decodes (see internal/termsetup). ctrl+j works everywhere without it.
func runTerminalSetup(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("terminal-setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "show what would change without writing anything")
	if err := fs.Parse(args); err != nil {
		return err
	}

	results := termsetup.Run(termsetup.Options{DryRun: *check, GOOS: runtime.GOOS})
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No Windows Terminal or VS Code settings found on this machine, nothing to change.")
		printTerminalAdvice(stdout)
		return nil
	}

	failed := false
	for _, r := range results {
		fmt.Fprintf(stdout, "%s: %s\n", r.Target, r.Path)
		switch r.Status {
		case termsetup.Added:
			fmt.Fprintf(stdout, "  done: %s\n", r.Detail)
			if r.Backup != "" {
				fmt.Fprintf(stdout, "  backup: %s\n", r.Backup)
			}
		case termsetup.WouldAdd:
			fmt.Fprintln(stdout, "  would add shift+enter and ctrl+enter (nothing written, --check)")
		case termsetup.Failed:
			failed = true
			fmt.Fprintf(stdout, "  failed: %s\n", r.Detail)
		default:
			fmt.Fprintf(stdout, "  %s\n", r.Detail)
		}
	}
	fmt.Fprintln(stdout)
	if !*check {
		fmt.Fprintln(stdout, "Restart the terminal for the change to apply. The key help at the bottom of the screen still shows ctrl+j.")
	}
	printTerminalAdvice(stdout)
	if failed {
		return fmt.Errorf("terminal setup failed for at least one file (nothing was changed in it)")
	}
	return nil
}

func printTerminalAdvice(out io.Writer) {
	fmt.Fprintln(out, "ctrl+j inserts a newline in every terminal. Terminals with a keyboard protocol (kitty, WezTerm, Ghostty, foot, recent Alacritty, iTerm2) also send shift+enter by themselves.")
}
