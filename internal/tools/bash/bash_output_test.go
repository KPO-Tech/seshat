package bash

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The output of a command that ends at once must not be lost. runCommand used to call cmd.Wait while goroutines were still reading the
// pipes, and Wait closes them: a fast command could finish before its output was read, and the model saw an empty result.
func TestFastCommandsKeepTheirOutput(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	tl := NewTool(DefaultToolConfig())
	const runs = 400
	lost := 0
	for i := 0; i < runs; i++ {
		want := fmt.Sprintf("line-%d", i)
		result, err := tl.executeCommand(context.Background(), &ExecutionContext{
			Command: "echo " + want + "; echo err-" + want + " >&2", Timeout: 30 * time.Second, MaxOutputSize: MaxOutputSize,
		})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if strings.TrimSpace(result.Stdout) != want || strings.TrimSpace(result.Stderr) != "err-"+want {
			lost++
			if lost <= 3 {
				t.Errorf("run %d: stdout %q, stderr %q, want %q and %q", i, result.Stdout, result.Stderr, want, "err-"+want)
			}
		}
	}
	if lost > 0 {
		t.Errorf("%d of %d fast commands lost part of their output", lost, runs)
	}
}

// The chunk callback still sees every byte, from both streams, in the order each stream produced it.
func TestOutputChunkCallbackSeesTheWholeOutput(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	tl := NewTool(DefaultToolConfig())
	var out, errOut strings.Builder
	result, err := tl.executeCommand(context.Background(), &ExecutionContext{
		Command: "for i in 1 2 3; do echo out-$i; echo err-$i >&2; done", Timeout: 30 * time.Second, MaxOutputSize: MaxOutputSize,
		OutputChunkCallback: func(chunk, stream string) {
			switch stream {
			case "stdout":
				out.WriteString(chunk)
			case "stderr":
				errOut.WriteString(chunk)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "out-1\nout-2\nout-3\n" || errOut.String() != "err-1\nerr-2\nerr-3\n" {
		t.Errorf("callback saw stdout %q, stderr %q", out.String(), errOut.String())
	}
	if result.Stdout != out.String() || result.Stderr != errOut.String() {
		t.Errorf("result (%q, %q) differs from what the callback saw (%q, %q)", result.Stdout, result.Stderr, out.String(), errOut.String())
	}
}
