package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
	"github.com/KPO-Tech/seshat-runtime/pkg/sdk"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/titlemodel"
)

func titleModelRoot() string { return os.Getenv(runtimepath.EnvRuntimeRoot) }

// runTitleModel is `seshat title-model install|status|enable|disable|remove`.
func runTitleModel(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return titleModelUsage(stderr)
	}
	switch args[0] {
	case "install":
		return titleModelInstall(ctx, args[1:], stdout, stderr)
	case "status":
		return titleModelStatus(stdout)
	case "enable", "disable":
		return titleModelSetEnabled(args[0] == "enable", stdout)
	case "remove":
		if err := titlemodel.Remove(titleModelRoot()); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "The title model was removed. Sessions are named by the chat provider again.")
		return nil
	default:
		return titleModelUsage(stderr)
	}
}

func titleModelUsage(w io.Writer) error {
	fmt.Fprintln(w, "Usage: seshat title-model install [--model ID] | status | enable | disable | remove")
	fmt.Fprintln(w, "Models:")
	for _, m := range titlemodel.Models {
		fmt.Fprintf(w, "  %-14s %s (%d MB) - %s\n", m.ID, m.Label, m.Size>>20, m.Description)
	}
	return fmt.Errorf("a subcommand is required")
}

func titleModelInstall(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("title-model install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelID := fs.String("model", titlemodel.DefaultModelID, "model to install")
	if err := fs.Parse(args); err != nil {
		return err
	}
	model, err := titlemodel.FindModel(*modelID)
	if err != nil {
		return err
	}
	root := titleModelRoot()
	in := titlemodel.NewInstaller(root)
	last := ""
	var lastPrint time.Time
	in.Progress = func(what string, done, total int64) {
		if what != last || time.Since(lastPrint) > time.Second || (total > 0 && done >= total) {
			last, lastPrint = what, time.Now()
			if total > 0 {
				fmt.Fprintf(stdout, "\r  %-24s %3d%%  (%d of %d MB)   ", what, done*100/total, done>>20, total>>20)
			} else {
				fmt.Fprintf(stdout, "\r  %-24s %d MB   ", what, done>>20)
			}
		}
	}
	fmt.Fprintf(stdout, "Installing the local title model (%s, about %d MB plus the llama.cpp server).\n", model.Label, model.Size>>20)
	st, err := in.Install(ctx, model.ID)
	fmt.Fprintln(stdout)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Done. Server: %s\n      Model:  %s\n", st.ServerPath, st.ModelPath)

	// Prove it works now rather than at the first session: start the server, ask for one title, stop it.
	fmt.Fprintln(stdout, "Checking that it runs...")
	started := time.Now()
	generator := titlemodel.NewGenerator(root, st)
	defer generator.Close()
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	title, err := generator.GenerateTitle(checkCtx, "How do I read a CSV file in Go and sum one column?")
	if err != nil {
		fmt.Fprintf(stdout, "The model is installed but did not answer: %v\nSee %s. Sessions keep being named by the chat provider.\n", err, titlemodel.StatePath(root)[:len(titlemodel.StatePath(root))-len("state.json")]+"server.log")
		return nil
	}
	fmt.Fprintf(stdout, "It works: %q in %.1f s (including loading the model).\n", title, time.Since(started).Seconds())
	fmt.Fprintln(stdout, "New sessions are now named on this machine. `seshat title-model disable` turns it off.")
	return nil
}

func titleModelStatus(stdout io.Writer) error {
	st := titlemodel.LoadState(titleModelRoot())
	switch {
	case st.ModelPath == "" && st.ServerPath == "":
		fmt.Fprintln(stdout, "Not installed. `seshat title-model install` downloads the model and the llama.cpp server; sessions are named by the chat provider until then.")
	case !st.Ready():
		fmt.Fprintln(stdout, "Installed, but a file is missing. Run `seshat title-model install` again.")
	case st.Enabled:
		fmt.Fprintf(stdout, "Installed and on: model %s, llama.cpp %s.\n", st.ModelID, st.LlamaTag)
	default:
		fmt.Fprintf(stdout, "Installed and off: model %s. `seshat title-model enable` turns it on.\n", st.ModelID)
	}
	return nil
}

func titleModelSetEnabled(on bool, stdout io.Writer) error {
	root := titleModelRoot()
	st := titlemodel.LoadState(root)
	if !st.Ready() {
		return fmt.Errorf("the title model is not installed: run `seshat title-model install`")
	}
	st.Enabled = on
	if err := titlemodel.SaveState(root, st); err != nil {
		return err
	}
	if on {
		fmt.Fprintln(stdout, "Sessions are named on this machine.")
	} else {
		fmt.Fprintln(stdout, "Sessions are named by the chat provider. The files stay; `enable` turns it back on.")
	}
	return nil
}

// localTitleGenerator is the local title model when the user installed it and left it on, else nil.
func localTitleGenerator() sdk.TitleGenerator {
	if g := titlemodel.Shared(titleModelRoot()); g != nil {
		return g
	}
	return nil
}
