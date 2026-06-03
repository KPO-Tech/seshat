package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ============================================================================
// Memory Manager
// ============================================================================

// Manager manages all memory types
type Manager struct {
	projectStore Store
	userStore    Store
	crossStore   Store

	project *ProjectMemory
	user    *UserMemory
	cross   *CrossSession
	catalog *Catalog
}

// NewManager creates a new memory manager
func NewManager() (*Manager, error) {
	basePath, err := getBaseMemoryPath()
	if err != nil {
		return nil, err
	}

	fs, err := NewFileStore(basePath)
	if err != nil {
		return nil, err
	}

	return &Manager{
		projectStore: fs,
		userStore:    fs,
		crossStore:   fs,
		catalog:      NewCatalog(),
	}, nil
}

// NewManagerWithPath creates a memory manager with explicit path
func NewManagerWithPath(basePath string) (*Manager, error) {
	fs, err := NewFileStore(basePath)
	if err != nil {
		return nil, err
	}

	return &Manager{
		projectStore: fs,
		userStore:    fs,
		crossStore:   fs,
		catalog:      NewCatalog(),
	}, nil
}

// LoadProject loads project memory
func (m *Manager) LoadProject(projectPath string) error {
	project, err := m.projectStore.LoadProjectMemory(projectPath)
	if err != nil {
		return fmt.Errorf("load project memory: %w", err)
	}
	m.project = project
	m.rebuildCatalog()
	return nil
}

// SaveProject saves project memory
func (m *Manager) SaveProject() error {
	if m.project == nil {
		return nil
	}
	return m.projectStore.SaveProjectMemory(m.project)
}

// LoadUser loads user memory
func (m *Manager) LoadUser() error {
	user, err := m.userStore.LoadUserMemory()
	if err != nil {
		return fmt.Errorf("load user memory: %w", err)
	}
	m.user = user
	m.rebuildCatalog()
	return nil
}

// SaveUser saves user memory
func (m *Manager) SaveUser() error {
	if m.user == nil {
		return nil
	}
	return m.userStore.SaveUserMemory(m.user)
}

// LoadCrossSession loads cross-session memory
func (m *Manager) LoadCrossSession() error {
	cross, err := m.crossStore.LoadCrossSession()
	if err != nil {
		return fmt.Errorf("load cross-session memory: %w", err)
	}
	m.cross = cross
	m.rebuildCatalog()
	return nil
}

// SaveCrossSession saves cross-session memory
func (m *Manager) SaveCrossSession() error {
	if m.cross == nil {
		return nil
	}
	return m.crossStore.SaveCrossSession(m.cross)
}

// LoadAll loads all memory types
func (m *Manager) LoadAll(projectPath string) error {
	if err := m.LoadProject(projectPath); err != nil {
		return err
	}
	if err := m.LoadUser(); err != nil {
		return err
	}
	if err := m.LoadCrossSession(); err != nil {
		return err
	}
	return nil
}

// SaveAll saves all memory types
func (m *Manager) SaveAll() error {
	if err := m.SaveProject(); err != nil {
		return err
	}
	if err := m.SaveUser(); err != nil {
		return err
	}
	if err := m.SaveCrossSession(); err != nil {
		return err
	}
	return nil
}

// GetProject returns the current project memory
func (m *Manager) GetProject() *ProjectMemory {
	return m.project
}

// GetUser returns the current user memory
func (m *Manager) GetUser() *UserMemory {
	return m.user
}

// GetCrossSession returns the cross-session memory
func (m *Manager) GetCrossSession() *CrossSession {
	return m.cross
}

// ============================================================================
// Entry Management
// ============================================================================

// LearnPreference adds a learned preference
func (m *Manager) LearnPreference(scope MemoryScope, key, value, source string) error {
	return m.learnScopedEntry(scope, MemoryTypePreference, key, value, source)
}

// LearnInstruction adds a learned persistent instruction.
func (m *Manager) LearnInstruction(scope MemoryScope, key, value, source string) error {
	return m.learnScopedEntry(scope, MemoryTypeInstruction, key, value, source)
}

// GetPreferences retrieves preferences for a scope
func (m *Manager) GetPreferences(scope MemoryScope) []*Entry {
	var entries []*Entry

	switch scope {
	case MemoryScopeProject:
		if m.project != nil {
			for _, e := range m.project.Entries {
				if e.Type == MemoryTypePreference {
					entries = append(entries, e)
				}
			}
		}
	case MemoryScopeUser:
		if m.user != nil {
			for _, e := range m.user.Entries {
				if e.Type == MemoryTypePreference {
					entries = append(entries, e)
				}
			}
		}
	}

	return entries
}

// AddSessionSummary adds a session summary for cross-session recall
func (m *Manager) AddSessionSummary(sessionID, projectPath, summary string, toolsUsed []string) error {
	if m.cross == nil {
		return fmt.Errorf("cross-session memory not loaded")
	}

	completedAt := time.Now()
	m.cross.SessionSummaries[sessionID] = &SessionSummary{
		SessionID:   sessionID,
		ProjectPath: projectPath,
		Summary:     summary,
		ToolsUsed:   toolsUsed,
		CompletedAt: completedAt,
	}
	if m.catalog != nil {
		_ = m.catalog.StoreEntry(Entry{
			ID:        sessionID,
			Scope:     MemoryScopeSession,
			Type:      MemoryTypeSummary,
			Key:       sessionID,
			Value:     summary,
			Content:   summary,
			Source:    "session_summary",
			CreatedAt: completedAt,
			UpdatedAt: completedAt,
			Tags:      append([]string{"session", "summary"}, toolsUsed...),
		})
	}

	return m.crossStore.SaveCrossSession(m.cross)
}

// GetProjectHistory retrieves past session summaries for a project
func (m *Manager) GetProjectHistory(projectPath string) []*SessionSummary {
	var summaries []*SessionSummary

	if m.cross == nil {
		return summaries
	}

	for _, s := range m.cross.SessionSummaries {
		if s.ProjectPath == projectPath {
			summaries = append(summaries, s)
		}
	}

	return summaries
}

// ============================================================================
// Context for LLM
// ============================================================================

// Context returns memory context for inclusion in LLM prompts
func (m *Manager) Context() string {
	sections := make([]string, 0, 4)

	if lines := m.contextLinesForEntries(m.projectEntriesByType(MemoryTypePreference, MemoryTypeInstruction)); len(lines) > 0 {
		sections = append(sections, "## Project Memory\n"+strings.Join(lines, "\n"))
	}
	if lines := m.contextLinesForEntries(m.userEntriesByType(MemoryTypePreference, MemoryTypeInstruction)); len(lines) > 0 {
		sections = append(sections, "## User Preferences\n"+strings.Join(lines, "\n"))
	}
	if lines := m.contextLinesForToolUsage(); len(lines) > 0 {
		sections = append(sections, "## Learned Tool Usage\n"+strings.Join(lines, "\n"))
	}
	if lines := m.contextLinesForProjectHistory(3); len(lines) > 0 {
		sections = append(sections, "## Recent Session History\n"+strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

// ============================================================================
// Helpers
// ============================================================================

func getBaseMemoryPath() (string, error) {
	// Check NEXUS_MEMORY_PATH env var
	if path := os.Getenv("NEXUS_MEMORY_PATH"); path != "" {
		return path, nil
	}

	// Default to ~/.nexus/memory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	return filepath.Join(homeDir, ".nexus", "memory"), nil
}

// EnsureDirectory ensures the memory directory exists
func EnsureDirectory() error {
	path, err := getBaseMemoryPath()
	if err != nil {
		return err
	}

	return os.MkdirAll(path, 0755)
}

// Search looks up entries in the central searchable catalog.
func (m *Manager) Search(query MemoryQuery) (*MemorySearchResult, error) {
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	return m.catalog.Search(query)
}

// StoreEntry stores an entry in the central catalog and persists scoped entries when possible.
func (m *Manager) StoreEntry(entry Entry) error {
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	if err := m.catalog.StoreEntry(entry); err != nil {
		return err
	}

	key := entry.Key
	if key == "" {
		key = entry.ID
	}
	switch entry.Scope {
	case MemoryScopeProject:
		if m.project != nil {
			cloned := entry
			m.project.Entries[key] = &cloned
			return m.projectStore.SaveProjectMemory(m.project)
		}
	case MemoryScopeUser:
		if m.user != nil {
			cloned := entry
			m.user.Entries[key] = &cloned
			return m.userStore.SaveUserMemory(m.user)
		}
	}

	return nil
}

func (m *Manager) GetEntry(id string) (*Entry, error) {
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	return m.catalog.GetEntry(id)
}

func (m *Manager) DeleteEntry(id string) error {
	if m.catalog == nil {
		return fmt.Errorf("memory catalog not initialized")
	}
	return m.catalog.DeleteEntry(id)
}

func (m *Manager) LearnToolUsage(toolName string, parameters map[string]any, success bool, err error) error {
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	if learnErr := m.catalog.LearnToolUsage(toolName, parameters, success, err); learnErr != nil {
		return learnErr
	}
	if m.project == nil {
		return nil
	}

	usage, usageErr := m.catalog.GetToolUsagePatterns(toolName)
	if usageErr != nil {
		return usageErr
	}

	if m.project.ToolUsage == nil {
		m.project.ToolUsage = make(map[string]*ToolUsageMemory)
	}
	m.project.ToolUsage[toolName] = cloneToolUsageMemory(usage)

	if m.project.Entries == nil {
		m.project.Entries = make(map[string]*Entry)
	}
	entryKey := fmt.Sprintf("tool_usage:%s", toolName)
	entry, exists := m.project.Entries[entryKey]
	if !exists || entry == nil {
		entry = NewEntry(MemoryScopeProject, MemoryTypeToolUsage, entryKey, "", "tool_execution")
	}
	entry.Value = describeToolUsage(usage)
	entry.Content = entry.Value
	entry.Source = "tool_execution"
	entry.Tags = []string{"tool", toolName}
	entry.UpdatedAt = time.Now()
	entry.Metadata = &EntryMetadata{
		ToolUsed:    toolName,
		Frequency:   usage.UsageCount,
		SuccessRate: usage.SuccessRate,
		LastUsedAt:  &usage.LastUsed,
	}
	m.project.Entries[entryKey] = entry

	return m.projectStore.SaveProjectMemory(m.project)
}

func (m *Manager) GetToolUsagePatterns(toolName string) (*ToolUsageMemory, error) {
	if m.catalog == nil {
		return nil, fmt.Errorf("memory catalog not initialized")
	}
	return m.catalog.GetToolUsagePatterns(toolName)
}

func (m *Manager) Stats() MemoryStats {
	if m.catalog == nil {
		return MemoryStats{EntriesByType: make(map[MemoryType]int), MostAccessed: []string{}}
	}
	return m.catalog.Stats()
}

func (m *Manager) Catalog() *Catalog {
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	return m.catalog
}

func (m *Manager) rebuildCatalog() {
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}

	entries := make([]Entry, 0)
	toolUsage := make(map[string]*ToolUsageMemory)
	if m.project != nil {
		for _, entry := range m.project.Entries {
			if entry != nil {
				entries = append(entries, *entry)
			}
		}
		for name, usage := range m.project.ToolUsage {
			if usage != nil {
				toolUsage[name] = cloneToolUsageMemory(usage)
			}
		}
	}
	if m.user != nil {
		for _, entry := range m.user.Entries {
			if entry != nil {
				entries = append(entries, *entry)
			}
		}
	}
	if m.cross != nil {
		for id, summary := range m.cross.SessionSummaries {
			if summary == nil {
				continue
			}
			entries = append(entries, Entry{
				ID:        id,
				Scope:     MemoryScopeSession,
				Type:      MemoryTypeSummary,
				Key:       id,
				Value:     summary.Summary,
				Content:   summary.Summary,
				Source:    "session_summary",
				CreatedAt: summary.CompletedAt,
				UpdatedAt: summary.CompletedAt,
				Tags:      append([]string{"session", "summary"}, summary.ToolsUsed...),
			})
		}
		for key, pattern := range m.cross.GlobalPatterns {
			if pattern == nil {
				continue
			}
			entries = append(entries, Entry{
				ID:         key,
				Scope:      MemoryScopeSession,
				Type:       MemoryTypePattern,
				Key:        key,
				Value:      pattern.Pattern,
				Content:    pattern.Description,
				Source:     "global_pattern",
				CreatedAt:  pattern.LastSeenAt,
				UpdatedAt:  pattern.LastSeenAt,
				Confidence: pattern.SuccessRate,
				Metadata: &EntryMetadata{
					Frequency:   pattern.Frequency,
					SuccessRate: pattern.SuccessRate,
					Tags:        append([]string(nil), pattern.Examples...),
				},
			})
		}
	}

	m.catalog.ResetEntries(entries)
	m.catalog.ResetToolUsage(toolUsage)
}

func (m *Manager) learnScopedEntry(scope MemoryScope, entryType MemoryType, key, value, source string) error {
	entry := NewEntry(scope, entryType, key, value, source)
	entry.Content = value
	entry.Tags = []string{string(entryType)}

	switch scope {
	case MemoryScopeProject:
		if m.project != nil {
			m.project.Entries[key] = entry
			if m.catalog != nil {
				_ = m.catalog.StoreEntry(*entry)
			}
			return m.projectStore.SaveProjectMemory(m.project)
		}
	case MemoryScopeUser:
		if m.user != nil {
			m.user.Entries[key] = entry
			if m.catalog != nil {
				_ = m.catalog.StoreEntry(*entry)
			}
			return m.userStore.SaveUserMemory(m.user)
		}
	}

	return nil
}

func (m *Manager) projectEntriesByType(types ...MemoryType) []*Entry {
	if m.project == nil {
		return nil
	}
	return filterEntriesByType(m.project.Entries, types...)
}

func (m *Manager) userEntriesByType(types ...MemoryType) []*Entry {
	if m.user == nil {
		return nil
	}
	return filterEntriesByType(m.user.Entries, types...)
}

func (m *Manager) contextLinesForEntries(entries []*Entry) []string {
	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", entry.Key, entry.Value))
	}
	return lines
}

func (m *Manager) contextLinesForToolUsage() []string {
	if m.project == nil || len(m.project.ToolUsage) == 0 {
		return nil
	}

	type toolUsageLine struct {
		Name  string
		Usage *ToolUsageMemory
	}
	ordered := make([]toolUsageLine, 0, len(m.project.ToolUsage))
	for name, usage := range m.project.ToolUsage {
		if usage == nil {
			continue
		}
		ordered = append(ordered, toolUsageLine{Name: name, Usage: usage})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Usage.UsageCount == ordered[j].Usage.UsageCount {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Usage.UsageCount > ordered[j].Usage.UsageCount
	})

	lines := make([]string, 0, len(ordered))
	for _, item := range ordered {
		lines = append(lines, fmt.Sprintf("- %s: %s", item.Name, describeToolUsage(item.Usage)))
	}
	return lines
}

func (m *Manager) contextLinesForProjectHistory(limit int) []string {
	if m.project == nil || m.cross == nil || limit <= 0 {
		return nil
	}

	summaries := m.GetProjectHistory(m.project.ProjectPath)
	if len(summaries) == 0 {
		return nil
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CompletedAt.After(summaries[j].CompletedAt)
	})
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	lines := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if summary == nil || summary.Summary == "" {
			continue
		}
		line := truncateMemoryLine(summary.Summary, 220)
		if !summary.CompletedAt.IsZero() {
			line = fmt.Sprintf("%s: %s", summary.CompletedAt.Format("2006-01-02"), line)
		}
		lines = append(lines, "- "+line)
	}
	return lines
}

func filterEntriesByType(entries map[string]*Entry, types ...MemoryType) []*Entry {
	if len(entries) == 0 || len(types) == 0 {
		return nil
	}

	allowed := make(map[MemoryType]struct{}, len(types))
	for _, entryType := range types {
		allowed[entryType] = struct{}{}
	}

	filtered := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if _, ok := allowed[entry.Type]; ok {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func describeToolUsage(usage *ToolUsageMemory) string {
	if usage == nil {
		return ""
	}
	return fmt.Sprintf("used %d times, %.0f%% success", usage.UsageCount, usage.SuccessRate*100)
}

func truncateMemoryLine(text string, max int) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(normalized)
	if max <= 0 || len(runes) <= max {
		return normalized
	}
	if max == 1 {
		return string(runes[:1])
	}
	return string(runes[:max-1]) + "…"
}
