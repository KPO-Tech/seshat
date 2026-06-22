package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/seshat/pkg/runtimepath"
)

func runSetup(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	checkOnly := fs.Bool("check", false, "report what is installed without installing anything")
	pythonVer := fs.String("python", "3.11", "Python version for the docling venv")
	extras := fs.String("extras", "", "docling-serve pip extras (e.g. gpu)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root := os.Getenv(runtimepath.EnvRuntimeRoot)
	if root == "" {
		root = runtimepath.DefaultConfigDir("seshat-cli")
	}
	venv := filepath.Join(root, ".venv")

	if *checkOnly {
		return setupCheck(stdout, root, venv)
	}
	return setupInstall(stdout, stderr, root, venv, *pythonVer, *extras)
}

// ── Check ─────────────────────────────────────────────────────────────────────

func setupCheck(out io.Writer, root, venv string) error {
	fmt.Fprintln(out, "Seshat setup status")
	fmt.Fprintln(out, "-------------------")
	fmt.Fprintf(out, "Runtime root : %s\n", root)

	uvPath, uvErr := exec.LookPath("uv")
	if uvErr == nil {
		uvVer, _ := cmdOutput("uv", "--version")
		fmt.Fprintf(out, "uv           : %s  (%s)\n", strings.TrimSpace(uvVer), uvPath)
	} else {
		fmt.Fprintln(out, "uv           : not found")
	}

	doclingBin := filepath.Join(venv, "bin", "docling-serve")
	if _, err := os.Stat(doclingBin); err == nil {
		fmt.Fprintf(out, "docling-serve: %s\n", doclingBin)
	} else {
		fmt.Fprintln(out, "docling-serve: not installed")
	}

	if _, err := os.Stat(root); err == nil {
		fmt.Fprintf(out, "runtime dir  : OK (%s)\n", root)
	} else {
		fmt.Fprintf(out, "runtime dir  : not yet created (created on first run)\n")
	}
	return nil
}

// ── Install ───────────────────────────────────────────────────────────────────

func setupInstall(stdout, stderr io.Writer, root, venv, pythonVer, extras string) error {
	logf := func(msg string, a ...any) { fmt.Fprintf(stdout, "[seshat] "+msg+"\n", a...) }
	fail := func(msg string, a ...any) error { return fmt.Errorf(msg, a...) }

	// 1. Install uv if missing
	if _, err := exec.LookPath("uv"); err != nil {
		logf("uv not found — installing via official installer...")
		if err := installUV(stdout, stderr); err != nil {
			return fail("uv installation failed: %w", err)
		}
		// uv installer places the binary in ~/.local/bin or ~/.cargo/bin
		extra := os.Getenv("HOME") + "/.local/bin" + string(os.PathListSeparator) +
			os.Getenv("HOME") + "/.cargo/bin"
		os.Setenv("PATH", extra+string(os.PathListSeparator)+os.Getenv("PATH"))

		if _, err := exec.LookPath("uv"); err != nil {
			return fail("uv installed but not found on PATH — reopen your terminal and retry")
		}
	}
	uvVer, _ := cmdOutput("uv", "--version")
	logf("uv: %s", strings.TrimSpace(uvVer))

	// 2. Create runtime root
	if err := os.MkdirAll(root, 0o750); err != nil {
		return fail("could not create runtime root %s: %w", root, err)
	}
	logf("Runtime root: %s", root)

	// 3. Create venv
	if _, err := os.Stat(venv); os.IsNotExist(err) {
		logf("Creating Python %s venv at %s ...", pythonVer, venv)
		if err := runCmd(stdout, stderr, "uv", "venv", venv, "--python", pythonVer, "--seed"); err != nil {
			return fail("venv creation failed: %w", err)
		}
	} else {
		logf("Venv already exists at %s", venv)
	}

	// 4. Install docling-serve
	pkg := "docling-serve"
	if extras != "" {
		pkg = "docling-serve[" + extras + "]"
	}
	logf("Installing %s ...", pkg)
	pythonBin := filepath.Join(venv, "bin", "python")
	if err := runCmd(stdout, stderr, "uv", "pip", "install", "--python", pythonBin, pkg); err != nil {
		return fail("docling-serve installation failed: %w", err)
	}

	// 5. Verify
	doclingBin := filepath.Join(venv, "bin", "docling-serve")
	if _, err := os.Stat(doclingBin); err != nil {
		return fail("docling-serve not found at %s after install", doclingBin)
	}

	logf("Setup complete.")
	fmt.Fprintln(stdout, "")
	fmt.Fprintf(stdout, "  Runtime:      %s\n", root)
	fmt.Fprintf(stdout, "  Venv:         %s\n", venv)
	fmt.Fprintf(stdout, "  Docling:      %s\n", doclingBin)
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "  seshat auto-starts docling on launch when the venv is present.")
	fmt.Fprintln(stdout, "  Run: seshat chat")
	fmt.Fprintln(stdout, "")
	return nil
}

// installUV downloads and runs the official uv installer.
func installUV(stdout, stderr io.Writer) error {
	// Try curl first, then wget.
	var cmd *exec.Cmd
	if _, err := exec.LookPath("curl"); err == nil {
		cmd = exec.Command("sh", "-c", "curl -LsSf https://astral.sh/uv/install.sh | sh")
	} else if _, err := exec.LookPath("wget"); err == nil {
		cmd = exec.Command("sh", "-c", "wget -qO- https://astral.sh/uv/install.sh | sh")
	} else {
		return fmt.Errorf("neither curl nor wget found; install uv manually: https://docs.astral.sh/uv/getting-started/installation/")
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// runCmd runs a command streaming output to stdout/stderr.
func runCmd(stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// cmdOutput runs a command and returns its combined output.
func cmdOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
