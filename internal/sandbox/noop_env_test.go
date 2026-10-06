package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The local executor gives a command the environment of the process minus its secrets, as the Docker executor gives it none of it
// (TestDockerExecutorDoesNotLeakHostEnvironment); what the request names still reaches the command.
func TestNoopExecutorDoesNotLeakSecretsOfTheProcess(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-this-must-not-leak")
	t.Setenv("SESHAT_S3_SECRET_ACCESS_KEY", "s3-this-must-not-leak")
	t.Setenv("MY_PLAIN_SETTING", "visible")
	t.Setenv("SESHAT_BASH_ENV_ALLOW", "")
	t.Setenv("SESHAT_BASH_INHERIT_ENV", "")

	res, err := NewNoopExecutor().Run(context.Background(), RunRequest{
		Command: "env",
		Shell:   []string{sh, "-c"},
		Env:     map[string]string{"ASKED_FOR_TOKEN": "given-by-the-caller"},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(res.Stdout, "this-must-not-leak") {
		t.Fatalf("a secret of the process reached the command:\n%s", res.Stdout)
	}
	for _, want := range []string{"MY_PLAIN_SETTING=visible", "ASKED_FOR_TOKEN=given-by-the-caller"} {
		if !strings.Contains(res.Stdout, want) {
			t.Fatalf("%s must reach the command:\n%s", want, res.Stdout)
		}
	}
}
