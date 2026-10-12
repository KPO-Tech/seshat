package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/pkg/sdk"
)

func runWorkflow(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: seshat workflow run <file> [--json] [--max-parallel N] [runtime flags]")
	}
	switch args[0] {
	case "run":
		return runWorkflowRun(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown workflow subcommand %q", args[0])
	}
}

func runWorkflowRun(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("workflow run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "")
	maxParallel := flags.Int("max-parallel", 4, "")
	model := flags.String("model", "", "")
	permissionMode := flags.String("permission-mode", "", "")
	cwd := flags.String("cwd", "", "")
	dbPath := flags.String("db", "", "")
	debug := flags.Bool("debug", false, "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: seshat workflow run <file> [--json] [--max-parallel N]")
	}

	def, err := sdk.LoadWorkflowFile(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}
	options, err := loadRuntimeOptions(runtimeOverrides{
		Model:          *model,
		PermissionMode: *permissionMode,
		WorkingDir:     *cwd,
		SQLitePath:     *dbPath,
		Debug:          debug,
	})
	if err != nil {
		return err
	}
	if err := validateProviderSetup(options); err != nil {
		return err
	}
	client, err := newClient(options, nil, nil, nil, nil, nil, nil)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.RunWorkflow(ctx, def, sdk.WorkflowOptions{MaxParallel: *maxParallel})
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	printWorkflowResult(stdout, result)
	if !result.Success {
		return fmt.Errorf("workflow failed")
	}
	return nil
}

func printWorkflowResult(out io.Writer, result sdk.WorkflowResult) {
	status := "ok"
	if !result.Success {
		status = "failed"
	}
	fmt.Fprintf(out, "workflow %s: %s (%s)\n", result.Name, status, result.Duration.Round(10_000_000))
	order := append([]string(nil), result.Order...)
	if len(order) == 0 {
		for id := range result.Results {
			order = append(order, id)
		}
		sort.Strings(order)
	}
	for _, id := range order {
		node := result.Results[id]
		if node.Success {
			fmt.Fprintf(out, "\n[%s] ok\n%s\n", node.ID, strings.TrimSpace(node.Output))
			continue
		}
		fmt.Fprintf(out, "\n[%s] failed: %s\n", node.ID, node.Error)
	}
}
