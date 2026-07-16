package main

import (
	"fmt"
	"os"

	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

func main() {
	if os.Getenv(runtimepath.EnvRuntimeRoot) == "" {
		os.Setenv(runtimepath.EnvRuntimeRoot, runtimepath.DefaultConfigDir("seshat-auto"))
	}

	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
