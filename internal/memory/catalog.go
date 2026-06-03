package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Searchable Memory Catalog
// ============================================================================

// Catalog stores searchable runtime memory entries and learned tool usage.
type Catalog struct {
	mu            sync.RWMutex
	entries       []Entry
	toolUsage     map[string]*ToolUsageMemory
	config        *MemoryConfig
	stats         MemoryStats
	statsMu       sync.Mutex // guards stats writes independent of mu
	index         *MemoryIndex
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// NewCatalog creates a searchable memory catalog with default configuration.
func NewCatalog() *Catalog {
	return NewCatalogWithConfig(DefaultMemoryConfig())
}

// NewCatalogWithConfig creates a searchable memory catalog with custom configuration.
func NewCatalogWithConfig(config *MemoryConfig) *Catalog {
	return &Catalog{
		entries:   make([]Entry, 0),
		toolUsage: make(map[string]*ToolUsageMemory),
		config:    config,
		stats: MemoryStats{
			EntriesByType: make(map[MemoryType]int),
		},
		index: &MemoryIndex{
			contentIndex: make(map[string][]int),
			tagIndex:     make(map[string][]int),
			typeIndex:    make(map[MemoryType][]int),
			sessionIndex: make(map[string][]int),
			toolIndex:    make(map[string][]int),
		},
		stopChan:      make(chan struct{}),
		cleanupTicker: time.NewTicker(1 * time.Hour),
	}
}

// Start starts background cleanup for the catalog.
func (ms *Catalog) Start() {
	go ms.cleanupLoop()
}

// Stop stops background cleanup for the catalog.
func (ms *Catalog) Stop() {
	ms.cleanupTicker.Stop()
	close(ms.stopChan)
}

// cleanupLoop periodically cleans up expired entries
func (ms *Catalog) cleanupLoop() {
	for {
		select {
		case <-ms.cleanupTicker.C:
			ms.CleanupExpired()
		case <-ms.stopChan:
			return
		}
	}
}

// StoreEntry stores a new memory entry
func (ms *Catalog) StoreEntry(entry Entry) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Set creation time if not set
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// Set ID if not set
	if entry.ID == "" {
		entry.ID = generateEntryID()
	}

	// Set importance if not set
	if entry.Importance == 0 {
		entry.Importance = ms.calculateImportance(&entry)
	}
	if entry.Confidence == 0 {
		entry.Confidence = 0.5
	}

	// Set expiration if not set
	if entry.ExpiresAt == nil && ms.config.DefaultTTL > 0 {
		expiresAt := entry.CreatedAt.Add(ms.config.DefaultTTL)
		entry.ExpiresAt = &expiresAt
	}

	// Add to entries
	ms.entries = append(ms.entries, entry)
	index := len(ms.entries) - 1

	// Update index
	ms.updateIndex(entry, index)

	// Update statistics
	ms.stats.TotalEntries++
	ms.stats.EntriesByType[entry.Type]++
	ms.stats.StorageSize += int64(len(entry.ID))

	return nil
}

// Search retrieves memories based on the provided query.
func (ms *Catalog) Search(query MemoryQuery) (*MemorySearchResult, error) {
	startTime := time.Now()

	ms.statsMu.Lock()
	ms.stats.TotalQueries++
	ms.statsMu.Unlock()

	ms.mu.RLock()
	results := ms.filterEntries(query)
	ms.sortByRelevance(results, query)
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	for _, entry := range results {
		ms.updateAccessStats(&entry)
	}
	ms.mu.RUnlock()

	queryTime := time.Since(startTime)

	ms.statsMu.Lock()
	if len(results) > 0 {
		ms.stats.SuccessfulRetrievals++
	} else {
		ms.stats.MissedRetrievals++
	}
	ms.statsMu.Unlock()

	return &MemorySearchResult{
		Entries:       results,
		Total:         len(results),
		Query:         query,
		ExecutionTime: queryTime,
	}, nil
}

// filterEntries filters memory entries based on query criteria
func (ms *Catalog) filterEntries(query MemoryQuery) []Entry {
	var results []Entry

	for _, entry := range ms.entries {
		// Skip expired entries
		if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
			continue
		}

		// Type filter
		if len(query.Types) > 0 && !ms.containsMemoryType(query.Types, entry.Type) {
			continue
		}

		// Importance filter
		if query.MinImportance > 0 && entry.Importance < query.MinImportance {
			continue
		}

		// Confidence filter
		if query.MinConfidence > 0 && entry.Confidence < query.MinConfidence {
			continue
		}

		// Session filter
		if query.SessionID != "" && entry.SessionID != query.SessionID {
			continue
		}

		// Tag filter
		if len(query.Tags) > 0 && !ms.matchesTags(entry, query.Tags) {
			continue
		}

		// Content search
		if query.Content != "" {
			if !ms.matchesContent(entry, query.Content, query.ExactMatch) {
				continue
			}
		}

		// Keyword search
		if len(query.Keywords) > 0 {
			if !ms.matchesKeywords(entry, query.Keywords) {
				continue
			}
		}

		// Tool filter (from metadata)
		if query.Tool != "" && entry.Metadata != nil && entry.Metadata.ToolUsed != query.Tool {
			continue
		}

		if query.TimeRange != nil {
			if entry.CreatedAt.Before(query.TimeRange.Start) || entry.CreatedAt.After(query.TimeRange.End) {
				continue
			}
		}

		results = append(results, entry)
	}

	return results
}

// sortByRelevance sorts entries by relevance score
func (ms *Catalog) sortByRelevance(entries []Entry, query MemoryQuery) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if ms.calculateRelevanceScore(&entries[i], query) < ms.calculateRelevanceScore(&entries[j], query) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// calculateRelevanceScore calculates relevance score for an entry
func (ms *Catalog) calculateRelevanceScore(entry *Entry, query MemoryQuery) float64 {
	score := 0.0

	// Importance score
	score += entry.Importance * 0.4

	// Confidence score
	score += entry.Confidence * 0.2

	// Access frequency (more recently accessed = higher score)
	if !entry.LastAccessed.IsZero() {
		hoursSinceAccess := time.Since(entry.LastAccessed).Hours()
		score += math.Max(0, (24-hoursSinceAccess)/24) * 0.3
	}

	// Content matching score
	if query.Content != "" && strings.Contains(entry.Content, query.Content) {
		score += 0.3
	}

	// Tag matching score
	if len(query.Keywords) > 0 {
		for _, keyword := range query.Keywords {
			if ms.containsString(entry.Tags, keyword) {
				score += 0.1
			}
		}
	}

	return math.Min(1.0, score)
}

// calculateImportance calculates importance score for an entry
func (ms *Catalog) calculateImportance(entry *Entry) float64 {
	importance := entry.Importance

	// Adjust based on content length (longer content might be more important)
	if len(entry.Content) > 100 {
		importance += 0.4
	} else if len(entry.Content) > 50 {
		importance += 0.2
	}

	// Adjust based on tags (more tags = more important)
	if len(entry.Tags) > 3 {
		importance += 0.3
	} else if len(entry.Tags) > 1 {
		importance += 0.2
	}

	// Base importance for any entry
	if importance == 0 {
		importance = 0.2
	}

	// Ensure within bounds
	if importance < ms.config.MinImportance {
		importance = ms.config.MinImportance
	}
	if importance > ms.config.MaxImportance {
		importance = ms.config.MaxImportance
	}

	return importance
}

// updateIndex updates search index with a new entry
func (ms *Catalog) updateIndex(entry Entry, index int) {
	// Content keywords
	for _, keyword := range ms.extractKeywords(entry.Content) {
		ms.index.contentIndex[keyword] = append(ms.index.contentIndex[keyword], index)
	}

	// Tags
	for _, tag := range entry.Tags {
		ms.index.tagIndex[tag] = append(ms.index.tagIndex[tag], index)
	}

	// Type
	ms.index.typeIndex[entry.Type] = append(ms.index.typeIndex[entry.Type], index)

	// Session
	if entry.SessionID != "" {
		ms.index.sessionIndex[entry.SessionID] = append(ms.index.sessionIndex[entry.SessionID], index)
	}

	// Tool (from metadata)
	if entry.Metadata != nil && entry.Metadata.ToolUsed != "" {
		ms.index.toolIndex[entry.Metadata.ToolUsed] = append(ms.index.toolIndex[entry.Metadata.ToolUsed], index)
	}
}

// updateAccessStats updates access statistics for an entry
func (ms *Catalog) updateAccessStats(entry *Entry) {
	entry.AccessCount++
	entry.LastAccessed = time.Now()

	// Apply importance decay if configured
	if ms.config.ImportanceDecay > 0 {
		decay := entry.Importance * ms.config.ImportanceDecay
		entry.Importance = math.Max(ms.config.MinImportance, entry.Importance-decay)
	}
}

// CleanupExpired removes expired memory entries
func (ms *Catalog) CleanupExpired() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()
	cleaned := 0
	keepEntries := []Entry{}

	for _, entry := range ms.entries {
		if entry.ExpiresAt != nil && now.After(*entry.ExpiresAt) {
			cleaned++
			ms.removeFromIndex(entry, len(keepEntries))
		} else {
			keepEntries = append(keepEntries, entry)
		}
	}

	ms.entries = keepEntries
	ms.stats.TotalEntries = len(keepEntries)

	return cleaned
}

// LearnToolUsage learns from tool execution.
func (ms *Catalog) LearnToolUsage(toolName string, parameters map[string]any, success bool, err error) error {
	if !ms.config.LearningEnabled {
		return nil
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Get or create tool usage memory
	toolMem, exists := ms.toolUsage[toolName]
	if !exists {
		toolMem = &ToolUsageMemory{
			ToolName:    toolName,
			UsageCount:  0,
			SuccessRate: 1.0,
			LastUsed:    time.Now(),
		}
		ms.toolUsage[toolName] = toolMem
	}

	// Update usage statistics
	toolMem.UsageCount++
	toolMem.LastUsed = time.Now()

	// Learn from this execution
	params := &ParameterPattern{
		Parameters: parameters,
		Frequency:  1,
		Success:    success,
		LastUsed:   &toolMem.LastUsed,
	}

	if success {
		toolMem.SuccessfulParameters = ms.mergeParameterPattern(toolMem.SuccessfulParameters, params)
	} else {
		toolMem.FailedParameters = ms.mergeParameterPattern(toolMem.FailedParameters, params)
	}

	// Calculate success rate
	totalAttempts := toolMem.UsageCount
	successCount := ms.countSuccessPatterns(toolMem.SuccessfulParameters)
	toolMem.SuccessRate = float64(successCount) / float64(totalAttempts)

	// Store as memory entry
	entry := Entry{
		Scope:   MemoryScopeProject,
		Type:    MemoryTypeToolUsage,
		Content: fmt.Sprintf("Tool usage: %s with parameters: %v", toolName, parameters),
		Source:  "tool_execution",
		Tags:    []string{"tool", toolName, map[bool]string{true: "success", false: "error"}[success]},
		Metadata: &EntryMetadata{
			ToolUsed:    toolName,
			Frequency:   toolMem.UsageCount,
			SuccessRate: toolMem.SuccessRate,
			LastUsedAt:  &toolMem.LastUsed,
		},
	}

	// Apply the same enrichment rules as StoreEntry without re-entering the lock.
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.ID == "" {
		entry.ID = generateEntryID()
	}
	if entry.Importance == 0 {
		entry.Importance = ms.calculateImportance(&entry)
	}
	if entry.ExpiresAt == nil && ms.config.DefaultTTL > 0 {
		expiresAt := entry.CreatedAt.Add(ms.config.DefaultTTL)
		entry.ExpiresAt = &expiresAt
	}

	ms.entries = append(ms.entries, entry)
	index := len(ms.entries) - 1
	ms.updateIndex(entry, index)
	ms.stats.TotalEntries++
	ms.stats.EntriesByType[entry.Type]++
	ms.stats.StorageSize += int64(len(entry.ID))

	return nil
}

// GetEntry returns a single entry by ID and updates its access metadata.
func (ms *Catalog) GetEntry(id string) (*Entry, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for i := range ms.entries {
		if ms.entries[i].ID == id {
			ms.updateAccessStats(&ms.entries[i])
			entry := ms.entries[i]
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("memory entry not found: %s", id)
}

// DeleteEntry removes a single entry by ID.
func (ms *Catalog) DeleteEntry(id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for i, entry := range ms.entries {
		if entry.ID != id {
			continue
		}
		ms.entries = append(ms.entries[:i], ms.entries[i+1:]...)
		ms.rebuildIndex()
		ms.recomputeStatsLocked()
		return nil
	}

	return fmt.Errorf("memory entry not found: %s", id)
}

// Entries returns a snapshot of the current catalog entries.
func (ms *Catalog) Entries() []Entry {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	entries := make([]Entry, len(ms.entries))
	copy(entries, ms.entries)
	return entries
}

// ResetEntries replaces catalog entries with a provided snapshot.
func (ms *Catalog) ResetEntries(entries []Entry) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.entries = make([]Entry, len(entries))
	copy(ms.entries, entries)
	ms.rebuildIndex()
	ms.recomputeStatsLocked()
}

// ToolUsageSnapshot returns a deep copy of learned tool usage patterns.
func (ms *Catalog) ToolUsageSnapshot() map[string]*ToolUsageMemory {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	snapshot := make(map[string]*ToolUsageMemory, len(ms.toolUsage))
	for name, usage := range ms.toolUsage {
		snapshot[name] = cloneToolUsageMemory(usage)
	}
	return snapshot
}

// ResetToolUsage replaces the learned tool usage state.
func (ms *Catalog) ResetToolUsage(toolUsage map[string]*ToolUsageMemory) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.toolUsage = make(map[string]*ToolUsageMemory, len(toolUsage))
	for name, usage := range toolUsage {
		ms.toolUsage[name] = cloneToolUsageMemory(usage)
	}
}

// GetToolUsagePatterns returns learned usage for one tool.
func (ms *Catalog) GetToolUsagePatterns(toolName string) (*ToolUsageMemory, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	toolMemory, ok := ms.toolUsage[toolName]
	if !ok {
		return nil, fmt.Errorf("tool usage memory not found: %s", toolName)
	}

	cloned := *toolMemory
	cloned.SuccessfulParameters = append([]ParameterPattern(nil), toolMemory.SuccessfulParameters...)
	cloned.FailedParameters = append([]ParameterPattern(nil), toolMemory.FailedParameters...)
	return &cloned, nil
}

// Stats returns current memory statistics.
func (ms *Catalog) Stats() MemoryStats {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.computeStatsLocked()
}

// Export exports all entries as JSON.
func (ms *Catalog) Export() ([]byte, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return json.MarshalIndent(ms.entries, "", "  ")
}

// Import imports entries from JSON.
func (ms *Catalog) Import(data []byte) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to unmarshal memory data: %w", err)
	}

	ms.entries = append(ms.entries, entries...)

	// Rebuild index
	ms.rebuildIndex()

	ms.recomputeStatsLocked()

	return nil
}

// ============================================================================
// Helper functions
// ============================================================================

// containsMemoryType checks if a type is in the list
func (ms *Catalog) containsMemoryType(types []MemoryType, target MemoryType) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}

// matchesContent checks if entry content matches query
func (ms *Catalog) matchesContent(entry Entry, content string, exactMatch bool) bool {
	if exactMatch {
		return entry.Content == content
	}
	return strings.Contains(strings.ToLower(entry.Content), strings.ToLower(content))
}

// matchesKeywords checks if entry matches any keywords
func (ms *Catalog) matchesKeywords(entry Entry, keywords []string) bool {
	entryLower := strings.ToLower(entry.Content)
	for _, keyword := range keywords {
		if strings.Contains(entryLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (ms *Catalog) matchesTags(entry Entry, tags []string) bool {
	for _, tag := range tags {
		if ms.containsString(entry.Tags, tag) {
			return true
		}
	}
	return false
}

// containsString checks if a string slice contains a target string
func (ms *Catalog) containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// extractKeywords extracts keywords from content
func (ms *Catalog) extractKeywords(content string) []string {
	keywords := []string{}
	currentWord := ""

	for _, char := range content {
		if char == ' ' || char == '\t' || char == '\n' || char == ',' || char == '.' {
			if len(currentWord) > 3 {
				keywords = append(keywords, strings.ToLower(currentWord))
			}
			currentWord = ""
		} else {
			currentWord += string(char)
		}
	}

	if len(currentWord) > 3 {
		keywords = append(keywords, strings.ToLower(currentWord))
	}

	return keywords
}

// removeFromIndex removes an entry from the index
func (ms *Catalog) removeFromIndex(entry Entry, index int) {
	// Remove from content index
	for _, keyword := range ms.extractKeywords(entry.Content) {
		ms.removeFromSlice(ms.index.contentIndex[keyword], index)
	}

	// Remove from tag index
	for _, tag := range entry.Tags {
		ms.removeFromSlice(ms.index.tagIndex[tag], index)
	}

	// Remove from type index
	ms.removeFromSlice(ms.index.typeIndex[entry.Type], index)

	// Remove from session index
	if entry.SessionID != "" {
		ms.removeFromSlice(ms.index.sessionIndex[entry.SessionID], index)
	}

	// Remove from tool index
	if entry.Metadata != nil && entry.Metadata.ToolUsed != "" {
		ms.removeFromSlice(ms.index.toolIndex[entry.Metadata.ToolUsed], index)
	}
}

// removeFromSlice removes an index from a slice
func (ms *Catalog) removeFromSlice(slice []int, index int) []int {
	for i, idx := range slice {
		if idx == index {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// rebuildIndex rebuilds entire search index
func (ms *Catalog) rebuildIndex() {
	ms.index = &MemoryIndex{
		contentIndex: make(map[string][]int),
		tagIndex:     make(map[string][]int),
		typeIndex:    make(map[MemoryType][]int),
		sessionIndex: make(map[string][]int),
		toolIndex:    make(map[string][]int),
	}

	for i, entry := range ms.entries {
		ms.updateIndex(entry, i)
	}
}

func (ms *Catalog) recomputeStatsLocked() {
	ms.stats = ms.computeStatsLocked()
}

func (ms *Catalog) computeStatsLocked() MemoryStats {
	// Snapshot query-level counters that are modified outside the data lock.
	ms.statsMu.Lock()
	totalQueries := ms.stats.TotalQueries
	successfulRetrievals := ms.stats.SuccessfulRetrievals
	missedRetrievals := ms.stats.MissedRetrievals
	queryLatency := ms.stats.QueryLatency
	storageSize := ms.stats.StorageSize
	ms.statsMu.Unlock()

	stats := MemoryStats{
		EntriesByType:        make(map[MemoryType]int),
		TotalQueries:         totalQueries,
		SuccessfulRetrievals: successfulRetrievals,
		MissedRetrievals:     missedRetrievals,
		QueryLatency:         queryLatency,
		StorageSize:          storageSize,
		MostAccessed:         []string{},
	}

	var totalImportance float64
	var totalConfidence float64
	type accessPair struct {
		id    string
		count int
	}
	mostAccessed := make([]accessPair, 0, len(ms.entries))

	for _, entry := range ms.entries {
		stats.TotalEntries++
		stats.EntriesByType[entry.Type]++
		stats.TotalAccessCount += int64(entry.AccessCount)
		totalImportance += entry.Importance
		totalConfidence += entry.Confidence
		mostAccessed = append(mostAccessed, accessPair{id: entry.ID, count: entry.AccessCount})

		createdAt := entry.CreatedAt
		if stats.OldestEntry == nil || createdAt.Before(*stats.OldestEntry) {
			ts := createdAt
			stats.OldestEntry = &ts
		}
		if stats.NewestEntry == nil || createdAt.After(*stats.NewestEntry) {
			ts := createdAt
			stats.NewestEntry = &ts
		}
	}

	if stats.TotalEntries > 0 {
		stats.AverageImportance = totalImportance / float64(stats.TotalEntries)
		stats.AverageConfidence = totalConfidence / float64(stats.TotalEntries)
	}

	sort.SliceStable(mostAccessed, func(i, j int) bool {
		return mostAccessed[i].count > mostAccessed[j].count
	})
	limit := 5
	if len(mostAccessed) < limit {
		limit = len(mostAccessed)
	}
	for i := 0; i < limit; i++ {
		stats.MostAccessed = append(stats.MostAccessed, mostAccessed[i].id)
	}

	return stats
}

// mergeParameterPattern merges a new pattern into existing patterns
func (ms *Catalog) mergeParameterPattern(patterns []ParameterPattern, newPattern *ParameterPattern) []ParameterPattern {
	// Look for similar existing pattern
	for i := range patterns {
		if ms.compareParameters(patterns[i].Parameters, newPattern.Parameters) {
			// Similar pattern found, increase frequency
			patterns[i].Frequency++
			patterns[i].LastUsed = newPattern.LastUsed
			return patterns
		}
	}

	// No similar pattern found, add new one
	return append(patterns, *newPattern)
}

// compareParameters compares two parameter maps
func (ms *Catalog) compareParameters(p1, p2 map[string]any) bool {
	if len(p1) != len(p2) {
		return false
	}

	for key, val1 := range p1 {
		val2, exists := p2[key]
		if !exists || val1 != val2 {
			return false
		}
	}

	return true
}

// countSuccessPatterns counts successful patterns
func (ms *Catalog) countSuccessPatterns(patterns []ParameterPattern) int {
	count := 0
	for _, pattern := range patterns {
		if pattern.Success {
			count += pattern.Frequency
		}
	}
	return count
}

// generateEntryID generates a unique entry ID
func generateEntryID() string {
	now := time.Now()
	return fmt.Sprintf("mem_%d_%d", now.UnixNano(), now.Nanosecond())
}

func cloneToolUsageMemory(usage *ToolUsageMemory) *ToolUsageMemory {
	if usage == nil {
		return nil
	}

	cloned := *usage
	cloned.SuccessfulParameters = cloneParameterPatterns(usage.SuccessfulParameters)
	cloned.FailedParameters = cloneParameterPatterns(usage.FailedParameters)
	return &cloned
}

func cloneParameterPatterns(patterns []ParameterPattern) []ParameterPattern {
	if len(patterns) == 0 {
		return nil
	}

	cloned := make([]ParameterPattern, len(patterns))
	for i, pattern := range patterns {
		cloned[i] = pattern
		if pattern.Parameters != nil {
			params := make(map[string]any, len(pattern.Parameters))
			for key, value := range pattern.Parameters {
				params[key] = value
			}
			cloned[i].Parameters = params
		}
	}
	return cloned
}
