package shared

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// ValidateFilePath
// ---------------------------------------------------------------------------

func TestValidateFilePath_DevicePathsBlocked(t *testing.T) {
	devices := []string{
		"/dev/zero", "/dev/random", "/dev/urandom",
		"/dev/null", "/dev/stdin", "/dev/stdout",
		"/dev/stderr", "/dev/tty", "/dev/full", "/dev/console",
	}
	for _, p := range devices {
		t.Run(p, func(t *testing.T) {
			if err := ValidateFilePath(p, "writing"); err == nil {
				t.Errorf("expected device path %q to be blocked", p)
			}
		})
	}
}

func TestValidateFilePath_TraversalBlocked(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"../../root/.ssh/id_rsa",
		"foo/../../../etc/hosts",
		"/tmp/../etc/shadow",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateFilePath(p, "writing"); err == nil {
				t.Errorf("expected traversal path %q to be blocked", p)
			}
		})
	}
}

func TestValidateFilePath_NormalPathsAllowed(t *testing.T) {
	cases := []string{
		"/tmp/myfile.txt",
		"/home/user/project/main.go",
		"relative/path/file.go",
		"file.txt",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateFilePath(p, "writing"); err != nil {
				t.Errorf("expected %q to be allowed, got: %v", p, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateUNCPathSecurity
// ---------------------------------------------------------------------------

func TestValidateUNCPathSecurity_Blocked(t *testing.T) {
	cases := []string{
		"//server/share/file.txt",
		`\\server\share\file.txt`,
		"//192.168.1.1/c$/windows/system32",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateUNCPathSecurity(p); err == nil {
				t.Errorf("expected UNC path %q to be blocked", p)
			}
		})
	}
}

func TestValidateUNCPathSecurity_Allowed(t *testing.T) {
	cases := []string{"/tmp/file.txt", "/home/user/project/file.go", "file.txt"}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateUNCPathSecurity(p); err != nil {
				t.Errorf("expected %q to be allowed: %v", p, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateSensitivePath
// ---------------------------------------------------------------------------

func TestValidateSensitivePath_EnvFilesBlocked(t *testing.T) {
	cases := []string{
		"/project/.env",
		"/project/.env.local",
		"/project/.env.production",
		"/project/.env.test",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateSensitivePath(p, "writeTool", "KEY=value"); err == nil {
				t.Errorf("expected .env path %q to be blocked", p)
			}
		})
	}
}

func TestValidateSensitivePath_SSHKeysBlocked(t *testing.T) {
	cases := []string{
		"/home/user/.ssh/id_rsa",
		"/home/user/.ssh/id_ed25519",
		"/root/.ssh/authorized_keys",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateSensitivePath(p, "writeTool", "content"); err == nil {
				t.Errorf("expected SSH key path %q to be blocked", p)
			}
		})
	}
}

func TestValidateSensitivePath_SettingsJSON(t *testing.T) {
	path := "/project/.nexus/settings.json"
	// Invalid JSON must be rejected.
	if err := ValidateSensitivePath(path, "writeTool", "{ bad json "); err == nil {
		t.Error("expected invalid JSON settings to be rejected")
	}
	// Valid JSON must pass.
	if err := ValidateSensitivePath(path, "writeTool", `{"autoApprove":true}`); err != nil {
		t.Errorf("expected valid JSON settings to be allowed: %v", err)
	}
}

func TestValidateSensitivePath_LocalSettingsJSON(t *testing.T) {
	path := "/project/.nexus/settings.local.json"
	if err := ValidateSensitivePath(path, "writeTool", "not json"); err == nil {
		t.Error("expected invalid JSON local settings to be rejected")
	}
}

func TestValidateSensitivePath_NormalFileAllowed(t *testing.T) {
	cases := []string{
		"/project/main.go",
		"/project/README.md",
		"/project/config.yaml",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := ValidateSensitivePath(p, "writeTool", "content"); err != nil {
				t.Errorf("expected %q to be allowed: %v", p, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsDangerousFile / IsDangerousDirectory
// ---------------------------------------------------------------------------

func TestIsDangerousFile(t *testing.T) {
	dangerous := []string{
		"/home/user/.gitconfig",
		"/project/.bashrc",
		"/home/user/.zshrc",
		"/home/user/.profile",
		"/home/user/.mcp.json",
		"/home/user/.claude.json",
		"/home/user/.ripgreprc",
		"/project/.gitmodules",
		"/home/user/.bash_profile",
		"/home/user/.zprofile",
	}
	for _, p := range dangerous {
		t.Run(p, func(t *testing.T) {
			if !IsDangerousFile(p) {
				t.Errorf("expected %q to be flagged as dangerous", p)
			}
		})
	}

	safe := []string{
		"/project/main.go", "/project/README.md", "/project/config.yaml",
		"/project/package.json", "/project/.gitignore",
	}
	for _, p := range safe {
		t.Run("safe:"+p, func(t *testing.T) {
			if IsDangerousFile(p) {
				t.Errorf("expected %q to be safe, got flagged", p)
			}
		})
	}
}

func TestIsDangerousDirectory(t *testing.T) {
	dangerous := []string{
		"/project/.git/config",
		"/project/.git/hooks/pre-commit",
		"/project/.vscode/settings.json",
		"/project/.idea/workspace.xml",
		"/project/.claude/settings.json",
		"/project/.openclaude/config.json",
	}
	for _, p := range dangerous {
		t.Run(p, func(t *testing.T) {
			if !IsDangerousDirectory(p) {
				t.Errorf("expected %q to be inside a dangerous directory", p)
			}
		})
	}

	safe := []string{
		"/project/src/main.go",
		"/project/docs/readme.md",
		"/project/.github/workflows/ci.yml",
	}
	for _, p := range safe {
		t.Run("safe:"+p, func(t *testing.T) {
			if IsDangerousDirectory(p) {
				t.Errorf("expected %q to be safe, got flagged", p)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsInWorkingDirectory
// ---------------------------------------------------------------------------

func TestIsInWorkingDirectory(t *testing.T) {
	wd, err := os.MkdirTemp("", "test-wd-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(wd)

	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join(wd, "file.txt"), true},
		{filepath.Join(wd, "sub", "nested", "file.go"), true},
		{wd, true},
		{"/etc/passwd", false},
		{"/tmp/outside.txt", false},
		{filepath.Dir(wd) + "/other_project/file.go", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := IsInWorkingDirectory(tc.path, wd)
			if got != tc.want {
				t.Errorf("IsInWorkingDirectory(%q, %q) = %v, want %v", tc.path, wd, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CheckDangerousRemovalPath
// ---------------------------------------------------------------------------

func TestCheckDangerousRemovalPath_Blocked(t *testing.T) {
	cases := []string{
		"/",
		"/usr",
		"/tmp",
		"/etc",
		"/var",
		"/home/user/*",
		"/project/**",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := CheckDangerousRemovalPath(p, "/project"); err == nil {
				t.Errorf("expected dangerous removal path %q to be blocked", p)
			}
		})
	}
}

func TestCheckDangerousRemovalPath_Allowed(t *testing.T) {
	cases := []string{
		"/project/build",
		"/project/dist/output.js",
		"/tmp/work123/cache/data",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if err := CheckDangerousRemovalPath(p, "/project"); err != nil {
				t.Errorf("expected safe path %q to be allowed: %v", p, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HasSuspiciousPattern (Linux-safe subset)
// ---------------------------------------------------------------------------

func TestHasSuspiciousPattern_Suspicious(t *testing.T) {
	cases := []string{
		`\\?\C:\Windows\System32`, // Windows long path prefix
		`//?/C:/Windows/System32`, // same, forward-slash form
		`\\.\COM1`,                // device name prefix
		"/project/file.",          // trailing dot
		"/project/file ",          // trailing space
		"//server/share/file.txt", // UNC (// form works on Linux too)
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if !HasSuspiciousPattern(p) {
				t.Errorf("expected %q to be flagged as suspicious", p)
			}
		})
	}
}

func TestHasSuspiciousPattern_Safe(t *testing.T) {
	cases := []string{
		"/project/main.go",
		"/project/src/utils.go",
		"file.txt",
		"/home/user/project/readme.md",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			if HasSuspiciousPattern(p) {
				t.Errorf("expected %q to be safe, got flagged as suspicious", p)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Symlink traversal (P2-4)
// ---------------------------------------------------------------------------

// A symlink that resolves outside the workspace root must be detected.
// We create a real symlink in a temp directory to make this concrete.
func TestSymlinkTraversal_DetectedByIsInWorkingDirectory(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	secret := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatalf("create secret file: %v", err)
	}
	// symlink inside workspace pointing outside
	link := filepath.Join(workspace, "escape.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	// Resolve the symlink target and check it is outside the workspace.
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	if IsInWorkingDirectory(resolved, workspace) {
		t.Errorf("symlink target %q should NOT be inside workspace %q", resolved, workspace)
	}
}

// The symlink itself (unresolved) appears to be inside the workspace.
func TestSymlinkTraversal_SymlinkAppearsInside(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	link := filepath.Join(workspace, "link.txt")
	// Link to a path outside (the parent temp dir).
	if err := os.Symlink(filepath.Join(tmp, "outside.txt"), link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	// Unresolved path appears to be inside.
	if !IsInWorkingDirectory(link, workspace) {
		t.Errorf("symlink path should appear inside workspace before resolution")
	}
}

// ---------------------------------------------------------------------------
// Sensitive system path access (P2-4)
// ---------------------------------------------------------------------------

// Direct reads of /etc/passwd, /etc/shadow, and similar credential stores
// must be caught by ValidateFilePath's traversal/device heuristics or by
// the dangerous-path checks.
func TestValidateFilePath_SensitiveSystemPaths(t *testing.T) {
	cases := []struct {
		path    string
		blocked bool
	}{
		// These contain traversal sequences and must be blocked.
		{"../etc/passwd", true},
		{"../../etc/shadow", true},
		{"../../../etc/ssh/ssh_host_rsa_key", true},
		// Absolute paths to sensitive files should reach ValidateSensitivePath,
		// but ValidateFilePath itself blocks device paths and clear traversals.
		{"/dev/null", true},
		{"/dev/stdin", true},
		// Safe project files must not be blocked.
		{"/tmp/test.txt", false},
		{"project/main.go", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			err := ValidateFilePath(tc.path, "reading")
			if tc.blocked && err == nil {
				t.Errorf("expected path %q to be blocked, but it was allowed", tc.path)
			}
			if !tc.blocked && err != nil {
				t.Errorf("expected path %q to be allowed, but got error: %v", tc.path, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Path traversal edge cases (P2-4)
// ---------------------------------------------------------------------------

func TestValidateFilePath_TraversalEdgeCases(t *testing.T) {
	cases := []string{
		// Multiple consecutive dots.
		"foo/../../etc/passwd",
		// Mixed slash styles (filepath.Clean handles these).
		"foo/..//../etc/passwd",
		// Traversal encoded as multiple parent hops.
		"a/b/c/../../../etc/shadow",
		// Traversal with trailing slash.
		"../etc/",
	}
	for _, p := range cases {
		p := p
		t.Run(p, func(t *testing.T) {
			if err := ValidateFilePath(p, "reading"); err == nil {
				t.Errorf("expected traversal path %q to be blocked", p)
			}
		})
	}
}

// IsInWorkingDirectory must block paths that escape via multiple parent hops.
func TestIsInWorkingDirectory_MultipleParentHops(t *testing.T) {
	workspace := "/home/user/project"
	outside := []string{
		"/home/user/project/../../../etc/passwd",
		"/etc/passwd",
		"/home/user",
		"/",
	}
	for _, p := range outside {
		p := p
		t.Run(p, func(t *testing.T) {
			if IsInWorkingDirectory(p, workspace) {
				t.Errorf("expected %q to be outside workspace %q", p, workspace)
			}
		})
	}
}

func TestIsInWorkingDirectory_InsidePaths(t *testing.T) {
	workspace := "/home/user/project"
	inside := []string{
		"/home/user/project/main.go",
		"/home/user/project/src/utils.go",
		"/home/user/project/",
	}
	for _, p := range inside {
		p := p
		t.Run(p, func(t *testing.T) {
			if !IsInWorkingDirectory(p, workspace) {
				t.Errorf("expected %q to be inside workspace %q", p, workspace)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CheckDangerousRemovalPath — additional cases (P2-4)
// ---------------------------------------------------------------------------

func TestCheckDangerousRemovalPath_SensitiveSystemDirs(t *testing.T) {
	blocked := []string{
		"/etc",
		"/usr",
		"/bin",
		"/var",
		"/root",
	}
	for _, p := range blocked {
		p := p
		t.Run(p, func(t *testing.T) {
			if err := CheckDangerousRemovalPath(p, "/project"); err == nil {
				t.Errorf("expected removal of system dir %q to be blocked", p)
			}
		})
	}
}
