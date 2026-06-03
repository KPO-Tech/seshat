package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

type runtimeOptions struct {
	Model                   sdk.ModelIdentifier
	PermissionMode          sdk.PermissionMode
	WorkingDir              string
	SQLitePath              string
	APIKey                  string
	BrowserRemoteControlURL string
	BrowserExecutablePath   string
	DoclingURL              string
	StorageGCEnabled        bool
	StorageGCInterval       time.Duration
	StorageGCLimit          int
	StorageGCNamespaces     []string
	Debug                   bool
}

type runtimeOverrides struct {
	Model          string
	PermissionMode string
	WorkingDir     string
	SQLitePath     string
	Debug          *bool
}

func loadRuntimeOptions(overrides runtimeOverrides) (runtimeOptions, error) {
	config, err := engineconfig.Load()
	if err != nil {
		return runtimeOptions{}, err
	}

	// Overlay secrets from the credentials DB so that search keys and provider
	// API keys stored there take effect without being in the YAML file.
	if database, dbErr := openCredentialsDB(config); dbErr == nil {
		loadCredsIntoConfig(database, &config)
		_ = database.Close()
		engineconfig.ApplySearchKeys(config)
	}

	if overrides.Debug != nil {
		config.Debug = *overrides.Debug
	}
	if value := strings.TrimSpace(overrides.Model); value != "" {
		config.Model = value
	}
	if value := strings.TrimSpace(overrides.WorkingDir); value != "" {
		config.Cwd = value
	}
	if value := strings.TrimSpace(overrides.SQLitePath); value != "" {
		config.DBPath = value
	}

	permissionMode, err := parsePermissionMode(overrides.PermissionMode)
	if err != nil {
		return runtimeOptions{}, err
	}

	workingDir := strings.TrimSpace(config.Cwd)
	if workingDir == "" || workingDir == "." {
		workingDir, err = os.Getwd()
		if err != nil {
			return runtimeOptions{}, fmt.Errorf("resolve working directory: %w", err)
		}
	}
	workingDir, err = filepath.Abs(workingDir)
	if err != nil {
		return runtimeOptions{}, fmt.Errorf("resolve working directory: %w", err)
	}

	model := resolveModel(config)
	apiKey := engineconfig.ResolveAPIKey(config, model.Provider)

	return runtimeOptions{
		Model:                   model,
		PermissionMode:          permissionMode,
		WorkingDir:              workingDir,
		SQLitePath:              engineconfig.EffectiveSessionDBPath(config),
		APIKey:                  apiKey,
		BrowserRemoteControlURL: strings.TrimSpace(config.BrowserRemoteControlURL),
		BrowserExecutablePath:   strings.TrimSpace(config.BrowserExecutablePath),
		DoclingURL:              strings.TrimSpace(config.DoclingURL),
		StorageGCEnabled:        config.StorageGCEnabled,
		StorageGCInterval:       parseDurationOrDefault(config.StorageGCInterval, time.Hour),
		StorageGCLimit:          config.StorageGCLimit,
		StorageGCNamespaces:     splitCommaList(config.StorageGCNamespaces),
		Debug:                   config.Debug,
	}, nil
}

func newClient(
	options runtimeOptions,
	promptFn sdk.PromptFn,
	progressFn func(sdk.ToolProgress),
	chunkFn func(sdk.ResponseChunk),
) (*sdk.Client, error) {
	client, err := sdk.NewClient(&sdk.ClientConfig{
		APIKey:                  options.APIKey,
		Model:                   options.Model,
		PermissionMode:          options.PermissionMode,
		AutoCompact:             true,
		PersistSessions:         true,
		SessionSQLitePath:       options.SQLitePath,
		PromptFn:                promptFn,
		ProgressFn:              progressFn,
		ResponseChunkFn:         chunkFn,
		WorkingDir:              options.WorkingDir,
		BrowserRemoteControlURL: options.BrowserRemoteControlURL,
		BrowserExecutablePath:   options.BrowserExecutablePath,
		DoclingURL:              options.DoclingURL,
		StorageGCEnabled:        options.StorageGCEnabled,
		StorageGCInterval:       options.StorageGCInterval,
		StorageGCLimit:          options.StorageGCLimit,
		StorageGCNamespaces:     options.StorageGCNamespaces,
	})
	if err != nil {
		return nil, fmt.Errorf("create SDK client: %w", err)
	}
	return client, nil
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parsePermissionMode(raw string) (sdk.PermissionMode, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return sdk.PermissionModeOnRequest, nil
	}
	if strings.EqualFold(value, "plan") {
		return "", fmt.Errorf("unsupported permission mode %q: plan is now an execution mode, not a permission mode", raw)
	}

	switch sdk.PermissionMode(strings.ToLower(value)) {
	case sdk.PermissionModeOnRequest:
		return sdk.PermissionModeOnRequest, nil
	case sdk.PermissionModeAuto:
		return sdk.PermissionModeAuto, nil
	case sdk.PermissionMode("acceptedits"):
		return sdk.PermissionMode("acceptEdits"), nil
	case sdk.PermissionModeBypass:
		return sdk.PermissionModeBypass, nil
	case sdk.PermissionModeNever:
		return sdk.PermissionModeNever, nil
	default:
		return "", fmt.Errorf("unsupported permission mode %q", raw)
	}
}

func resolveModel(config engineconfig.Config) sdk.ModelIdentifier {
	raw := strings.TrimSpace(config.Model)
	model := engineconfig.ParseModelIdentifier(raw)
	if engineconfig.HasExplicitProviderPrefix(raw) {
		return model
	}

	provider := engineconfig.DetectProviderFromModel(raw)
	if provider == "" {
		_, provider = engineconfig.EffectiveAPIKeyAndProvider(config)
	}
	if provider == "" {
		provider = model.Provider
	}
	model.Provider = provider
	return model
}
