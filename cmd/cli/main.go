package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

func main() {
	// Pin the CLI runtime root to the platform config dir (nexus-cli),
	// isolated from the nexus-product backend (nexus). NEXUS_RUNTIME_ROOT takes precedence.
	if os.Getenv(runtimepath.EnvRuntimeRoot) == "" {
		os.Setenv(runtimepath.EnvRuntimeRoot, runtimepath.DefaultConfigDir("nexus-cli"))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
