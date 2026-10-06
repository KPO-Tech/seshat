package envfilter

import (
	"slices"
	"strings"
	"testing"
)

func TestIsSensitiveByName(t *testing.T) {
	t.Parallel()
	secret := []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENROUTER_API_KEY", "MISTRAL_API_KEY", "ZHIPUAI_API_KEY", "TAVILY_API_KEY",
		"GOOGLE_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS", "RAG_COHERE_RERANKING_API_KEY", "RAG_JINA_API_KEY", "LANGSEARCH_API_KEY",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "SESHAT_S3_SECRET_ACCESS_KEY", "SESHAT_S3_ACCESS_KEY_ID",
		"SESHAT_DB_DSN", "SESHAT_PGVECTOR_DSN", "SESHAT_ADMIN_PASSWORD", "SESHAT_SLACK_BOT_TOKEN", "SESHAT_SLACK_SIGNING_SECRET",
		"GITHUB_TOKEN", "GH_TOKEN", "NPM_TOKEN", "HF_TOKEN", "PGPASSWORD", "MYSQL_PWD", "DATABASE_URL", "REDIS_PASSWORD", "DB_PASSWORD",
		"STRIPE_SECRET_KEY", "SECRET_KEY", "DJANGO_SECRET_KEY", "JWT_SECRET", "CLIENT_SECRET", "GPG_PASSPHRASE", "SOME_PRIVATE_KEY",
		"app-api-key", "Api_Key", "MY_SERVICE_CREDENTIALS", "CONNECTION_STRING", "AZURE_STORAGE_CONNECTION_STRING", "SENTRY_DSN",
		"ENCRYPTION_KEY", "SESSION_KEY", "BEARER", "CSRFTOKEN",
	}
	for _, name := range secret {
		if !IsSensitive(name, "x") {
			t.Errorf("%s should be taken out", name)
		}
	}
	harmless := []string{
		"PATH", "HOME", "USER", "USERNAME", "LANG", "LC_ALL", "TERM", "SHELL", "PWD", "OLDPWD", "TMPDIR", "TEMP", "TMP", "EDITOR", "PAGER",
		"SSH_AUTH_SOCK", "XAUTHORITY", "DISPLAY", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "GOPATH", "GOFLAGS", "GOCACHE", "NODE_ENV",
		"NODE_OPTIONS", "PYTHONPATH", "VIRTUAL_ENV", "JAVA_HOME", "CARGO_HOME", "TOKENIZERS_PARALLELISM", "KEYBOARD", "MONKEY", "SystemRoot",
		"ComSpec", "PATHEXT", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "PROGRAMFILES", "SESHAT_MODEL", "SESHAT_RUNTIME_ROOT",
		"SESHAT_BASH_ENV_ALLOW", "CI", "GITHUB_ACTIONS", "AWS_REGION", "AWS_PROFILE", "GOOGLE_CLOUD_PROJECT", "COLORTERM", "XDG_CONFIG_HOME",
	}
	for _, name := range harmless {
		if IsSensitive(name, "x") {
			t.Errorf("%s should be kept", name)
		}
	}
}

func TestIsSensitiveByValue(t *testing.T) {
	t.Parallel()
	secret := map[string]string{
		"SERVICE": "https://user:hunter2@example.com/path", "REDIS": "redis://:pw@host:6379", "REMOTE": "postgres://app:pw@db/prod",
		"A": "sk-abcdefghijklmnopqrstuvwxyz0123", "B": "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "C": "AKIAIOSFODNN7EXAMPLE",
		"D": "github_pat_11ABCDEFG0abcdefghijklmnopqrstuvwxyz", "E": "xoxb-1234567890-abcdefghij", "F": "AIzaSyA-1234567890abcdefghijklmnopqrstu",
		"G": "-----BEGIN RSA PRIVATE KEY-----\nMIIE", "H": "glpat-abcdefghijklmnop1234",
	}
	for name, value := range secret {
		if !IsSensitive(name, value) {
			t.Errorf("%s=%q should be taken out by its value", name, value)
		}
	}
	harmless := map[string]string{
		"URL": "https://example.com/path", "SHORT": "sk-1", "PATHISH": "/usr/local/bin:/usr/bin", "EMAIL": "me@example.com",
		"NOPASS": "https://user@example.com/", "SENTENCE": "a long value that is only a sentence with no secret in it",
		"WINDRIVE": `C:\Users\me\AppData`,
	}
	for name, value := range harmless {
		if IsSensitive(name, value) {
			t.Errorf("%s=%q should be kept", name, value)
		}
	}
}

func TestFilterKeepsWorkingVariablesAndTakesOutSecrets(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/usr/bin", "HOME=/home/me", "ANTHROPIC_API_KEY=sk-ant-xxx", "AWS_SECRET_ACCESS_KEY=abc", "=C:=C:\\dir", "NOEQUALS", "LANG=C"}
	got := Filter(env, Policy{})
	want := []string{"PATH=/usr/bin", "HOME=/home/me", "=C:=C:\\dir", "NOEQUALS", "LANG=C"}
	if !slices.Equal(got, want) {
		t.Fatalf("Filter = %v, want %v", got, want)
	}
}

func TestFilterAllowListAndInherit(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/usr/bin", "GITHUB_TOKEN=ghp_x", "ANTHROPIC_API_KEY=k", "AWS_SECRET_ACCESS_KEY=s", "AWS_SESSION_TOKEN=t"}

	got := Filter(env, Policy{Allow: []string{"github_token", "AWS_*"}})
	for _, name := range []string{"GITHUB_TOKEN=", "AWS_SECRET_ACCESS_KEY=", "AWS_SESSION_TOKEN=", "PATH="} {
		if !hasPrefix(got, name) {
			t.Errorf("%s should be kept by the allow list: %v", name, got)
		}
	}
	if hasPrefix(got, "ANTHROPIC_API_KEY=") {
		t.Errorf("a variable that is not allowed must stay out: %v", got)
	}

	all := Filter(env, Policy{Inherit: true})
	if !slices.Equal(all, env) {
		t.Errorf("Inherit keeps everything: %v", all)
	}
	all[0] = "CHANGED"
	if env[0] == "CHANGED" {
		t.Error("Filter must not alias its input")
	}
}

func hasPrefix(env []string, prefix string) bool {
	return slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, prefix) })
}

func TestEnvironAndPolicyFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-secret")
	t.Setenv("SESHAT_S3_SECRET_ACCESS_KEY", "s3secret")
	t.Setenv("MY_HARMLESS_SETTING", "yes")
	t.Setenv("SESHAT_BASH_INHERIT_ENV", "")
	t.Setenv("SESHAT_BASH_ENV_ALLOW", "")

	got := Environ()
	if hasPrefix(got, "ANTHROPIC_API_KEY=") || hasPrefix(got, "SESHAT_S3_SECRET_ACCESS_KEY=") {
		t.Fatal("the secrets of the process must not reach a command")
	}
	if !hasPrefix(got, "MY_HARMLESS_SETTING=") {
		t.Fatal("a harmless variable must reach the command")
	}

	t.Setenv("SESHAT_BASH_ENV_ALLOW", " ANTHROPIC_API_KEY , SESHAT_S3_* ,")
	if p := PolicyFromEnv(); p.Inherit || !slices.Equal(p.Allow, []string{"ANTHROPIC_API_KEY", "SESHAT_S3_*"}) {
		t.Fatalf("policy = %+v", p)
	}
	if got := Environ(); !hasPrefix(got, "ANTHROPIC_API_KEY=") || !hasPrefix(got, "SESHAT_S3_SECRET_ACCESS_KEY=") {
		t.Fatal("the allow list must bring the variables back")
	}

	t.Setenv("SESHAT_BASH_ENV_ALLOW", "")
	t.Setenv("SESHAT_BASH_INHERIT_ENV", "true")
	if got := Environ(); !hasPrefix(got, "ANTHROPIC_API_KEY=") {
		t.Fatal("SESHAT_BASH_INHERIT_ENV=true restores the former behaviour")
	}
}
