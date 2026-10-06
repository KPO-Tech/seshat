package bash

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The command a model asks for must not see the secrets of the process, and must still see the rest of its environment.
func TestBuildEnvironmentTakesOutSecretsAndKeepsTheRest(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-this-must-not-leak")
	t.Setenv("SESHAT_DB_DSN", "postgres://app:hunter2@db/prod")
	t.Setenv("MY_PLAIN_SETTING", "visible")
	t.Setenv("SESHAT_BASH_ENV_ALLOW", "")
	t.Setenv("SESHAT_BASH_INHERIT_ENV", "")

	env := NewTool(DefaultToolConfig()).buildEnvironment(map[string]string{"ASKED_FOR": "yes", "EXPLICIT_TOKEN": "given-by-the-call"})
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, leaked := range []string{"\nANTHROPIC_API_KEY=", "\nSESHAT_DB_DSN="} {
		if strings.Contains(joined, leaked) {
			t.Errorf("%s reached the command", strings.TrimSpace(leaked))
		}
	}
	for _, kept := range []string{"\nMY_PLAIN_SETTING=visible\n", "\nASKED_FOR=yes\n", "\nEXPLICIT_TOKEN=given-by-the-call\n"} {
		if !strings.Contains(joined, kept) {
			t.Errorf("%s did not reach the command", strings.TrimSpace(kept))
		}
	}
}

// End to end: `env` run through the tool does not print the key.
func TestExecuteCommandDoesNotPrintTheSecretsOfTheProcess(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	t.Setenv("OPENAI_API_KEY", "sk-this-must-not-leak-1234567890")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/leak")
	t.Setenv("MY_PLAIN_SETTING", "visible")
	t.Setenv("SESHAT_BASH_ENV_ALLOW", "")
	t.Setenv("SESHAT_BASH_INHERIT_ENV", "")

	tl := NewTool(DefaultToolConfig())
	result, err := tl.executeCommand(context.Background(), &ExecutionContext{
		Command: "env", Timeout: 30 * time.Second, MaxOutputSize: MaxOutputSize,
	})
	if err != nil {
		t.Fatalf("executeCommand: %v", err)
	}
	out := result.Stdout
	if strings.Contains(out, "this-must-not-leak") || strings.Contains(out, "wJalrXUtnFEMI") || strings.Contains(out, "OPENAI_API_KEY") || strings.Contains(out, "AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("a secret of the process was printed by `env`:\n%s", out)
	}
	if !strings.Contains(out, "MY_PLAIN_SETTING=visible") {
		t.Fatalf("the rest of the environment must still reach the command:\n%s", out)
	}
}
