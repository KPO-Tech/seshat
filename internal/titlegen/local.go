package titlegen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

const defaultMaxInputRunes = 500

// LocalConfig describes a local, no-server title model. The first supported
// runtime is llama.cpp-compatible GGUF through llama-cli; Hugging Face is used
// as a model-file source, not as a remote inference provider.
type LocalConfig struct {
	Enabled        bool
	Runtime        string
	ExecutablePath string
	ModelPath      string
	HFRepo         string
	HFFile         string
	CacheDir       string
	Args           []string
	Timeout        time.Duration
}

// LocalGenerator generates session titles by running a local model process.
type LocalGenerator struct {
	config LocalConfig
}

func NewLocalGenerator(config LocalConfig) (*LocalGenerator, error) {
	if !config.Enabled {
		return nil, nil
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if strings.TrimSpace(config.Runtime) == "" {
		config.Runtime = "llama.cpp"
	}
	if config.CacheDir == "" {
		config.CacheDir = runtimepath.TitleModelsDir("")
	}
	g := &LocalGenerator{config: config}
	if _, err := g.resolveExecutable(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *LocalGenerator) GenerateTitle(ctx context.Context, firstUserMsg string) (string, error) {
	if g == nil {
		return "", fmt.Errorf("title generator is nil")
	}
	modelPath, err := g.ensureModel(ctx)
	if err != nil {
		return "", err
	}
	executable, err := g.resolveExecutable()
	if err != nil {
		return "", err
	}
	prompt := buildPrompt(firstUserMsg)
	args := g.args(modelPath, prompt)

	runCtx, cancel := context.WithTimeout(ctx, g.config.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return "", runCtx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("run title model: %w: %s", err, strings.TrimSpace(string(out)))
	}
	title := parseTitleOutput(string(out))
	if title == "" {
		return "", fmt.Errorf("title model produced no usable title")
	}
	return title, nil
}

func (g *LocalGenerator) ensureModel(ctx context.Context) (string, error) {
	if path := strings.TrimSpace(g.config.ModelPath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("title model path %q: %w", path, err)
		}
		return path, nil
	}
	repo := strings.Trim(strings.TrimSpace(g.config.HFRepo), "/")
	file := strings.Trim(strings.TrimSpace(g.config.HFFile), "/")
	if repo == "" || file == "" {
		return "", fmt.Errorf("title local model requires model_path or hf_repo + hf_file")
	}
	cachePath := filepath.Join(g.config.CacheDir, "huggingface", filepath.FromSlash(repo), filepath.FromSlash(file))
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, file)
	tmp := cachePath + ".partial"
	if err := downloadFile(ctx, url, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return cachePath, nil
}

func (g *LocalGenerator) resolveExecutable() (string, error) {
	if path := strings.TrimSpace(g.config.ExecutablePath); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		if found, err := exec.LookPath(path); err == nil {
			return found, nil
		}
		return "", fmt.Errorf("title local runtime executable not found: %s", path)
	}
	candidates := []string{"llama-cli", "llama"}
	if runtime.GOOS == "windows" {
		candidates = []string{"llama-cli.exe", "llama.exe", "llama-cli", "llama"}
	}
	for _, candidate := range candidates {
		if found, err := exec.LookPath(candidate); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("title local runtime executable not found; install llama.cpp or set SESHAT_TITLE_LOCAL_EXECUTABLE")
}

func (g *LocalGenerator) args(modelPath, prompt string) []string {
	if len(g.config.Args) > 0 {
		out := make([]string, 0, len(g.config.Args))
		for _, arg := range g.config.Args {
			arg = strings.ReplaceAll(arg, "{model}", modelPath)
			arg = strings.ReplaceAll(arg, "{prompt}", prompt)
			arg = strings.ReplaceAll(arg, "{max_tokens}", "32")
			out = append(out, arg)
		}
		return out
	}
	return []string{
		"-m", modelPath,
		"-p", prompt,
		"-n", "32",
		"--temp", "0",
	}
}

func buildPrompt(firstUserMsg string) string {
	runes := []rune(strings.TrimSpace(firstUserMsg))
	if len(runes) > defaultMaxInputRunes {
		firstUserMsg = string(runes[:defaultMaxInputRunes])
	}
	return "Generate one ultra-short chat title, maximum 6 words, same language as the user. Reply with only the title.\n\nUser message:\n" + firstUserMsg + "\n\nTitle:"
}

func parseTitleOutput(output string) string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, ">") || strings.EqualFold(line, "Title:") {
			continue
		}
		if idx := strings.LastIndex(line, "Title:"); idx >= 0 {
			line = strings.TrimSpace(line[idx+len("Title:"):])
		}
		line = strings.Trim(line, "\"'` \t")
		line = strings.TrimRight(line, ".:;!?")
		fields := strings.Fields(line)
		if len(fields) > 8 {
			line = strings.Join(fields[:8], " ")
		}
		return strings.TrimSpace(line)
	}
	return ""
}

func downloadFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(os.Getenv("HF_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}
