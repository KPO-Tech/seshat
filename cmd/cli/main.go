package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/EngineerProjects/seshat/pkg/runtimepath"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	// Pin the CLI runtime root to the platform config dir (seshat-cli),
	// isolated from the seshat-product backend (seshat). SESHAT_RUNTIME_ROOT takes precedence.
	if os.Getenv(runtimepath.EnvRuntimeRoot) == "" {
		os.Setenv(runtimepath.EnvRuntimeRoot, runtimepath.DefaultConfigDir("seshat-cli"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
