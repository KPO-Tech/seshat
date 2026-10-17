package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	engineconfig "github.com/KPO-Tech/seshat-runtime/pkg/config"
	"github.com/KPO-Tech/seshat-runtime/pkg/doctor"
)

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the diagnostic report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := engineconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	report := doctor.Run(ctx, doctor.Options{Version: version, Config: cfg})
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		doctor.PrintText(stdout, report)
	}
	if report.HasFailures() {
		return fmt.Errorf("doctor found failing checks")
	}
	return nil
}
