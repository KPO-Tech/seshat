package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Memory Types
// ============================================================================

// MemoryScope represents the scope of memory
type MemoryScope string

const (
	MemoryScopeProject MemoryScope = "project" // Per project (.nexus/)
	MemoryScopeUser    MemoryScope = "user"    // Global user (~/.nexus/)
	MemoryScopeSession MemoryScope = "session" // Current session
)

// MemoryType represents the type of memory
type MemoryType string

const (
	MemoryTypePreference  MemoryType = "preference"  // User preferences
	MemoryTypeInstruction MemoryType = "instruction" // Custom instructions
	MemoryTypePattern     MemoryType = "pattern"     // Learned patterns
	MemoryTypeSummary     MemoryType = "summary"     // Session summary
	MemoryTypeKnowledge   MemoryType = "knowledge"   // Project knowledge

	// Advanced types from agent system
	MemoryTypeToolUsage    MemoryType = "tool_usage"   // Learned patterns from tool usage
	MemoryTypeConversation MemoryType = "conversation" // Conversation context and patterns
	MemoryTypeError        MemoryType = "error"        // Learned error patterns and solutions
	MemoryTypeContext      MemoryType = "context"      // Situational context memory
	MemoryTypeSuccess      MemoryType = "success"      // Successful patterns and approaches
)

// ============================================================================
// Entry: Single memory entry
// ============================================================================

// Entry represents a single memory entry
type Entry struct {
	ID         string         `json:"id"`
	Scope      MemoryScope    `json:"scope"`
	Type       MemoryType     `json:"type"`
	Key        string         `json:"key"` // Unique key for deduplication
	Value      string         `json:"value"`
	Source     string         `json:"source"`     // Where this was learned
	Confidence float64        `json:"confidence"` // 0-1, how confident we are
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Metadata   *EntryMetadata `json:"metadata,omitempty"`

	// Advanced features from agent system
	Content      string     `json:"content"`              // Full content (expanded from Value)
	Tags         []string   `json:"tags"`                 // Categorization tags
	Importance   float64    `json:"importance"`           // 0-1, importance score
	AccessCount  int        `json:"access_count"`         // Access tracking
	LastAccessed time.Time  `json:"last_accessed"`        // Last access time
	ExpiresAt    *time.Time `json:"expires_at,omitempty"` // Optional expiration
	SessionID    string     `json:"session_id,omitempty"` // Optional session context
}

// EntryMetadata contains additional entry metadata
type EntryMetadata struct {
	FilePath    string     `json:"file_path,omitempty"`
	LineNumber  int        `json:"line_number,omitempty"`
	ToolUsed    string     `json:"tool_used,omitempty"`
	Frequency   int        `json:"frequency"` // How many times used
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	SuccessRate float64    `json:"success_rate"` // 0-1
	Tags        []string   `json:"tags,omitempty"`
}

// NewEntry creates a new memory entry
func NewEntry(scope MemoryScope, memType MemoryType, key, value, source string) *Entry {
	now := time.Now()
	return &Entry{
		ID:         uuid.New().String(),
		Scope:      scope,
		Type:       memType,
		Key:        key,
		Value:      value,
		Source:     source,
		Confidence: 0.5,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// ============================================================================
// ProjectMemory: Memory specific to a project
// ============================================================================

// ProjectMemory holds project-specific memory
type ProjectMemory struct {
	ProjectPath string                      `json:"project_path"`
	RootID      string                      `json:"root_id"` // Root dir ID (git repo or hash)
	Entries     map[string]*Entry           `json:"entries"` // Key -> Entry
	ToolUsage   map[string]*ToolUsageMemory `json:"tool_usage,omitempty"`
	Config      *ProjectConfig              `json:"config"`
	LoadedAt    time.Time                   `json:"loaded_at"`
}

// ProjectConfig holds project memory configuration
type ProjectConfig struct {
	Enabled           bool     `json:"enabled"`
	AutoLearnPatterns bool     `json:"auto_learn_patterns"`
	MaxEntries        int      `json:"max_entries"`
	RetentionDays     int      `json:"retention_days"`
	IncludePatterns   []string `json:"include_patterns,omitempty"`
	ExcludePatterns   []string `json:"exclude_patterns,omitempty"`
}

// DefaultProjectConfig returns default project memory config
func DefaultProjectConfig() *ProjectConfig {
	return &ProjectConfig{
		Enabled:           true,
		AutoLearnPatterns: true,
		MaxEntries:        1000,
		RetentionDays:     30,
	}
}

// NewProjectMemory creates new project memory
func NewProjectMemory(projectPath string) *ProjectMemory {
	return &ProjectMemory{
		ProjectPath: projectPath,
		Entries:     make(map[string]*Entry),
		ToolUsage:   make(map[string]*ToolUsageMemory),
		Config:      DefaultProjectConfig(),
		LoadedAt:    time.Now(),
	}
}

// ============================================================================
// UserMemory: Global user memory
// ============================================================================

// UserMemory holds user-wide memory
type UserMemory struct {
	UserID   string            `json:"user_id"`
	Entries  map[string]*Entry `json:"entries"`
	Config   *UserConfig       `json:"config"`
	LoadedAt time.Time         `json:"loaded_at"`
}

// UserConfig holds user memory configuration
type UserConfig struct {
	Enabled           bool `json:"enabled"`
	AutoLearnPatterns bool `json:"auto_learn_patterns"`
	MaxEntries        int  `json:"max_entries"`
	ShareWithProjects bool `json:"share_with_projects"`
}

// DefaultUserConfig returns default user memory config
func DefaultUserConfig() *UserConfig {
	return &UserConfig{
		Enabled:           true,
		AutoLearnPatterns: true,
		MaxEntries:        5000,
		ShareWithProjects: true,
	}
}

// NewUserMemory creates new user memory
func NewUserMemory(userID string) *UserMemory {
	return &UserMemory{
		UserID:   userID,
		Entries:  make(map[string]*Entry),
		Config:   DefaultUserConfig(),
		LoadedAt: time.Now(),
	}
}

// ============================================================================
// CrossSession: Memory across sessions
// ============================================================================

// CrossSession holds cross-session memory
type CrossSession struct {
	SessionSummaries map[string]*SessionSummary `json:"session_summaries"` // SessionID -> Summary
	GlobalPatterns   map[string]*PatternEntry   `json:"global_patterns"`   // Key -> Pattern
	LoadedAt         time.Time                  `json:"loaded_at"`
}

// SessionSummary is a summary of a past session
type SessionSummary struct {
	SessionID         string    `json:"session_id"`
	ProjectPath       string    `json:"project_path"`
	Summary           string    `json:"summary"`
	ToolsUsed         []string  `json:"tools_used"`
	KeyLearned        []string  `json:"key_learned"`
	ErrorsEncountered []string  `json:"errors_encountered"`
	CompletedAt       time.Time `json:"completed_at"`
}

// PatternEntry represents a learned pattern
type PatternEntry struct {
	Key         string    `json:"key"`
	Pattern     string    `json:"pattern"`
	Description string    `json:"description"`
	Frequency   int       `json:"frequency"`
	SuccessRate float64   `json:"success_rate"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	Examples    []string  `json:"examples"`
}

// NewCrossSession creates new cross-session memory
func NewCrossSession() *CrossSession {
	return &CrossSession{
		SessionSummaries: make(map[string]*SessionSummary),
		GlobalPatterns:   make(map[string]*PatternEntry),
		LoadedAt:         time.Now(),
	}
}

// ============================================================================
// MemoryStore: Interface for memory storage
// ============================================================================

// Store interface for memory persistence
type Store interface {
	LoadProjectMemory(projectPath string) (*ProjectMemory, error)
	SaveProjectMemory(m *ProjectMemory) error
	LoadUserMemory() (*UserMemory, error)
	SaveUserMemory(m *UserMemory) error
	LoadCrossSession() (*CrossSession, error)
	SaveCrossSession(m *CrossSession) error
}

// ============================================================================
// FileStore: File-based memory storage
// ============================================================================

// FileStore implements Store using JSON files
type FileStore struct {
	basePath string
}

// NewFileStore creates a new file store
func NewFileStore(basePath string) (*FileStore, error) {
	if basePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		basePath = filepath.Join(homeDir, ".nexus", "memory")
	}

	// Create directory if needed
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	return &FileStore{basePath: basePath}, nil
}

// LoadProjectMemory loads project-specific memory
func (s *FileStore) LoadProjectMemory(projectPath string) (*ProjectMemory, error) {
	// Generate stable ID for project (git root or hash of path)
	rootID := getProjectRootID(projectPath)

	filePath := filepath.Join(s.basePath, "projects", rootID+".json")

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return NewProjectMemory(projectPath), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project memory: %w", err)
	}

	var mem ProjectMemory
	if err := json.Unmarshal(data, &mem); err != nil {
		return nil, fmt.Errorf("parse project memory: %w", err)
	}

	return &mem, nil
}

// SaveProjectMemory saves project memory
func (s *FileStore) SaveProjectMemory(m *ProjectMemory) error {
	rootID := getProjectRootID(m.ProjectPath)

	// Ensure projects directory
	dir := filepath.Join(s.basePath, "projects")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}

	filePath := filepath.Join(dir, rootID+".json")

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project memory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write project memory: %w", err)
	}

	return nil
}

// LoadUserMemory loads user-wide memory
func (s *FileStore) LoadUserMemory() (*UserMemory, error) {
	filePath := filepath.Join(s.basePath, "user.json")

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return NewUserMemory("default"), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read user memory: %w", err)
	}

	var mem UserMemory
	if err := json.Unmarshal(data, &mem); err != nil {
		return nil, fmt.Errorf("parse user memory: %w", err)
	}

	return &mem, nil
}

// SaveUserMemory saves user memory
func (s *FileStore) SaveUserMemory(m *UserMemory) error {
	filePath := filepath.Join(s.basePath, "user.json")

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal user memory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write user memory: %w", err)
	}

	return nil
}

// LoadCrossSession loads cross-session memory
func (s *FileStore) LoadCrossSession() (*CrossSession, error) {
	filePath := filepath.Join(s.basePath, "cross_session.json")

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return NewCrossSession(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cross-session memory: %w", err)
	}

	var mem CrossSession
	if err := json.Unmarshal(data, &mem); err != nil {
		return nil, fmt.Errorf("parse cross-session memory: %w", err)
	}

	return &mem, nil
}

// SaveCrossSession saves cross-session memory
func (s *FileStore) SaveCrossSession(m *CrossSession) error {
	filePath := filepath.Join(s.basePath, "cross_session.json")

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cross-session memory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write cross-session memory: %w", err)
	}

	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// getProjectRootID generates a stable ID for a project
func getProjectRootID(projectPath string) string {
	// Try to find .git directory
	gitPath := filepath.Join(projectPath, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		// Use git root - check for HEAD
		headPath := filepath.Join(gitPath, "HEAD")
		if data, err := os.ReadFile(headPath); err == nil {
			content := string(data)
			// Check for gitdir reference
			if len(content) > 10 && content[:10] == "gitdir: " {
				// It's a worktree reference, find actual repo
				refPath := strings.TrimSpace(content[9:])
				if filepath.IsAbs(refPath) {
					return filepath.Base(refPath)
				}
				return filepath.Base(filepath.Join(projectPath, refPath))
			}
			// Just use project dir name as identifier
			return filepath.Base(projectPath)
		}
	}

	// Fallback: hash the path
	hash := fnv32(projectPath)
	return fmt.Sprintf("project_%d", hash)
}

func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

// ============================================================================
// Advanced Memory Structures (from agent system)
// ============================================================================

// MemoryQuery represents a query for memory retrieval
type MemoryQuery struct {
	Types         []MemoryType `json:"types"`                // Filter by types
	Content       string       `json:"content"`              // Search in content
	Tags          []string     `json:"tags,omitempty"`       // Filter by tags
	MinImportance float64      `json:"min_importance"`       // Filter by minimum importance
	MinConfidence float64      `json:"min_confidence"`       // Filter by minimum confidence
	SessionID     string       `json:"session_id"`           // Filter by session
	Tool          string       `json:"tool"`                 // Filter by tool name
	Keywords      []string     `json:"keywords"`             // Specific keywords to match
	ExactMatch    bool         `json:"exact_match"`          // Require exact match
	TimeRange     *TimeRange   `json:"time_range,omitempty"` // Filter by time range
	Limit         int          `json:"limit"`                // Maximum results
}

// TimeRange represents a time range for filtering memory entries.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MemorySearchResult represents search results
type MemorySearchResult struct {
	Entries       []Entry       `json:"entries"`       // Found entries
	Total         int           `json:"total"`         // Total matches
	Query         MemoryQuery   `json:"query"`         // Executed query
	ExecutionTime time.Duration `json:"executionTime"` // Search duration
}

// MemoryIndex provides fast searching capabilities
type MemoryIndex struct {
	contentIndex map[string][]int     // content keyword -> entry indices
	tagIndex     map[string][]int     // tag -> entry indices
	typeIndex    map[MemoryType][]int // type -> entry indices
	sessionIndex map[string][]int     // session ID -> entry indices
	toolIndex    map[string][]int     // tool name -> entry indices
}

// ToolUsageMemory tracks usage patterns for a tool
type ToolUsageMemory struct {
	ToolName             string             `json:"tool_name"`
	SuccessfulParameters []ParameterPattern `json:"successful_parameters"`
	FailedParameters     []ParameterPattern `json:"failed_parameters"`
	TypicalUsage         string             `json:"typical_usage"`
	SuccessRate          float64            `json:"success_rate"`
	UsageCount           int                `json:"usage_count"`
	LastUsed             time.Time          `json:"last_used"`
}

// ParameterPattern represents a specific parameter pattern
type ParameterPattern struct {
	Parameters map[string]any `json:"parameters"`
	Frequency  int            `json:"frequency"`
	Success    bool           `json:"success"`
	LastUsed   *time.Time     `json:"last_used,omitempty"`
}

// MemoryConfig configures memory system behavior
type MemoryConfig struct {
	MaxEntries           int                   `json:"max_entries"`
	DefaultTTL           time.Duration         `json:"default_ttl"`
	LearningEnabled      bool                  `json:"learning_enabled"`
	ImportanceDecay      float64               `json:"importance_decay"` // 0-1, per access
	MinImportance        float64               `json:"min_importance"`
	MaxImportance        float64               `json:"max_importance"`
	EnableSemanticSearch bool                  `json:"enable_semantic_search"`
	IndexingEnabled      bool                  `json:"indexing_enabled"`
	RetentionPolicy      MemoryRetentionPolicy `json:"retention_policy"`
}

// MemoryRetentionPolicy defines type-specific retention windows.
type MemoryRetentionPolicy struct {
	ToolUsageRetention    time.Duration `json:"tool_usage_retention"`
	ConversationRetention time.Duration `json:"conversation_retention"`
	ErrorRetention        time.Duration `json:"error_retention"`
	ContextRetention      time.Duration `json:"context_retention"`
	SuccessRetention      time.Duration `json:"success_retention"`
}

// DefaultMemoryConfig returns default memory configuration
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		MaxEntries:           10000,
		DefaultTTL:           7 * 24 * time.Hour, // 7 days
		LearningEnabled:      true,
		ImportanceDecay:      0.1, // 10% decay per access
		MinImportance:        0.1,
		MaxImportance:        1.0,
		EnableSemanticSearch: true,
		IndexingEnabled:      true,
		RetentionPolicy: MemoryRetentionPolicy{
			ToolUsageRetention:    30 * 24 * time.Hour,
			ConversationRetention: 7 * 24 * time.Hour,
			ErrorRetention:        30 * 24 * time.Hour,
			ContextRetention:      3 * 24 * time.Hour,
			SuccessRetention:      14 * 24 * time.Hour,
		},
	}
}

// MemoryStats tracks memory system statistics
type MemoryStats struct {
	TotalEntries         int                `json:"total_entries"`
	EntriesByType        map[MemoryType]int `json:"entries_by_type"`
	TotalQueries         int64              `json:"total_queries"`
	SuccessfulRetrievals int64              `json:"successful_retrievals"`
	MissedRetrievals     int64              `json:"missed_retrievals"`
	QueryLatency         time.Duration      `json:"query_latency"`
	StorageSize          int64              `json:"storage_size"` // bytes
	AverageImportance    float64            `json:"average_importance"`
	AverageConfidence    float64            `json:"average_confidence"`
	TotalAccessCount     int64              `json:"total_access_count"`
	MostAccessed         []string           `json:"most_accessed"`
	OldestEntry          *time.Time         `json:"oldest_entry,omitempty"`
	NewestEntry          *time.Time         `json:"newest_entry,omitempty"`
}

var _ Store = (*FileStore)(nil)
