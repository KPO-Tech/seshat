package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/KPO-Tech/seshat/pkg/runtimepath"
	"gopkg.in/yaml.v3"
)

func DefaultConfigPath() string {
	return runtimepath.Join("", "config.yaml")
}

func Save(config Config) error {
	return SaveAt(DefaultConfigPath(), config)
}

func SaveAt(path string, config Config) error {
	payload, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
	}

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
