package memory

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

// ============================================================================
// Memory Manager
// ============================================================================

// Manager manages all memory types. Safe for concurrent use: every method
// takes an explicit projectID (see ProjectID) for anything project-scoped,
// rather than relying on "whichever project was loaded most recently" - a
// single Manager (and the Engine that owns it) can serve multiple sessions
// against different projects without one clobbering another's state.
type Manager struct {
	projectStore Store
	userStore    Store
	crossStore   Store

	mu       sync.Mutex
	projects map[string]*ProjectMemory // keyed by ProjectID(path)
	user     *UserMemory
	cross    *CrossSession
	catalog  *Catalog
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
		projects:     make(map[string]*ProjectMemory),
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
		projects:     make(map[string]*ProjectMemory),
		catalog:      NewCatalog(),
	}, nil
}

// LoadProject loads project memory for projectPath and returns its stable
// ID (ProjectID(projectPath)) - callers use this ID on every subsequent
// project-scoped call instead of passing the path again.
func (m *Manager) LoadProject(projectPath string) (string, error) {
	project, err := m.projectStore.LoadProjectMemory(projectPath)
	if err != nil {
		return "", fmt.Errorf("load project memory: %w", err)
	}
	id := ProjectID(projectPath)

	m.mu.Lock()
	if m.projects == nil {
		m.projects = make(map[string]*ProjectMemory)
	}
	m.projects[id] = project
	m.mu.Unlock()

	m.rebuildCatalog()
	return id, nil
}

// SaveProject saves projectID's memory.
func (m *Manager) SaveProject(projectID string) error {
	m.mu.Lock()
	project := m.projects[projectID]
	m.mu.Unlock()
	if project == nil {
		return nil
	}
	return m.projectStore.SaveProjectMemory(project)
}

// LoadUser loads user memory
func (m *Manager) LoadUser() error {
	user, err := m.userStore.LoadUserMemory()
	if err != nil {
		return fmt.Errorf("load user memory: %w", err)
	}
	m.mu.Lock()
	m.user = user
	m.mu.Unlock()
	m.rebuildCatalog()
	return nil
}

// SaveUser saves user memory
func (m *Manager) SaveUser() error {
	m.mu.Lock()
	user := m.user
	m.mu.Unlock()
	if user == nil {
		return nil
	}
	return m.userStore.SaveUserMemory(user)
}

// LoadCrossSession loads cross-session memory
func (m *Manager) LoadCrossSession() error {
	cross, err := m.crossStore.LoadCrossSession()
	if err != nil {
		return fmt.Errorf("load cross-session memory: %w", err)
	}
	m.mu.Lock()
	m.cross = cross
	m.mu.Unlock()
	m.rebuildCatalog()
	return nil
}

// SaveCrossSession saves cross-session memory
func (m *Manager) SaveCrossSession() error {
	m.mu.Lock()
	cross := m.cross
	m.mu.Unlock()
	if cross == nil {
		return nil
	}
	return m.crossStore.SaveCrossSession(cross)
}

// LoadAll loads all memory types and returns projectPath's project ID.
func (m *Manager) LoadAll(projectPath string) (string, error) {
	id, err := m.LoadProject(projectPath)
	if err != nil {
		return "", err
	}
	if err := m.LoadUser(); err != nil {
		return "", err
	}
	if err := m.LoadCrossSession(); err != nil {
		return "", err
	}
	return id, nil
}

// SaveAll saves user and cross-session memory, plus projectID's project
// memory if projectID is non-empty.
func (m *Manager) SaveAll(projectID string) error {
	if projectID != "" {
		if err := m.SaveProject(projectID); err != nil {
			return err
		}
	}
	if err := m.SaveUser(); err != nil {
		return err
	}
	if err := m.SaveCrossSession(); err != nil {
		return err
	}
	return nil
}

// GetProject returns projectID's project memory, or nil if not loaded.
func (m *Manager) GetProject(projectID string) *ProjectMemory {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.projects[projectID]
}

// GetUser returns the current user memory
func (m *Manager) GetUser() *UserMemory {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.user
}

// GetCrossSession returns the cross-session memory
func (m *Manager) GetCrossSession() *CrossSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cross
}

// ============================================================================
// Entry Management
// ============================================================================

// LearnPreference adds a learned preference. projectID is only used when
// scope == MemoryScopeProject.
func (m *Manager) LearnPreference(projectID string, scope MemoryScope, key, value, source string) error {
	return m.learnScopedEntry(projectID, scope, MemoryTypePreference, key, value, source)
}

// LearnInstruction adds a learned persistent instruction. projectID is only
// used when scope == MemoryScopeProject.
func (m *Manager) LearnInstruction(projectID string, scope MemoryScope, key, value, source string) error {
	return m.learnScopedEntry(projectID, scope, MemoryTypeInstruction, key, value, source)
}

// GetPreferences retrieves preferences for a scope. projectID is only used
// when scope == MemoryScopeProject.
func (m *Manager) GetPreferences(projectID string, scope MemoryScope) []*Entry {
	var entries []*Entry

	m.mu.Lock()
	defer m.mu.Unlock()

	switch scope {
	case MemoryScopeProject:
		if project := m.projects[projectID]; project != nil {
			for _, e := range project.Entries {
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
	m.mu.Lock()
	if m.cross == nil {
		m.mu.Unlock()
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
	cross := m.cross
	catalog := m.catalog
	m.mu.Unlock()

	if catalog != nil {
		_ = catalog.StoreEntry(Entry{
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

	return m.crossStore.SaveCrossSession(cross)
}

// GetProjectHistory retrieves past session summaries for a project
func (m *Manager) GetProjectHistory(projectPath string) []*SessionSummary {
	var summaries []*SessionSummary

	m.mu.Lock()
	defer m.mu.Unlock()

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

// Context returns memory context for inclusion in LLM prompts, scoped to
// projectID (pass "" for none - only user preferences/tool usage/history
// sections will be empty).
func (m *Manager) Context(projectID string) string {
	sections := make([]string, 0, 4)

	if lines := m.contextLinesForEntries(m.projectEntriesByType(projectID, MemoryTypePreference, MemoryTypeInstruction)); len(lines) > 0 {
		sections = append(sections, "## Project Memory\n"+strings.Join(lines, "\n"))
	}
	if lines := m.contextLinesForEntries(m.userEntriesByType(MemoryTypePreference, MemoryTypeInstruction)); len(lines) > 0 {
		sections = append(sections, "## User Preferences\n"+strings.Join(lines, "\n"))
	}
	if lines := m.contextLinesForToolUsage(projectID); len(lines) > 0 {
		sections = append(sections, "## Learned Tool Usage\n"+strings.Join(lines, "\n"))
	}
	if lines := m.contextLinesForProjectHistory(projectID, 3); len(lines) > 0 {
		sections = append(sections, "## Recent Session History\n"+strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

// ============================================================================
// Helpers
// ============================================================================

func getBaseMemoryPath() (string, error) {
	// Check SESHAT_MEMORY_PATH env var
	if path := os.Getenv("SESHAT_MEMORY_PATH"); path != "" {
		return path, nil
	}

	// Default to <runtime-root>/memory — honours SESHAT_RUNTIME_ROOT so the CLI
	// (seshat-cli) and the product backend (seshat) stay isolated automatically.
	return runtimepath.Join("", "memory"), nil
}

// EnsureDirectory ensures the memory directory exists
func EnsureDirectory() error {
	path, err := getBaseMemoryPath()
	if err != nil {
		return err
	}

	return os.MkdirAll(path, 0755)
}

// Search looks up entries in the central searchable catalog. Project-scoped
// entries (Scope == MemoryScopeProject) are only returned when their
// ProjectID matches projectID; user/session-scoped entries are always
// visible regardless of projectID. Pass "" for projectID to exclude every
// project-scoped entry.
func (m *Manager) Search(projectID string, query MemoryQuery) (*MemorySearchResult, error) {
	m.mu.Lock()
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	catalog := m.catalog
	m.mu.Unlock()

	result, err := catalog.Search(query)
	if err != nil || result == nil {
		return result, err
	}
	filtered := make([]Entry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		if entry.Scope == MemoryScopeProject && entry.ProjectID != projectID {
			continue
		}
		filtered = append(filtered, entry)
	}
	result.Entries = filtered
	result.Total = len(filtered)
	return result, nil
}

// StoreEntry stores an entry in the central catalog and persists it to
// projectID's project memory (when entry.Scope == MemoryScopeProject) or
// user memory (when entry.Scope == MemoryScopeUser).
func (m *Manager) StoreEntry(projectID string, entry Entry) error {
	m.mu.Lock()
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	if entry.Scope == MemoryScopeProject {
		entry.ProjectID = projectID
	}
	catalog := m.catalog
	m.mu.Unlock()

	if err := catalog.StoreEntry(entry); err != nil {
		return err
	}

	key := entry.Key
	if key == "" {
		key = entry.ID
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	switch entry.Scope {
	case MemoryScopeProject:
		if project := m.projects[projectID]; project != nil {
			cloned := entry
			project.Entries[key] = &cloned
			return m.projectStore.SaveProjectMemory(project)
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

// GetEntry looks up an entry by its unique ID - unambiguous regardless of
// project, so it takes no projectID.
func (m *Manager) GetEntry(id string) (*Entry, error) {
	m.mu.Lock()
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	catalog := m.catalog
	m.mu.Unlock()
	return catalog.GetEntry(id)
}

// DeleteEntry removes id from the catalog AND from its owning project/user
// memory map - deleting only from the catalog (the previous behavior) meant
// the entry silently reappeared on the next rebuildCatalog (i.e. the next
// Load*), since that rebuild repopulates the catalog straight from these
// same maps.
func (m *Manager) DeleteEntry(id string) error {
	m.mu.Lock()
	if m.catalog == nil {
		m.mu.Unlock()
		return fmt.Errorf("memory catalog not initialized")
	}
	catalog := m.catalog
	m.mu.Unlock()

	entry, err := catalog.GetEntry(id)
	if err != nil {
		return err
	}
	if err := catalog.DeleteEntry(id); err != nil {
		return err
	}
	if entry == nil {
		return nil
	}

	key := entry.Key
	if key == "" {
		key = entry.ID
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	switch entry.Scope {
	case MemoryScopeProject:
		if project := m.projects[entry.ProjectID]; project != nil {
			delete(project.Entries, key)
			return m.projectStore.SaveProjectMemory(project)
		}
	case MemoryScopeUser:
		if m.user != nil {
			delete(m.user.Entries, key)
			return m.userStore.SaveUserMemory(m.user)
		}
	}

	return nil
}

// LearnToolUsage records a tool call outcome against the catalog's global
// search index and, when projectID is non-empty, as the authoritative
// per-project usage record (the source GetToolUsagePatterns prefers).
func (m *Manager) LearnToolUsage(projectID, toolName string, parameters map[string]any, success bool, err error) error {
	m.mu.Lock()
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	catalog := m.catalog
	m.mu.Unlock()

	if learnErr := catalog.LearnToolUsage(toolName, parameters, success, err); learnErr != nil {
		return learnErr
	}
	if projectID == "" {
		return nil
	}

	usage, usageErr := catalog.GetToolUsagePatterns(toolName)
	if usageErr != nil {
		return usageErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	project := m.projects[projectID]
	if project == nil {
		return nil
	}

	if project.ToolUsage == nil {
		project.ToolUsage = make(map[string]*ToolUsageMemory)
	}
	project.ToolUsage[toolName] = cloneToolUsageMemory(usage)

	if project.Entries == nil {
		project.Entries = make(map[string]*Entry)
	}
	entryKey := fmt.Sprintf("tool_usage:%s", toolName)
	entry, exists := project.Entries[entryKey]
	if !exists || entry == nil {
		entry = NewEntry(MemoryScopeProject, MemoryTypeToolUsage, entryKey, "", "tool_execution")
	}
	entry.ProjectID = projectID
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
	project.Entries[entryKey] = entry

	return m.projectStore.SaveProjectMemory(project)
}

// GetToolUsagePatterns returns toolName's usage stats for projectID,
// preferring that project's own persisted record over the catalog's
// cross-project aggregate (which mixes stats from every project sharing
// this Manager's catalog once more than one is loaded).
func (m *Manager) GetToolUsagePatterns(projectID, toolName string) (*ToolUsageMemory, error) {
	m.mu.Lock()
	if project := m.projects[projectID]; project != nil {
		if usage := project.ToolUsage[toolName]; usage != nil {
			m.mu.Unlock()
			return usage, nil
		}
	}
	if m.catalog == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("memory catalog not initialized")
	}
	catalog := m.catalog
	m.mu.Unlock()
	return catalog.GetToolUsagePatterns(toolName)
}

func (m *Manager) Stats() MemoryStats {
	m.mu.Lock()
	if m.catalog == nil {
		m.mu.Unlock()
		return MemoryStats{EntriesByType: make(map[MemoryType]int), MostAccessed: []string{}}
	}
	catalog := m.catalog
	m.mu.Unlock()
	return catalog.Stats()
}

func (m *Manager) Catalog() *Catalog {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.catalog == nil {
		m.catalog = NewCatalog()
	}
	return m.catalog
}

func (m *Manager) rebuildCatalog() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.catalog == nil {
		m.catalog = NewCatalog()
	}

	entries := make([]Entry, 0)
	toolUsage := make(map[string]*ToolUsageMemory)
	for projectID, project := range m.projects {
		if project == nil {
			continue
		}
		for _, entry := range project.Entries {
			if entry == nil {
				continue
			}
			cloned := *entry
			cloned.ProjectID = projectID
			entries = append(entries, cloned)
		}
		for name, usage := range project.ToolUsage {
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

func (m *Manager) learnScopedEntry(projectID string, scope MemoryScope, entryType MemoryType, key, value, source string) error {
	entry := NewEntry(scope, entryType, key, value, source)
	entry.Content = value
	entry.Tags = []string{string(entryType)}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch scope {
	case MemoryScopeProject:
		if project := m.projects[projectID]; project != nil {
			entry.ProjectID = projectID
			project.Entries[key] = entry
			if m.catalog != nil {
				_ = m.catalog.StoreEntry(*entry)
			}
			return m.projectStore.SaveProjectMemory(project)
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

func (m *Manager) projectEntriesByType(projectID string, types ...MemoryType) []*Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	project := m.projects[projectID]
	if project == nil {
		return nil
	}
	return filterEntriesByType(project.Entries, types...)
}

func (m *Manager) userEntriesByType(types ...MemoryType) []*Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *Manager) contextLinesForToolUsage(projectID string) []string {
	m.mu.Lock()
	project := m.projects[projectID]
	m.mu.Unlock()
	if project == nil || len(project.ToolUsage) == 0 {
		return nil
	}

	type toolUsageLine struct {
		Name  string
		Usage *ToolUsageMemory
	}
	ordered := make([]toolUsageLine, 0, len(project.ToolUsage))
	for name, usage := range project.ToolUsage {
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

func (m *Manager) contextLinesForProjectHistory(projectID string, limit int) []string {
	m.mu.Lock()
	project := m.projects[projectID]
	cross := m.cross
	m.mu.Unlock()
	if project == nil || cross == nil || limit <= 0 {
		return nil
	}

	summaries := m.GetProjectHistory(project.ProjectPath)
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
