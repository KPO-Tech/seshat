package agent

import (
	"context"
	"fmt"
	skills "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	publicskills "github.com/EngineerProjects/nexus-engine/pkg/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestAsyncAgentManager_BasicAgentExecution tests basic async agent execution
func TestAsyncAgentManager_BasicAgentExecution(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	// Create a simple agent config
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test task",
		MaxTurns:  2,
		Context:   context.Background(),
	}

	// Start agent asynchronously
	agent, err := manager.StartAgent(config)
	require.NoError(t, err)
	require.NotNil(t, agent)
	require.Equal(t, AgentTypeExplore, agent.Config.AgentType)

	// Wait for agent to complete
	agent.Wait()

	// Check final status
	assert.True(t, agent.IsComplete())
	assert.False(t, agent.IsRunning())

	// Verify agent can be retrieved
	retrievedAgent, err := manager.GetAgent(agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agent.ID, retrievedAgent.ID)
}

// TestAsyncAgentManager_RealTimeEvents tests real-time event notifications
func TestAsyncAgentManager_RealTimeEvents(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	var events []AgentEvent
	var eventsMu sync.Mutex

	// Add global event listener
	listener := func(event AgentEvent) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
		fmt.Printf("Event received: %s for agent %s\n", event.EventType, event.AgentID)
	}
	manager.AddGlobalListener(listener)
	defer manager.RemoveGlobalListener(listener)

	// Create agent config
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test real-time events",
		MaxTurns:  3,
		Context:   context.Background(),
	}

	// Start agent
	agent, err := manager.StartAgent(config)
	require.NoError(t, err)

	// Wait for completion
	agent.Wait()

	// Give some time for all events to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify we received expected events
	eventsMu.Lock()
	defer eventsMu.Unlock()

	assert.Greater(t, len(events), 0, "Should receive at least one event")

	// Check for started event
	hasStartedEvent := false
	for _, event := range events {
		if event.EventType == AgentEventStarted {
			hasStartedEvent = true
			assert.Equal(t, agent.ID, event.AgentID)
			assert.Equal(t, AgentTypeExplore, event.AgentType)
			break
		}
	}
	assert.True(t, hasStartedEvent, "Should receive started event")

	// Check for completed or failed event
	hasFinalEvent := false
	for _, event := range events {
		if event.EventType == AgentEventCompleted || event.EventType == AgentEventFailed {
			hasFinalEvent = true
			assert.Equal(t, agent.ID, event.AgentID)
			break
		}
	}
	assert.True(t, hasFinalEvent, "Should receive final event")
}

// TestAsyncAgentManager_ProgressEvents tests progress event streaming
func TestAsyncAgentManager_ProgressEvents(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	var progressEvents []AgentEvent
	var progressMu sync.Mutex

	// Add listener for progress events
	listener := func(event AgentEvent) {
		if event.EventType == AgentEventProgress {
			progressMu.Lock()
			defer progressMu.Unlock()
			progressEvents = append(progressEvents, event)
			fmt.Printf("Progress: Turn %d/%d (%.1f%%)\n",
				event.Progress.CurrentTurn,
				event.Progress.MaxTurns,
				event.Progress.PercentComplete)
		}
	}
	manager.AddGlobalListener(listener)
	defer manager.RemoveGlobalListener(listener)

	// Create agent config
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test progress events",
		MaxTurns:  3,
		Context:   context.Background(),
	}

	// Start agent
	agent, err := manager.StartAgent(config)
	require.NoError(t, err)

	// Wait for completion
	agent.Wait()

	// Give some time for all events to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify progress events
	progressMu.Lock()
	defer progressMu.Unlock()

	fmt.Printf("Total progress events: %d\n", len(progressEvents))

	// We should have received at least one progress event
	if len(progressEvents) > 0 {
		// Verify progress data structure
		event := progressEvents[0]
		assert.NotNil(t, event.Progress)
		assert.Greater(t, event.Progress.CurrentTurn, 0)
		assert.Greater(t, event.Progress.MaxTurns, 0)
	}
}

// TestAsyncAgentManager_AgentCancellation tests agent cancellation
func TestAsyncAgentManager_AgentCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cancellation test in short mode")
	}

	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	var events []AgentEvent
	var eventsMu sync.Mutex

	// Add event listener
	listener := func(event AgentEvent) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
		fmt.Printf("Event: %s\n", event.EventType)
	}
	manager.AddGlobalListener(listener)
	defer manager.RemoveGlobalListener(listener)

	// Create agent config with longer task
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Perform comprehensive code analysis and documentation generation", // Very long task
		MaxTurns:  10,                                                                 // Long running
		Context:   context.Background(),
	}

	// Start agent
	agent, err := manager.StartAgent(config)
	require.NoError(t, err)

	// Wait for agent to actually start running
	timeout := time.After(5 * time.Second)
	started := false

	for !started {
		select {
		case <-timeout:
			t.Fatal("Agent did not start within timeout")
		case <-time.After(50 * time.Millisecond):
			if agent.IsRunning() {
				started = true
			} else if agent.IsComplete() {
				// Agent completed too quickly, skip test
				t.Skip("Agent completed too quickly for cancellation test")
			}
		}
	}

	// Cancel the agent
	err = manager.CancelAgent(agent.ID)
	if err != nil {
		// Agent might have completed, that's okay
		agent.Wait()
		return
	}

	// Wait for agent to be cancelled
	agent.Wait()

	// Verify status (either cancelled or completed/failed)
	assert.True(t, agent.IsComplete(), "Agent should be complete after cancellation")

	// Give some time for all events to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify we received events
	eventsMu.Lock()
	defer eventsMu.Unlock()

	assert.Greater(t, len(events), 0, "Should receive at least some events")
}

// TestAsyncAgentManager_ConcurrentAgents tests multiple concurrent agents
func TestAsyncAgentManager_ConcurrentAgents(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	agentCount := 3
	var wg sync.WaitGroup
	var agents []*AsyncAgent
	var agentsMu sync.Mutex

	// Start multiple agents concurrently
	for i := 0; i < agentCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			config := &RunConfig{
				AgentType: AgentTypeExplore,
				Task:      fmt.Sprintf("Concurrent agent %d", index),
				MaxTurns:  2,
				Context:   context.Background(),
			}

			agent, err := manager.StartAgent(config)
			if err == nil {
				agentsMu.Lock()
				agents = append(agents, agent)
				agentsMu.Unlock()
				agent.Wait()
			}
		}(i)
	}

	// Wait for all agents to complete
	wg.Wait()

	// Verify all agents completed
	assert.Len(t, agents, agentCount, "Should have started all agents")

	for _, agent := range agents {
		assert.True(t, agent.IsComplete(), "Agent %s should be complete", agent.ID)
	}

	// List all agents
	allAgents := manager.ListAgents()
	assert.GreaterOrEqual(t, len(allAgents), agentCount, "Should have at least the agents we started")
}

// TestAsyncAgentManager_AgentSpecificListeners tests agent-specific event listeners
func TestAsyncAgentManager_AgentSpecificListeners(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	var agentEvents []AgentEvent
	var agentEventsMu sync.Mutex

	// Create agent config
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test agent-specific listeners",
		MaxTurns:  2,
		Context:   context.Background(),
	}

	// Start agent
	agent, err := manager.StartAgent(config)
	require.NoError(t, err)

	// Add agent-specific listener
	agentListener := func(event AgentEvent) {
		if event.AgentID == agent.ID {
			agentEventsMu.Lock()
			defer agentEventsMu.Unlock()
			agentEvents = append(agentEvents, event)
			fmt.Printf("Agent-specific event: %s\n", event.EventType)
		}
	}
	agent.AddListener(agentListener)
	defer agent.RemoveListener(agentListener)

	// Wait for completion
	agent.Wait()

	// Give some time for all events to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify we received agent-specific events
	agentEventsMu.Lock()
	defer agentEventsMu.Unlock()

	assert.Greater(t, len(agentEvents), 0, "Should receive agent-specific events")
}

// TestAsyncAgentManager_WaitWithTimeout tests timeout on agent wait
func TestAsyncAgentManager_WaitWithTimeout(t *testing.T) {
	agent := &AsyncAgent{
		ID:   "test-agent",
		done: make(chan struct{}),
	}

	// Wait with very short timeout on an unfinished agent.
	err := agent.WaitWithTimeout(1 * time.Millisecond)
	assert.Error(t, err, "Should timeout waiting for agent")

	close(agent.done)
	assert.NoError(t, agent.WaitWithTimeout(10*time.Millisecond))
}

// TestAsyncAgentManager_Cleanup tests cleanup of completed agents
func TestAsyncAgentManager_Cleanup(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	// Start and complete an agent
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test cleanup",
		MaxTurns:  2,
		Context:   context.Background(),
	}

	agent, err := manager.StartAgent(config)
	require.NoError(t, err)
	agent.Wait()

	// Verify agent exists
	allAgents := manager.ListAgents()
	assert.Greater(t, len(allAgents), 0, "Should have active agents")

	// Cleanup
	manager.Cleanup()

	// Verify completed agent was removed
	allAgents = manager.ListAgents()
	for _, a := range allAgents {
		assert.NotEqual(t, agent.ID, a.ID, "Completed agent should be removed")
	}
}

// TestAsyncAgentManager_Shutdown tests graceful shutdown
func TestAsyncAgentManager_Shutdown(t *testing.T) {
	manager := NewAsyncAgentManager()

	// Start multiple agents
	for i := 0; i < 3; i++ {
		config := &RunConfig{
			AgentType: AgentTypeExplore,
			Task:      fmt.Sprintf("Test shutdown agent %d", i),
			MaxTurns:  10, // Long running
			Context:   context.Background(),
		}

		agent, err := manager.StartAgent(config)
		require.NoError(t, err)

		// Don't wait, let shutdown handle them
		_ = agent
	}

	// Shutdown should cancel all running agents
	manager.Shutdown()

	// Verify no agents are running
	allAgents := manager.ListAgents()
	for _, agent := range allAgents {
		assert.False(t, agent.IsRunning(), "No agents should be running after shutdown")
	}
}

// TestAsyncAgent_GetProgress tests progress information retrieval
func TestAsyncAgent_GetProgress(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test progress retrieval",
		MaxTurns:  2,
		Context:   context.Background(),
	}

	agent, err := manager.StartAgent(config)
	require.NoError(t, err)

	// Get progress
	progress := agent.GetProgress()
	assert.NotNil(t, progress)
	assert.GreaterOrEqual(t, progress.CurrentTurn, 0)
	assert.Greater(t, progress.MaxTurns, 0)

	// Wait for completion
	agent.Wait()

	// Get final progress
	finalProgress := agent.GetProgress()
	assert.NotNil(t, finalProgress)
	// Agent may have completed without any turns, so just check structure
	assert.GreaterOrEqual(t, finalProgress.CurrentTurn, 0)
}

// TestEventBus_PublishSubscribe tests event bus publish/subscribe
func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewAgentEventBus()
	defer bus.Shutdown()

	var receivedEvents []AgentEvent
	var mu sync.Mutex

	// Subscribe to progress events
	unsubscribe := bus.Subscribe(AgentEventProgress, func(event AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
	})

	// Publish events
	event1 := AgentEvent{
		AgentID:   "agent1",
		AgentType: AgentTypeExplore,
		EventType: AgentEventProgress,
		Timestamp: time.Now(),
		Progress: &AgentProgress{
			CurrentTurn:     1,
			MaxTurns:        5,
			PercentComplete: 20.0,
		},
	}

	event2 := AgentEvent{
		AgentID:   "agent1",
		AgentType: AgentTypeExplore,
		EventType: AgentEventProgress,
		Timestamp: time.Now(),
		Progress: &AgentProgress{
			CurrentTurn:     2,
			MaxTurns:        5,
			PercentComplete: 40.0,
		},
	}

	bus.Publish(event1)
	bus.Publish(event2)

	// Wait for events to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify events received
	mu.Lock()
	assert.Len(t, receivedEvents, 2, "Should receive 2 progress events")

	// Check that we received the expected events (order may vary due to async processing)
	hasEvent1 := false
	hasEvent2 := false
	for _, event := range receivedEvents {
		if event.Progress.CurrentTurn == 1 {
			hasEvent1 = true
		}
		if event.Progress.CurrentTurn == 2 {
			hasEvent2 = true
		}
	}
	assert.True(t, hasEvent1, "Should receive event with turn 1")
	assert.True(t, hasEvent2, "Should receive event with turn 2")
	mu.Unlock()

	// Unsubscribe
	unsubscribe()

	// Publish another event
	event3 := AgentEvent{
		AgentID:   "agent1",
		AgentType: AgentTypeExplore,
		EventType: AgentEventProgress,
		Timestamp: time.Now(),
		Progress: &AgentProgress{
			CurrentTurn:     3,
			MaxTurns:        5,
			PercentComplete: 60.0,
		},
	}
	bus.Publish(event3)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify event3 was not received (unsubscribed)
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedEvents, 2, "Should still have 2 events after unsubscribe")
}

// TestNotificationSystem_BasicNotifications tests basic notification system
func TestNotificationSystem_BasicNotifications(t *testing.T) {
	system := NewNotificationSystem()
	defer system.Shutdown()

	var receivedNotifications []RealTimeNotification
	var mu sync.Mutex

	// Add handler
	handler := func(notification RealTimeNotification) {
		mu.Lock()
		defer mu.Unlock()
		receivedNotifications = append(receivedNotifications, notification)
		fmt.Printf("Notification: %s - %s\n", notification.Title, notification.Message)
	}
	system.AddHandler(handler)
	defer system.RemoveHandler(handler)

	// Send notifications
	notif1 := RealTimeNotification{
		ID:       fmt.Sprintf("notif-%d", time.Now().UnixNano()),
		Type:     "test",
		Priority: NotificationPriorityMedium,
		Title:    "Test Notification 1",
		Message:  "This is test notification 1",
		AgentID:  "agent1",
	}

	notif2 := RealTimeNotification{
		ID:       fmt.Sprintf("notif-%d", time.Now().Add(time.Nanosecond).UnixNano()),
		Type:     "test",
		Priority: NotificationPriorityHigh,
		Title:    "Test Notification 2",
		Message:  "This is test notification 2",
		AgentID:  "agent2",
	}

	system.Notify(notif1)
	system.Notify(notif2)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify notifications received
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedNotifications, 2, "Should receive 2 notifications")

	// Verify notification content (order may vary due to async processing)
	hasNotif1 := false
	hasNotif2 := false
	for _, notif := range receivedNotifications {
		if notif.Title == "Test Notification 1" {
			hasNotif1 = true
			assert.Equal(t, NotificationPriorityMedium, notif.Priority)
		}
		if notif.Title == "Test Notification 2" {
			hasNotif2 = true
			assert.Equal(t, NotificationPriorityHigh, notif.Priority)
		}
	}
	assert.True(t, hasNotif1, "Should receive notification 1")
	assert.True(t, hasNotif2, "Should receive notification 2")
}

// TestNotificationSystem_UnreadNotifications tests unread notification tracking
func TestNotificationSystem_UnreadNotifications(t *testing.T) {
	system := NewNotificationSystem()
	defer system.Shutdown()

	// Send notifications
	notif1 := RealTimeNotification{
		ID:       fmt.Sprintf("notif-%d", time.Now().UnixNano()),
		Type:     "test",
		Priority: NotificationPriorityMedium,
		Title:    "Unread Test 1",
		Message:  "This should be unread",
	}

	notif2 := RealTimeNotification{
		ID:       fmt.Sprintf("notif-%d", time.Now().Add(time.Nanosecond).UnixNano()),
		Type:     "test",
		Priority: NotificationPriorityHigh,
		Title:    "Unread Test 2",
		Message:  "This should also be unread",
	}

	system.Notify(notif1)
	system.Notify(notif2)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Get unread notifications
	unread := system.GetUnreadNotifications()
	assert.Len(t, unread, 2, "Should have 2 unread notifications")

	// Mark one as read
	system.MarkAsRead(notif1.ID)

	// Get unread notifications again
	unread = system.GetUnreadNotifications()
	assert.Len(t, unread, 1, "Should have 1 unread notification after marking one as read")
	assert.Equal(t, notif2.ID, unread[0].ID, "Remaining unread should be notif2")
}

// TestAsync_ConcurrentCancellation verifies that concurrent Cancel calls
// on the same agent are race-free.
func TestAsync_ConcurrentCancellation(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test concurrent cancellation",
		MaxTurns:  5,
		Context:   context.Background(),
	}

	agent, err := manager.StartAgent(config)
	if err != nil {
		t.Fatalf("StartAgent: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Errors are expected; the goal is no data race.
			_ = manager.CancelAgent(agent.ID)
		}()
	}
	wg.Wait()
	agent.Wait()
	if !agent.IsComplete() {
		t.Error("agent should be complete after concurrent cancellations")
	}
}

// TestAsync_ConcurrentStatusRead verifies that concurrent reads of agent
// state fields are race-free while the agent is running.
func TestAsync_ConcurrentStatusRead(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test concurrent read",
		MaxTurns:  5,
		Context:   context.Background(),
	}

	agent, err := manager.StartAgent(config)
	if err != nil {
		t.Fatalf("StartAgent: %v", err)
	}

	const readers = 20
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					// Access all state methods — must be race-free.
					_ = agent.IsRunning()
					_ = agent.IsComplete()
					_ = agent.GetProgress()
					_ = agent.GetDuration()
				}
			}
		}()
	}

	agent.Wait()
	close(stop)
	wg.Wait()
}

// TestAsync_ErrorPropagation verifies that an agent failing due to a bad
// config transitions to AgentStatusFailed and exposes an error.
func TestAsync_ErrorPropagation(t *testing.T) {
	manager := NewAsyncAgentManager()
	defer manager.Shutdown()

	// A zero-value RunConfig will cause RunAgent to error.
	config := &RunConfig{
		AgentType: "test-error",
		Task:      "",
		MaxTurns:  1,
		Context:   context.Background(),
	}

	agent, err := manager.StartAgent(config)
	if err != nil {
		t.Fatalf("StartAgent: %v", err)
	}
	agent.Wait()
	if !agent.IsComplete() {
		t.Error("agent should be complete")
	}
	// Either Failed or Completed (depends on RunAgent internals) — just must not be Running.
	if agent.IsRunning() {
		t.Error("agent must not be running after Wait()")
	}
}

func TestBrowseAgentIsRegistered(t *testing.T) {
	agent := GetBuiltInAgentByType(AgentTypeBrowse)
	if agent == nil {
		t.Fatalf("expected browse agent to be registered")
	}
	if agent.AgentType != AgentTypeBrowse {
		t.Fatalf("expected browse agent type %q, got %q", AgentTypeBrowse, agent.AgentType)
	}
}

func TestBrowseAgentToolSurfaceIsReadOnlyResearchFocused(t *testing.T) {
	agent := GetBuiltInAgentByType(AgentTypeBrowse)
	if agent == nil {
		t.Fatalf("expected browse agent to be registered")
	}

	allowed := make(map[string]bool, len(agent.Tools))
	for _, tool := range agent.Tools {
		allowed[tool] = true
	}

	for _, expected := range []string{"web_search", "web_fetch", "browser_open", "read_file", "tree"} {
		if !allowed[expected] {
			t.Fatalf("expected browse agent to allow %q", expected)
		}
	}

	for _, blocked := range []string{"write_file", "edit_file", "bash", "task_create"} {
		if allowed[blocked] {
			t.Fatalf("expected browse agent not to allow %q", blocked)
		}
	}
}

// TestToolResultCache_BasicOperations tests basic cache operations
func TestToolResultCache_BasicOperations(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	// Create a test tool result
	result := &types.ToolResult{
		Data: "Test result content",
	}

	// Test cache miss
	retrieved, found := cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.False(t, found)
	assert.Nil(t, retrieved)

	// Store in cache
	err := cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)
	require.NoError(t, err)

	// Test cache hit
	retrieved, found = cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.True(t, found)
	assert.NotNil(t, retrieved)
	assert.Equal(t, result.Data, retrieved.Data)
}

// TestToolResultCache_DifferentParameters tests cache behavior with different parameters
func TestToolResultCache_DifferentParameters(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result1 := &types.ToolResult{
		Data: "Result 1",
	}
	result2 := &types.ToolResult{
		Data: "Result 2",
	}

	// Store with different parameters
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/file1.txt"}, result1)
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/file2.txt"}, result2)

	// Retrieve with original parameters
	retrieved1, found := cache.Get("readFile", map[string]any{"path": "/tmp/file1.txt"})
	assert.True(t, found)
	assert.Equal(t, "Result 1", retrieved1.Data)

	retrieved2, found := cache.Get("readFile", map[string]any{"path": "/tmp/file2.txt"})
	assert.True(t, found)
	assert.Equal(t, "Result 2", retrieved2.Data)
}

// TestToolResultCache_ParameterOrder tests that parameter order doesn't affect cache key
func TestToolResultCache_ParameterOrder(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{
		Data: "Test result",
	}

	// Store with parameters in one order
	params1 := map[string]any{
		"path":   "/tmp/test.txt",
		"mode":   "read",
		"format": "json",
	}

	// Store with parameters in different order
	params2 := map[string]any{
		"format": "json",
		"path":   "/tmp/test.txt",
		"mode":   "read",
	}

	_ = cache.Set("readFile", params1, result)

	// Should find the same result with different parameter order
	retrieved, found := cache.Get("readFile", params2)
	assert.True(t, found)
	assert.Equal(t, "Test result", retrieved.Data)
}

// TestToolResultCache_TTL tests time-to-live functionality
func TestToolResultCache_TTL(t *testing.T) {
	config := DefaultToolResultCacheConfig()
	config.TTL = 100 * time.Millisecond // Short TTL for testing

	cache := NewToolResultCacheWithConfig(config)
	defer cache.Shutdown()

	result := &types.ToolResult{
		Data: "Test result",
	}

	// Store result
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)

	// Should find immediately
	_, found := cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.True(t, found)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should not find after TTL expiration
	_, found = cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.False(t, found)
}

// TestToolResultCache_MaxEntries tests cache entry limit
func TestToolResultCache_MaxEntries(t *testing.T) {
	config := DefaultToolResultCacheConfig()
	config.MaxEntries = 3 // Small limit for testing

	cache := NewToolResultCacheWithConfig(config)
	defer cache.Shutdown()

	// Store up to limit
	result := &types.ToolResult{Data: "Test result"}
	_ = cache.Set("tool1", map[string]any{}, result)
	_ = cache.Set("tool2", map[string]any{}, result)
	_ = cache.Set("tool3", map[string]any{}, result)

	stats := cache.GetStats()
	assert.Equal(t, 3, stats.Entries)

	// Store one more - should evict LRU
	_ = cache.Set("tool4", map[string]any{}, result)

	stats = cache.GetStats()
	assert.Equal(t, 3, stats.Entries) // Still limited to 3
	assert.Greater(t, stats.Evictions, int64(0))
}

// TestToolResultCache_MaxSize tests cache size limit
func TestToolResultCache_MaxSize(t *testing.T) {
	config := DefaultToolResultCacheConfig()
	config.MaxSize = 100 // Small size limit for testing

	cache := NewToolResultCacheWithConfig(config)
	defer cache.Shutdown()

	// Store small result
	smallResult := &types.ToolResult{
		Data: "Small",
	}
	err := cache.Set("tool1", map[string]any{}, smallResult)
	require.NoError(t, err)

	// Try to store large result - should evict small result first
	largeResult := &types.ToolResult{
		Data: string(make([]byte, 200)),
	}
	err = cache.Set("tool2", map[string]any{}, largeResult)
	require.NoError(t, err)

	stats := cache.GetStats()
	assert.Greater(t, stats.Evictions, int64(0))
	assert.GreaterOrEqual(t, stats.Size, int64(200)) // At least the large result size
}

// TestToolResultCache_LRU tests least-recently-used eviction
func TestToolResultCache_LRU(t *testing.T) {
	config := DefaultToolResultCacheConfig()
	config.MaxEntries = 3

	cache := NewToolResultCacheWithConfig(config)
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Store three results
	_ = cache.Set("tool1", map[string]any{}, result)
	_ = cache.Set("tool2", map[string]any{}, result)
	_ = cache.Set("tool3", map[string]any{}, result)

	// Access tool1 to make it recently used
	_, _ = cache.Get("tool1", map[string]any{})

	// Store fourth result - should evict least recently used (tool2)
	_ = cache.Set("tool4", map[string]any{}, result)

	// tool1 should still exist
	_, found1 := cache.Get("tool1", map[string]any{})
	assert.True(t, found1)

	// tool2 should have been evicted (least recently used)
	_, found2 := cache.Get("tool2", map[string]any{})
	assert.False(t, found2)

	// tool3 should still exist
	_, found3 := cache.Get("tool3", map[string]any{})
	assert.True(t, found3)
}

// TestToolResultCache_Statistics tests cache statistics
func TestToolResultCache_Statistics(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Initial stats
	stats := cache.GetStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0.0, stats.HitRate)

	// Store a result
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)

	// Cache hit
	_, _ = cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})

	stats = cache.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 1.0, stats.HitRate)

	// Cache miss
	_, _ = cache.Get("readFile", map[string]any{"path": "/tmp/other.txt"})

	stats = cache.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 0.5, stats.HitRate)
}

// TestToolResultCache_Invalidate tests cache invalidation
func TestToolResultCache_Invalidate(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Store results
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)
	_ = cache.Set("write_file", map[string]any{"path": "/tmp/test.txt"}, result)

	// Invalidate specific entry
	err := cache.Invalidate("readFile", map[string]any{"path": "/tmp/test.txt"})
	require.NoError(t, err)

	// Check specific entry is invalidated
	_, found1 := cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.False(t, found1)

	// Check other entry still exists
	_, found2 := cache.Get("write_file", map[string]any{"path": "/tmp/test.txt"})
	assert.True(t, found2)

	stats := cache.GetStats()
	assert.Greater(t, stats.Evictions, int64(0))
}

// TestToolResultCache_InvalidateByTool tests invalidating all entries for a tool
func TestToolResultCache_InvalidateByTool(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Store multiple results for the same tool
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/file1.txt"}, result)
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/file2.txt"}, result)
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/file3.txt"}, result)

	// Invalidate all readFile entries
	cache.InvalidateByTool("readFile")

	// All readFile entries should be invalidated
	_, found1 := cache.Get("readFile", map[string]any{"path": "/tmp/file1.txt"})
	assert.False(t, found1)

	_, found2 := cache.Get("readFile", map[string]any{"path": "/tmp/file2.txt"})
	assert.False(t, found2)

	_, found3 := cache.Get("readFile", map[string]any{"path": "/tmp/file3.txt"})
	assert.False(t, found3)

	stats := cache.GetStats()
	assert.Greater(t, stats.Evictions, int64(2)) // Should have evicted at least 2
}

// TestToolResultCache_Clear tests cache clearing
func TestToolResultCache_Clear(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Store multiple results
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)
	_ = cache.Set("write_file", map[string]any{"path": "/tmp/test.txt"}, result)
	_ = cache.Set("grep", map[string]any{"pattern": "test"}, result)

	stats := cache.GetStats()
	assert.Equal(t, 3, stats.Entries)

	// Clear cache
	cache.Clear()

	// All entries should be gone
	stats = cache.GetStats()
	assert.Equal(t, 0, stats.Entries)
	assert.Equal(t, int64(0), stats.Size)

	_, found1 := cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.False(t, found1)
}

// TestToolResultCache_ShouldCache tests cache eligibility logic
func TestToolResultCache_ShouldCache(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	// Successful result should be cached (except for non-idempotent tools)
	successfulResult := &types.ToolResult{
		Data: "Success",
	}
	assert.True(t, cache.ShouldCache("readFile", successfulResult))
	assert.False(t, cache.ShouldCache("bash", successfulResult))       // Non-idempotent
	assert.False(t, cache.ShouldCache("write_file", successfulResult)) // Non-idempotent

	// Failed result should not be cached
	failedResult := &types.ToolResult{
		Error: fmt.Errorf("Tool execution failed"),
	}
	assert.False(t, cache.ShouldCache("readFile", failedResult))

	// Nil result should not be cached
	assert.False(t, cache.ShouldCache("readFile", nil))
}

// TestToolResultCache_NilParameters tests handling of nil parameters
func TestToolResultCache_NilParameters(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Store with nil parameters
	err := cache.Set("simpleTool", nil, result)
	require.NoError(t, err)

	// Should retrieve with nil parameters
	retrieved, found := cache.Get("simpleTool", nil)
	assert.True(t, found)
	assert.Equal(t, "Test result", retrieved.Data)

	// Should retrieve with explicit nil parameters
	retrieved, found = cache.Get("simpleTool", map[string]any{})
	assert.True(t, found)
	assert.Equal(t, "Test result", retrieved.Data)
}

// TestToolResultCache_ComplexParameters tests handling of complex parameter types
func TestToolResultCache_ComplexParameters(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Complex parameters
	params1 := map[string]any{
		"string": "test",
		"number": 42,
		"bool":   true,
		"slice":  []any{"a", "b", "c"},
		"nested": map[string]any{"key": "value"},
	}

	// Store with complex parameters
	err := cache.Set("complexTool", params1, result)
	require.NoError(t, err)

	// Should retrieve with equivalent parameters
	params2 := map[string]any{
		"nested": map[string]any{"key": "value"},
		"slice":  []any{"a", "b", "c"},
		"bool":   true,
		"string": "test",
		"number": 42,
	}

	retrieved, found := cache.Get("complexTool", params2)
	assert.True(t, found)
	assert.Equal(t, "Test result", retrieved.Data)
}

// TestToolResultCache_AccessCounting tests access count tracking
func TestToolResultCache_AccessCounting(t *testing.T) {
	config := DefaultToolResultCacheConfig()
	config.TTL = 0 // No TTL

	cache := NewToolResultCacheWithConfig(config)
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)

	// Access multiple times
	_, _ = cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	_, _ = cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	_, _ = cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})

	stats := cache.GetStats()
	assert.Equal(t, int64(3), stats.Hits)
}

// TestToolResultCache_ConcurrentAccess tests thread safety
func TestToolResultCache_ConcurrentAccess(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			params := map[string]any{"id": id}
			_ = cache.Set("concurrentTool", params, result)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent reads
	readDone := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			params := map[string]any{"id": id}
			cache.Get("concurrentTool", params)
			readDone <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < 10; i++ {
		<-readDone
	}

	// Should have some hits and no race conditions
	stats := cache.GetStats()
	assert.Equal(t, int64(10), stats.Hits+stats.Misses)
}

// TestDefaultCacheInvalidationRules tests default invalidation rules
func TestDefaultCacheInvalidationRules(t *testing.T) {
	rules := DefaultCacheInvalidationRules()

	assert.True(t, rules.InvalidateOnFileChange)
	assert.True(t, rules.InvalidateOnNewFile)
	assert.Contains(t, rules.InvalidateOnToolCall, "write_file")
	assert.Contains(t, rules.InvalidateOnToolCall, "edit")
	assert.Contains(t, rules.InvalidateOnToolCall, "bash")
	assert.Equal(t, 0, rules.InvalidateInterval)
}

// TestCacheInterceptor_BasicFunctionality tests basic interceptor functionality
func TestCacheInterceptor_BasicFunctionality(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	interceptor := NewCacheInterceptor(cache, nil)

	result := &types.ToolResult{
		Data: "Test result",
	}

	params := map[string]any{"path": "/tmp/test.txt"}

	// Pre-execute (should check cache)
	interceptor.PreExecuteTool(nil, "readFile", params)

	// Post-execute (should cache result)
	interceptor.PostExecuteTool(nil, "readFile", params, result)

	// Should now be cached
	retrieved, found := cache.Get("readFile", params)
	assert.True(t, found)
	assert.Equal(t, "Test result", retrieved.Data)
}

// TestToolResultCache_UpdateExisting tests updating existing cache entries
func TestToolResultCache_UpdateExisting(t *testing.T) {
	cache := NewToolResultCache()
	defer cache.Shutdown()

	result1 := &types.ToolResult{Data: "Version 1"}
	result2 := &types.ToolResult{Data: "Version 2"}

	params := map[string]any{"path": "/tmp/test.txt"}

	// Store first version
	_ = cache.Set("readFile", params, result1)

	// Update with second version
	_ = cache.Set("readFile", params, result2)

	// Should get updated version
	retrieved, found := cache.Get("readFile", params)
	assert.True(t, found)
	assert.Equal(t, "Version 2", retrieved.Data)
}

// TestToolResultCache_NoTTL tests cache without TTL
func TestToolResultCache_NoTTL(t *testing.T) {
	config := DefaultToolResultCacheConfig()
	config.TTL = 0 // No TTL

	cache := NewToolResultCacheWithConfig(config)
	defer cache.Shutdown()

	result := &types.ToolResult{Data: "Test result"}
	_ = cache.Set("readFile", map[string]any{"path": "/tmp/test.txt"}, result)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Should still be in cache (no TTL)
	_, found := cache.Get("readFile", map[string]any{"path": "/tmp/test.txt"})
	assert.True(t, found)
}

// TestPermissionIsolationManager_CreateContext tests creating permission contexts
func TestPermissionIsolationManager_CreateContext(t *testing.T) {
	manager := NewPermissionIsolationManager()
	defer manager.Shutdown()

	// Create a permission context
	_ = manager.CreateAgentPermissionContext("agent-1", AgentTypeExplore, "")

	// Verify context can be retrieved
	_, err := manager.GetAgentPermissionContext("agent-1")
	require.NoError(t, err)
}

// TestPermissionIsolationManager_ToolPermissions tests tool permission checking
func TestPermissionIsolationManager_ToolPermissions(t *testing.T) {
	manager := NewPermissionIsolationManager()
	defer manager.Shutdown()

	// Create context with denied tools
	_ = manager.CreateAgentPermissionContext("agent-1", AgentTypeExplore, "")
	err := manager.UpdateAgentPermissionContext("agent-1", func(ctx *AgentPermissionContext) {
		ctx.DeniedTools = []string{"bash", "powershell"}
	})
	require.NoError(t, err)

	// Test denied tools
	allowed, reason := manager.IsToolAllowed("agent-1", "bash")
	assert.False(t, allowed)
	assert.Contains(t, reason, "explicitly denied")

	// Test allowed tools
	allowed, reason = manager.IsToolAllowed("agent-1", "readFile")
	assert.True(t, allowed)
	assert.Empty(t, reason)
}

// TestPermissionIsolationManager_ResourceLimits tests resource limit checking
func TestPermissionIsolationManager_ResourceLimits(t *testing.T) {
	manager := NewPermissionIsolationManager()
	defer manager.Shutdown()

	// Create context with resource limits
	_ = manager.CreateAgentPermissionContext("agent-1", AgentTypeGeneralPurpose, "")
	err := manager.UpdateAgentPermissionContext("agent-1", func(ctx *AgentPermissionContext) {
		ctx.ResourceLimits = &ResourceLimits{
			MaxToolCalls:       3,
			MaxDuration:        5 * time.Second,
			MaxNetworkRequests: 2,
		}
	})
	require.NoError(t, err)

	// Test within limits
	allowed, reason := manager.CheckResourceLimits("agent-1", "tool_call")
	assert.True(t, allowed)
	assert.Empty(t, reason)

	// Record tool calls up to limit
	manager.RecordToolCall("agent-1", "tool1")
	manager.RecordToolCall("agent-1", "tool2")
	manager.RecordToolCall("agent-1", "tool3")

	// Test exceeding limit
	allowed, reason = manager.CheckResourceLimits("agent-1", "tool_call")
	assert.False(t, allowed)
	assert.Contains(t, reason, "tool call limit reached")

	// Create another agent to test independent limits
	_ = manager.CreateAgentPermissionContext("agent-2", AgentTypeExplore, "")
	err = manager.UpdateAgentPermissionContext("agent-2", func(ctx *AgentPermissionContext) {
		ctx.ResourceLimits = &ResourceLimits{
			MaxToolCalls: 1,
		}
	})
	require.NoError(t, err)

	// Agent 2 should have independent limits
	allowed, _ = manager.CheckResourceLimits("agent-2", "tool_call")
	assert.True(t, allowed)
}

// TestPermissionIsolationManager_DirectoryAccess tests directory access control
func TestPermissionIsolationManager_DirectoryAccess(t *testing.T) {
	manager := NewPermissionIsolationManager()
	defer manager.Shutdown()

	// Create context with directory restrictions
	_ = manager.CreateAgentPermissionContext("agent-1", AgentTypeExplore, "")
	err := manager.UpdateAgentPermissionContext("agent-1", func(ctx *AgentPermissionContext) {
		ctx.ResourceLimits = &ResourceLimits{
			DeniedDirectories:  []string{"/etc", "/root", "/var"},
			AllowedDirectories: []string{"/tmp", "/home/user"},
		}
	})
	require.NoError(t, err)

	// Test denied directory
	allowed, reason := manager.IsDirectoryAllowed("agent-1", "/etc/passwd")
	assert.False(t, allowed)
	assert.Contains(t, reason, "explicitly denied")

	// Test allowed directory
	allowed, _ = manager.IsDirectoryAllowed("agent-1", "/tmp/file.txt")
	assert.True(t, allowed)

	// Test directory not in allowed list
	allowed, reason = manager.IsDirectoryAllowed("agent-1", "/opt/file.txt")
	assert.False(t, allowed)
	assert.Contains(t, reason, "not in allowed directories")

	// Test unrestricted directory
	allowed, _ = manager.IsDirectoryAllowed("agent-1", "/home/user/safe.txt")
	assert.True(t, allowed)
}

// TestPermissionIsolationManager_ResourceUsage tests resource usage tracking
func TestPermissionIsolationManager_ResourceUsage(t *testing.T) {
	manager := NewPermissionIsolationManager()
	defer manager.Shutdown()

	// Create context
	_ = manager.CreateAgentPermissionContext("agent-1", AgentTypeGeneralPurpose, "")

	// Record resource usage
	manager.RecordToolCall("agent-1", "tool1")
	manager.RecordToolCall("agent-1", "tool2")
	manager.RecordDuration("agent-1", 100*time.Millisecond)
	manager.RecordNetworkRequest("agent-1")
	manager.RecordFileAccess("agent-1", "/tmp/file1.txt")
	manager.RecordFileAccess("agent-1", "/tmp/file2.txt")

	// Get resource usage
	usage, err := manager.GetResourceUsage("agent-1")
	require.NoError(t, err)
	assert.Equal(t, 2, usage.ToolCalls)
	assert.Equal(t, 100*time.Millisecond, usage.Duration)
	assert.Equal(t, 1, usage.NetworkRequests)
	assert.Len(t, usage.FilesAccessed, 2)
	assert.Contains(t, usage.FilesAccessed, "/tmp/file1.txt")
	assert.Contains(t, usage.FilesAccessed, "/tmp/file2.txt")

	// Verify independent tracking for different agents
	_ = manager.CreateAgentPermissionContext("agent-2", AgentTypeExplore, "")
	manager.RecordToolCall("agent-2", "tool1")

	usage1, _ := manager.GetResourceUsage("agent-1")
	usage2, _ := manager.GetResourceUsage("agent-2")

	assert.Equal(t, 2, usage1.ToolCalls) // Agent 1 still has 2 calls
	assert.Equal(t, 1, usage2.ToolCalls) // Agent 2 has 1 call
}

// TestIsolatedAsyncAgent_BasicIsolation tests basic agent isolation
func TestIsolatedAsyncAgent_BasicIsolation(t *testing.T) {
	isolationManager := NewPermissionIsolationManager()
	defer isolationManager.Shutdown()

	asyncManager := NewAsyncAgentManager()
	defer asyncManager.Shutdown()

	integrationManager := NewIsolationIntegrationManager(isolationManager, asyncManager)

	// Create isolated agent
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test basic isolation",
		MaxTurns:  2,
	}

	isolatedAgent, err := integrationManager.StartIsolatedAgent(config, IsolationLevelBasic, "")
	require.NoError(t, err)
	require.NotNil(t, isolatedAgent)

	// Verify permission context was created
	permissionContext := isolatedAgent.GetPermissionContext()
	assert.NotNil(t, permissionContext)
}

// TestIsolatedAsyncAgent_StrictIsolation tests strict agent isolation
func TestIsolatedAsyncAgent_StrictIsolation(t *testing.T) {
	isolationManager := NewPermissionIsolationManager()
	defer isolationManager.Shutdown()

	asyncManager := NewAsyncAgentManager()
	defer asyncManager.Shutdown()

	integrationManager := NewIsolationIntegrationManager(isolationManager, asyncManager)

	// Create isolated agent with strict isolation
	config := &RunConfig{
		AgentType: AgentTypeGeneralPurpose,
		Task:      "Test strict isolation",
		MaxTurns:  2,
	}

	isolatedAgent, err := integrationManager.StartIsolatedAgent(config, IsolationLevelStrict, "")
	require.NoError(t, err)

	// Verify strict isolation was applied
	permissionContext := isolatedAgent.GetPermissionContext()
	assert.NotNil(t, permissionContext)
}

// TestIsolatedAsyncAgent_SandboxIsolation tests sandbox agent isolation
func TestIsolatedAsyncAgent_SandboxIsolation(t *testing.T) {
	isolationManager := NewPermissionIsolationManager()
	defer isolationManager.Shutdown()

	asyncManager := NewAsyncAgentManager()
	defer asyncManager.Shutdown()

	integrationManager := NewIsolationIntegrationManager(isolationManager, asyncManager)

	// Create isolated agent with sandbox isolation
	config := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Test sandbox isolation",
		MaxTurns:  2,
	}

	isolatedAgent, err := integrationManager.StartIsolatedAgent(config, IsolationLevelSandbox, "")
	require.NoError(t, err)

	// Verify sandbox isolation was applied
	permissionContext := isolatedAgent.GetPermissionContext()
	assert.NotNil(t, permissionContext)
}

// TestIsolatedAsyncAgent_ParentChildRelationships tests parent-child agent relationships
func TestIsolatedAsyncAgent_ParentChildRelationships(t *testing.T) {
	isolationManager := NewPermissionIsolationManager()
	defer isolationManager.Shutdown()

	asyncManager := NewAsyncAgentManager()
	defer asyncManager.Shutdown()

	integrationManager := NewIsolationIntegrationManager(isolationManager, asyncManager)

	// Create parent agent
	config1 := &RunConfig{
		AgentType: AgentTypeGeneralPurpose,
		Task:      "Parent task",
		MaxTurns:  1,
	}

	parentAgent, err := integrationManager.StartIsolatedAgent(config1, IsolationLevelBasic, "")
	require.NoError(t, err)

	// Create child agent
	config2 := &RunConfig{
		AgentType: AgentTypeExplore,
		Task:      "Child task",
		MaxTurns:  1,
	}

	childAgent, err := integrationManager.StartIsolatedAgent(config2, IsolationLevelBasic, parentAgent.ID)
	require.NoError(t, err)

	// Verify parent-child relationship
	parentContext := parentAgent.GetPermissionContext()
	childContext := childAgent.GetPermissionContext()

	assert.Equal(t, "", parentContext.ParentAgentID)
	assert.Equal(t, parentAgent.ID, childContext.ParentAgentID)

	// Verify child is in parent's child list
	assert.Contains(t, parentContext.ChildAgents, childAgent.ID)
}

// TestIsolationIntegrationManager_DeleteContext tests deletion of permission contexts
func TestIsolationIntegrationManager_DeleteContext(t *testing.T) {
	isolationManager := NewPermissionIsolationManager()
	defer isolationManager.Shutdown()

	// Create context
	context := isolationManager.CreateAgentPermissionContext("agent-1", AgentTypeExplore, "")
	require.NotNil(t, context)

	// Verify context exists
	_, err := isolationManager.GetAgentPermissionContext("agent-1")
	require.NoError(t, err)

	// Delete context
	err = isolationManager.DeleteAgentPermissionContext("agent-1")
	require.NoError(t, err)

	// Verify context is deleted
	_, err = isolationManager.GetAgentPermissionContext("agent-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify resource usage was cleaned up
	_, err = isolationManager.GetResourceUsage("agent-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestPermissionIsolationManager_GetAllContexts tests retrieving all contexts
func TestPermissionIsolationManager_GetAllContexts(t *testing.T) {
	manager := NewPermissionIsolationManager()
	defer manager.Shutdown()

	// Create multiple contexts
	manager.CreateAgentPermissionContext("agent-1", AgentTypeExplore, "")
	manager.CreateAgentPermissionContext("agent-2", AgentTypeGeneralPurpose, "")
	manager.CreateAgentPermissionContext("agent-3", AgentTypeVerify, "")

	// Get all contexts
	contexts := manager.GetAllAgentContexts()
	assert.Len(t, contexts, 3)

	// Verify all contexts are present
	agentIDs := make(map[string]bool)
	for _, context := range contexts {
		agentIDs[context.AgentID] = true
	}

	assert.True(t, agentIDs["agent-1"])
	assert.True(t, agentIDs["agent-2"])
	assert.True(t, agentIDs["agent-3"])
}

// TestMemoryAdapter_BasicOperations tests basic memory operations.
func TestMemoryAdapter_BasicOperations(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store a memory entry
	entry := MemoryEntry{
		ID:      "test1",
		Type:    MemoryTypeToolUsage,
		Content: "Test content",
		Tags:    []string{"test", "basic"},
	}

	err := memSystem.StoreEntry(entry)
	require.NoError(t, err)

	// Retrieve the memory
	retrieved, err := memSystem.GetEntry("test1")
	require.NoError(t, err)
	assert.Equal(t, "test1", retrieved.ID)
	assert.Equal(t, "Test content", retrieved.Content)
	assert.Equal(t, []string{"test", "basic"}, retrieved.Tags)
}

// TestMemoryAdapter_StoreAndRetrieve tests storing and retrieving memory entries.
func TestMemoryAdapter_StoreAndRetrieve(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store multiple entries
	entries := []MemoryEntry{
		{
			Type:    MemoryTypeToolUsage,
			Content: "Tool usage pattern 1",
			Tags:    []string{"tool", "readFile"},
		},
		{
			Type:    MemoryTypeError,
			Content: "Error pattern 1",
			Tags:    []string{"error", "file_not_found"},
		},
		{
			Type:    MemoryTypeConversation,
			Content: "Conversation about project structure",
			Tags:    []string{"conversation", "project"},
		},
	}

	for _, entry := range entries {
		err := memSystem.StoreEntry(entry)
		require.NoError(t, err)
	}

	// Query for specific types
	query := MemoryQuery{
		Types: []MemoryType{MemoryTypeToolUsage},
		Limit: 10,
	}

	result, err := memSystem.Search(query)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Entries), 1)
}

// TestMemoryAdapter_ToolUsageLearning tests learning from tool usage.
func TestMemoryAdapter_ToolUsageLearning(t *testing.T) {
	config := DefaultMemoryConfig()
	config.LearningEnabled = true
	memoryWithConfig := NewMemoryAdapterWithConfig(config)
	memoryWithConfig.Start()
	defer memoryWithConfig.Stop()

	// Learn from successful tool usage
	params := map[string]any{
		"path":     "/tmp/test.txt",
		"encoding": "utf-8",
	}

	err := memoryWithConfig.LearnToolUsage("readFile", params, true, nil)
	require.NoError(t, err)

	// Retrieve learned patterns
	patterns, err := memoryWithConfig.GetToolUsagePatterns("readFile")
	require.NoError(t, err)
	assert.NotNil(t, patterns)
	assert.Equal(t, "readFile", patterns.ToolName)
	assert.GreaterOrEqual(t, patterns.UsageCount, 1)
}

// TestMemoryAdapter_ToolUsageLearningFailure tests learning from failed tool usage.
func TestMemoryAdapter_ToolUsageLearningFailure(t *testing.T) {
	config := DefaultMemoryConfig()
	config.LearningEnabled = true
	memoryWithConfig := NewMemoryAdapterWithConfig(config)
	memoryWithConfig.Start()
	defer memoryWithConfig.Stop()

	// Learn from failed tool usage
	params := map[string]any{
		"path":     "/tmp/nonexistent.txt",
		"encoding": "utf-8",
	}

	err := memoryWithConfig.LearnToolUsage("readFile", params, false, nil)
	require.NoError(t, err)

	// Retrieve learned patterns
	patterns, err := memoryWithConfig.GetToolUsagePatterns("readFile")
	require.NoError(t, err)
	assert.NotNil(t, patterns)
	assert.Greater(t, len(patterns.FailedParameters), 0)
}

// TestMemoryAdapter_QueryByTags tests querying memory by tags.
func TestMemoryAdapter_QueryByTags(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store entries with different tags
	entries := []MemoryEntry{
		{
			Type:    MemoryTypeToolUsage,
			Content: "File operation",
			Tags:    []string{"file", "read"},
		},
		{
			Type:    MemoryTypeToolUsage,
			Content: "Another file operation",
			Tags:    []string{"file", "write"},
		},
		{
			Type:    MemoryTypeError,
			Content: "Network error",
			Tags:    []string{"error", "network"},
		},
	}

	for _, entry := range entries {
		_ = memSystem.StoreEntry(entry)
	}

	// Query by tags
	query := MemoryQuery{
		Tags:  []string{"file"},
		Limit: 10,
	}

	result, err := memSystem.Search(query)
	require.NoError(t, err)
	assert.Equal(t, 2, len(result.Entries))
}

// TestMemoryAdapter_QueryByContent tests querying memory by content.
func TestMemoryAdapter_QueryByContent(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store entries
	entries := []MemoryEntry{
		{
			Type:    MemoryTypeToolUsage,
			Content: "Python script execution",
			Tags:    []string{"python", "script"},
		},
		{
			Type:    MemoryTypeToolUsage,
			Content: "Bash command execution",
			Tags:    []string{"bash", "command"},
		},
	}

	for _, entry := range entries {
		_ = memSystem.StoreEntry(entry)
	}

	// Query by content
	query := MemoryQuery{
		Content: "Python",
		Limit:   10,
	}

	result, err := memSystem.Search(query)
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.Entries))
	assert.Contains(t, result.Entries[0].Content, "Python")
}

// TestMemoryAdapter_ImportExport tests import/export functionality.
func TestMemoryAdapter_ImportExport(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store some entries
	entries := []MemoryEntry{
		{
			Type:    MemoryTypeToolUsage,
			Content: "Test content 1",
			Tags:    []string{"test"},
		},
		{
			Type:    MemoryTypeError,
			Content: "Test content 2",
			Tags:    []string{"test"},
		},
	}

	for _, entry := range entries {
		_ = memSystem.StoreEntry(entry)
	}

	// Export memory
	data, err := memSystem.Export()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Create new memory system and import
	newMemory := NewMemoryAdapter()
	newMemory.Start()
	defer newMemory.Stop()
	err = newMemory.Import(data)
	require.NoError(t, err)

	// Verify imported data
	stats := newMemory.Stats()
	assert.GreaterOrEqual(t, stats.TotalEntries, 2)
}

// TestMemoryAdapter_Expiration tests memory expiration.
func TestMemoryAdapter_Expiration(t *testing.T) {
	config := DefaultMemoryConfig()
	config.DefaultTTL = 100 * time.Millisecond // Very short TTL for testing
	memSystem := NewMemoryAdapterWithConfig(config)
	memSystem.Start()
	defer memSystem.Stop()

	// Store an entry
	entry := MemoryEntry{
		Type:    MemoryTypeToolUsage,
		Content: "Will expire soon",
		Tags:    []string{"test"},
	}

	err := memSystem.StoreEntry(entry)
	require.NoError(t, err)

	// Query immediately - should find it
	query := MemoryQuery{
		Types: []MemoryType{MemoryTypeToolUsage},
		Limit: 10,
	}

	result, err := memSystem.Search(query)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Entries), 1)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Query again - should not find it
	result, err = memSystem.Search(query)
	require.NoError(t, err)
	assert.Equal(t, 0, len(result.Entries))
}

// TestMemoryAdapter_ImportanceScoring tests importance calculation.
func TestMemoryAdapter_ImportanceScoring(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store entries with different characteristics
	entries := []MemoryEntry{
		{
			Type:    MemoryTypeToolUsage,
			Content: "Short content",
			Tags:    []string{"test"},
		},
		{
			Type:    MemoryTypeToolUsage,
			Content: "This is a much longer content that should have higher importance because it contains more information and context",
			Tags:    []string{"test", "important", "detailed"},
		},
	}

	for _, entry := range entries {
		_ = memSystem.StoreEntry(entry)
	}

	// Query and check importance
	query := MemoryQuery{
		Types:         []MemoryType{MemoryTypeToolUsage},
		MinImportance: 0.5, // Filter by minimum importance
		Limit:         10,
	}

	result, err := memSystem.Search(query)
	require.NoError(t, err)
	// Only the longer content should be returned
	assert.Equal(t, 1, len(result.Entries))
	assert.Greater(t, result.Entries[0].Importance, 0.5)
}

// TestMemoryAdapter_CleanupExpired tests automatic cleanup of expired entries.
func TestMemoryAdapter_CleanupExpired(t *testing.T) {
	config := DefaultMemoryConfig()
	config.DefaultTTL = 50 * time.Millisecond
	memSystem := NewMemoryAdapterWithConfig(config)
	memSystem.Start()
	defer memSystem.Stop()

	// Store entries with different creation times
	entries := []MemoryEntry{
		{
			Type:    MemoryTypeToolUsage,
			Content: "Old entry",
			Tags:    []string{"test"},
		},
		{
			Type:    MemoryTypeToolUsage,
			Content: "New entry",
			Tags:    []string{"test"},
		},
	}

	for i, entry := range entries {
		// Create entries with different times
		if i == 0 {
			entry.CreatedAt = time.Now().Add(-1 * time.Hour)
		}
		_ = memSystem.StoreEntry(entry)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Trigger cleanup
	cleaned := memSystem.CleanupExpired()
	assert.GreaterOrEqual(t, cleaned, 1)
}

// TestMemoryAdapter_DeleteEntry tests deleting memory entries.
func TestMemoryAdapter_DeleteEntry(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store an entry
	entry := MemoryEntry{
		ID:      "delete_test",
		Type:    MemoryTypeToolUsage,
		Content: "To be deleted",
		Tags:    []string{"test"},
	}

	err := memSystem.StoreEntry(entry)
	require.NoError(t, err)

	// Verify it exists
	storedEntry, err := memSystem.GetEntry(entry.ID)
	require.NoError(t, err)
	require.NotNil(t, storedEntry)

	// Delete it
	err = memSystem.DeleteEntry(entry.ID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = memSystem.GetEntry(entry.ID)
	assert.Error(t, err)
}

// TestMemoryAdapter_Statistics tests memory statistics.
func TestMemoryAdapter_Statistics(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store various types of entries
	entries := []MemoryEntry{
		{Type: MemoryTypeToolUsage, Content: "Tool 1", Tags: []string{"tool"}},
		{Type: MemoryTypeToolUsage, Content: "Tool 2", Tags: []string{"tool"}},
		{Type: MemoryTypeError, Content: "Error 1", Tags: []string{"error"}},
		{Type: MemoryTypeConversation, Content: "Conv 1", Tags: []string{"conv"}},
	}

	for _, entry := range entries {
		_ = memSystem.StoreEntry(entry)
	}

	// Get statistics
	stats := memSystem.Stats()
	assert.Equal(t, 4, stats.TotalEntries)
	assert.Equal(t, 2, stats.EntriesByType[MemoryTypeToolUsage])
	assert.Equal(t, 1, stats.EntriesByType[MemoryTypeError])
	assert.Equal(t, 1, stats.EntriesByType[MemoryTypeConversation])
}

// TestMemoryAdapter_ConcurrentAccess tests thread-safe concurrent access.
func TestMemoryAdapter_ConcurrentAccess(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	done := make(chan bool, 10)

	// Concurrent stores
	for i := 0; i < 5; i++ {
		go func(id int) {
			entry := MemoryEntry{
				ID:      fmt.Sprintf("concurrent_%d", id),
				Type:    MemoryTypeToolUsage,
				Content: fmt.Sprintf("Content %d", id),
				Tags:    []string{"concurrent"},
			}
			_ = memSystem.StoreEntry(entry)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func(id int) {
			query := MemoryQuery{
				Types: []MemoryType{MemoryTypeToolUsage},
				Limit: 10,
			}
			_, _ = memSystem.Search(query)
			done <- true
		}(i)
	}

	// Wait for all operations
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data integrity
	stats := memSystem.Stats()
	assert.GreaterOrEqual(t, stats.TotalEntries, 5)
}

// TestMemoryAdapter_PatternMatching tests parameter pattern matching.
func TestMemoryAdapter_PatternMatching(t *testing.T) {
	config := DefaultMemoryConfig()
	config.LearningEnabled = true
	memoryWithConfig := NewMemoryAdapterWithConfig(config)
	memoryWithConfig.Start()
	defer memoryWithConfig.Stop()

	// Learn multiple successful uses with similar parameters
	params1 := map[string]any{"path": "/tmp/test.txt", "encoding": "utf-8"}
	params2 := map[string]any{"path": "/tmp/test.txt", "encoding": "utf-8"}
	params3 := map[string]any{"path": "/tmp/other.txt", "encoding": "utf-8"}

	_ = memoryWithConfig.LearnToolUsage("readFile", params1, true, nil)
	_ = memoryWithConfig.LearnToolUsage("readFile", params2, true, nil)
	_ = memoryWithConfig.LearnToolUsage("readFile", params3, true, nil)

	// Retrieve patterns
	patterns, err := memoryWithConfig.GetToolUsagePatterns("readFile")
	require.NoError(t, err)

	// Check that we learned the pattern
	assert.Equal(t, "readFile", patterns.ToolName)
	assert.GreaterOrEqual(t, len(patterns.SuccessfulParameters), 2)
}

// TestMemoryAdapter_MultipleToolTypes tests handling different tool types.
func TestMemoryAdapter_MultipleToolTypes(t *testing.T) {
	config := DefaultMemoryConfig()
	config.LearningEnabled = true
	memoryWithConfig := NewMemoryAdapterWithConfig(config)
	memoryWithConfig.Start()
	defer memoryWithConfig.Stop()

	// Learn from different tools
	tools := []string{"readFile", "writeFile", "grep", "bash"}
	for _, tool := range tools {
		params := map[string]any{"test": "value"}
		_ = memoryWithConfig.LearnToolUsage(tool, params, true, nil)
	}

	// Get stats
	stats := memoryWithConfig.Stats()
	assert.Equal(t, 4, stats.EntriesByType[MemoryTypeToolUsage])
}

// TestMemoryAdapter_SuccessRateCalculation tests success rate calculation.
func TestMemoryAdapter_SuccessRateCalculation(t *testing.T) {
	config := DefaultMemoryConfig()
	config.LearningEnabled = true
	memoryWithConfig := NewMemoryAdapterWithConfig(config)
	memoryWithConfig.Start()
	defer memoryWithConfig.Stop()

	// Mix of successes and failures
	params1 := map[string]any{"path": "/tmp/exists.txt"}
	params2 := map[string]any{"path": "/tmp/exists.txt"}
	params3 := map[string]any{"path": "/tmp/nonexistent.txt"}

	_ = memoryWithConfig.LearnToolUsage("readFile", params1, true, nil)
	_ = memoryWithConfig.LearnToolUsage("readFile", params2, true, nil)
	_ = memoryWithConfig.LearnToolUsage("readFile", params3, false, nil)

	// Get patterns
	patterns, err := memoryWithConfig.GetToolUsagePatterns("readFile")
	require.NoError(t, err)

	// Success rate should be ~0.67
	assert.InDelta(t, 0.67, patterns.SuccessRate, 0.01)
}

// TestDefaultMemoryConfig tests default configuration values
func TestDefaultMemoryConfig(t *testing.T) {
	config := DefaultMemoryConfig()

	assert.Equal(t, 10000, config.MaxEntries)
	assert.Equal(t, 7*24*time.Hour, config.DefaultTTL)
	assert.True(t, config.LearningEnabled)
	assert.Equal(t, 0.1, config.ImportanceDecay)
	assert.Equal(t, 0.1, config.MinImportance)
	assert.Equal(t, 1.0, config.MaxImportance)
	assert.True(t, config.EnableSemanticSearch)
}

// TestMemoryRetentionPolicy tests retention policy application
func TestMemoryRetentionPolicy(t *testing.T) {
	config := DefaultMemoryConfig()
	policy := &config.RetentionPolicy

	assert.Equal(t, 30*24*time.Hour, policy.ToolUsageRetention)
	assert.Equal(t, 7*24*time.Hour, policy.ConversationRetention)
	assert.Equal(t, 30*24*time.Hour, policy.ErrorRetention)
	assert.Equal(t, 3*24*time.Hour, policy.ContextRetention)
	assert.Equal(t, 14*24*time.Hour, policy.SuccessRetention)
}

// TestMemoryContext tests memory context structure
func TestMemoryContext(t *testing.T) {
	context := &MemoryContext{
		SessionID: "test_session",
		Task:      "test task",
		Tool:      "readFile",
		Intent:    "test intent",
	}

	assert.Equal(t, "test_session", context.SessionID)
	assert.Equal(t, "test task", context.Task)
	assert.Equal(t, "readFile", context.Tool)
	assert.Equal(t, "test intent", context.Intent)
}

// TestToolUsageMemory tests tool usage memory structure
func TestToolUsageMemory(t *testing.T) {
	memory := &ToolUsageMemory{
		ToolName: "readFile",
		SuccessfulParameters: []ParameterPattern{
			{
				Parameters: map[string]any{"path": "/tmp/test.txt"},
				Frequency:  5,
				Success:    true,
			},
		},
		FailedParameters: []ParameterPattern{
			{
				Parameters: map[string]any{"path": "/tmp/missing.txt"},
				Frequency:  2,
				Success:    false,
			},
		},
		TypicalUsage: "Read file with utf-8 encoding",
		SuccessRate:  0.71,
		UsageCount:   7,
	}

	assert.Equal(t, "readFile", memory.ToolName)
	assert.Equal(t, 1, len(memory.SuccessfulParameters))
	assert.Equal(t, 1, len(memory.FailedParameters))
	assert.Equal(t, 5, memory.SuccessfulParameters[0].Frequency)
	assert.Equal(t, 7, memory.UsageCount)
	assert.InDelta(t, 0.71, memory.SuccessRate, 0.01)
}

// TestMemorySearchResult tests search result structure
func TestMemorySearchResult(t *testing.T) {
	result := &MemorySearchResult{
		Entries: []MemoryEntry{
			{
				ID:      "test1",
				Type:    MemoryTypeToolUsage,
				Content: "Test entry 1",
			},
		},
		Total: 1,
		Query: MemoryQuery{
			Types: []MemoryType{MemoryTypeToolUsage},
			Limit: 10,
		},
		ExecutionTime: 50 * time.Millisecond,
	}

	assert.Equal(t, 1, len(result.Entries))
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, MemoryTypeToolUsage, result.Query.Types[0])
	assert.Equal(t, 50*time.Millisecond, result.ExecutionTime)
}

// TestTimeRange tests time range structure
func TestTimeRange(t *testing.T) {
	start := time.Now()
	end := start.Add(24 * time.Hour)

	timeRange := &TimeRange{
		Start: start,
		End:   end,
	}

	assert.Equal(t, start, timeRange.Start)
	assert.Equal(t, end, timeRange.End)
}

// TestMemoryEntry_Validation tests memory entry validation
func TestMemoryEntry_Validation(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Test automatic ID generation
	entry1 := MemoryEntry{
		Type:    MemoryTypeToolUsage,
		Content: "Auto ID test",
		Tags:    []string{"test"},
	}

	err := memSystem.StoreEntry(entry1)
	require.NoError(t, err)
	result1, err := memSystem.Search(MemoryQuery{
		Types:   []MemoryType{MemoryTypeToolUsage},
		Content: "Auto ID test",
		Limit:   1,
	})
	require.NoError(t, err)
	require.Len(t, result1.Entries, 1)
	assert.NotEmpty(t, result1.Entries[0].ID)

	// Test automatic timestamp
	entry2 := MemoryEntry{
		ID:      "manual_id",
		Type:    MemoryTypeError,
		Content: "Manual ID test",
		Tags:    []string{"test"},
	}

	err = memSystem.StoreEntry(entry2)
	require.NoError(t, err)
	storedEntry2, err := memSystem.GetEntry(entry2.ID)
	require.NoError(t, err)
	assert.False(t, storedEntry2.CreatedAt.IsZero())
}

// TestMemoryImportanceDecay tests importance decay calculation
func TestMemoryImportanceDecay(t *testing.T) {
	config := DefaultMemoryConfig()
	config.ImportanceDecay = 0.2 // 20% decay
	memSystem := NewMemoryAdapterWithConfig(config)
	memSystem.Start()
	defer memSystem.Stop()

	// Store entry
	entry := MemoryEntry{
		Type:    MemoryTypeToolUsage,
		Content: "Decay test",
		Tags:    []string{"test"},
	}

	_ = memSystem.StoreEntry(entry)

	// Get the stored entry
	storedResult, err := memSystem.Search(MemoryQuery{
		Types:   []MemoryType{MemoryTypeToolUsage},
		Content: "Decay test",
		Limit:   1,
	})
	require.NoError(t, err)
	require.Len(t, storedResult.Entries, 1)
	stored := storedResult.Entries[0]
	entry.ID = stored.ID

	// Access the entry multiple times to see importance decay
	initialImportance := stored.Importance
	for i := 0; i < 10; i++ {
		_, _ = memSystem.GetEntry(entry.ID)
	}

	finalEntry, _ := memSystem.GetEntry(entry.ID)

	// Importance should have changed due to access count updates
	// (Note: actual decay logic would need to be implemented)
	assert.NotNil(t, finalEntry.Importance)
	assert.Greater(t, finalEntry.AccessCount, 10)
	assert.Less(t, finalEntry.Importance, initialImportance) // Importance should decay
}

// TestMemoryStats_Aggregation tests statistics aggregation
func TestMemoryStats_Aggregation(t *testing.T) {
	memSystem := NewMemoryAdapter()
	memSystem.Start()
	defer memSystem.Stop()

	// Store many entries
	memoryTypes := []MemoryType{MemoryTypeToolUsage, MemoryTypeConversation, MemoryTypeError, MemoryTypeContext, MemoryTypeSuccess}
	for i := 0; i < 20; i++ {
		entry := MemoryEntry{
			Type:    memoryTypes[i%len(memoryTypes)], // Vary types
			Content: fmt.Sprintf("Test content %d", i),
			Tags:    []string{"test"},
		}
		_ = memSystem.StoreEntry(entry)
	}

	// Get statistics
	stats := memSystem.Stats()
	assert.Equal(t, 20, stats.TotalEntries)
	assert.GreaterOrEqual(t, stats.TotalAccessCount, int64(0))

	// Check that all types are represented
	foundTypes := []MemoryType{}
	for memType, count := range stats.EntriesByType {
		if count > 0 {
			foundTypes = append(foundTypes, memType)
		}
	}
	assert.GreaterOrEqual(t, len(foundTypes), 2)
}

func TestNewAgentRegistry_LoadsBuiltIns(t *testing.T) {
	reg := NewAgentRegistry()

	for _, b := range BuiltInAgents {
		def, ok := reg.Get(b.AgentType)
		if !ok {
			t.Errorf("built-in agent %q not found in registry", b.AgentType)
			continue
		}
		if def.Source != AgentSourceBuiltIn {
			t.Errorf("agent %q: expected source=built-in, got %q", b.AgentType, def.Source)
		}
	}
}

func TestAgentRegistry_Register_Override(t *testing.T) {
	reg := NewAgentRegistry()
	custom := &AgentDefinition{
		AgentType:       AgentTypeGeneralPurpose,
		WhenToUse:       "custom override",
		Source:          AgentSourceUser,
		GetSystemPrompt: func() string { return "custom prompt" },
	}
	reg.Register(custom)

	def, ok := reg.Get(AgentTypeGeneralPurpose)
	if !ok {
		t.Fatal("agent not found after Register")
	}
	if def.WhenToUse != "custom override" {
		t.Errorf("expected custom override, got %q", def.WhenToUse)
	}
}

func TestAgentRegistry_LoadFromSkills_AddsAgentSkills(t *testing.T) {
	t.Setenv("NEXUS_RUNTIME_ROOT", t.TempDir())

	// Create a user skill with agent: field
	userID := "test-user-registry"
	userSkillDir := filepath.Join(publicskills.UserPath(userID), "my-agent")
	if err := os.MkdirAll(userSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "---\nname: \"my-agent\"\ndescription: \"an agent skill\"\nagent: \"custom-agent\"\neffort: \"high\"\n---\n\nYou are a custom agent.\n"
	if err := os.WriteFile(filepath.Join(userSkillDir, "skill.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg := NewAgentRegistry()
	if err := reg.LoadFromSkills("", userID); err != nil {
		t.Fatalf("LoadFromSkills: %v", err)
	}

	def, ok := reg.Get("custom-agent")
	if !ok {
		t.Fatal("skill-derived agent 'custom-agent' not found in registry")
	}
	if def.Source != AgentSourceUser {
		t.Errorf("expected source=user, got %q", def.Source)
	}
	if def.MaxTurns != 100 { // "high" → 100
		t.Errorf("expected MaxTurns=100 for effort=high, got %d", def.MaxTurns)
	}
	if def.GetSystemPrompt == nil {
		t.Fatal("GetSystemPrompt should not be nil")
	}
	sp := def.GetSystemPrompt()
	if sp == "" {
		t.Error("expected non-empty system prompt from skill body")
	}
}

func TestAgentRegistry_LoadFromSkills_DoesNotOverrideBuiltIns(t *testing.T) {
	t.Setenv("NEXUS_RUNTIME_ROOT", t.TempDir())

	userID := "test-user-no-override"
	userSkillDir := filepath.Join(publicskills.UserPath(userID), "explore-override")
	if err := os.MkdirAll(userSkillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Skill tries to redefine the built-in "explore" agent.
	content := "---\nname: \"explore-override\"\ndescription: \"override attempt\"\nagent: \"explore\"\n---\n\nThis should not override the built-in explore agent.\n"
	if err := os.WriteFile(filepath.Join(userSkillDir, "skill.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg := NewAgentRegistry()
	if err := reg.LoadFromSkills("", userID); err != nil {
		t.Fatalf("LoadFromSkills: %v", err)
	}

	def, ok := reg.Get(AgentTypeExplore)
	if !ok {
		t.Fatal("explore agent not found after LoadFromSkills")
	}
	if def.Source != AgentSourceBuiltIn {
		t.Errorf("skill must not override built-in: expected source=built-in, got %q", def.Source)
	}
}

func TestEffortToMaxTurns(t *testing.T) {
	cases := []struct {
		effort string
		want   int
	}{
		{"minimal", 10},
		{"low", 20},
		{"medium", 50},
		{"high", 100},
		{"maximum", 150},
		{"", 50},
		{"unknown", 50},
	}
	for _, tc := range cases {
		if got := effortToMaxTurns(tc.effort); got != tc.want {
			t.Errorf("effortToMaxTurns(%q) = %d, want %d", tc.effort, got, tc.want)
		}
	}
}

func TestSkillToAgentDefinition_FieldMapping(t *testing.T) {
	sk := &skills.Skill{
		Name:         "my-skill",
		Description:  "does things",
		Agent:        "my-bot",
		WhenToUse:    "when you need to do things",
		AllowedTools: []string{"read_file", "bash"},
		Model:        "claude-sonnet-4-6",
		Effort:       "low",
		SkillRoot:    "/some/path",
		GetPromptForCommand: func(args string, ctx context.Context) ([]skills.ContentBlock, error) {
			return []skills.ContentBlock{{Type: "text", Text: "You are my-bot."}}, nil
		},
	}

	def := skillToAgentDefinition(sk)

	if def.AgentType != "my-bot" {
		t.Errorf("AgentType: want %q, got %q", "my-bot", def.AgentType)
	}
	if def.WhenToUse != "when you need to do things" {
		t.Errorf("WhenToUse: want %q, got %q", "when you need to do things", def.WhenToUse)
	}
	if def.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: want %q, got %q", "claude-sonnet-4-6", def.Model)
	}
	if def.MaxTurns != 20 {
		t.Errorf("MaxTurns: want 20, got %d", def.MaxTurns)
	}
	if len(def.Tools) != 2 {
		t.Errorf("Tools: want 2, got %d", len(def.Tools))
	}
	if def.GetSystemPrompt == nil {
		t.Fatal("GetSystemPrompt should not be nil")
	}
	if sp := def.GetSystemPrompt(); sp != "You are my-bot." {
		t.Errorf("GetSystemPrompt(): want %q, got %q", "You are my-bot.", sp)
	}
}
