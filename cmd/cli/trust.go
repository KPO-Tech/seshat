package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/KPO-Tech/seshat/internal/seshattui/config"
)

// runTrust shows, grants or withdraws the trust of the configuration files of a project (see internal/seshattui/config/trust.go):
// until a project is trusted, what its .seshat.json says about MCP servers, hooks, LSP servers, providers, allowed tools and extra
// paths is ignored, because a repository that was just cloned can carry any of it.
func runTrust(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("trust", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.Bool("status", false, "show the configuration files of the project and whether they are trusted")
	untrust := fs.Bool("untrust", false, "withdraw the trust of the configuration files of the project")
	cwd := fs.String("cwd", "", "project directory (default: the current directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := strings.TrimSpace(*cwd)
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return err
		}
	}

	switch {
	case *untrust:
		removed, err := config.UntrustProject(dir)
		if err != nil {
			return err
		}
		if len(removed) == 0 {
			fmt.Fprintln(stdout, "No trusted configuration file in this project.")
			return nil
		}
		for _, f := range removed {
			fmt.Fprintf(stdout, "no longer trusted: %s\n", f)
		}
		return nil
	case *status:
		return printTrustStatus(dir, stdout)
	}

	files := config.ProjectConfigStatus(dir)
	if len(files) == 0 {
		fmt.Fprintln(stdout, "This project has no configuration file (.seshat.json, .seshat/seshat.json): nothing to trust.")
		return nil
	}
	fmt.Fprintln(stdout, "You are about to trust these configuration files, as they are now:")
	for _, f := range files {
		fmt.Fprintf(stdout, "  %s%s\n", f.Path, describeRestricted(f.Restricted))
	}
	fmt.Fprintln(stdout, "A trusted file can start programs (MCP servers, hooks, language servers), choose the provider that gets your API key, and allow tools without asking. Trust only a project you wrote or read.")
	trusted, err := config.TrustProject(dir)
	if err != nil {
		return err
	}
	for _, f := range trusted {
		fmt.Fprintf(stdout, "trusted: %s\n", f)
	}
	fmt.Fprintln(stdout, "A file that changes has to be trusted again (run this command again).")
	return nil
}

func describeRestricted(sections []string) string {
	if len(sections) == 0 {
		return " (nothing that needs trust)"
	}
	return " (holds: " + strings.Join(sections, ", ") + ")"
}

func printTrustStatus(dir string, stdout io.Writer) error {
	files := config.ProjectConfigStatus(dir)
	if len(files) == 0 {
		fmt.Fprintln(stdout, "This project has no configuration file (.seshat.json, .seshat/seshat.json).")
		return nil
	}
	for _, f := range files {
		state := "not trusted"
		switch {
		case len(f.Restricted) == 0:
			state = "nothing that needs trust"
		case f.Trusted:
			state = "trusted"
		}
		fmt.Fprintf(stdout, "%-24s %s%s\n", state, f.Path, describeRestricted(f.Restricted))
	}
	return nil
}
