// Package envfilter gives a command that the model asked to run the environment of the process, minus what looks like a secret.
//
// A command the model runs can print its whole environment (`env`, `printenv`, `curl -d "$(env)" ...`), so an instruction hidden
// in a web page, a tool result or a file of the repository can make it send the provider keys, the database password or the
// cloud credentials of the host to anyone. The filter takes out the variables whose name says secret (API_KEY, TOKEN, SECRET,
// PASSWORD, DSN, ...), the families of the providers Seshat talks to, and the ones whose value is plainly a credential (a URL
// with a password in it, a private key, the usual token prefixes). PATH, HOME, LANG, the proxy and build settings and the like
// stay: the command still works.
//
// Two settings, read from the environment of the process:
//
//	SESHAT_BASH_ENV_ALLOW=NAME,PREFIX_*  names (or prefixes ending in *) to keep although they look like secrets
//	SESHAT_BASH_INHERIT_ENV=true         no filtering at all (the former behaviour)
package envfilter

import (
	"os"
	"regexp"
	"strings"
)

// Policy says what Filter keeps.
type Policy struct {
	// Inherit keeps everything.
	Inherit bool
	// Allow lists names (case-insensitive), or prefixes ending in "*", that are kept although they look like secrets.
	Allow []string
}

// PolicyFromEnv reads the policy from SESHAT_BASH_INHERIT_ENV and SESHAT_BASH_ENV_ALLOW.
func PolicyFromEnv() Policy {
	p := Policy{}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SESHAT_BASH_INHERIT_ENV"))) {
	case "1", "true", "yes":
		p.Inherit = true
	}
	for _, name := range strings.Split(os.Getenv("SESHAT_BASH_ENV_ALLOW"), ",") {
		if name = strings.TrimSpace(name); name != "" {
			p.Allow = append(p.Allow, name)
		}
	}
	return p
}

// Environ returns the environment of the process for a command run on behalf of the model.
func Environ() []string {
	return Filter(os.Environ(), PolicyFromEnv())
}

// Filter returns env ("NAME=value" entries) without the variables that look like secrets, unless the policy keeps them.
func Filter(env []string, p Policy) []string {
	if p.Inherit {
		return append([]string(nil), env...)
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		// an entry with no name, or no "=" (Windows keeps "=C:=C:\dir" for the current directory of a drive): not a variable
		if !ok || name == "" || allowed(name, p.Allow) || !IsSensitive(name, value) {
			out = append(out, entry)
		}
	}
	return out
}

func allowed(name string, allow []string) bool {
	for _, a := range allow {
		if prefix, ok := strings.CutSuffix(a, "*"); ok {
			if len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix) {
				return true
			}
		} else if strings.EqualFold(name, a) {
			return true
		}
	}
	return false
}

// secretFamilies are the variables of the providers and services Seshat talks to: all of a family goes, whatever the rest of the
// name says (a base URL or a model name is of no use to a shell command).
var secretFamilies = []string{
	"ANTHROPIC_", "OPENAI_", "AZURE_OPENAI_", "OPENROUTER_", "MISTRAL_", "GEMINI_", "GOOGLE_API", "GOOGLE_APPLICATION_CREDENTIALS",
	"ZHIPUAI_", "ZAI_", "XAI_", "GROQ_", "DEEPSEEK_", "COHERE_", "JINA_", "VOYAGE_", "TAVILY_", "EXA_", "LANGSEARCH_",
	"PERPLEXITY_", "FIREWORKS_", "TOGETHER_", "REPLICATE_", "HUGGINGFACE_", "SERPER_", "BRAVE_", "FIRECRAWL_",
	"AWS_ACCESS", "AWS_SECRET", "AWS_SESSION", "AWS_SECURITY", "AZURE_CLIENT", "AZURE_STORAGE", "AZURE_TENANT",
	"SESHAT_DB_", "SESHAT_PGVECTOR_", "SESHAT_S3_", "SESHAT_ADMIN_", "SESHAT_SLACK_", "SESHAT_OPENSEARCH_",
}

// secretWords are the words of a variable name (split at "_") that make it a secret, as a word or as the end of one
// (PGPASSWORD, GITHUB_TOKEN). TOKENIZERS_PARALLELISM is not one: "TOKENIZERS" does not end with a secret word.
var secretWords = []string{"PASSWORD", "PASSWD", "PASSPHRASE", "SECRET", "SECRETS", "TOKEN", "TOKENS", "APIKEY", "CREDENTIAL", "CREDENTIALS", "BEARER", "DSN"}

// secretPairs are two consecutive words that make a name a secret.
var secretPairs = [][2]string{
	{"API", "KEY"}, {"ACCESS", "KEY"}, {"PRIVATE", "KEY"}, {"SECRET", "KEY"}, {"AUTH", "KEY"}, {"AUTH", "TOKEN"}, {"SESSION", "KEY"},
	{"SIGNING", "KEY"}, {"ENCRYPTION", "KEY"}, {"MASTER", "KEY"}, {"CLIENT", "SECRET"}, {"CONNECTION", "STRING"},
	{"DATABASE", "URL"}, {"DB", "URL"}, {"MYSQL", "PWD"},
}

var (
	urlWithPassword = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@]*:[^/\s@]+@`)
	tokenPrefixes   = regexp.MustCompile(`^(sk-[A-Za-z0-9_-]{16,}|sk_(live|test)_[A-Za-z0-9]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{16,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,}|npm_[A-Za-z0-9]{30,})`)
)

// IsSensitive reports whether a variable looks like a secret, from its name and from its value.
func IsSensitive(name, value string) bool {
	upper := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	for _, family := range secretFamilies {
		if strings.HasPrefix(upper, family) {
			return true
		}
	}
	words := strings.Split(upper, "_")
	for i, word := range words {
		for _, secret := range secretWords {
			if word == secret || (len(word) > len(secret) && strings.HasSuffix(word, secret) && secret != "DSN") {
				return true
			}
		}
		if i+1 < len(words) {
			for _, pair := range secretPairs {
				if word == pair[0] && words[i+1] == pair[1] {
					return true
				}
			}
		}
	}
	if len(words) > 1 && words[len(words)-1] == "KEY" {
		return true
	}
	return valueLooksSecret(value)
}

func valueLooksSecret(value string) bool {
	value = strings.TrimSpace(value)
	if urlWithPassword.MatchString(value) {
		return true
	}
	if len(value) < 16 {
		return false
	}
	return tokenPrefixes.MatchString(value) || (strings.Contains(value, "-----BEGIN") && strings.Contains(value, "PRIVATE KEY"))
}
