package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/titlemodel"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/updater"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	// Pin the CLI runtime root to the platform config dir (seshat-cli),
	// isolated from the seshat-product backend (seshat). SESHAT_RUNTIME_ROOT takes precedence.
	if os.Getenv(runtimepath.EnvRuntimeRoot) == "" {
		os.Setenv(runtimepath.EnvRuntimeRoot, runtimepath.DefaultConfigDir("seshat-cli"))
	}

	// A replacement leaves the previous binary aside (Windows cannot delete a running one): remove it now.
	if exe, err := updater.CurrentExecutable(); err == nil {
		updater.CleanupOld(exe)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// The local title model, if it was started, is a child process of this run.
	defer titlemodel.CloseShared()

	if err := execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		titlemodel.CloseShared() // os.Exit skips the deferred call
		os.Exit(1)
	}
}
