package builtin

import (
	"context"

	bashTool "github.com/EngineerProjects/nexus-engine/internal/tools/bash"
	editTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/edit"
	fsTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/fs"
	globTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/glob"
	grepTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/grep"
	notebookEditTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/notebook_edit"
	patchTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/patch"
	fileReadTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	readURLTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/read_url"
	writeTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/write"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	agentsTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/agents"
	askUserQuestionTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/ask_user"
	docxTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/docx"
	fimtool "github.com/EngineerProjects/nexus-engine/internal/tools/special/fim"
	goalTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/goal"
	imagegenTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/imagegen"
	lspTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/lsp"
	memoryTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/memory"
	monitorTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/monitor"
	ragTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/rag"
	requestPermissionsTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/request_permissions"
	stttool "github.com/EngineerProjects/nexus-engine/internal/tools/special/stt"
	toolSearchTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/tool_search"
	ttstool "github.com/EngineerProjects/nexus-engine/internal/tools/special/tts"
	worktreeTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/worktree"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
	nexusSkillTool "github.com/EngineerProjects/nexus-engine/internal/tools/system/nexusskill"
	planTool "github.com/EngineerProjects/nexus-engine/internal/tools/system/plan"
	skillTool "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills"
	taskTool "github.com/EngineerProjects/nexus-engine/internal/tools/task"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/tools/web/browser"
	webfetchTool "github.com/EngineerProjects/nexus-engine/internal/tools/web/fetch"
	webSearchTool "github.com/EngineerProjects/nexus-engine/internal/tools/web/search"
)

// RegisterBuiltinTools registers all built-in tools in the registry
func RegisterBuiltinTools(reg *tool.Registry) error {
	return RegisterBuiltinToolsWithConfig(reg, nil)
}

// RegisterBuiltinToolsWithConfig registers all built-in tools in the registry for a host runtime.
func RegisterBuiltinToolsWithConfig(reg *tool.Registry, config *Config) error {
	config = normalizeConfig(config)

	// Connect Skills to MCP manager
	skillTool.SetMCPManager(config.MCPManager)

	askUserConfig := &askUserQuestionTool.Config{
		Timeout:  askUserQuestionTool.DefaultConfig().Timeout,
		PromptFn: config.PromptFn,
	}
	if config.EnablePromptReaderFallback {
		askUserConfig.InputReader = config.InputReader
		if askUserConfig.InputReader == nil {
			askUserConfig.InputReader = askUserQuestionTool.DefaultConfig().InputReader
		}
	}

	skillRuntimeTool := skillTool.NewSkillTool(nil)
	skillRuntimeTool.SetCwd(config.WorkingDir)
	skillRuntimeTool.SetUserID(config.UserID)

	webFetchConfig := webfetchTool.DefaultToolConfig()
	webFetchConfig.BrowserManager = config.BrowserManager
	webFetchConfig.ArtifactStore = config.ArtifactStore

	fileReadConfig := fileReadTool.DefaultToolConfig()
	fileReadConfig.DoclingURL = config.DoclingURL

	readURLConfig := readURLTool.Config{DoclingURL: config.DoclingURL}

	tools := []tool.Tool{
		bashTool.NewTool(bashTool.DefaultToolConfig()),
		bashTool.NewWriteStdinTool(),
		fileReadTool.NewTool(fileReadConfig),
		readURLTool.NewTool(readURLConfig),
		globTool.NewGlobTool(config.WorkingDir),
		grepTool.NewGrepTool(config.WorkingDir),
		writeTool.NewWriteTool(config.WorkingDir),
		editTool.NewEditTool(config.WorkingDir),
		patchTool.NewApplyPatchTool(config.WorkingDir),
		fsTool.NewCreateDirectoryTool(config.WorkingDir),
		fsTool.NewGetMetadataTool(config.WorkingDir),
		fsTool.NewListDirectoryTool(config.WorkingDir),
		fsTool.NewRemoveTool(config.WorkingDir),
		notebookEditTool.NewTool(),
		docxTool.NewDocxTool(config.WorkingDir),
		askUserQuestionTool.NewTool(askUserConfig),
		webfetchTool.NewTool(webFetchConfig),
		webSearchTool.NewTool(),
		browsercore.NewOpenTool(config.BrowserManager),
		browsercore.NewNavigateTool(config.BrowserManager),
		browsercore.NewSnapshotTool(config.BrowserManager),
		browsercore.NewExtractTool(config.BrowserManager),
		browsercore.NewListPagesTool(config.BrowserManager),
		browsercore.NewNetworkListTool(config.BrowserManager),
		browsercore.NewDownloadListTool(config.BrowserManager),
		browsercore.NewSearchContentTool(config.BrowserManager),
		browsercore.NewGetNetworkPolicyTool(config.BrowserManager),
		browsercore.NewSetNetworkPolicyTool(config.BrowserManager),
		browsercore.NewSelectPageTool(config.BrowserManager),
		browsercore.NewClosePageTool(config.BrowserManager),
		browsercore.NewClickTool(config.BrowserManager),
		browsercore.NewTypeTool(config.BrowserManager),
		browsercore.NewPressTool(config.BrowserManager),
		browsercore.NewScrollTool(config.BrowserManager),
		browsercore.NewWaitTool(config.BrowserManager),
		browsercore.NewScreenshotTool(config.BrowserManager),
		lspTool.NewLspTool(config.WorkingDir),
		monitorTool.NewMonitorTool(config.WorkingDir),
		taskTool.NewTaskStopTool(),
		taskTool.NewTaskListTool(),
		taskTool.NewTaskGetTool(),
		taskTool.NewTaskCreateTool(),
		taskTool.NewTaskUpdateTool(),
		planTool.NewEnterPlanModeTool(planTool.DefaultEnterPlanModeConfig("")),
		planTool.NewExitPlanModeTool(planTool.DefaultExitPlanModeConfig("")),
		planTool.NewSubmitPlanTool(buildSubmitPlanConfig(config)),
		toolSearchTool.NewToolSearchTool(reg),
		mcp.NewMCPTool(config.MCPManager),
		skillRuntimeTool,
		nexusSkillTool.NewListTool(),
		nexusSkillTool.NewReadTool(),
		nexusSkillTool.NewValidateTool(),
		worktreeTool.NewEnterWorktreeTool(worktreeTool.DefaultEnterWorktreeConfig()),
		worktreeTool.NewExitWorktreeTool(worktreeTool.DefaultExitWorktreeConfig()),
		ragTool.NewSearchTool(config.RAGService),
		ragTool.NewIngestTool(config.RAGService),
		imagegenTool.NewTool(config.ImageGenerator),
		ttstool.NewTool(config.TTSGenerator),
		stttool.NewTool(config.STTTranscriber),
		fimtool.New(config.FIMCompleter),
		requestPermissionsTool.NewTool(),
		memoryTool.NewCreateEntitiesTool(config.LongTermMemory),
		memoryTool.NewAddObservationsTool(config.LongTermMemory),
		memoryTool.NewSearchNodesTool(config.LongTermMemory),
		memoryTool.NewOpenNodesTool(config.LongTermMemory),
		// Goal system (Phase 6) — store is a process-level singleton, no config needed.
		goalTool.NewCreateGoalTool(),
		goalTool.NewGetGoalTool(),
		goalTool.NewUpdateGoalTool(),
		// Agent control plane (Phase 5) — wait/list/send/close use the default async manager.
		// spawn_agent is registered in sdk/client.go (needs the live engine instance).
		agentsTool.NewWaitAgentTool(),
		agentsTool.NewListAgentsTool(),
		agentsTool.NewSendAgentMessageTool(),
		agentsTool.NewCloseAgentTool(),
		// agentTool (synchronous): registered separately in sdk/client.go after engine creation.
	}

	for _, builtinTool := range tools {
		if builtinTool == nil {
			continue // Skip tools that couldn't be created (e.g., missing required config)
		}
		if err := reg.Register(builtinTool); err != nil {
			return err
		}
	}

	return nil
}

// NewBuiltinRegistry creates a new registry with all built-in tools registered
func NewBuiltinRegistry() (*tool.Registry, error) {
	return NewBuiltinRegistryWithConfig(nil)
}

// NewBuiltinRegistryWithConfig creates a new registry with all built-in tools registered for a host runtime.
func NewBuiltinRegistryWithConfig(config *Config) (*tool.Registry, error) {
	reg := tool.NewRegistry()
	if err := RegisterBuiltinToolsWithConfig(reg, config); err != nil {
		return nil, err
	}
	return reg, nil
}

func buildSubmitPlanConfig(config *Config) *planTool.SubmitPlanConfig {
	if config == nil || config.PlanStore == nil {
		return nil
	}
	return &planTool.SubmitPlanConfig{
		UserID: config.UserID,
		PersistFn: func(ctx context.Context, planID, sessionID, userID, slug, filename, content string) (string, int, error) {
			return config.PlanStore.CreateOrUpdate(ctx, planID, sessionID, userID, slug, filename, content)
		},
	}
}
