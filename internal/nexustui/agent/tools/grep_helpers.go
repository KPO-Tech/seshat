package tools

// Minimal grep helpers used by references.go to locate symbols in the
// codebase before asking the LSP for authoritative reference positions.
// The full nexus-engine grep tool is registered separately via the SDK.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/fsext"
)

type grepMatch struct {
	path     string
	modTime  time.Time
	lineNum  int
	charNum  int
	lineText string
}

func searchFiles(ctx context.Context, pattern, rootPath, include string, limit int) ([]grepMatch, bool, error) {
	matches, err := searchWithRipgrep(ctx, pattern, rootPath, include)
	if err != nil {
		matches, err = searchFilesWithRegex(pattern, rootPath, include)
		if err != nil {
			return nil, false, err
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}
	return matches, truncated, nil
}

// ─── ripgrep backend ─────────────────────────────────────────────────────────

type ripgrepMatch struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func searchWithRipgrep(ctx context.Context, pattern, path, include string) ([]grepMatch, error) {
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, err
	}
	args := []string{"--json", "--follow", "-e", pattern, path}
	if include != "" {
		args = append(args, "--glob", include)
	}
	cmd := exec.CommandContext(ctx, rgPath, args...)
	for _, ignoreFile := range []string{".gitignore", ".nexusignore"} {
		ignorePath := filepath.Join(path, ignoreFile)
		if _, err := os.Stat(ignorePath); err == nil {
			cmd.Args = append(cmd.Args, "--ignore-file", ignorePath)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []grepMatch{}, nil
		}
		return nil, err
	}
	var matches []grepMatch
	for line := range bytes.SplitSeq(bytes.TrimSpace(output), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var m ripgrepMatch
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if m.Type != "match" {
			continue
		}
		for _, sm := range m.Data.Submatches {
			fi, err := os.Stat(m.Data.Path.Text)
			if err != nil {
				continue
			}
			matches = append(matches, grepMatch{
				path:     m.Data.Path.Text,
				modTime:  fi.ModTime(),
				lineNum:  m.Data.LineNumber,
				charNum:  sm.Start + 1,
				lineText: strings.TrimSpace(m.Data.Lines.Text),
			})
			break
		}
	}
	return matches, nil
}

// ─── regex fallback ───────────────────────────────────────────────────────────

func searchFilesWithRegex(pattern, rootPath, include string) ([]grepMatch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	var includeRE *regexp.Regexp
	if include != "" {
		includeRE, _ = regexp.Compile(globToRegexPattern(include))
	}
	walker := fsext.NewFastGlobWalker(rootPath)
	var matches []grepMatch
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if walker.ShouldSkip(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if walker.ShouldSkip(path) {
			return nil
		}
		base := filepath.Base(path)
		if base != "." && strings.HasPrefix(base, ".") {
			return nil
		}
		if includeRE != nil && !includeRE.MatchString(path) {
			return nil
		}
		lineNum, charNum, lineText, found := fileContainsPattern(path, re)
		if found {
			matches = append(matches, grepMatch{
				path:     path,
				modTime:  info.ModTime(),
				lineNum:  lineNum,
				charNum:  charNum,
				lineText: lineText,
			})
			if len(matches) >= 200 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return matches, err
}

func fileContainsPattern(filePath string, pattern *regexp.Regexp) (lineNum, charNum int, lineText string, found bool) {
	if !isTextFile(filePath) {
		return
	}
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewReader(f)
	n := 0
	for {
		line, err := scanner.ReadString('\n')
		n++
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if loc := pattern.FindStringIndex(line); loc != nil {
			return n, loc[0] + 1, line, true
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
	}
	return
}

func isTextFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	ct := http.DetectContentType(buf[:n])
	return strings.HasPrefix(ct, "text/") ||
		ct == "application/json" ||
		ct == "application/xml" ||
		ct == "application/javascript" ||
		ct == "application/x-sh"
}

func globToRegexPattern(glob string) string {
	p := strings.ReplaceAll(glob, ".", "\\.")
	p = strings.ReplaceAll(p, "*", ".*")
	p = strings.ReplaceAll(p, "?", ".")
	return p
}
