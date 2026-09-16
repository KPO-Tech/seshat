package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Memory Types
// ============================================================================

// MemoryScope represents the scope of memory
type MemoryScope string

const (
	MemoryScopeProject MemoryScope = "project" // Per project (.seshat/)
	MemoryScopeUser    MemoryScope = "user"    // Global user (~/.seshat/)
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
	ProjectID    string     `json:"project_id,omitempty"` // Set when Scope == MemoryScopeProject; see ProjectID()
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
		basePath = filepath.Join(homeDir, ".seshat", "memory")
	}

	// Create directory if needed. 0700: memory files can hold personal
	// conversation content, preferences, and learned instructions.
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	return &FileStore{basePath: basePath}, nil
}

// LoadProjectMemory loads project-specific memory
func (s *FileStore) LoadProjectMemory(projectPath string) (*ProjectMemory, error) {
	rootID := ProjectID(projectPath)

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
	rootID := ProjectID(m.ProjectPath)

	// Ensure projects directory
	dir := filepath.Join(s.basePath, "projects")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create projects dir: %w", err)
	}

	filePath := filepath.Join(dir, rootID+".json")

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal project memory: %w", err)
	}

	if err := atomicWriteFile(filePath, data, 0600); err != nil {
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

	if err := atomicWriteFile(filePath, data, 0600); err != nil {
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

	if err := atomicWriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write cross-session memory: %w", err)
	}

	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// ProjectID returns a stable identifier for projectPath: the hex-encoded
// SHA256 of its canonical (symlink-resolved, cleaned, absolute) form. Two
// different paths never collide just because they share a basename - unlike
// the git-repo case this replaces, which returned filepath.Base(projectPath)
// unconditionally (so /home/a/backend and /home/b/backend would have shared
// one memory file). Not based on the git remote: a plain path hash means a
// renamed/moved project directory starts a fresh memory history rather than
// silently merging with an unrelated project that happens to reuse the
// address space of a stale hash - the same trade-off the previous fnv32
// path-hash fallback (for non-git directories) already made.
func ProjectID(projectPath string) string {
	canonical := projectPath
	if abs, err := filepath.Abs(projectPath); err == nil {
		canonical = abs
	}
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = resolved
	}
	canonical = filepath.Clean(canonical)

	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// atomicWriteFile writes data to path by first writing to a temp file in
// the same directory, fsyncing it, then renaming it into place. A plain
// os.WriteFile can leave a truncated/partial file behind if the process
// dies mid-write (crash, power loss, OOM kill) - the next read would then
// hit a corrupt, unparseable JSON file instead of the previous good state.
// rename is atomic on the same filesystem, which creating the temp file in
// path's own directory guarantees.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file into place: %w", err)
	}
	return nil
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
