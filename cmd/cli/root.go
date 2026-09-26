package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runChat(ctx, nil, stdin, stdout, stderr)
	}

	switch strings.TrimSpace(args[0]) {
	case "chat":
		return runChat(ctx, args[1:], stdin, stdout, stderr)
	case "config":
		return runConfig(args[1:], stdin, stdout, stderr)
	case "companion":
		return runCompanion(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(ctx, args[1:], stdout, stderr)
	case "ps":
		return runBackgroundPS(args[1:], stdout, stderr)
	case "logs":
		return runBackgroundLogs(args[1:], stdout, stderr)
	case "attach":
		return runBackgroundAttach(args[1:], stdout, stderr)
	case "kill":
		return runBackgroundKill(args[1:], stdout, stderr)
	case "run":
		return runOnce(ctx, args[1:], stdin, stdout, stderr)
	case "repomap":
		return runRepoMap(ctx, args[1:], stdout, stderr)
	case "sessions":
		return runSessions(ctx, args[1:], stdin, stdout, stderr)
	case "workflow", "workflows":
		return runWorkflow(ctx, args[1:], stdout, stderr)
	case "memory":
		return runMemory(args[1:], stdout, stderr)
	case "login":
		return runLogin(ctx, args[1:], stdout, stderr)
	case "setup":
		return runSetup(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  seshat chat    [--resume ID] [--continue] [--show-thinking]")
	fmt.Fprintln(out, "                [--model PROVIDER:MODEL] [--permission-mode MODE]")
	fmt.Fprintln(out, "                [--cwd DIR] [--db PATH]")
	fmt.Fprintln(out, "                [--no-tui]   force text mode (skip interactive TUI)")
	fmt.Fprintln(out, "                --resume ID  resume a specific session by ID")
	fmt.Fprintln(out, "                --continue   resume the most recently updated session")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat config  [--provider NAME] [--model MODEL] [--api-key KEY]")
	fmt.Fprintln(out, "                [--region REGION] [--project-id ID] [--base-url URL]")
	fmt.Fprintln(out, "                [--resource ID] [--cwd DIR] [--db PATH]")
	fmt.Fprintln(out, "                [--search]   configure search tool keys only")
	fmt.Fprintln(out, "                [--print]    show current config without editing")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat companion [show|init|enable|disable|name]")
	fmt.Fprintln(out, "                Configure the headless companion profile injected into sessions.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat doctor  [--json]")
	fmt.Fprintln(out, "                Check local config, storage, provider credentials, and helper tools.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat repomap [--tokens N] [--focus PATH] [--focus-symbol NAME] [--cwd DIR]")
	fmt.Fprintln(out, "                Print a compact structural map of the repository.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat run     [--bg] [--name NAME] [--show-thinking] [--model PROVIDER:MODEL]")
	fmt.Fprintln(out, "                [--permission-mode MODE] [--cwd DIR] [--db PATH] \"PROMPT\"")
	fmt.Fprintln(out, "                --bg       run the prompt as a local background child process")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat ps       List local background sessions.")
	fmt.Fprintln(out, "  seshat logs     <id-or-name> [-f] [--stream stdout|stderr]")
	fmt.Fprintln(out, "  seshat attach   <id-or-name>   Show the follow-log command for now.")
	fmt.Fprintln(out, "  seshat kill     <id-or-name>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat sessions [--db PATH] <subcommand>")
	fmt.Fprintln(out, "    list   [--status active|closed] [--n N]")
	fmt.Fprintln(out, "    delete <id...> | --all")
	fmt.Fprintln(out, "    prune  [--older-than N] [--closed]")
	fmt.Fprintln(out, "    info   <id...>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat workflow run <file> [--json] [--max-parallel N]")
	fmt.Fprintln(out, "                Run a static YAML/JSON workflow DAG.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat memory  [--project DIR] [--scope user|project|cross]")
	fmt.Fprintln(out, "                [--action show|set|clear|context] [--key KEY] [--value VALUE]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat login   [--provider openai|anthropic] [--client-id ID]")
	fmt.Fprintln(out, "                Authenticate via browser using your ChatGPT subscription.")
	fmt.Fprintln(out, "                Runs a device-code flow — no API key required.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat setup   [--check] [--python VERSION] [--extras EXTRAS]")
	fmt.Fprintln(out, "                Install uv + the bundled docling-serve document-reader adapter.")
	fmt.Fprintln(out, "                --check  show status without installing.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  seshat version  Print the current version.")
}
