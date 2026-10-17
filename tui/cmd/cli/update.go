package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/updater"
)

// checkTimeout bounds the release check made at start-up: a slow network must not hold the terminal.
const checkTimeout = 2 * time.Second

// envNoUpdateCheck turns the start-up check and the offer off.
const envNoUpdateCheck = "SESHAT_NO_UPDATE_CHECK"

func updateStatePath() string {
	return updater.StatePath(os.Getenv(runtimepath.EnvRuntimeRoot))
}

// runUpdate is `seshat update [--check] [--yes]`.
func runUpdate(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	checkOnly := fs.Bool("check", false, "only say whether a newer release exists")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if !updater.IsRelease(version) {
		fmt.Fprintf(stdout, "This is a development build (%s): there is no release to compare it with. Build it again, or install a release: https://github.com/%s/releases\n", version, updater.DefaultRepo)
		return nil
	}

	client := updater.NewClient()
	fmt.Fprintf(stdout, "You have %s. Looking for a newer release...\n", version)
	lookCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	release, err := client.Latest(lookCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("cannot find the latest release: %w", err)
	}
	// Whatever the answer, remember it so that the start-up offer is in step with it.
	state := updater.LoadState(updateStatePath())
	state.CheckedAt, state.Latest = time.Now(), release.Tag
	_ = state.Save(updateStatePath())

	if !updater.Newer(version, release.Tag) {
		fmt.Fprintf(stdout, "seshat %s is up to date (latest release: %s).\n", version, release.Tag)
		return nil
	}
	fmt.Fprintf(stdout, "A newer release is available: %s.\n", release.Tag)
	if *checkOnly {
		fmt.Fprintln(stdout, "Run `seshat update` to install it.")
		return nil
	}
	if !*yes {
		ok, err := confirm(stdin, stdout, fmt.Sprintf("Install %s now?", release.Tag))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stdout, "Not installed.")
			return nil
		}
	}
	return installRelease(ctx, client, release, stdout)
}

// installRelease downloads release and replaces the running binary with it.
func installRelease(ctx context.Context, client *updater.Client, release updater.Release, stdout io.Writer) error {
	exe, err := updater.CurrentExecutable()
	if err != nil {
		return fmt.Errorf("cannot find the running binary: %w", err)
	}
	fmt.Fprintf(stdout, "Downloading %s...\n", release.Tag)
	if err := client.Update(ctx, release, exe); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Updated %s to %s. Start seshat again to use it.\n", exe, release.Tag)
	return nil
}

// confirm asks a yes/no question, yes by default.
func confirm(stdin io.Reader, stdout io.Writer, question string) (bool, error) {
	fmt.Fprintf(stdout, "%s [Y/n] ", question)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes", "o", "oui":
		return true, nil
	}
	return false, nil
}

// updateChecksAllowed says whether this run may look for an update and talk to the user about it: a release
// build, run by a person in a terminal, who has not turned it off.
func updateChecksAllowed(stdin io.Reader) bool {
	if !updater.IsRelease(version) {
		return false
	}
	if os.Getenv(envNoUpdateCheck) != "" || os.Getenv("CI") != "" {
		return false
	}
	file, ok := stdin.(*os.File)
	return ok && isatty.IsTerminal(file.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// offerUpdate is called before the terminal interface starts. When a release newer than this binary exists, it
// proposes to install it: yes installs it and asks to start again, "later" stays quiet for a day, "skip" stays
// quiet until a still newer release. The check itself (one request to GitHub, at most 2 seconds, once a day) never
// blocks the launch for longer, and any failure is silent: starting seshat matters more than updating it.
// It returns true when seshat was updated and should not go on with the old binary.
func offerUpdate(ctx context.Context, stdin io.Reader, stdout io.Writer) bool {
	if !updateChecksAllowed(stdin) {
		return false
	}
	path := updateStatePath()
	now := time.Now()
	state := updater.LoadState(path)
	if state.Due(now) {
		client := updater.NewClient()
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		state.Refresh(checkCtx, client, now)
		cancel()
		_ = state.Save(path)
	}
	tag, ok := state.Offer(version, now)
	if !ok {
		return false
	}

	fmt.Fprintf(stdout, "\nseshat %s is available (you have %s).\n", tag, version)
	fmt.Fprint(stdout, "Update now? [Y]es / [l]ater / [s]kip this version: ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes", "o", "oui":
		client := updater.NewClient()
		release, err := client.Latest(ctx)
		if err == nil {
			err = installRelease(ctx, client, release, stdout)
		}
		if err != nil {
			fmt.Fprintf(stdout, "The update failed: %v\nContinuing with %s. Run `seshat update` to try again.\n\n", err, version)
			return false
		}
		return true
	case "s", "skip":
		state.SkippedTag = tag
		_ = state.Save(path)
		fmt.Fprintf(stdout, "Skipped %s. You will be told about a newer release.\n\n", tag)
	default:
		state.SnoozeUntil = now.Add(updater.SnoozeFor)
		_ = state.Save(path)
		fmt.Fprintln(stdout, "Later. `seshat update` installs it whenever you want.")
		fmt.Fprintln(stdout)
	}
	return false
}
