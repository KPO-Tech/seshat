package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/pkg/companion"
	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
)

func runCompanion(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runCompanionShow(nil, stdout, stderr)
	}
	switch args[0] {
	case "show", "status":
		return runCompanionShow(args[1:], stdout, stderr)
	case "init":
		return runCompanionInit(args[1:], stdout, stderr)
	case "enable":
		return updateCompanion(stdout, func(p companion.Profile) companion.Profile {
			p.Enabled = true
			return p
		})
	case "disable":
		return updateCompanion(stdout, func(p companion.Profile) companion.Profile {
			p.Enabled = false
			return p
		})
	case "name":
		if len(args) < 2 {
			return fmt.Errorf("usage: seshat companion name <name>")
		}
		name := strings.TrimSpace(strings.Join(args[1:], " "))
		return updateCompanion(stdout, func(p companion.Profile) companion.Profile {
			p.Name = name
			return p
		})
	default:
		return fmt.Errorf("unknown companion subcommand %q", args[0])
	}
}

func runCompanionShow(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("companion show", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	profile, err := companion.Load("")
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(profile)
	}
	state := "disabled"
	if profile.Enabled {
		state = "enabled"
	}
	fmt.Fprintf(stdout, "companion: %s (%s)\n", profile.Name, state)
	if profile.Style != "" {
		fmt.Fprintf(stdout, "style: %s\n", profile.Style)
	}
	if len(profile.Traits) > 0 {
		fmt.Fprintf(stdout, "traits: %s\n", strings.Join(profile.Traits, ", "))
	}
	if profile.Instructions != "" {
		fmt.Fprintf(stdout, "instructions: %s\n", profile.Instructions)
	}
	fmt.Fprintf(stdout, "path: %s\n", runtimepath.CompanionPath(""))
	return nil
}

func runCompanionInit(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("companion init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("name", "", "")
	style := flags.String("style", "", "")
	traits := flags.String("traits", "", "")
	instructions := flags.String("instructions", "", "")
	disabled := flags.Bool("disabled", false, "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	profile := companion.DefaultProfile()
	if strings.TrimSpace(*name) != "" {
		profile.Name = strings.TrimSpace(*name)
	}
	if strings.TrimSpace(*style) != "" {
		profile.Style = strings.TrimSpace(*style)
	}
	if strings.TrimSpace(*traits) != "" {
		profile.Traits = splitCSV(*traits)
	}
	if strings.TrimSpace(*instructions) != "" {
		profile.Instructions = strings.TrimSpace(*instructions)
	}
	if *disabled {
		profile.Enabled = false
	}
	if err := companion.Save("", profile); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Saved companion %s at %s\n", companion.Normalize(profile).Name, runtimepath.CompanionPath(""))
	return nil
}

func updateCompanion(stdout io.Writer, mutate func(companion.Profile) companion.Profile) error {
	profile, err := companion.Load("")
	if err != nil {
		return err
	}
	profile = mutate(profile)
	if err := companion.Save("", profile); err != nil {
		return err
	}
	state := "disabled"
	if profile.Enabled {
		state = "enabled"
	}
	fmt.Fprintf(stdout, "Companion %s is %s.\n", companion.Normalize(profile).Name, state)
	return nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
