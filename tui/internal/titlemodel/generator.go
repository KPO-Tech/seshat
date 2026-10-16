package titlemodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// idleStop is how long the server keeps its memory after the last title: it is stopped, and started again by the
// next one, so that an idle seshat does not hold half a gigabyte.
const idleStop = 10 * time.Minute

// maxInputRunes is how much of the first message the model reads.
const maxInputRunes = 500

// titlePrompt is the same instruction the engine gives the chat provider.
const titlePrompt = `You generate ultra-short session titles.
Rules:
- Maximum 6 words
- No quotes, no punctuation at the end
- Match the language of the user message
- Be specific and descriptive, not generic (avoid words like "Chat", "Question", "Discussion")
- Reply with ONLY the title, nothing else`

// Generator names sessions with the local model. It satisfies the engine's TitleGenerator. The server starts at the
// first title, is shared by every session of the process, and stops when idle or on Close.
type Generator struct {
	root string
	st   State
	// start launches the server; replaced in tests.
	start func(ctx context.Context, st State) (*Server, error)

	mu     sync.Mutex
	server *Server
	timer  *time.Timer
	closed bool
	http   *http.Client
}

var (
	sharedMu  sync.Mutex
	sharedGen *Generator
	sharedFor string
)

// Shared returns the generator of the process for a runtime root, or nil when the model is not installed or is
// turned off. Several clients are built in one run (changing the model builds a new one) and they must share one
// server.
func Shared(root string) *Generator {
	st := LoadState(root)
	if !st.Enabled || !st.Ready() {
		return nil
	}
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedGen == nil || sharedFor != root {
		sharedGen = NewGenerator(root, st)
		sharedFor = root
	}
	return sharedGen
}

// CloseShared stops the shared server, if any.
func CloseShared() {
	sharedMu.Lock()
	gen := sharedGen
	sharedGen = nil
	sharedMu.Unlock()
	if gen != nil {
		gen.Close()
	}
}

// NewGenerator returns a generator for an installation.
func NewGenerator(root string, st State) *Generator {
	g := &Generator{root: root, st: st, http: &http.Client{Timeout: 30 * time.Second}}
	g.start = func(ctx context.Context, st State) (*Server, error) {
		logFile, err := LogFile(root)
		var logWriter io.Writer
		if err == nil {
			logWriter = logFile
			defer logFile.Close()
		}
		// The server keeps writing to the file after this function returns; the descriptor is inherited by the
		// child, so closing ours is fine.
		return StartServer(ctx, st.ServerPath, st.ModelPath, logWriter)
	}
	return g
}

// GenerateTitle returns a short title for the first message of a session.
func (g *Generator) GenerateTitle(ctx context.Context, firstUserMsg string) (string, error) {
	if g == nil {
		return "", errors.New("title model is not installed")
	}
	server, err := g.ensureServer(ctx)
	if err != nil {
		return "", err
	}
	defer g.touch()

	runes := []rune(strings.TrimSpace(firstUserMsg))
	if len(runes) > maxInputRunes {
		runes = runes[:maxInputRunes]
	}
	body, err := json.Marshal(map[string]any{
		"model": "title",
		"messages": []map[string]string{
			{"role": "system", "content": titlePrompt},
			// Two worked examples: a model this small follows the shape of an answer far better than a rule.
			{"role": "user", "content": "How can I make my React component re-render less often?"},
			{"role": "assistant", "content": "Reduce React component re-renders"},
			{"role": "user", "content": "Peux-tu corriger l'erreur de connexion à la base de données dans mon script Python ?"},
			{"role": "assistant", "content": "Erreur de connexion base de données Python"},
			{"role": "user", "content": string(runes)},
		},
		"max_tokens":  32,
		"temperature": 0,
		"stream":      false,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.BaseURL()+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ask the title model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the title model answered %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("read the title: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("the title model returned no choice")
	}
	title := CleanTitle(out.Choices[0].Message.Content)
	if title == "" {
		return "", errors.New("the title model returned an empty title")
	}
	return title, nil
}

// CleanTitle keeps the first line of a model's answer, without quotes, a "Title:" label or closing punctuation, and
// at most 8 words.
func CleanTitle(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i := strings.Index(strings.ToLower(line), "title:"); i == 0 {
			line = strings.TrimSpace(line[len("title:"):])
		}
		line = strings.Trim(line, "\"'`*# \t")
		line = strings.TrimRight(line, ".:;!?")
		fields := strings.Fields(line)
		if len(fields) > 8 {
			fields = fields[:8]
		}
		return strings.Join(fields, " ")
	}
	return ""
}

func (g *Generator) ensureServer(ctx context.Context) (*Server, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, errors.New("title model is closed")
	}
	if g.server != nil && g.server.Alive() {
		if g.timer != nil {
			g.timer.Stop()
		}
		return g.server, nil
	}
	server, err := g.start(ctx, g.st)
	if err != nil {
		return nil, err
	}
	g.server = server
	return server, nil
}

// touch arms the idle timer after a title.
func (g *Generator) touch() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.server == nil {
		return
	}
	if g.timer != nil {
		g.timer.Stop()
	}
	server := g.server
	g.timer = time.AfterFunc(idleStop, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.server == server {
			g.server = nil
		}
		server.Stop()
	})
}

// Close stops the server and refuses further titles.
func (g *Generator) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	if g.timer != nil {
		g.timer.Stop()
	}
	if g.server != nil {
		g.server.Stop()
		g.server = nil
	}
}
