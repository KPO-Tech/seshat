package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/EngineerProjects/nexus-engine/internal/monitoring"
	crushcommon "github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	uimodel "github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/model"
	crushws "github.com/EngineerProjects/nexus-engine/internal/nexustui/workspace"
	"github.com/EngineerProjects/nexus-engine/internal/python"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	uv "github.com/charmbracelet/ultraviolet"
)

// runNexusTUI starts the Crush-based TUI. It reuses the same
// SDK configuration and session wiring as the original TUI but delegates
// all rendering to the copied Crush UI layer.
func runNexusTUI(ctx context.Context, options runtimeOptions, initialSessionID string, continueLast bool) error {
	ensureNexusTUIRuntimeRoot()
	if err := validateProviderSetup(options); err != nil {
		return err
	}

	// Auto-start docling-serve when no URL is configured and the managed venv
	// has docling-serve installed. Starts non-blocking; the tool falls back to
	// "not configured" during the few seconds it takes to warm up.
	var doclingManager *python.DoclingManager
	if options.DoclingURL == "" || strings.EqualFold(options.DoclingURL, "auto") {
		if mgr := python.DefaultDoclingManager(); mgr != nil {
			if err := mgr.Start(ctx); err == nil {
				doclingManager = mgr
				options.DoclingURL = mgr.BaseURL()
			}
		}
	}

	options.Monitoring = buildTUIMonitoring()
	if lf := openCLILogFile(); lf != nil {
		log.SetOutput(lf)
	} else {
		log.SetOutput(io.Discard)
	}

	modelStr := ""
	if options.Model.Provider != "" {
		modelStr = string(options.Model.Provider) + ":" + options.Model.Model
	}

	ws := crushws.NewNexusWorkspace(nil, options.WorkingDir, modelStr)
	ws.SetStartupConfig(options.SQLitePath, options.PermissionMode, options.Monitoring)

	client, err := newClient(
		options,
		ws.PromptFn,
		ws.OnProgress,
		ws.OnChunk,
		ws.OnRuntimeEvent,
		ws.OnSessionTitled,
		ws.PlanStore(),
	)
	if err != nil {
		return err
	}
	ws.SetSDKClient(client)

	// Probe env vars, credentials DB, and Ollama in the background so the
	// TUI starts immediately while provider detection completes concurrently.
	go ws.DetectProviders()

	com := crushcommon.DefaultCommon(ws)
	uiModel := uimodel.New(com, initialSessionID, continueLast)

	var env uv.Environ = os.Environ()
	p := tea.NewProgram(
		uiModel,
		tea.WithEnvironment(env),
		tea.WithContext(ctx),
		tea.WithFilter(uimodel.MouseEventFilter),
	)
	go ws.Subscribe(p)

	_, runErr := p.Run()

	ws.Shutdown()
	if doclingManager != nil {
		doclingManager.Stop()
	}
	return runErr
}

func buildTUIMonitoring() *sdk.MonitoringSystem {
	logDir := runtimepath.LogsDir("")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "app.log")
	logger := monitoring.NewLoggerWithConfig(&monitoring.LoggerConfig{
		Level:    monitoring.LogLevelInfo,
		Output:   "file",
		FilePath: logPath,
		Format:   "text",
	})
	return monitoring.NewSystem(logger)
}

func defaultNexusTUIRuntimeRoot() string {
	return runtimepath.DefaultConfigDir("nexus-tui")
}

func ensureNexusTUIRuntimeRoot() {
	if strings.TrimSpace(os.Getenv(runtimepath.EnvRuntimeRoot)) != "" {
		return
	}
	_ = os.Setenv(runtimepath.EnvRuntimeRoot, defaultNexusTUIRuntimeRoot())
}

func openCLILogFile() *os.File {
	config, err := engineconfig.Load()
	if err != nil {
		return nil
	}
	logDir := filepath.Join(config.RuntimeRoot, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(logDir, "cli.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil
	}
	return f
}
