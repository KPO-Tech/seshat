package workspace

import (
	"context"
	"os"
	"strings"

	tuiconfig "github.com/KPO-Tech/seshat/internal/seshattui/config"
	engineconfig "github.com/KPO-Tech/seshat/pkg/config"
	"github.com/KPO-Tech/seshat/pkg/doctor"
	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

func (w *SeshatWorkspace) DoctorReport(ctx context.Context) doctor.Report {
	cfg := w.Config()
	engineCfg := engineconfig.DefaultConfig()
	engineCfg.RuntimeRoot = runtimepath.ResolveRoot("")
	engineCfg.Cwd = w.WorkingDir()
	engineCfg.SessionDBPath = w.sqlitePath
	engineCfg.DocumentReaderURL = os.Getenv("DOCUMENT_READER_URL")

	if cfg != nil {
		if selected, ok := cfg.Models[tuiconfig.SelectedModelTypeLarge]; ok {
			if selected.Provider != "" && selected.Model != "" {
				engineCfg.Model = selected.Provider + ":" + selected.Model
			}
		}
		if cfg.Options != nil && cfg.Options.Debug {
			engineCfg.Debug = true
		}
	}

	if provider, model := w.splitModel(); engineCfg.Model == "" && provider != "" && model != "" {
		engineCfg.Model = provider + ":" + model
	}

	if provider := strings.TrimSpace(string(engineconfig.ParseModelIdentifier(engineCfg.Model).Provider)); provider != "" {
		if key := w.resolveAPIKey(provider); key != "" {
			engineCfg.APIKey = key
		}
		if baseURL := w.resolveProviderBaseURL(provider); baseURL != "" {
			engineCfg.ProviderBaseURL = baseURL
		}
	}

	return doctor.Run(ctx, doctor.Options{
		Version: "tui",
		Config:  engineCfg,
	})
}
