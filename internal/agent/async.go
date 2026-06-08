package agent

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Async Agent Types
// ---------------------------------------------------------------------------

// AgentEventType represents the type of async agent event
type AgentEventType string

const (
	AgentEventStarted    AgentEventType = "started"
	AgentEventCompleted  AgentEventType = "completed"
	AgentEventFailed     AgentEventType = "failed"
	AgentEventProgress   AgentEventType = "progress"
	AgentEventCancelled  AgentEventType = "cancelled"
	AgentEventTurnUpdate AgentEventType = "turn_update"
	AgentEventToolUse    AgentEventType = "tool_use"
)

// AgentEvent represents a real-time event from an async agent
type AgentEvent struct {
	// AgentID is the unique identifier for this agent instance
	AgentID string `json:"agentId"`

	// AgentType is the type of agent (general-purpose, explore, plan, verify)
	AgentType string `json:"agentType"`

	// EventType is the type of event
	EventType AgentEventType `json:"eventType"`

	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Task is the original task description
	Task string `json:"task,omitempty"`

	// Progress data (for progress events)
	Progress *AgentProgress `json:"progress,omitempty"`

	// Result data (for completed/failed events)
	Result *RunResult `json:"result,omitempty"`

	// Error message (for failed events)
	Error string `json:"error,omitempty"`

	// Turn number (for turn_update events)
	Turn int `json:"turn,omitempty"`

	// Tool name (for tool_use events)
	ToolName string `json:"toolName,omitempty"`

	// Additional metadata
	Metadata map[string]any `json:"metadata,omitempty"`
}

// AgentProgress represents progress information for an async agent
type AgentProgress struct {
	// CurrentTurn number
	CurrentTurn int `json:"currentTurn"`

	// MaxTurns limit
	MaxTurns int `json:"maxTurns"`

	// ToolUses so far
	ToolUses int `json:"toolUses"`

	// CurrentOutput so far
	Output string `json:"output"`

	// Percentage complete (estimated)
	PercentComplete float64 `json:"percentComplete"`
}

// AgentEventListener is a callback function for agent events
type AgentEventListener func(event AgentEvent)

// AgentStatus represents the current status of an async agent
type AgentStatus string

const (
	AgentStatusPending   AgentStatus = "pending"
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusCancelled AgentStatus = "cancelled"
)

// AsyncAgent represents an asynchronous agent execution
type AsyncAgent struct {
	// Unique identifier
	ID string

	// Agent configuration
	Config *RunConfig

	// Nickname is the optional human-friendly name assigned at spawn time.
	// Mirrors Codex's agent_nickname in CollabAgentRef.
	Nickname string

	// Role is the optional role assigned at spawn time (e.g. "reviewer").
	// Mirrors Codex's agent_role in CollabAgentRef.
	Role string

	// SessionID is the engine session ID used by this agent's run. Set after
	// the run completes and exposes the session for resumption via resume_agent.
	SessionID types.SessionID

	// Current status
	Status AgentStatus

	// Context for cancellation
	Ctx    context.Context
	Cancel context.CancelFunc

	// Start time
	StartTime time.Time

	// End time
	EndTime time.Time

	// Current turn count
	CurrentTurn int

	// Current output so far
	CurrentOutput string

	// Tool uses count
	ToolUses int

	// Final result (when completed)
	Result *RunResult

	// Error if failed
	Error error

	// pendingMessages holds messages sent via SendMessage that will be
	// injected as continuation prompts between turns.
	pendingMessages []string
	messagesMu      sync.Mutex

	// Listeners for events
	listeners   []AgentEventListener
	listenersMu sync.RWMutex

	// Progress tracking (CurrentTurn, CurrentOutput, ToolUses)
	progressMu sync.RWMutex

	// State tracking (Status, Error, EndTime, Result)
	stateMu sync.RWMutex

	// Finalization channel
	done chan struct{}
}

// ---------------------------------------------------------------------------
// Async Agent Manager
// ---------------------------------------------------------------------------

// AsyncAgentManager manages concurrent async agent executions
type AsyncAgentManager struct {
	// Active agents
	agents   map[string]*AsyncAgent
	agentsMu sync.RWMutex

	// Event dispatcher
	eventChan chan AgentEvent

	// Global event listeners
	globalListeners   []AgentEventListener
	globalListenersMu sync.RWMutex

	// Worker pool for event dispatch
	workerPoolSize int
	workersWg      sync.WaitGroup

	// Tracks active agent goroutines so Shutdown can wait for them to finish
	agentsWg sync.WaitGroup

	// Shutdown flag
	shutdown   bool
	shutdownMu sync.RWMutex

	// Event dispatcher context
	dispatcherCtx    context.Context
	dispatcherCancel context.CancelFunc
}

// NewAsyncAgentManager creates a new async agent manager
func NewAsyncAgentManager() *AsyncAgentManager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &AsyncAgentManager{
		agents:           make(map[string]*AsyncAgent),
		eventChan:        make(chan AgentEvent, 1000), // Buffered channel for events
		workerPoolSize:   5,                           // Default 5 workers for event dispatch
		dispatcherCtx:    ctx,
		dispatcherCancel: cancel,
	}

	// Start event dispatcher
	manager.startEventDispatcher()

	return manager
}

// SetWorkerPoolSize sets the size of the worker pool for event dispatch
func (m *AsyncAgentManager) SetWorkerPoolSize(size int) {
	m.workersWg.Wait()
	m.workerPoolSize = size
	m.startEventDispatcher()
}

// StartEventDispatcher starts the event dispatcher workers
func (m *AsyncAgentManager) startEventDispatcher() {
	for i := 0; i < m.workerPoolSize; i++ {
		m.workersWg.Add(1)
		go m.eventWorker()
	}
}

// eventWorker processes events from the event channel
func (m *AsyncAgentManager) eventWorker() {
	defer m.workersWg.Done()

	for {
		select {
		case event := <-m.eventChan:
			m.dispatchEvent(event)

		case <-m.dispatcherCtx.Done():
			return
		}
	}
}

// dispatchEvent dispatches an event to all registered listeners
func (m *AsyncAgentManager) dispatchEvent(event AgentEvent) {
	// Get global listeners
	m.globalListenersMu.RLock()
	globalListeners := make([]AgentEventListener, len(m.globalListeners))
	copy(globalListeners, m.globalListeners)
	m.globalListenersMu.RUnlock()

	// Dispatch to global listeners
	for _, listener := range globalListeners {
		go func(l AgentEventListener, e AgentEvent) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("async-manager event listener panic", "panic", r, "event_type", e.EventType)
				}
			}()
			l(e)
		}(listener, event)
	}

	// Get agent-specific listeners
	m.agentsMu.RLock()
	agent, exists := m.agents[event.AgentID]
	m.agentsMu.RUnlock()

	if exists {
		agent.listenersMu.RLock()
		agentListeners := make([]AgentEventListener, len(agent.listeners))
		copy(agentListeners, agent.listeners)
		agent.listenersMu.RUnlock()

		// Dispatch to agent-specific listeners
		for _, listener := range agentListeners {
			go func(l AgentEventListener, e AgentEvent) {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("async-manager agent listener panic", "panic", r, "agent_id", e.AgentID)
					}
				}()
				l(e)
			}(listener, event)
		}
	}
}

// AddGlobalListener adds a global event listener for all agents
func (m *AsyncAgentManager) AddGlobalListener(listener AgentEventListener) {
	m.globalListenersMu.Lock()
	defer m.globalListenersMu.Unlock()
	m.globalListeners = append(m.globalListeners, listener)
}

// RemoveGlobalListener removes a global event listener by pointer identity.
func (m *AsyncAgentManager) RemoveGlobalListener(listener AgentEventListener) {
	m.globalListenersMu.Lock()
	defer m.globalListenersMu.Unlock()

	listenerPtr := reflect.ValueOf(listener).Pointer()
	for i, l := range m.globalListeners {
		if reflect.ValueOf(l).Pointer() == listenerPtr {
			m.globalListeners = append(m.globalListeners[:i], m.globalListeners[i+1:]...)
			break
		}
	}
}

// StartAgent starts an agent asynchronously and returns immediately.
// RunConfig.Nickname and RunConfig.Role (if set) are copied to the agent for identification.
func (m *AsyncAgentManager) StartAgent(config *RunConfig) (*AsyncAgent, error) {
	m.shutdownMu.RLock()
	shutdown := m.shutdown
	m.shutdownMu.RUnlock()

	if shutdown {
		return nil, fmt.Errorf("async manager is shutting down")
	}

	// Generate unique agent ID
	agentID := generateAgentID(config.AgentType)

	// Create async agent
	ctx, cancel := context.WithCancel(context.Background())

	asyncAgent := &AsyncAgent{
		ID:            agentID,
		Config:        config,
		Nickname:      config.Nickname,
		Role:          config.Role,
		Status:        AgentStatusPending,
		Ctx:           ctx,
		Cancel:        cancel,
		StartTime:     time.Now(),
		CurrentTurn:   0,
		CurrentOutput: "",
		ToolUses:      0,
		done:          make(chan struct{}),
	}

	// Register agent
	m.agentsMu.Lock()
	m.agents[agentID] = asyncAgent
	m.agentsMu.Unlock()

	// Start agent in background, tracking the goroutine so Shutdown can wait.
	m.agentsWg.Add(1)
	go func() {
		defer m.agentsWg.Done()
		m.runAgent(asyncAgent)
	}()

	// Send started event
	m.emitEvent(asyncAgent, AgentEventStarted, nil, nil, nil)

	return asyncAgent, nil
}

// runAgent executes an agent asynchronously with progress tracking
func (m *AsyncAgentManager) runAgent(agent *AsyncAgent) {
	defer close(agent.done)
	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("agent panic: %v", r)
			agent.stateMu.Lock()
			agent.Error = panicErr
			agent.Status = AgentStatusFailed
			agent.EndTime = time.Now()
			agent.stateMu.Unlock()
			m.emitEvent(agent, AgentEventFailed, nil, nil, panicErr)
		}
	}()

	agent.stateMu.Lock()
	agent.Status = AgentStatusRunning
	agent.stateMu.Unlock()

	// Create a config with progress callback and message injection.
	config := *agent.Config
	// Replace the caller-supplied context (typically the parent session's turn
	// context, which gets canceled when that turn ends) with the async agent's
	// own independent context. Without this, the sub-agent's API calls and
	// permission prompts fail as soon as the parent turn finishes.
	config.Context = agent.Ctx

	// Wire ContinuationMessage to drain pending inter-agent messages.
	// If no message is queued, fall back to the existing callback or the default.
	prevContinuation := config.ContinuationMessage
	config.ContinuationMessage = func(turn int, output string) string {
		if msg := agent.dequeuePendingMessage(); msg != "" {
			return msg
		}
		if prevContinuation != nil {
			return prevContinuation(turn, output)
		}
		return "" // runner uses its own default
	}

	config.Callback = func(turn int, output string, toolUses int) {
		agent.progressMu.Lock()
		agent.CurrentTurn = turn
		agent.CurrentOutput = output
		agent.ToolUses = toolUses

		maxTurns := config.MaxTurns
		if maxTurns == 0 {
			maxTurns = DefaultMaxTurns
		}
		percentComplete := float64(turn) / float64(maxTurns) * 100
		agent.progressMu.Unlock()

		progress := &AgentProgress{
			CurrentTurn:     turn,
			MaxTurns:        maxTurns,
			ToolUses:        toolUses,
			Output:          output,
			PercentComplete: percentComplete,
		}
		m.emitEvent(agent, AgentEventProgress, nil, progress, nil)
	}

	// Run the agent
	result, err := RunAgent(&config)

	// Sync final counts from result so GetProgress() is accurate after completion.
	if result != nil {
		agent.progressMu.Lock()
		agent.ToolUses = result.ToolUses
		agent.progressMu.Unlock()
		if result.SessionID != "" {
			agent.stateMu.Lock()
			agent.SessionID = result.SessionID
			agent.stateMu.Unlock()
		}
	}

	endTime := time.Now()
	agent.stateMu.Lock()
	agent.EndTime = endTime
	if err != nil {
		agent.Error = err
		agent.Status = AgentStatusFailed
	} else if result.Success {
		agent.Result = result
		agent.Status = AgentStatusCompleted
	} else {
		agent.Error = fmt.Errorf("%s", result.Error)
		agent.Status = AgentStatusFailed
	}
	finalErr := agent.Error
	finalResult := agent.Result
	finalStatus := agent.Status
	agent.stateMu.Unlock()

	if finalStatus == AgentStatusCompleted {
		m.emitEvent(agent, AgentEventCompleted, finalResult, nil, nil)
	} else {
		slog.Error("async agent failed",
			"agent_id", agent.ID,
			"agent_type", agent.Config.AgentType,
			"error", finalErr,
		)
		m.emitEvent(agent, AgentEventFailed, nil, nil, finalErr)
	}
}

// CancelAgent cancels a running agent
func (m *AsyncAgentManager) CancelAgent(agentID string) error {
	m.agentsMu.RLock()
	agent, exists := m.agents[agentID]
	m.agentsMu.RUnlock()

	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	agent.stateMu.Lock()
	if agent.Status != AgentStatusRunning {
		status := agent.Status
		agent.stateMu.Unlock()
		return fmt.Errorf("agent is not running: %s (status: %s)", agentID, status)
	}
	agent.Cancel()
	agent.Status = AgentStatusCancelled
	agent.EndTime = time.Now()
	agent.stateMu.Unlock()

	m.emitEvent(agent, AgentEventCancelled, nil, nil, fmt.Errorf("agent cancelled"))

	return nil
}

// GetAgent retrieves an async agent by ID
func (m *AsyncAgentManager) GetAgent(agentID string) (*AsyncAgent, error) {
	m.agentsMu.RLock()
	defer m.agentsMu.RUnlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	return agent, nil
}

// ListAgents returns all active agents
func (m *AsyncAgentManager) ListAgents() []*AsyncAgent {
	m.agentsMu.RLock()
	defer m.agentsMu.RUnlock()

	agents := make([]*AsyncAgent, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}

	return agents
}

// Cleanup removes completed agents from memory
func (m *AsyncAgentManager) Cleanup() {
	m.agentsMu.Lock()
	defer m.agentsMu.Unlock()

	for id, agent := range m.agents {
		agent.stateMu.RLock()
		status := agent.Status
		agent.stateMu.RUnlock()

		if status == AgentStatusCompleted ||
			status == AgentStatusFailed ||
			status == AgentStatusCancelled {
			delete(m.agents, id)
		}
	}
}

// Shutdown gracefully shuts down the async manager
func (m *AsyncAgentManager) Shutdown() {
	m.shutdownMu.Lock()
	m.shutdown = true
	m.shutdownMu.Unlock()

	// Cancel all running agents.
	m.agentsMu.Lock()
	for _, agent := range m.agents {
		agent.stateMu.Lock()
		if agent.Status == AgentStatusRunning {
			agent.Cancel()
			agent.Status = AgentStatusCancelled
		}
		agent.stateMu.Unlock()
	}
	m.agentsMu.Unlock()

	// Wait for all agent goroutines to finish (including those that were still
	// starting up when we cancelled). Without this, Shutdown returns before
	// goroutines transition out of AgentStatusRunning.
	m.agentsWg.Wait()

	// Release memory held by completed/failed/cancelled agents now that all
	// goroutines are done and no new ones can start (shutdown=true).
	m.Cleanup()

	// Stop event dispatcher and wait for event workers.
	m.dispatcherCancel()
	m.workersWg.Wait()

	// Close event channel
	close(m.eventChan)
}

// SendMessage enqueues a message to be delivered to the agent on its next inter-turn
// continuation. Returns an error if the agent does not exist or is no longer active.
// Mirrors Codex's send_inter_agent_communication / sendInput tool.
func (m *AsyncAgentManager) SendMessage(agentID string, message string) error {
	m.agentsMu.RLock()
	agent, exists := m.agents[agentID]
	m.agentsMu.RUnlock()
	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}
	agent.stateMu.RLock()
	status := agent.Status
	agent.stateMu.RUnlock()
	if status != AgentStatusRunning && status != AgentStatusPending {
		return fmt.Errorf("agent %s is not active (status: %s)", agentID, status)
	}
	agent.messagesMu.Lock()
	agent.pendingMessages = append(agent.pendingMessages, message)
	agent.messagesMu.Unlock()
	return nil
}

// CloseAgent gracefully terminates an agent and removes it from the registry.
// Mirrors Codex's close_agent / shutdown_live_agent.
// Unlike CancelAgent (which only works for running agents), CloseAgent works for
// any non-final state and removes the agent from memory after shutdown.
func (m *AsyncAgentManager) CloseAgent(agentID string) error {
	m.agentsMu.RLock()
	agent, exists := m.agents[agentID]
	m.agentsMu.RUnlock()
	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}
	agent.stateMu.Lock()
	if agent.Status == AgentStatusRunning || agent.Status == AgentStatusPending {
		agent.Cancel()
		agent.Status = AgentStatusCancelled
		agent.EndTime = time.Now()
		agent.stateMu.Unlock()
		m.emitEvent(agent, AgentEventCancelled, nil, nil, fmt.Errorf("agent closed"))
	} else {
		agent.stateMu.Unlock()
	}
	// Remove from registry so future list_agents calls don't include it.
	m.agentsMu.Lock()
	delete(m.agents, agentID)
	m.agentsMu.Unlock()
	return nil
}

// CloseAllAgents terminates every tracked async agent and removes them from the registry.
func (m *AsyncAgentManager) CloseAllAgents() int {
	m.agentsMu.RLock()
	ids := make([]string, 0, len(m.agents))
	for id := range m.agents {
		ids = append(ids, id)
	}
	m.agentsMu.RUnlock()

	closed := 0
	for _, id := range ids {
		if err := m.CloseAgent(id); err == nil {
			closed++
		}
	}
	return closed
}

// emitEvent emits an event for an agent
func (m *AsyncAgentManager) emitEvent(agent *AsyncAgent, eventType AgentEventType, result *RunResult, progress *AgentProgress, err error) {
	// Snapshot volatile state under stateMu to avoid data races.
	agent.stateMu.RLock()
	status := agent.Status
	endTime := agent.EndTime
	agent.stateMu.RUnlock()

	event := AgentEvent{
		AgentID:   agent.ID,
		AgentType: agent.Config.AgentType,
		EventType: eventType,
		Timestamp: time.Now(),
		Task:      agent.Config.Task,
		Result:    result,
		Progress:  progress,
		Error:     "",
		Metadata:  make(map[string]any),
	}

	if err != nil {
		event.Error = err.Error()
	}

	// Add progress data if available and not explicitly provided
	if progress == nil && status == AgentStatusRunning {
		agent.progressMu.RLock()
		event.Progress = &AgentProgress{
			CurrentTurn:     agent.CurrentTurn,
			MaxTurns:        agent.Config.MaxTurns,
			ToolUses:        agent.ToolUses,
			Output:          agent.CurrentOutput,
			PercentComplete: 0, // Will be calculated in progress events
		}
		agent.progressMu.RUnlock()
	}

	// Add metadata — StartTime is safe without lock (written once before goroutine starts).
	event.Metadata["status"] = string(status)
	event.Metadata["start_time"] = agent.StartTime

	if !endTime.IsZero() {
		event.Metadata["end_time"] = endTime
		duration := endTime.Sub(agent.StartTime)
		event.Metadata["duration_ms"] = duration.Milliseconds()
	}

	// Send event to channel (non-blocking)
	m.shutdownMu.RLock()
	shutdown := m.shutdown
	m.shutdownMu.RUnlock()

	if shutdown {
		return // Don't send events if manager is shutting down
	}

	select {
	case m.eventChan <- event:
	default:
		slog.Warn("async-manager event channel full, dropping event", "event_type", eventType)
	}
}

// ---------------------------------------------------------------------------
// Async Agent Methods
// ---------------------------------------------------------------------------

// dequeuePendingMessage pops and returns the oldest pending message, or "" if none.
func (a *AsyncAgent) dequeuePendingMessage() string {
	a.messagesMu.Lock()
	defer a.messagesMu.Unlock()
	if len(a.pendingMessages) == 0 {
		return ""
	}
	msg := a.pendingMessages[0]
	a.pendingMessages = a.pendingMessages[1:]
	return msg
}

// CollabStatus maps Nexus AgentStatus to the Codex CollabAgentStatus vocabulary
// ("pendingInit" | "running" | "interrupted" | "completed" | "errored" | "shutdown" | "notFound").
func (a *AsyncAgent) CollabStatus() string {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	switch a.Status {
	case AgentStatusPending:
		return "pendingInit"
	case AgentStatusRunning:
		return "running"
	case AgentStatusCompleted:
		return "completed"
	case AgentStatusFailed:
		return "errored"
	case AgentStatusCancelled:
		return "shutdown"
	default:
		return "notFound"
	}
}

// AddListener adds an event listener for this specific agent.
// If the agent has already started, replay a snapshot so late subscribers do
// not depend on event timing to observe the current state.
func (a *AsyncAgent) AddListener(listener AgentEventListener) {
	a.listenersMu.Lock()
	a.listeners = append(a.listeners, listener)
	a.listenersMu.Unlock()

	event, ok := a.snapshotEvent()
	if !ok {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("async-agent listener replay panic", "panic", r, "agent_id", event.AgentID)
			}
		}()
		listener(event)
	}()
}

// RemoveListener removes an event listener by pointer identity.
func (a *AsyncAgent) RemoveListener(listener AgentEventListener) {
	a.listenersMu.Lock()
	defer a.listenersMu.Unlock()

	listenerPtr := reflect.ValueOf(listener).Pointer()
	for i, l := range a.listeners {
		if reflect.ValueOf(l).Pointer() == listenerPtr {
			a.listeners = append(a.listeners[:i], a.listeners[i+1:]...)
			break
		}
	}
}

func (a *AsyncAgent) snapshotEvent() (AgentEvent, bool) {
	a.stateMu.RLock()
	status := a.Status
	startTime := a.StartTime
	endTime := a.EndTime
	result := a.Result
	err := a.Error
	a.stateMu.RUnlock()

	eventType := AgentEventType("")
	switch status {
	case AgentStatusRunning:
		eventType = AgentEventStarted
	case AgentStatusCompleted:
		eventType = AgentEventCompleted
	case AgentStatusFailed:
		eventType = AgentEventFailed
	case AgentStatusCancelled:
		eventType = AgentEventCancelled
	default:
		return AgentEvent{}, false
	}

	a.progressMu.RLock()
	progress := &AgentProgress{
		CurrentTurn:     a.CurrentTurn,
		MaxTurns:        a.Config.MaxTurns,
		ToolUses:        a.ToolUses,
		Output:          a.CurrentOutput,
		PercentComplete: 0,
	}
	a.progressMu.RUnlock()

	if progress.MaxTurns == 0 {
		progress.MaxTurns = DefaultMaxTurns
	}
	if progress.MaxTurns > 0 {
		progress.PercentComplete = float64(progress.CurrentTurn) / float64(progress.MaxTurns) * 100
	}

	event := AgentEvent{
		AgentID:   a.ID,
		AgentType: a.Config.AgentType,
		EventType: eventType,
		Timestamp: time.Now(),
		Task:      a.Config.Task,
		Progress:  progress,
		Result:    result,
		Metadata: map[string]any{
			"status":     string(status),
			"start_time": startTime,
		},
	}
	if err != nil {
		event.Error = err.Error()
	}
	if !endTime.IsZero() {
		event.Metadata["end_time"] = endTime
		event.Metadata["duration_ms"] = endTime.Sub(startTime).Milliseconds()
	}

	return event, true
}

// Wait waits for the agent to complete (or be cancelled)
func (a *AsyncAgent) Wait() {
	<-a.done
}

// WaitWithTimeout waits for the agent to complete with a timeout
func (a *AsyncAgent) WaitWithTimeout(timeout time.Duration) error {
	select {
	case <-a.done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for agent %s to complete", a.ID)
	}
}

// GetProgress returns current progress information
func (a *AsyncAgent) GetProgress() *AgentProgress {
	a.progressMu.RLock()
	defer a.progressMu.RUnlock()

	maxTurns := a.Config.MaxTurns
	if maxTurns == 0 {
		maxTurns = DefaultMaxTurns
	}

	percentComplete := float64(a.CurrentTurn) / float64(maxTurns) * 100

	return &AgentProgress{
		CurrentTurn:     a.CurrentTurn,
		MaxTurns:        maxTurns,
		ToolUses:        a.ToolUses,
		Output:          a.CurrentOutput,
		PercentComplete: percentComplete,
	}
}

// IsRunning returns true if the agent is currently running
func (a *AsyncAgent) IsRunning() bool {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.Status == AgentStatusRunning
}

// IsComplete returns true if the agent has completed (successfully or with error)
func (a *AsyncAgent) IsComplete() bool {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.Status == AgentStatusCompleted ||
		a.Status == AgentStatusFailed ||
		a.Status == AgentStatusCancelled
}

// GetDuration returns the duration of the agent execution
func (a *AsyncAgent) GetDuration() time.Duration {
	a.stateMu.RLock()
	endTime := a.EndTime
	a.stateMu.RUnlock()

	if endTime.IsZero() {
		return time.Since(a.StartTime) // StartTime is safe: written once before goroutine starts
	}
	return endTime.Sub(a.StartTime)
}

// ---------------------------------------------------------------------------
// Utility Functions
// ---------------------------------------------------------------------------

// generateAgentID generates a unique agent ID
func generateAgentID(agentType string) string {
	return fmt.Sprintf("%s-%d", agentType, time.Now().UnixNano())
}

// Default async manager instance
var defaultAsyncManager *AsyncAgentManager
var defaultAsyncManagerOnce sync.Once

// GetDefaultAsyncManager returns the default async manager instance
func GetDefaultAsyncManager() *AsyncAgentManager {
	defaultAsyncManagerOnce.Do(func() {
		defaultAsyncManager = NewAsyncAgentManager()
	})
	return defaultAsyncManager
}
