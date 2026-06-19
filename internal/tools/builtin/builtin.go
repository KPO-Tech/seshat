package builtin

import (
	"context"
	"fmt"

	agentsTool "github.com/EngineerProjects/nexus-engine/internal/tools/agents"
	bashTool "github.com/EngineerProjects/nexus-engine/internal/tools/bash"
	editTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/edit"
	fsTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/fs"
	globTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/glob"
	grepTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/grep"
	patchTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/patch"
	fileReadTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
	readURLTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/read_url"
	writeTool "github.com/EngineerProjects/nexus-engine/internal/tools/files/write"
	gitTool "github.com/EngineerProjects/nexus-engine/internal/tools/git"
	calculatorTool "github.com/EngineerProjects/nexus-engine/internal/tools/math/calculator"
	financialTool "github.com/EngineerProjects/nexus-engine/internal/tools/math/financial"
	statisticsTool "github.com/EngineerProjects/nexus-engine/internal/tools/math/statistics"
	unitsTool "github.com/EngineerProjects/nexus-engine/internal/tools/math/units"
	multimediaTool "github.com/EngineerProjects/nexus-engine/internal/tools/multimedia"
	notebookTool "github.com/EngineerProjects/nexus-engine/internal/tools/notebook"
	discordTool "github.com/EngineerProjects/nexus-engine/internal/tools/notifications/discord"
	emailTool "github.com/EngineerProjects/nexus-engine/internal/tools/notifications/email"
	slackTool "github.com/EngineerProjects/nexus-engine/internal/tools/notifications/slack"
	telegramTool "github.com/EngineerProjects/nexus-engine/internal/tools/notifications/telegram"
	whatsappTool "github.com/EngineerProjects/nexus-engine/internal/tools/notifications/whatsapp"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	devtoTool "github.com/EngineerProjects/nexus-engine/internal/tools/social/devto"
	hnTool "github.com/EngineerProjects/nexus-engine/internal/tools/social/hackernews"
	askUserQuestionTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/ask_user"
	fimtool "github.com/EngineerProjects/nexus-engine/internal/tools/special/fim"
	goalTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/goal"
	lspTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/lsp"
	memoryTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/memory"
	ragTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/rag"
	requestPermissionsTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/request_permissions"
	toolSearchTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/tool_search"
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
		bashTool.NewJobOutputTool(),
		bashTool.NewJobKillTool(),
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
		notebookTool.NewCreateTool(),
		notebookTool.NewReadTool(),
		notebookTool.NewWriteTool(),
		notebookTool.NewEditTool(),
		notebookTool.NewExecuteTool(),
		notebookTool.NewRunTool(),
		notebookTool.NewKernelTool(),
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
		bashTool.NewMonitorTool(config.WorkingDir),
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
		multimediaTool.NewImageGenTool(config.ImageGenerator),
		multimediaTool.NewTTSTool(config.TTSGenerator),
		multimediaTool.NewSTTTool(config.STTTranscriber),
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

		// VCS — git tools (stubs, IsEnabled=false until implemented via os/exec)
		gitTool.NewStatusTool(),
		gitTool.NewLogTool(),
		gitTool.NewDiffTool(),
		gitTool.NewCommitTool(),
		gitTool.NewBranchTool(),

		// Notifications — messaging platforms (stubs, IsEnabled=false until implemented)
		slackTool.NewSendTool(),
		discordTool.NewSendTool(),
		telegramTool.NewSendTool(),
		emailTool.NewSendTool(),
		whatsappTool.NewSendTool(),

		// Social / community tools (fully implemented, no auth required)
		hnTool.NewSearchTool(),
		hnTool.NewStoriesTool(),
		hnTool.NewItemTool(),
		devtoTool.NewFeedTool(),
		devtoTool.NewArticleTool(),
		devtoTool.NewPublishTool(),
		// Reddit, Twitter, LinkedIn, WhatsApp: stubs disabled until implemented (IsEnabled=false)
	}

	for _, builtinTool := range tools {
		if builtinTool == nil {
			continue // Skip tools that couldn't be created (e.g., missing required config)
		}
		if err := reg.Register(builtinTool); err != nil {
			return err
		}
	}

	// Math tools use factory functions that return (Tool, error).
	mathFactories := []func() (tool.Tool, error){
		calculatorTool.New,
		unitsTool.New,
		statisticsTool.New,
		financialTool.New,
	}
	for _, factory := range mathFactories {
		t, err := factory()
		if err != nil {
			return fmt.Errorf("failed to build math tool: %w", err)
		}
		if err := reg.Register(t); err != nil {
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
