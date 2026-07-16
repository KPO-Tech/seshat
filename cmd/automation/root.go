package main

import (
	"fmt"
	"os"

	"github.com/KPO-Tech/seshat/cmd/automation/workflows"
)

func execute(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "tech-watch":
		return workflows.RunTechWatch(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
	return nil
}

func printUsage() {
	fmt.Print(`seshat-auto — Seshat automation workflows

Usage:
  seshat-auto <command> [flags]

Commands:
  tech-watch    Fetch recent tech news and print a digest
  help          Show this help

Run 'seshat-auto <command> --help' for command-specific flags.
`)
}
