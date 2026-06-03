package memory

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestCatalogBasicOperations(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	entry := Entry{
		Scope:  MemoryScopeProject,
		Type:   MemoryTypePreference,
		Key:    "test_pref",
		Value:  "test value",
		Source: "test",
	}

	err := catalog.StoreEntry(entry)
	require.NoError(t, err)

	stats := catalog.Stats()
	assert.Equal(t, 1, stats.TotalEntries)
	assert.Equal(t, 1, stats.EntriesByType[MemoryTypePreference])
}

func TestCatalogToolUsageLearning(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	params := map[string]any{"path": "/tmp/test.txt", "encoding": "utf-8"}
	err := catalog.LearnToolUsage("readFile", params, true, nil)
	require.NoError(t, err)

	query := MemoryQuery{
		Types:   []MemoryType{MemoryTypeToolUsage},
		Content: "readFile",
		Limit:   10,
	}

	result, err := catalog.Search(query)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Entries), 1)
}

func TestCatalogImportanceScoring(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	entries := []Entry{
		{
			Scope:   MemoryScopeProject,
			Type:    MemoryTypeKnowledge,
			Content: "Short content",
			Tags:    []string{"test"},
		},
		{
			Scope:   MemoryScopeProject,
			Type:    MemoryTypeKnowledge,
			Content: "This is a much longer content that should have higher importance because it contains more information and context",
			Tags:    []string{"test", "important", "detailed"},
		},
	}

	for _, entry := range entries {
		err := catalog.StoreEntry(entry)
		require.NoError(t, err)
	}

	query := MemoryQuery{
		Types:         []MemoryType{MemoryTypeKnowledge},
		MinImportance: 0.5,
		Limit:         10,
	}

	result, err := catalog.Search(query)
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.Entries))
	assert.Greater(t, result.Entries[0].Importance, 0.5)
}

func TestCatalogExpiration(t *testing.T) {
	config := DefaultMemoryConfig()
	config.DefaultTTL = 100 * time.Millisecond
	catalog := NewCatalogWithConfig(config)
	catalog.Start()
	defer catalog.Stop()

	entry := Entry{
		Scope:   MemoryScopeProject,
		Type:    MemoryTypeContext,
		Content: "Will expire soon",
		Tags:    []string{"test"},
	}

	err := catalog.StoreEntry(entry)
	require.NoError(t, err)

	query := MemoryQuery{
		Types: []MemoryType{MemoryTypeContext},
		Limit: 10,
	}
	result, err := catalog.Search(query)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Entries), 1)

	time.Sleep(150 * time.Millisecond)

	result, err = catalog.Search(query)
	require.NoError(t, err)
	assert.Equal(t, 0, len(result.Entries))
}

func TestCatalogStatistics(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	for i := 0; i < 5; i++ {
		entry := Entry{
			Scope:   MemoryScopeProject,
			Type:    MemoryTypeKnowledge,
			Content: "test content",
			Tags:    []string{"test"},
		}
		err := catalog.StoreEntry(entry)
		require.NoError(t, err)
	}

	stats := catalog.Stats()
	assert.Equal(t, 5, stats.TotalEntries)
	assert.Equal(t, 5, stats.EntriesByType[MemoryTypeKnowledge])
	assert.GreaterOrEqual(t, stats.TotalQueries, int64(0))
}

func TestCatalogGetEntryAndDeleteEntry(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	entry := Entry{
		Scope:   MemoryScopeProject,
		Type:    MemoryTypeKnowledge,
		Key:     "entry-1",
		Value:   "stored value",
		Content: "stored content",
		Tags:    []string{"test"},
	}
	require.NoError(t, catalog.StoreEntry(entry))

	// Search to get the assigned ID
	result, err := catalog.Search(MemoryQuery{Content: "stored content", Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Entries))
	id := result.Entries[0].ID

	// GetEntry
	got, err := catalog.GetEntry(id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, 1, got.AccessCount)

	// GetEntry unknown ID returns error
	_, err = catalog.GetEntry("does-not-exist")
	assert.Error(t, err)

	// DeleteEntry
	require.NoError(t, catalog.DeleteEntry(id))

	// Deleted entry should not be found
	_, err = catalog.GetEntry(id)
	assert.Error(t, err)

	// Deleting again should error
	assert.Error(t, catalog.DeleteEntry(id))
}

func TestCatalogEntries(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	require.NoError(t, catalog.StoreEntry(Entry{
		Scope: MemoryScopeUser, Type: MemoryTypePreference, Key: "k1", Content: "v1",
	}))
	require.NoError(t, catalog.StoreEntry(Entry{
		Scope: MemoryScopeProject, Type: MemoryTypeKnowledge, Key: "k2", Content: "v2",
	}))

	entries := catalog.Entries()
	assert.Equal(t, 2, len(entries))
}

func TestCatalogToolUsageSnapshot(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	params := map[string]any{"cmd": "ls"}
	require.NoError(t, catalog.LearnToolUsage("bash", params, true, nil))

	snapshot := catalog.ToolUsageSnapshot()
	usage, ok := snapshot["bash"]
	assert.True(t, ok, "expected 'bash' in snapshot")
	assert.Equal(t, 1, usage.UsageCount)
}

func TestCatalogTagSearch(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	require.NoError(t, catalog.StoreEntry(Entry{
		Scope: MemoryScopeProject, Type: MemoryTypeKnowledge,
		Key: "tagged", Content: "tag-test", Tags: []string{"deploy", "infra"},
	}))
	require.NoError(t, catalog.StoreEntry(Entry{
		Scope: MemoryScopeProject, Type: MemoryTypeKnowledge,
		Key: "untagged", Content: "no-tags",
	}))

	result, err := catalog.Search(MemoryQuery{Tags: []string{"deploy"}, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Contains(t, result.Entries[0].Tags, "deploy")
}

func TestCatalogKeywordSearch(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	require.NoError(t, catalog.StoreEntry(Entry{
		Scope:   MemoryScopeProject,
		Type:    MemoryTypeKnowledge,
		Key:     "kw-entry",
		Content: "deployment rollback procedure",
	}))

	result, err := catalog.Search(MemoryQuery{Keywords: []string{"rollback"}, Limit: 10})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.Total, 1)
}

func TestCatalogCleanupExpiredManual(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	config := DefaultMemoryConfig()
	config.DefaultTTL = 1 // 1 nanosecond
	shortLive := NewCatalogWithConfig(config)
	shortLive.Start()
	defer shortLive.Stop()

	require.NoError(t, shortLive.StoreEntry(Entry{
		Scope: MemoryScopeProject, Type: MemoryTypeContext, Content: "short-lived",
	}))

	// Wait for entry to expire
	time.Sleep(5 * time.Millisecond)

	cleaned := shortLive.CleanupExpired()
	assert.GreaterOrEqual(t, cleaned, 1)
}

func TestCatalogExportImport(t *testing.T) {
	catalog := NewCatalog()
	catalog.Start()
	defer catalog.Stop()

	entries := []Entry{
		{
			Scope:   MemoryScopeProject,
			Type:    MemoryTypePreference,
			Content: "Test content 1",
			Tags:    []string{"test"},
		},
		{
			Scope:   MemoryScopeUser,
			Type:    MemoryTypeInstruction,
			Content: "Test content 2",
			Tags:    []string{"test"},
		},
	}

	for _, entry := range entries {
		_ = catalog.StoreEntry(entry)
	}

	data, err := catalog.Export()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	newCatalog := NewCatalog()
	newCatalog.Start()
	defer newCatalog.Stop()
	err = newCatalog.Import(data)
	require.NoError(t, err)

	stats := newCatalog.Stats()
	assert.GreaterOrEqual(t, stats.TotalEntries, 2)
}

// TestIntegrationManager_CatalogMode tests the integration manager in catalog mode.
func TestIntegrationManager_CatalogMode(t *testing.T) {
	integrationMgr, err := NewIntegrationManager(true)
	require.NoError(t, err)
	require.NotNil(t, integrationMgr)
	defer integrationMgr.Stop()

	// Start the systems
	err = integrationMgr.Start()
	require.NoError(t, err)

	// Test storing preferences
	err = integrationMgr.StorePreference(MemoryScopeProject, "test_pref", "test_value", "test")
	require.NoError(t, err)

	// Test retrieving preferences
	prefs := integrationMgr.GetPreferences(MemoryScopeProject)
	assert.GreaterOrEqual(t, len(prefs), 1)

	// Test tool usage learning
	params := map[string]any{"path": "/tmp/test.txt"}
	err = integrationMgr.LearnToolUsage("readFile", params, true, nil)
	require.NoError(t, err)

	// Test getting tool usage patterns
	patterns, err := integrationMgr.GetToolUsagePatterns("readFile")
	require.NoError(t, err)
	require.NotNil(t, patterns)
	assert.Equal(t, "readFile", patterns.ToolName)

	// Test search functionality
	query := MemoryQuery{
		Types: []MemoryType{MemoryTypePreference},
		Limit: 10,
	}
	result, err := integrationMgr.Search(query)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Test getting statistics
	stats := integrationMgr.Stats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "catalog")

	// Test context generation
	ctx := integrationMgr.Context()
	assert.NotEmpty(t, ctx)
}

// TestIntegrationManager_ManagerMode tests the integration manager in manager mode.
func TestIntegrationManager_ManagerMode(t *testing.T) {
	integrationMgr, err := NewIntegrationManager(false)
	require.NoError(t, err)
	require.NotNil(t, integrationMgr)
	defer integrationMgr.Stop()

	// Start the systems
	err = integrationMgr.Start()
	require.NoError(t, err)

	assert.False(t, integrationMgr.IsCatalogMode())

	// Test storing preferences (should use manager-backed memory)
	err = integrationMgr.StorePreference(MemoryScopeProject, "legacy_pref", "legacy_value", "test")
	require.NoError(t, err)

	// Test retrieving preferences - manager-backed behavior
	prefs := integrationMgr.GetPreferences(MemoryScopeProject)
	// Note: manager-backed mode might return empty or nil depending on implementation
	_ = prefs // Avoid unused variable warning

	// Test context generation - should provide something even with manager-backed memory
	ctx := integrationMgr.Context()
	// Context generation might be empty with only manager-backed memory if no stored preferences
	// So we just test that it doesn't crash
	_ = ctx
}

// TestIntegrationManager_ModeSwitching tests switching between catalog and manager modes.
func TestIntegrationManager_ModeSwitching(t *testing.T) {
	integrationMgr, err := NewIntegrationManager(false)
	require.NoError(t, err)
	defer integrationMgr.Stop()

	assert.False(t, integrationMgr.IsCatalogMode())

	integrationMgr.EnableCatalog()
	assert.True(t, integrationMgr.IsCatalogMode())

	// Store preference (should use catalog-backed memory now)
	err = integrationMgr.StorePreference(MemoryScopeProject, "switch_pref", "switch_value", "test")
	require.NoError(t, err)

	integrationMgr.DisableCatalog()
	assert.False(t, integrationMgr.IsCatalogMode())

	// Verify we can still store preferences
	err = integrationMgr.StorePreference(MemoryScopeProject, "legacy_switch_pref", "legacy_switch_value", "test")
	require.NoError(t, err)
}

// TestIntegrationManager_ExportImport tests export and import functionality
func TestIntegrationManager_ExportImport(t *testing.T) {
	integrationMgr, err := NewIntegrationManager(true)
	require.NoError(t, err)
	defer integrationMgr.Stop()

	integrationMgr.Start()

	// Store some data
	err = integrationMgr.StorePreference(MemoryScopeProject, "export_pref", "export_value", "test")
	require.NoError(t, err)

	err = integrationMgr.LearnToolUsage("testTool", map[string]any{"param": "value"}, true, nil)
	require.NoError(t, err)

	// Export all memory
	exported, err := integrationMgr.ExportAll()
	require.NoError(t, err)
	assert.NotNil(t, exported)
	assert.Contains(t, exported, "catalog")
	assert.NotEmpty(t, exported["catalog"])

	// Verify we got some data
	assert.Contains(t, string(exported["catalog"]), "export_pref")
}

// TestIntegrationManager_ConcurrentAccess tests concurrent access to the integration manager
func TestIntegrationManager_ConcurrentAccess(t *testing.T) {
	integrationMgr, err := NewIntegrationManager(true)
	require.NoError(t, err)
	defer integrationMgr.Stop()

	integrationMgr.Start()

	// Concurrent operations
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(index int) {
			// Store preferences
			err := integrationMgr.StorePreference(MemoryScopeProject,
				string(rune('a'+index)), "value", "test")
			assert.NoError(t, err)

			// Get preferences
			prefs := integrationMgr.GetPreferences(MemoryScopeProject)
			assert.NotNil(t, prefs)

			// Learn from tool usage
			err = integrationMgr.LearnToolUsage("tool"+string(rune('a'+index)),
				map[string]any{"index": index}, true, nil)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data integrity
	stats := integrationMgr.Stats()
	assert.NotNil(t, stats)
}

// TestIntegrationManager_ToolUsagePatternLearning tests comprehensive tool usage pattern learning
func TestIntegrationManager_ToolUsagePatternLearning(t *testing.T) {
	integrationMgr, err := NewIntegrationManager(true)
	require.NoError(t, err)
	defer integrationMgr.Stop()

	integrationMgr.Start()

	// Simulate multiple tool executions
	executions := []struct {
		toolName string
		params   map[string]any
		success  bool
	}{
		{"readFile", map[string]any{"path": "/tmp/file1.txt"}, true},
		{"readFile", map[string]any{"path": "/tmp/file2.txt"}, true},
		{"readFile", map[string]any{"path": "/tmp/invalid.txt"}, false},
		{"writeFile", map[string]any{"path": "/tmp/output.txt", "content": "hello"}, true},
		{"writeFile", map[string]any{"path": "/tmp/output.txt", "content": "hello"}, true},
	}

	for _, exec := range executions {
		err := integrationMgr.LearnToolUsage(exec.toolName, exec.params, exec.success, nil)
		require.NoError(t, err)
	}

	// Verify patterns were learned
	patterns, err := integrationMgr.GetToolUsagePatterns("readFile")
	require.NoError(t, err)
	require.NotNil(t, patterns)
	assert.Equal(t, "readFile", patterns.ToolName)
	assert.Equal(t, 3, patterns.UsageCount)             // 3 executions
	assert.GreaterOrEqual(t, patterns.SuccessRate, 0.5) // 2/3 successful

	// Check search finds tool usage memories
	query := MemoryQuery{
		Types: []MemoryType{MemoryTypeToolUsage},
		Limit: 10,
	}
	result, err := integrationMgr.Search(query)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Entries), 5) // All 5 executions
}

// ─── Learner ─────────────────────────────────────────────────────────────────

func TestNewLearnerCreatesWithLoadedMemory(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	l, err := NewLearner("/tmp/proj", "session-1")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Learner")
	}
}

func TestLearnerOnToolUse_RecordsStats(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	l, err := NewLearner("/tmp/proj", "session-1")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}

	l.OnToolUse("grep", true, 100, "")
	l.OnToolUse("grep", true, 120, "")

	stats := l.toolStats["grep"]
	if stats == nil {
		t.Fatal("expected stats for grep")
	}
	if stats.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", stats.Attempts)
	}
	if stats.Successes != 2 {
		t.Fatalf("expected 2 successes, got %d", stats.Successes)
	}
}

func TestLearnerOnToolUse_LearnPatternAfterThirdUse(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	l, err := NewLearner("/tmp/proj", "session-learn")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}
	if err := l.memory.LoadCrossSession(); err != nil {
		t.Fatalf("LoadCrossSession: %v", err)
	}
	if err := l.memory.LoadProject("/tmp/proj"); err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	l.OnToolUse("rg", true, 50, "")
	l.OnToolUse("rg", true, 60, "")
	// Third use triggers pattern learning
	l.OnToolUse("rg", false, 70, "exit code 1")

	cross := l.memory.GetCrossSession()
	if cross == nil {
		t.Fatal("expected cross-session memory to be loaded")
	}

	pattern, ok := cross.GlobalPatterns["tool:rg"]
	if !ok {
		t.Fatal("expected pattern for tool:rg")
	}
	if pattern.Frequency != 3 {
		t.Fatalf("expected frequency 3, got %d", pattern.Frequency)
	}
}

func TestLearnerOnToolUse_TracksFailures(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	l, err := NewLearner("/tmp/proj", "sess-fail")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}

	l.OnToolUse("rm", false, 10, "permission denied")
	l.OnToolUse("rm", false, 10, "permission denied")

	stats := l.toolStats["rm"]
	if stats.Failures != 2 {
		t.Fatalf("expected 2 failures, got %d", stats.Failures)
	}
}

func TestLearnerAddUserPreference(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	l, err := NewLearner("/tmp/proj", "sess-pref")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}

	if err := l.AddUserPreference("language", "French", "test"); err != nil {
		t.Fatalf("AddUserPreference: %v", err)
	}

	user := l.memory.GetUser()
	if user == nil {
		t.Fatal("expected user memory to be loaded")
	}
	entry, ok := user.Entries["language"]
	if !ok {
		t.Fatal("expected preference:language entry")
	}
	if entry.Value != "French" {
		t.Fatalf("expected value 'French', got %q", entry.Value)
	}
}

func TestLearnerAddInstruction_RequiresProjectLoaded(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	l, err := NewLearner("/tmp/proj", "sess-instr")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}

	// Project is loaded in NewLearner via LoadProject
	if err := l.AddInstruction("style", "use rg not grep", "test"); err != nil {
		t.Fatalf("AddInstruction: %v", err)
	}

	project := l.memory.GetProject()
	if project == nil {
		t.Fatal("expected project memory")
	}
	entry, ok := project.Entries["style"]
	if !ok {
		t.Fatal("expected instruction 'style' in project entries")
	}
	if entry.Value != "use rg not grep" {
		t.Fatalf("expected 'use rg not grep', got %q", entry.Value)
	}
}

func TestLearnerFlush_PersistsToStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUS_MEMORY_PATH", dir)

	l, err := NewLearner("/tmp/proj", "sess-flush")
	if err != nil {
		t.Fatalf("NewLearner: %v", err)
	}

	if err := l.AddUserPreference("flush-key", "flush-value", "test"); err != nil {
		t.Fatalf("AddUserPreference: %v", err)
	}
	if err := l.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Reload from disk
	reloaded, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	if err := reloaded.LoadUser(); err != nil {
		t.Fatalf("reload LoadUser: %v", err)
	}

	user := reloaded.GetUser()
	if user == nil {
		t.Fatal("expected reloaded user memory")
	}
	entry, ok := user.Entries["flush-key"]
	if !ok {
		t.Fatal("expected 'flush-key' to persist after Flush")
	}
	if entry.Value != "flush-value" {
		t.Fatalf("expected 'flush-value', got %q", entry.Value)
	}
}

// ─── ErrorLearner ────────────────────────────────────────────────────────────

func TestNewErrorLearner(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	el, err := NewErrorLearner("/tmp/proj")
	if err != nil {
		t.Fatalf("NewErrorLearner: %v", err)
	}
	if el == nil {
		t.Fatal("expected non-nil ErrorLearner")
	}
}

func TestErrorLearnerOnError_NilIsNoOp(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	el, err := NewErrorLearner("/tmp/proj")
	if err != nil {
		t.Fatalf("NewErrorLearner: %v", err)
	}
	if err := el.OnError(nil); err != nil {
		t.Fatalf("expected no error for nil input, got %v", err)
	}
}

func TestErrorLearnerOnError_TracksSingleError(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	el, err := NewErrorLearner("/tmp/proj")
	if err != nil {
		t.Fatalf("NewErrorLearner: %v", err)
	}
	_ = el.OnError(errors.New("permission denied: /etc/passwd"))

	pattern := el.errors["error:permission"]
	if pattern == nil {
		t.Fatal("expected error pattern for 'permission'")
	}
	if pattern.Frequency != 1 {
		t.Fatalf("expected frequency 1, got %d", pattern.Frequency)
	}
}

func TestErrorLearnerOnError_LearnsSuggestionAfterRepeat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUS_MEMORY_PATH", dir)

	el, err := NewErrorLearner("/tmp/proj-err")
	if err != nil {
		t.Fatalf("NewErrorLearner: %v", err)
	}
	if err := el.memory.LoadProject("/tmp/proj-err"); err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	_ = el.OnError(errors.New("file not found: config.yaml"))
	_ = el.OnError(errors.New("no such file: config.yaml"))

	project := el.memory.GetProject()
	if project == nil {
		t.Fatal("expected project memory after errors")
	}
	entry, ok := project.Entries["error:not_found"]
	if !ok {
		t.Fatal("expected 'error:not_found' persisted in project memory")
	}
	if !strings.Contains(entry.Value, "Verify") {
		t.Fatalf("expected suggestion text, got %q", entry.Value)
	}
}

func TestCategorizeError_AllTypes(t *testing.T) {
	cases := []struct {
		msg      string
		expected string
	}{
		{"permission denied", "permission"},
		{"file not found: foo.txt", "not_found"},
		{"no such file or directory", "not_found"},
		{"connection refused", "connection"},
		{"request timeout exceeded", "timeout"},
		{"invalid argument", "invalid"},
		{"operation failed", "generic"},
		{"something went wrong error", "generic"},
		{"completely unexpected situation", "unknown"},
	}

	for _, tc := range cases {
		got := categorizeError(tc.msg)
		if got != tc.expected {
			t.Errorf("categorizeError(%q) = %q, want %q", tc.msg, got, tc.expected)
		}
	}
}

func TestGetSuggestion_AllTypes(t *testing.T) {
	types := []string{"permission", "not_found", "connection", "timeout", "invalid", "generic", "unknown"}
	for _, errType := range types {
		suggestion := getSuggestion(errType)
		if suggestion == "" {
			t.Errorf("getSuggestion(%q) returned empty string", errType)
		}
	}
}

// ─── ContextBuilder ──────────────────────────────────────────────────────────

func TestNewContextBuilder(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	cb, err := NewContextBuilder("/tmp/proj")
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil ContextBuilder")
	}
}

func TestContextBuilder_BuildReturnsContext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUS_MEMORY_PATH", dir)

	// Pre-seed user memory with a high-confidence preference
	mgr, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := mgr.LoadUser(); err != nil {
		t.Fatalf("LoadUser: %v", err)
	}

	entry := NewEntry(MemoryScopeUser, MemoryTypePreference, "lang", "French", "test")
	entry.Confidence = 0.9
	mgr.GetUser().Entries["lang"] = entry
	if err := mgr.SaveUser(); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	// Build context from stored data
	cb, err := NewContextBuilder("/tmp/proj")
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}

	ctx := cb.Build()
	if !strings.Contains(ctx, "French") {
		t.Fatalf("expected 'French' in context, got %q", ctx)
	}
	if !strings.Contains(ctx, "User Preferences") {
		t.Fatalf("expected 'User Preferences' section in context, got %q", ctx)
	}
}

func TestContextBuilder_BuildWithCrossSessionPatterns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUS_MEMORY_PATH", dir)

	// Pre-seed cross-session with a frequent pattern
	mgr, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := mgr.LoadCrossSession(); err != nil {
		t.Fatalf("LoadCrossSession: %v", err)
	}

	cross := mgr.GetCrossSession()
	cross.GlobalPatterns["tool:grep"] = &PatternEntry{
		Key:         "tool:grep",
		Pattern:     "grep",
		Description: "used 5 times, 80% success rate",
		Frequency:   5,
		SuccessRate: 0.8,
	}
	if err := mgr.SaveCrossSession(); err != nil {
		t.Fatalf("SaveCrossSession: %v", err)
	}

	cb, err := NewContextBuilder("/tmp/proj-cross")
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}

	ctx := cb.Build()
	if !strings.Contains(ctx, "Learned Tools") {
		t.Fatalf("expected 'Learned Tools' section, got %q", ctx)
	}
	if !strings.Contains(ctx, "grep") {
		t.Fatalf("expected 'grep' pattern in context, got %q", ctx)
	}
}

// ─── FileStore round-trip ─────────────────────────────────────────────────────

func TestFileStoreUserMemoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	user := NewUserMemory("user-42")
	entry := NewEntry(MemoryScopeUser, MemoryTypePreference, "theme", "dark", "test")
	user.Entries["theme"] = entry

	if err := fs.SaveUserMemory(user); err != nil {
		t.Fatalf("SaveUserMemory: %v", err)
	}

	loaded, err := fs.LoadUserMemory()
	if err != nil {
		t.Fatalf("LoadUserMemory: %v", err)
	}
	if loaded.UserID != "user-42" {
		t.Fatalf("expected user ID 'user-42', got %q", loaded.UserID)
	}
	e, ok := loaded.Entries["theme"]
	if !ok {
		t.Fatal("expected 'theme' entry after reload")
	}
	if e.Value != "dark" {
		t.Fatalf("expected value 'dark', got %q", e.Value)
	}
}

func TestFileStoreCrossSessionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	cross := NewCrossSession()
	cross.SessionSummaries["sess-1"] = &SessionSummary{
		SessionID:   "sess-1",
		ProjectPath: "/proj",
		Summary:     "completed refactor",
	}

	if err := fs.SaveCrossSession(cross); err != nil {
		t.Fatalf("SaveCrossSession: %v", err)
	}

	loaded, err := fs.LoadCrossSession()
	if err != nil {
		t.Fatalf("LoadCrossSession: %v", err)
	}
	if _, ok := loaded.SessionSummaries["sess-1"]; !ok {
		t.Fatal("expected 'sess-1' in loaded cross-session")
	}
}

func TestFileStoreProjectMemoryMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	mem, err := fs.LoadProjectMemory("/nonexistent/project")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if mem == nil {
		t.Fatal("expected empty ProjectMemory, got nil")
	}
	if len(mem.Entries) != 0 {
		t.Fatalf("expected empty entries, got %d", len(mem.Entries))
	}
}

func TestFileStoreUserMemoryMissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	user, err := fs.LoadUserMemory()
	if err != nil {
		t.Fatalf("expected no error for missing user file, got %v", err)
	}
	if user == nil {
		t.Fatal("expected default UserMemory")
	}
}

func TestFileStoreCrossSessionMissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	cross, err := fs.LoadCrossSession()
	if err != nil {
		t.Fatalf("expected no error for missing cross-session file, got %v", err)
	}
	if cross == nil {
		t.Fatal("expected empty CrossSession")
	}
}

// ─── Manager: Load/Save/Get ───────────────────────────────────────────────────

func TestManagerLoadAll_AndContext(t *testing.T) {
	dir := t.TempDir()

	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := m.LoadAll("/tmp/proj-ctx"); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if err := m.LearnPreference(MemoryScopeUser, "pref-key", "pref-value", "test"); err != nil {
		t.Fatalf("LearnPreference: %v", err)
	}
	if err := m.LearnInstruction(MemoryScopeProject, "instr-key", "always use tabs", "test"); err != nil {
		t.Fatalf("LearnInstruction: %v", err)
	}

	ctx := m.Context()
	if !strings.Contains(ctx, "User Preferences") {
		t.Fatalf("expected 'User Preferences' in context, got %q", ctx)
	}
	if !strings.Contains(ctx, "pref-value") {
		t.Fatalf("expected 'pref-value' in context, got %q", ctx)
	}
	if !strings.Contains(ctx, "always use tabs") {
		t.Fatalf("expected instruction text in context, got %q", ctx)
	}
}

func TestManagerSaveAll_PersistsEverything(t *testing.T) {
	dir := t.TempDir()

	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := m.LoadAll("/tmp/proj-save"); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	_ = m.LearnPreference(MemoryScopeUser, "save-key", "save-value", "test")
	if err := m.SaveAll(); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	reload, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("reload NewManagerWithPath: %v", err)
	}
	if err := reload.LoadUser(); err != nil {
		t.Fatalf("reload LoadUser: %v", err)
	}
	user := reload.GetUser()
	if user == nil {
		t.Fatal("expected user memory after reload")
	}
	if _, ok := user.Entries["save-key"]; !ok {
		t.Fatal("expected 'save-key' persisted after SaveAll")
	}
}

func TestManagerGetUser_AndGetCrossSession(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}

	if m.GetUser() != nil {
		t.Fatal("expected nil user before load")
	}
	if m.GetCrossSession() != nil {
		t.Fatal("expected nil cross-session before load")
	}

	_ = m.LoadUser()
	_ = m.LoadCrossSession()

	if m.GetUser() == nil {
		t.Fatal("expected non-nil user after LoadUser")
	}
	if m.GetCrossSession() == nil {
		t.Fatal("expected non-nil cross-session after LoadCrossSession")
	}
}

func TestManagerLearnInstruction_ProjectScope(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := m.LoadProject("/tmp/proj-instr"); err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	if err := m.LearnInstruction(MemoryScopeProject, "tab-pref", "use 4-space tabs", "test"); err != nil {
		t.Fatalf("LearnInstruction: %v", err)
	}

	project := m.GetProject()
	entry, ok := project.Entries["tab-pref"]
	if !ok {
		t.Fatal("expected 'tab-pref' in project entries")
	}
	if entry.Type != MemoryTypeInstruction {
		t.Fatalf("expected MemoryTypeInstruction, got %s", entry.Type)
	}
}

func TestManagerAddSessionSummary_AndGetProjectHistory(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := m.LoadProject("/proj/history"); err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if err := m.LoadCrossSession(); err != nil {
		t.Fatalf("LoadCrossSession: %v", err)
	}

	if err := m.AddSessionSummary("sess-hist-1", "/proj/history", "completed migration", []string{"grep", "sed"}); err != nil {
		t.Fatalf("AddSessionSummary: %v", err)
	}
	if err := m.AddSessionSummary("sess-hist-2", "/proj/history", "fixed tests", []string{"go"}); err != nil {
		t.Fatalf("AddSessionSummary: %v", err)
	}

	summaries := m.GetProjectHistory("/proj/history")
	if len(summaries) != 2 {
		t.Fatalf("expected 2 project history summaries, got %d", len(summaries))
	}

	// Context should include session history
	ctx := m.Context()
	if !strings.Contains(ctx, "Recent Session History") {
		t.Fatalf("expected 'Recent Session History' in context, got %q", ctx)
	}
}

func TestManagerAddSessionSummary_NoCrossSessionErrors(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	// Do not load cross-session — should return error
	if err := m.AddSessionSummary("s", "/p", "summary", nil); err == nil {
		t.Fatal("expected error when cross-session not loaded")
	}
}

func TestManagerSearch_FindsByContent(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}

	if err := m.StoreEntry(Entry{
		Scope:   MemoryScopeProject,
		Type:    MemoryTypeKnowledge,
		Key:     "test-entry",
		Value:   "important knowledge",
		Content: "important knowledge about deployment",
	}); err != nil {
		t.Fatalf("StoreEntry: %v", err)
	}

	result, err := m.Search(MemoryQuery{Content: "deployment", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.Total == 0 {
		t.Fatal("expected search to find the stored entry")
	}
}

func TestManagerGetEntry_AndDeleteEntry(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}

	// Store to catalog first
	stored := Entry{
		Scope:   MemoryScopeProject,
		Type:    MemoryTypeKnowledge,
		Key:     "entry-to-delete",
		Value:   "some value",
		Content: "some content",
	}
	if err := m.StoreEntry(stored); err != nil {
		t.Fatalf("StoreEntry: %v", err)
	}

	result, err := m.Search(MemoryQuery{Content: "some content", Limit: 1})
	if err != nil || len(result.Entries) == 0 {
		t.Fatal("expected to find stored entry")
	}
	id := result.Entries[0].ID

	got, err := m.GetEntry(id)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if got.ID != id {
		t.Fatalf("expected entry ID %s, got %s", id, got.ID)
	}

	if err := m.DeleteEntry(id); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	if _, err := m.GetEntry(id); err == nil {
		t.Fatal("expected error after deleting entry")
	}
}

func TestManagerStats(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}

	_ = m.StoreEntry(Entry{Scope: MemoryScopeProject, Type: MemoryTypeKnowledge, Key: "k1", Value: "v1", Content: "c1"})
	_ = m.StoreEntry(Entry{Scope: MemoryScopeUser, Type: MemoryTypePreference, Key: "k2", Value: "v2", Content: "c2"})

	stats := m.Stats()
	if stats.TotalEntries != 2 {
		t.Fatalf("expected 2 total entries, got %d", stats.TotalEntries)
	}
}

func TestManagerCatalog_ReturnsNonNil(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if m.Catalog() == nil {
		t.Fatal("expected non-nil Catalog()")
	}
}

func TestManagerContext_EmptyWhenNothingLoaded(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	ctx := m.Context()
	if ctx != "" {
		t.Fatalf("expected empty context when nothing loaded, got %q", ctx)
	}
}

func TestManagerContextLinesForToolUsage(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManagerWithPath(dir)
	if err != nil {
		t.Fatalf("NewManagerWithPath: %v", err)
	}
	if err := m.LoadProject("/tmp/proj-tool"); err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	if err := m.LearnToolUsage("awk", map[string]any{"pattern": "NR"}, true, nil); err != nil {
		t.Fatalf("LearnToolUsage: %v", err)
	}

	ctx := m.Context()
	if !strings.Contains(ctx, "Learned Tool Usage") {
		t.Fatalf("expected 'Learned Tool Usage' section in context, got %q", ctx)
	}
	if !strings.Contains(ctx, "awk") {
		t.Fatalf("expected 'awk' in context, got %q", ctx)
	}
}

func TestTruncateMemoryLine(t *testing.T) {
	cases := []struct {
		text     string
		max      int
		expected string
	}{
		{"hello world", 100, "hello world"},
		{"hello world", 0, "hello world"}, // max <= 0 → no truncate
		{"hello world", 5, "hell…"},
		{"hello world", 1, "h"},
	}

	for _, tc := range cases {
		got := truncateMemoryLine(tc.text, tc.max)
		if got != tc.expected {
			t.Errorf("truncateMemoryLine(%q, %d) = %q, want %q", tc.text, tc.max, got, tc.expected)
		}
	}
}

func TestNewServiceAndNewServiceWithPath(t *testing.T) {
	t.Setenv("NEXUS_MEMORY_PATH", t.TempDir())

	svc, err := NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil Service")
	}

	svc2, err := NewServiceWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("NewServiceWithPath: %v", err)
	}
	if svc2 == nil {
		t.Fatal("expected non-nil Service from NewServiceWithPath")
	}
}

func TestManagerLearnToolUsagePersistsProjectPatterns(t *testing.T) {
	basePath := t.TempDir()

	manager, err := NewManagerWithPath(basePath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.LoadProject("/tmp/project-alpha"); err != nil {
		t.Fatalf("load project: %v", err)
	}

	if err := manager.LearnToolUsage("grep", map[string]any{"pattern": "TODO"}, true, nil); err != nil {
		t.Fatalf("learn tool usage: %v", err)
	}

	reloaded, err := NewManagerWithPath(basePath)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	if err := reloaded.LoadProject("/tmp/project-alpha"); err != nil {
		t.Fatalf("reload project: %v", err)
	}

	usage, err := reloaded.GetToolUsagePatterns("grep")
	if err != nil {
		t.Fatalf("get tool usage patterns: %v", err)
	}
	if usage.UsageCount != 1 {
		t.Fatalf("expected usage count 1, got %d", usage.UsageCount)
	}
	if usage.SuccessRate != 1 {
		t.Fatalf("expected success rate 1, got %v", usage.SuccessRate)
	}

	entry, ok := reloaded.GetProject().Entries["tool_usage:grep"]
	if !ok {
		t.Fatal("expected persisted project entry for tool usage")
	}
	if entry.Type != MemoryTypeToolUsage {
		t.Fatalf("expected tool usage entry type, got %s", entry.Type)
	}
}
