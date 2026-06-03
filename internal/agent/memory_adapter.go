package agent

import (
	"time"

	corememory "github.com/EngineerProjects/nexus-engine/internal/memory"
)

// ---------------------------------------------------------------------------
// Memory System Types and Configuration
// ---------------------------------------------------------------------------

type MemoryType string

const (
	MemoryTypeToolUsage    MemoryType = "tool_usage"
	MemoryTypeConversation MemoryType = "conversation"
	MemoryTypeError        MemoryType = "error"
	MemoryTypeContext      MemoryType = "context"
	MemoryTypeSuccess      MemoryType = "success"
	MemoryTypePreference   MemoryType = "preference"
	MemoryTypeInstruction  MemoryType = "instruction"
	MemoryTypePattern      MemoryType = "pattern"
	MemoryTypeSummary      MemoryType = "summary"
	MemoryTypeKnowledge    MemoryType = "knowledge"
)

type MemoryEntry struct {
	ID           string         `json:"id"`
	Type         MemoryType     `json:"type"`
	Content      string         `json:"content"`
	Context      *MemoryContext `json:"context"`
	Tags         []string       `json:"tags"`
	Importance   float64        `json:"importance"`
	Confidence   float64        `json:"confidence"`
	AccessCount  int            `json:"accessCount"`
	LastAccessed time.Time      `json:"lastAccessed"`
	CreatedAt    time.Time      `json:"createdAt"`
	ExpiresAt    *time.Time     `json:"expiresAt,omitempty"`
}

type MemoryContext struct {
	SessionID    string   `json:"sessionId,omitempty"`
	Task         string   `json:"task,omitempty"`
	Tool         string   `json:"tool,omitempty"`
	Error        string   `json:"error,omitempty"`
	Solution     string   `json:"solution,omitempty"`
	RelatedFiles []string `json:"relatedFiles,omitempty"`
	Intent       string   `json:"intent,omitempty"`
}

type ToolUsageMemory struct {
	ToolName             string             `json:"toolName"`
	SuccessfulParameters []ParameterPattern `json:"successfulParameters"`
	FailedParameters     []ParameterPattern `json:"failedParameters"`
	TypicalUsage         string             `json:"typicalUsage"`
	SuccessRate          float64            `json:"successRate"`
	LastUsed             time.Time          `json:"lastUsed"`
	UsageCount           int                `json:"usageCount"`
}

type ParameterPattern struct {
	Parameters map[string]any `json:"parameters"`
	Frequency  int            `json:"frequency"`
	Success    bool           `json:"success"`
}

type ConversationMemory struct {
	Topic        string            `json:"topic"`
	Participants []string          `json:"participants"`
	KeyPoints    []string          `json:"keyPoints"`
	Questions    []string          `json:"questions"`
	Answers      []string          `json:"answers"`
	TaskProgress map[string]string `json:"taskProgress"`
}

type ErrorMemory struct {
	ErrorType          string            `json:"errorType"`
	ErrorPattern       string            `json:"errorPattern"`
	Frequency          int               `json:"frequency"`
	Solutions          []SolutionPattern `json:"solutions"`
	Context            string            `json:"context"`
	PreventiveMeasures []string          `json:"preventiveMeasures"`
}

type SolutionPattern struct {
	Solution    string   `json:"solution"`
	SuccessRate float64  `json:"successRate"`
	AppliesTo   []string `json:"appliesTo"`
}

type MemoryQuery struct {
	Types         []MemoryType   `json:"types"`
	Content       string         `json:"content"`
	Tags          []string       `json:"tags"`
	Context       *MemoryContext `json:"context"`
	Limit         int            `json:"limit"`
	MinImportance float64        `json:"minImportance"`
	MinConfidence float64        `json:"minConfidence"`
	SessionID     string         `json:"sessionId"`
	Tool          string         `json:"tool"`
	TimeRange     *TimeRange     `json:"timeRange,omitempty"`
}

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type MemorySearchResult struct {
	Entries       []MemoryEntry `json:"entries"`
	Total         int           `json:"total"`
	Query         MemoryQuery   `json:"query"`
	ExecutionTime time.Duration `json:"executionTime"`
}

type MemoryConfig struct {
	MaxEntries           int                   `json:"maxEntries"`
	DefaultTTL           time.Duration         `json:"defaultTTL"`
	LearningEnabled      bool                  `json:"learningEnabled"`
	ImportanceDecay      float64               `json:"importanceDecay"`
	MinImportance        float64               `json:"minImportance"`
	MaxImportance        float64               `json:"maxImportance"`
	EnableSemanticSearch bool                  `json:"enableSemanticSearch"`
	RetentionPolicy      MemoryRetentionPolicy `json:"retentionPolicy"`
}

type MemoryRetentionPolicy struct {
	ToolUsageRetention    time.Duration `json:"toolUsageRetention"`
	ConversationRetention time.Duration `json:"conversationRetention"`
	ErrorRetention        time.Duration `json:"errorRetention"`
	ContextRetention      time.Duration `json:"contextRetention"`
	SuccessRetention      time.Duration `json:"successRetention"`
}

func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		MaxEntries:           10000,
		DefaultTTL:           7 * 24 * time.Hour,
		LearningEnabled:      true,
		ImportanceDecay:      0.1,
		MinImportance:        0.1,
		MaxImportance:        1.0,
		EnableSemanticSearch: true,
		RetentionPolicy: MemoryRetentionPolicy{
			ToolUsageRetention:    30 * 24 * time.Hour,
			ConversationRetention: 7 * 24 * time.Hour,
			ErrorRetention:        30 * 24 * time.Hour,
			ContextRetention:      3 * 24 * time.Hour,
			SuccessRetention:      14 * 24 * time.Hour,
		},
	}
}

type MemoryStats struct {
	TotalEntries      int                `json:"totalEntries"`
	EntriesByType     map[MemoryType]int `json:"entriesByType"`
	AverageImportance float64            `json:"averageImportance"`
	AverageConfidence float64            `json:"averageConfidence"`
	TotalAccessCount  int64              `json:"totalAccessCount"`
	MostAccessed      []string           `json:"mostAccessed"`
	OldestEntry       *time.Time         `json:"oldestEntry,omitempty"`
	NewestEntry       *time.Time         `json:"newestEntry,omitempty"`
}

// ---------------------------------------------------------------------------
// Central Memory Adapter
// ---------------------------------------------------------------------------

type MemoryAdapter struct {
	catalog *corememory.Catalog
	config  *MemoryConfig
}

func NewMemoryAdapter() *MemoryAdapter {
	return NewMemoryAdapterWithConfig(DefaultMemoryConfig())
}

func NewMemoryAdapterWithConfig(config *MemoryConfig) *MemoryAdapter {
	if config == nil {
		config = DefaultMemoryConfig()
	}
	return &MemoryAdapter{
		catalog: corememory.NewCatalogWithConfig(toCoreConfig(config)),
		config:  config,
	}
}

func (ms *MemoryAdapter) Start() {
	ms.catalog.Start()
}

func (ms *MemoryAdapter) Stop() {
	ms.catalog.Stop()
}

func (ms *MemoryAdapter) StoreEntry(entry MemoryEntry) error {
	return ms.catalog.StoreEntry(toCoreEntry(entry))
}

func (ms *MemoryAdapter) Search(query MemoryQuery) (*MemorySearchResult, error) {
	result, err := ms.catalog.Search(toCoreQuery(query))
	if err != nil {
		return nil, err
	}
	return fromCoreSearchResult(result, query), nil
}

func (ms *MemoryAdapter) GetEntry(id string) (*MemoryEntry, error) {
	entry, err := ms.catalog.GetEntry(id)
	if err != nil {
		return nil, err
	}
	converted := fromCoreEntry(*entry)
	return &converted, nil
}

func (ms *MemoryAdapter) DeleteEntry(id string) error {
	return ms.catalog.DeleteEntry(id)
}

func (ms *MemoryAdapter) LearnToolUsage(toolName string, parameters map[string]any, success bool, err error) error {
	return ms.catalog.LearnToolUsage(toolName, parameters, success, err)
}

func (ms *MemoryAdapter) GetToolUsagePatterns(toolName string) (*ToolUsageMemory, error) {
	patterns, err := ms.catalog.GetToolUsagePatterns(toolName)
	if err != nil {
		return nil, err
	}
	return fromCoreToolUsage(*patterns), nil
}

func (ms *MemoryAdapter) Stats() MemoryStats {
	return fromCoreStats(ms.catalog.Stats())
}

func (ms *MemoryAdapter) CleanupExpired() int {
	return ms.catalog.CleanupExpired()
}

func (ms *MemoryAdapter) Export() ([]byte, error) {
	return ms.catalog.Export()
}

func (ms *MemoryAdapter) Import(data []byte) error {
	return ms.catalog.Import(data)
}

func toCoreConfig(config *MemoryConfig) *corememory.MemoryConfig {
	return &corememory.MemoryConfig{
		MaxEntries:           config.MaxEntries,
		DefaultTTL:           config.DefaultTTL,
		LearningEnabled:      config.LearningEnabled,
		ImportanceDecay:      config.ImportanceDecay,
		MinImportance:        config.MinImportance,
		MaxImportance:        config.MaxImportance,
		EnableSemanticSearch: config.EnableSemanticSearch,
		RetentionPolicy: corememory.MemoryRetentionPolicy{
			ToolUsageRetention:    config.RetentionPolicy.ToolUsageRetention,
			ConversationRetention: config.RetentionPolicy.ConversationRetention,
			ErrorRetention:        config.RetentionPolicy.ErrorRetention,
			ContextRetention:      config.RetentionPolicy.ContextRetention,
			SuccessRetention:      config.RetentionPolicy.SuccessRetention,
		},
	}
}

func toCoreEntry(entry MemoryEntry) corememory.Entry {
	converted := corememory.Entry{
		ID:           entry.ID,
		Type:         corememory.MemoryType(entry.Type),
		Content:      entry.Content,
		Tags:         append([]string(nil), entry.Tags...),
		Importance:   entry.Importance,
		Confidence:   entry.Confidence,
		AccessCount:  entry.AccessCount,
		LastAccessed: entry.LastAccessed,
		CreatedAt:    entry.CreatedAt,
		ExpiresAt:    entry.ExpiresAt,
	}
	if entry.Context != nil {
		converted.SessionID = entry.Context.SessionID
		if entry.Context.Tool != "" {
			converted.Metadata = &corememory.EntryMetadata{
				ToolUsed: entry.Context.Tool,
			}
		}
	}
	return converted
}

func fromCoreEntry(entry corememory.Entry) MemoryEntry {
	converted := MemoryEntry{
		ID:           entry.ID,
		Type:         MemoryType(entry.Type),
		Content:      entry.Content,
		Tags:         append([]string(nil), entry.Tags...),
		Importance:   entry.Importance,
		Confidence:   entry.Confidence,
		AccessCount:  entry.AccessCount,
		LastAccessed: entry.LastAccessed,
		CreatedAt:    entry.CreatedAt,
		ExpiresAt:    entry.ExpiresAt,
	}
	if entry.SessionID != "" || (entry.Metadata != nil && entry.Metadata.ToolUsed != "") {
		converted.Context = &MemoryContext{SessionID: entry.SessionID}
		if entry.Metadata != nil {
			converted.Context.Tool = entry.Metadata.ToolUsed
		}
	}
	return converted
}

func toCoreQuery(query MemoryQuery) corememory.MemoryQuery {
	types := make([]corememory.MemoryType, 0, len(query.Types))
	for _, memoryType := range query.Types {
		types = append(types, corememory.MemoryType(memoryType))
	}

	sessionID := query.SessionID
	toolName := query.Tool
	if query.Context != nil {
		if sessionID == "" {
			sessionID = query.Context.SessionID
		}
		if toolName == "" {
			toolName = query.Context.Tool
		}
	}

	var timeRange *corememory.TimeRange
	if query.TimeRange != nil {
		timeRange = &corememory.TimeRange{
			Start: query.TimeRange.Start,
			End:   query.TimeRange.End,
		}
	}

	return corememory.MemoryQuery{
		Types:         types,
		Content:       query.Content,
		Tags:          append([]string(nil), query.Tags...),
		MinImportance: query.MinImportance,
		MinConfidence: query.MinConfidence,
		SessionID:     sessionID,
		Tool:          toolName,
		Limit:         query.Limit,
		TimeRange:     timeRange,
	}
}

func fromCoreSearchResult(result *corememory.MemorySearchResult, originalQuery MemoryQuery) *MemorySearchResult {
	if result == nil {
		return nil
	}
	entries := make([]MemoryEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		entries = append(entries, fromCoreEntry(entry))
	}
	return &MemorySearchResult{
		Entries:       entries,
		Total:         result.Total,
		Query:         originalQuery,
		ExecutionTime: result.ExecutionTime,
	}
}

func fromCoreToolUsage(toolUsage corememory.ToolUsageMemory) *ToolUsageMemory {
	converted := &ToolUsageMemory{
		ToolName:     toolUsage.ToolName,
		TypicalUsage: toolUsage.TypicalUsage,
		SuccessRate:  toolUsage.SuccessRate,
		LastUsed:     toolUsage.LastUsed,
		UsageCount:   toolUsage.UsageCount,
	}
	for _, pattern := range toolUsage.SuccessfulParameters {
		converted.SuccessfulParameters = append(converted.SuccessfulParameters, ParameterPattern{
			Parameters: pattern.Parameters,
			Frequency:  pattern.Frequency,
			Success:    pattern.Success,
		})
	}
	for _, pattern := range toolUsage.FailedParameters {
		converted.FailedParameters = append(converted.FailedParameters, ParameterPattern{
			Parameters: pattern.Parameters,
			Frequency:  pattern.Frequency,
			Success:    pattern.Success,
		})
	}
	return converted
}

func fromCoreStats(stats corememory.MemoryStats) MemoryStats {
	return MemoryStats{
		TotalEntries:      stats.TotalEntries,
		EntriesByType:     convertStatsTypes(stats.EntriesByType),
		AverageImportance: stats.AverageImportance,
		AverageConfidence: stats.AverageConfidence,
		TotalAccessCount:  stats.TotalAccessCount,
		MostAccessed:      append([]string(nil), stats.MostAccessed...),
		OldestEntry:       stats.OldestEntry,
		NewestEntry:       stats.NewestEntry,
	}
}

func convertStatsTypes(types map[corememory.MemoryType]int) map[MemoryType]int {
	converted := make(map[MemoryType]int, len(types))
	for memoryType, count := range types {
		converted[MemoryType(memoryType)] = count
	}
	return converted
}
