package engine

import (
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func generateID() string {
	return uuid.New().String()
}

func currentTime() time.Time {
	return time.Now()
}

// nextTurnID allocates a fresh turn identifier for the next submitted user turn.
func nextTurnID(messages []types.Message) types.TurnID {
	_ = messages
	return types.NewTurnID(generateID())
}

func trimmedStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func resolveWorkingDirectory(configured string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	workingDir, err := os.Getwd()
	if err != nil || workingDir == "" {
		return "."
	}
	return workingDir
}

func sessionToolSurfaceProfile(metadata *types.SessionMetadata) string {
	if metadata != nil && metadata.Additional != nil {
		if profile, ok := metadata.Additional["tool_surface_profile"].(string); ok && strings.TrimSpace(profile) != "" {
			return profile
		}
	}
	return tool.ToolSurfaceProfileMonoRun
}

// migrateSessionMetadata upgrades persisted metadata from old schema versions.
func migrateSessionMetadata(meta *types.SessionMetadata) {
	if meta == nil {
		return
	}
	// V0 → V1: stamp the current schema version on legacy sessions that have none.
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = types.SessionMetadataSchemaVersion
	}
}

// loopConfigFromConfig builds a LoopConfig from engine Config, falling back to
// normalizeLoopConfig defaults for any zero/unset values.
func loopConfigFromConfig(config *Config) *LoopConfig {
	lc := &LoopConfig{
		AutoCompact: config.AutoCompact,
		MaxTurns:    config.MaxTurns,
	}
	if config.MaxIterations > 0 {
		lc.MaxIterations = config.MaxIterations
	}
	if config.TurnTokenBudget > 0 {
		lc.TurnTokenBudget = config.TurnTokenBudget
	}
	if config.BudgetContinuationLimit > 0 {
		lc.BudgetContinuationLimit = config.BudgetContinuationLimit
	}
	if config.ContinuationNudgeLimit > 0 {
		lc.ContinuationNudgeLimit = config.ContinuationNudgeLimit
	}
	if len(config.StopHooks) > 0 {
		lc.StopHooks = config.StopHooks
	}
	return normalizeLoopConfig(lc)
}
