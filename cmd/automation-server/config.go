package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type config struct {
	Port        string // SESHAT_AUTOMATION_PORT       (default: 8090)
	DBPath      string // SESHAT_AUTOMATION_DB          (default: ~/.seshat-automation/data.db)
	APIKey      string // SESHAT_AUTOMATION_API_KEY     — bearer token; empty = no auth
	Model       string // SESHAT_AUTOMATION_MODEL       (default: anthropic:claude-sonnet-4-6)
	SeshatAIURL string // SESHAT_AI_URL                 — base URL of seshat-ai, e.g. http://localhost:8080
	// If set, jobs resolve LLM provider creds from seshat-ai at execution time
	// instead of using the local seshat config / environment.
}

func loadConfig() (*config, error) {
	port := os.Getenv("SESHAT_AUTOMATION_PORT")
	if port == "" {
		port = "8090"
	}

	dbPath := os.Getenv("SESHAT_AUTOMATION_DB")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dir := filepath.Join(home, ".seshat-automation")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		dbPath = filepath.Join(dir, "data.db")
	}

	model := os.Getenv("SESHAT_AUTOMATION_MODEL")
	if model == "" {
		model = "anthropic:claude-sonnet-4-6"
	}

	return &config{
		Port:        port,
		DBPath:      dbPath,
		APIKey:      os.Getenv("SESHAT_AUTOMATION_API_KEY"),
		Model:       model,
		SeshatAIURL: os.Getenv("SESHAT_AI_URL"),
	}, nil
}
