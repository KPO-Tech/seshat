package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// A project is not always the user's own: a repository that was just cloned can carry a .seshat.json, or a .seshat/seshat.json, and
// those files are merged into the configuration. Some of what they say runs something or sends something somewhere: an MCP server
// in stdio mode is a command line started when Seshat opens, a hook is a shell command, an LSP server too, a provider has a base URL
// and a key (and a value written $(...) is evaluated by a shell), a list of tools to allow without asking removes the questions, and
// extra context paths can take any file of the machine to the model provider.
//
// So the configuration files of a project are read with those sections removed, unless the user trusted them: `seshat trust` in the
// project records, for each of its configuration files, the hash of what is in it now. A file that changes has to be trusted again,
// as a changed file may say something else. The global configuration (in the runtime root) is the user's own and is never restricted.

// restrictedSections are the top-level keys that an untrusted project configuration may not set.
var restrictedSections = []string{
	"mcp", "mcpServers", "lsp", "hooks", "permissions", "providers", "image_generation", "text_to_speech", "speech_to_text",
}

// restrictedPaths are keys inside sections that an untrusted project configuration may keep otherwise.
var restrictedPaths = []string{"options.context_paths", "options.skills_paths"}

const trustFileName = "trusted_projects.json"

// trustStore maps the path of a project configuration file to the SHA-256 of the content the user trusted.
type trustStore struct {
	Files map[string]string `json:"files"`
}

var (
	trustMu       sync.Mutex
	noticesMu     sync.Mutex
	projectNotice = map[string][]string{} // file -> the sections that were ignored
)

func trustStorePath() string {
	return filepath.Join(filepath.Dir(GlobalConfigData()), trustFileName)
}

func trustKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	key := filepath.ToSlash(filepath.Clean(abs))
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func contentSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadTrustStore() trustStore {
	store := trustStore{Files: map[string]string{}}
	data, err := os.ReadFile(trustStorePath())
	if err != nil {
		return store
	}
	if err := json.Unmarshal(data, &store); err != nil || store.Files == nil {
		return trustStore{Files: map[string]string{}}
	}
	return store
}

func saveTrustStore(store trustStore) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trustStorePath()), 0o700); err != nil {
		return err
	}
	return atomicWriteFile(trustStorePath(), data, 0o600)
}

// isTrustedProjectFile reports whether the user trusted this file with exactly this content.
func isTrustedProjectFile(path string, data []byte) bool {
	trustMu.Lock()
	defer trustMu.Unlock()
	return loadTrustStore().Files[trustKey(path)] == contentSum(data)
}

// IsTrustedProjectFile is isTrustedProjectFile, exported for hosts outside this package that need
// the same hash-based trust check on a single file: the engine's project instruction files
// (SESHAT.md, AGENTS.md, .seshat/instructions.md - see instructionFileNames) use this directly,
// since unlike the JSON configuration files above, their entire content is either trusted or not -
// there is no restrictedSections notion for free-form prose. Wire this into
// sdk.ClientConfig.TrustProjectInstructions from a host that already uses this trust store (see
// cmd/cli/runtime.go and internal/seshattui/workspace/seshat_workspace.go).
func IsTrustedProjectFile(path string, data []byte) bool {
	return isTrustedProjectFile(path, data)
}

// instructionFileNames are the project instruction files the engine looks for in the working
// directory, in priority order (see internal/engine/project_instructions.go's
// candidateInstructionFiles, which this must stay in sync with). They share this package's trust
// store with the JSON configuration files: one `seshat trust` covers both.
var instructionFileNames = []string{"SESHAT.md", "AGENTS.md", filepath.Join(".seshat", "instructions.md")}

// projectInstructionFiles lists the instruction files of workingDir that exist.
func projectInstructionFiles(workingDir string) []string {
	var files []string
	for _, name := range instructionFileNames {
		path := filepath.Join(workingDir, name)
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}
	return files
}

// restrictedIn lists the restricted sections that a configuration holds.
func restrictedIn(data []byte) []string {
	var found []string
	for _, key := range restrictedSections {
		if gjson.GetBytes(data, key).Exists() {
			found = append(found, key)
		}
	}
	for _, path := range restrictedPaths {
		if gjson.GetBytes(data, path).Exists() {
			found = append(found, path)
		}
	}
	return found
}

// readProjectConfig returns the content of a project configuration file as it may be applied: all of it when the user trusted it,
// without its restricted sections otherwise (and the ignored sections are noted for ProjectConfigNotices).
func readProjectConfig(path string, data []byte) []byte {
	found := restrictedIn(data)
	noticesMu.Lock()
	delete(projectNotice, trustKey(path))
	noticesMu.Unlock()
	if len(found) == 0 || isTrustedProjectFile(path, data) {
		return data
	}
	out := data
	for _, key := range append(append([]string{}, restrictedSections...), restrictedPaths...) {
		if gjson.GetBytes(out, key).Exists() {
			if next, err := sjson.DeleteBytes(out, key); err == nil {
				out = next
			}
		}
	}
	noticesMu.Lock()
	projectNotice[trustKey(path)] = found
	noticesMu.Unlock()
	slog.Warn("project configuration is not trusted: some sections are ignored (run `seshat trust` in the project to apply them)",
		"file", path, "ignored", strings.Join(found, ", "))
	return out
}

// ProjectConfigNotices describes the sections of project configuration files that were ignored because the file is not trusted.
func ProjectConfigNotices() []string {
	noticesMu.Lock()
	defer noticesMu.Unlock()
	var files []string
	for file := range projectNotice {
		files = append(files, file)
	}
	sort.Strings(files)
	var out []string
	for _, file := range files {
		out = append(out, fmt.Sprintf("%s: %s ignored; run `seshat trust` in this project to apply them", file, strings.Join(projectNotice[file], ", ")))
	}
	return out
}

// projectConfigFiles lists the configuration files of the project of workingDir that exist: the ones found from the directory up to
// the git root, and the workspace one.
func projectConfigFiles(workingDir string) []string {
	var files []string
	for _, path := range lookupConfigs(workingDir) {
		if !isGlobalConfigPath(path) {
			files = append(files, path)
		}
	}
	files = append(files, filepath.Join(workingDir, defaultDataDirectory, fmt.Sprintf("%s.json", appName)))
	var existing []string
	seen := map[string]bool{}
	for _, f := range files {
		if _, err := os.Stat(f); err == nil && !seen[trustKey(f)] {
			seen[trustKey(f)] = true
			existing = append(existing, f)
		}
	}
	return existing
}

func isGlobalConfigPath(path string) bool {
	key := trustKey(path)
	return key == trustKey(GlobalConfig()) || key == trustKey(GlobalConfigData())
}

// ProjectFileStatus is the state of one project configuration file.
type ProjectFileStatus struct {
	Path       string
	Trusted    bool
	Restricted []string // the restricted sections the file holds
}

// ProjectConfigStatus reports, for each configuration file of the project of workingDir, whether it is trusted and what it holds
// that would be ignored otherwise.
func ProjectConfigStatus(workingDir string) []ProjectFileStatus {
	var out []ProjectFileStatus
	for _, path := range projectConfigFiles(workingDir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, ProjectFileStatus{Path: path, Trusted: isTrustedProjectFile(path, data), Restricted: restrictedIn(data)})
	}
	for _, path := range projectInstructionFiles(workingDir) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		trusted := isTrustedProjectFile(path, data)
		var restricted []string
		if !trusted {
			restricted = []string{"instructions"}
		}
		out = append(out, ProjectFileStatus{Path: path, Trusted: trusted, Restricted: restricted})
	}
	return out
}

// TrustProject records the content of every configuration and instruction file of the project of
// workingDir as trusted, and returns the files.
func TrustProject(workingDir string) ([]string, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	store := loadTrustStore()
	var trusted []string
	for _, path := range append(projectConfigFiles(workingDir), projectInstructionFiles(workingDir)...) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		store.Files[trustKey(path)] = contentSum(data)
		trusted = append(trusted, path)
	}
	if err := saveTrustStore(store); err != nil {
		return nil, err
	}
	return trusted, nil
}

// UntrustProject removes the trust recorded for the configuration and instruction files of the
// project of workingDir, and returns the files.
func UntrustProject(workingDir string) ([]string, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	store := loadTrustStore()
	var removed []string
	for _, path := range append(projectConfigFiles(workingDir), projectInstructionFiles(workingDir)...) {
		if _, ok := store.Files[trustKey(path)]; ok {
			delete(store.Files, trustKey(path))
			removed = append(removed, path)
		}
	}
	if err := saveTrustStore(store); err != nil {
		return nil, err
	}
	return removed, nil
}

// keepTrustAfterOwnWrite is called after Seshat itself wrote a project file at the request of the user (a setting changed in the
// interface): the file stays trusted if it was trusted before, or did not exist, and not otherwise, so that a file that came with
// the repository does not become trusted because the user changed an unrelated setting.
func keepTrustAfterOwnWrite(path string, before []byte, existed bool, after []byte) {
	trustMu.Lock()
	defer trustMu.Unlock()
	store := loadTrustStore()
	if existed && store.Files[trustKey(path)] != contentSum(before) && len(restrictedIn(before)) > 0 {
		return
	}
	store.Files[trustKey(path)] = contentSum(after)
	if err := saveTrustStore(store); err != nil {
		slog.Warn("could not record the trust of a configuration file", "file", path, "error", err)
	}
}
