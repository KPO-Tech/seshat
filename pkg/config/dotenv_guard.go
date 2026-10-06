package config

import (
	"fmt"
	"strings"
)

// The .env file of the working directory belongs to whatever directory Seshat was started in, that is, possibly to a repository that
// was just cloned: it must be able to say "use this model" or "this is my API key", and not to run a command or to change where
// Seshat looks for its configuration.
//
// Two things in a .env value would run something: a shell substitution ($(...) or a backquote), which ExpandShellValues evaluates
// at load time, and a variable that the operating system or the tools read to decide what to run or where (PATH, SHELL, LD_PRELOAD,
// GIT_SSH_COMMAND, NODE_OPTIONS, ...) or that Seshat reads to decide where its own configuration is, or how closed its commands are.
// Both are left out, with a warning on the standard error output; they still work from the real environment of the process
// and from the user's own configuration file, which is the user's.

// dotenvBlockedNames are variable names a .env file may not set; dotenvBlockedPrefixes are families of them.
var (
	dotenvBlockedNames = map[string]bool{
		"PATH": true, "PATHEXT": true, "SHELL": true, "COMSPEC": true, "HOME": true, "USERPROFILE": true, "TMPDIR": true, "TEMP": true, "TMP": true,
		"BASH_ENV": true, "ENV": true, "IFS": true, "PROMPT_COMMAND": true, "PS4": true, "SYSTEMROOT": true, "WINDIR": true,
		"NODE_OPTIONS": true, "NODE_PATH": true, "RUBYOPT": true, "RUBYLIB": true, "CLASSPATH": true, "JAVA_TOOL_OPTIONS": true, "_JAVA_OPTIONS": true,
		"EDITOR": true, "VISUAL": true, "PAGER": true, "BROWSER": true, "SSH_ASKPASS": true, "SUDO_ASKPASS": true,
		"SESHAT_RUNTIME_ROOT": true, "SESHAT_BASH_INHERIT_ENV": true, "SESHAT_BASH_ENV_ALLOW": true,
	}
	dotenvBlockedPrefixes = []string{
		"LD_", "DYLD_", "GIT_", "PYTHON", "PERL", "NPM_CONFIG_", "SESHAT_GRPC_ALLOW_", "SESHAT_GRPC_AUTH_", "SESHAT_GRPC_TLS_",
	}
)

// dotenvRefusal says why a .env entry must not be applied, or returns "" when it can be.
func dotenvRefusal(name, value string) string {
	upper := strings.ToUpper(name)
	if dotenvBlockedNames[upper] {
		return fmt.Sprintf("%s decides what is run or where Seshat looks for its configuration", name)
	}
	for _, prefix := range dotenvBlockedPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return fmt.Sprintf("%s* variables change what is run or how Seshat is protected", prefix)
		}
	}
	if strings.Contains(value, "$(") || strings.Contains(value, "`") {
		return "its value contains a shell substitution"
	}
	return ""
}
