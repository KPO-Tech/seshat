package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// shellSubstRe matches $(...) in config values.
var shellSubstRe = regexp.MustCompile(`\$\(([^)]+)\)`)

// ExpandShellValues walks a Config and evaluates any field value of the form
// "$(command args)" by running the command in a shell. This mirrors crush's
// config shell-eval feature. The expansion runs at config load time and is
// memoized by storing the result back into the config.
//
// Only string fields that contain "$(" are processed. Failed expansions leave
// the original string intact and log a warning — they don't abort startup.
func ExpandShellValues(cfg *Config) {
	cfg.APIKey = expandField(cfg.APIKey)
	cfg.DBPath = expandField(cfg.DBPath)
	cfg.SessionDBPath = expandField(cfg.SessionDBPath)
	cfg.ProviderBaseURL = expandField(cfg.ProviderBaseURL)
	cfg.EmbedderAPIKey = expandField(cfg.EmbedderAPIKey)
	cfg.S3AccessKeyID = expandField(cfg.S3AccessKeyID)
	cfg.S3SecretAccessKey = expandField(cfg.S3SecretAccessKey)
	cfg.DBDSN = expandField(cfg.DBDSN)
	cfg.PgVectorDSN = expandField(cfg.PgVectorDSN)
	cfg.OpenSearchPassword = expandField(cfg.OpenSearchPassword)
	cfg.OpenSearchAPIKey = expandField(cfg.OpenSearchAPIKey)
	cfg.ChromaAPIKey = expandField(cfg.ChromaAPIKey)
	cfg.TavilyAPIKey = expandField(cfg.TavilyAPIKey)
	cfg.ExaAPIKey = expandField(cfg.ExaAPIKey)
	cfg.JinaAPIKey = expandField(cfg.JinaAPIKey)
}

// expandField evaluates $(...) substitutions within s.
// The string is returned unchanged if no substitutions are found.
func expandField(s string) string {
	if !strings.Contains(s, "$(") {
		return s
	}
	return shellSubstRe.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the command between $( and ).
		inner := match[2 : len(match)-1]
		result, err := runShell(inner)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seshat: shell eval failed for %q: %v\n", inner, err)
			return match // leave original on failure
		}
		return result
	})
}

// runShell executes cmd in a POSIX shell and returns trimmed stdout.
func runShell(cmd string) (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	var out bytes.Buffer
	c := exec.Command(shell, "-c", cmd)
	c.Stdout = &out
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
