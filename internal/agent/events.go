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
// Async Agent Hook Events
// ---------------------------------------------------------------------------

const (
	// HookEventAsyncAgentStarted is fired when an async agent starts execution
	HookEventAsyncAgentStarted types.HookEvent = "async_agent_started"

	// HookEventAsyncAgentProgress is fired when an async agent reports progress
	HookEventAsyncAgentProgress types.HookEvent = "async_agent_progress"

	// HookEventAsyncAgentCompleted is fired when an async agent completes successfully
	HookEventAsyncAgentCompleted types.HookEvent = "async_agent_completed"

	// HookEventAsyncAgentFailed is fired when an async agent fails
	HookEventAsyncAgentFailed types.HookEvent = "async_agent_failed"

	// HookEventAsyncAgentCancelled is fired when an async agent is cancelled
	HookEventAsyncAgentCancelled types.HookEvent = "async_agent_cancelled"

	// HookEventAsyncAgentTurnUpdate is fired when an async agent completes a turn
	HookEventAsyncAgentTurnUpdate types.HookEvent = "async_agent_turn_update"

	// HookEventAsyncAgentToolUse is fired when an async agent uses a tool
	HookEventAsyncAgentToolUse types.HookEvent = "async_agent_tool_use"
)

// ---------------------------------------------------------------------------
// Hook Event Data Structures
// ---------------------------------------------------------------------------

// AsyncAgentStartedData contains data for async_agent_started hook event
type AsyncAgentStartedData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	Task      string         `json:"task"`
	StartTime time.Time      `json:"startTime"`
	Config    map[string]any `json:"config"`
	Metadata  map[string]any `json:"metadata"`
}

// AsyncAgentProgressData contains data for async_agent_progress hook event
type AsyncAgentProgressData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	Progress  *AgentProgress `json:"progress"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
}

// AsyncAgentCompletedData contains data for async_agent_completed hook event
type AsyncAgentCompletedData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	Result    *RunResult     `json:"result"`
	Duration  time.Duration  `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
}

// AsyncAgentFailedData contains data for async_agent_failed hook event
type AsyncAgentFailedData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	Error     string         `json:"error"`
	Duration  time.Duration  `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
}

// AsyncAgentCancelledData contains data for async_agent_cancelled hook event
type AsyncAgentCancelledData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	Duration  time.Duration  `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
	Reason    string         `json:"reason,omitempty"`
	Metadata  map[string]any `json:"metadata"`
}

// AsyncAgentTurnUpdateData contains data for async_agent_turn_update hook event
type AsyncAgentTurnUpdateData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	Turn      int            `json:"turn"`
	Output    string         `json:"output"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
}

// AsyncAgentToolUseData contains data for async_agent_tool_use hook event
type AsyncAgentToolUseData struct {
	AgentID   string         `json:"agentId"`
	AgentType string         `json:"agentType"`
	ToolName  string         `json:"toolName"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
}

// ---------------------------------------------------------------------------
// Event Bus Interface
// ---------------------------------------------------------------------------

// EventBus provides a publish-subscribe mechanism for agent events
type EventBus interface {
	// Subscribe subscribes to events of a specific type
	Subscribe(eventType AgentEventType, handler AgentEventListener) func()

	// Publish publishes an event to all subscribers
	Publish(event AgentEvent)

	// Unsubscribe unsubscribes a handler from all events
	Unsubscribe(handler AgentEventListener)
}

// AgentEventBus implements a simple publish-subscribe event bus for agent events
type AgentEventBus struct {
	subscribers   map[AgentEventType][]AgentEventListener
	subscribersMu sync.RWMutex
	eventChan     chan AgentEvent
	workers       int
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewAgentEventBus creates a new agent event bus
func NewAgentEventBus() *AgentEventBus {
	ctx, cancel := context.WithCancel(context.Background())

	bus := &AgentEventBus{
		subscribers: make(map[AgentEventType][]AgentEventListener),
		eventChan:   make(chan AgentEvent, 1000), // Buffered channel
		workers:     3,                           // Default 3 workers
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start event dispatcher
	bus.startDispatcher()

	return bus
}

// SetWorkers sets the number of worker goroutines for event dispatch
func (b *AgentEventBus) SetWorkers(count int) {
	b.workers = count
}

// startDispatcher starts the event dispatcher workers
func (b *AgentEventBus) startDispatcher() {
	for i := 0; i < b.workers; i++ {
		b.wg.Add(1)
		go b.dispatchWorker()
	}
}

// dispatchWorker processes events from the event channel
func (b *AgentEventBus) dispatchWorker() {
	defer b.wg.Done()

	for {
		select {
		case event := <-b.eventChan:
			b.dispatchToSubscribers(event)

		case <-b.ctx.Done():
			return
		}
	}
}

// dispatchToSubscribers dispatches an event to all subscribed handlers
func (b *AgentEventBus) dispatchToSubscribers(event AgentEvent) {
	b.subscribersMu.RLock()
	handlers := b.subscribers[event.EventType]
	b.subscribersMu.RUnlock()

	// Make a copy of handlers to avoid holding lock during dispatch
	handlersCopy := make([]AgentEventListener, len(handlers))
	copy(handlersCopy, handlers)

	for _, handler := range handlersCopy {
		go func(h AgentEventListener, e AgentEvent) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("event-bus handler panic", "panic", r, "event_type", e.EventType)
				}
			}()
			h(e)
		}(handler, event)
	}
}

// Subscribe subscribes to events of a specific type
// Returns an unsubscribe function
func (b *AgentEventBus) Subscribe(eventType AgentEventType, handler AgentEventListener) func() {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	b.subscribers[eventType] = append(b.subscribers[eventType], handler)

	// Return unsubscribe function
	return func() {
		b.subscribersMu.Lock()
		defer b.subscribersMu.Unlock()

		handlers := b.subscribers[eventType]
		handlerPtr := reflect.ValueOf(handler).Pointer()
		for i, h := range handlers {
			if reflect.ValueOf(h).Pointer() == handlerPtr {
				b.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}
}

// Publish publishes an event to the event bus
func (b *AgentEventBus) Publish(event AgentEvent) {
	select {
	case b.eventChan <- event:
	default:
		slog.Warn("event-bus channel full, dropping event", "event_type", event.EventType)
	}
}

// Unsubscribe removes a handler from all event types by pointer identity.
// This works for named functions and function variables, but NOT for method
// values (e.g. obj.Method creates a new function value on each access).
// Prefer using the closure returned by Subscribe for reliable unsubscription.
func (b *AgentEventBus) Unsubscribe(handler AgentEventListener) {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()

	handlerPtr := reflect.ValueOf(handler).Pointer()
	for eventType, handlers := range b.subscribers {
		for i, h := range handlers {
			if reflect.ValueOf(h).Pointer() == handlerPtr {
				b.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}
}

// Shutdown gracefully shuts down the event bus
func (b *AgentEventBus) Shutdown() {
	b.cancel()
	b.wg.Wait()
	close(b.eventChan)
}

// ---------------------------------------------------------------------------
// Real-time Notification System
// ---------------------------------------------------------------------------

// NotificationPriority represents the priority of a notification
type NotificationPriority string

const (
	NotificationPriorityLow    NotificationPriority = "low"
	NotificationPriorityMedium NotificationPriority = "medium"
	NotificationPriorityHigh   NotificationPriority = "high"
	NotificationPriorityUrgent NotificationPriority = "urgent"
)

// RealTimeNotification represents a real-time notification
type RealTimeNotification struct {
	// Unique notification ID
	ID string `json:"id"`

	// Type of notification
	Type string `json:"type"`

	// Priority level
	Priority NotificationPriority `json:"priority"`

	// Title of the notification
	Title string `json:"title"`

	// Detailed message
	Message string `json:"message"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`

	// Associated agent ID (if applicable)
	AgentID string `json:"agentId,omitempty"`

	// Associated data
	Data map[string]any `json:"data,omitempty"`

	// Whether notification is read
	Read bool `json:"read"`

	// Expiration time (optional)
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

// NotificationHandler handles real-time notifications
type NotificationHandler func(notification RealTimeNotification)

// NotificationSystem manages real-time notifications
type NotificationSystem struct {
	notifications   []RealTimeNotification
	notificationsMu sync.RWMutex

	handlers   []NotificationHandler
	handlersMu sync.RWMutex

	notificationChan chan RealTimeNotification
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewNotificationSystem creates a new notification system
func NewNotificationSystem() *NotificationSystem {
	ctx, cancel := context.WithCancel(context.Background())

	system := &NotificationSystem{
		notifications:    make([]RealTimeNotification, 0),
		notificationChan: make(chan RealTimeNotification, 100),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Start notification processor
	go system.processNotifications()

	return system
}

// AddHandler adds a notification handler
func (ns *NotificationSystem) AddHandler(handler NotificationHandler) {
	ns.handlersMu.Lock()
	defer ns.handlersMu.Unlock()
	ns.handlers = append(ns.handlers, handler)
}

// RemoveHandler removes a notification handler by pointer identity.
func (ns *NotificationSystem) RemoveHandler(handler NotificationHandler) {
	ns.handlersMu.Lock()
	defer ns.handlersMu.Unlock()

	handlerPtr := reflect.ValueOf(handler).Pointer()
	for i, h := range ns.handlers {
		if reflect.ValueOf(h).Pointer() == handlerPtr {
			ns.handlers = append(ns.handlers[:i], ns.handlers[i+1:]...)
			break
		}
	}
}

// Notify sends a notification
func (ns *NotificationSystem) Notify(notification RealTimeNotification) {
	notification.Timestamp = time.Now()
	notification.Read = false

	select {
	case ns.notificationChan <- notification:
	default:
		slog.Warn("notification-system channel full, dropping notification")
	}
}

// processNotifications processes incoming notifications
func (ns *NotificationSystem) processNotifications() {
	for {
		select {
		case notification := <-ns.notificationChan:
			ns.handleNotification(notification)

		case <-ns.ctx.Done():
			return
		}
	}
}

// handleNotification handles a single notification
func (ns *NotificationSystem) handleNotification(notification RealTimeNotification) {
	// Store notification
	ns.notificationsMu.Lock()
	ns.notifications = append(ns.notifications, notification)
	ns.notificationsMu.Unlock()

	// Dispatch to handlers
	ns.handlersMu.RLock()
	handlers := make([]NotificationHandler, len(ns.handlers))
	copy(handlers, ns.handlers)
	ns.handlersMu.RUnlock()

	for _, handler := range handlers {
		go func(h NotificationHandler, n RealTimeNotification) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("notification-system handler panic", "panic", r)
				}
			}()
			h(n)
		}(handler, notification)
	}
}

// GetNotifications returns all notifications
func (ns *NotificationSystem) GetNotifications() []RealTimeNotification {
	ns.notificationsMu.RLock()
	defer ns.notificationsMu.RUnlock()

	notifications := make([]RealTimeNotification, len(ns.notifications))
	copy(notifications, ns.notifications)

	return notifications
}

// GetUnreadNotifications returns unread notifications
func (ns *NotificationSystem) GetUnreadNotifications() []RealTimeNotification {
	ns.notificationsMu.RLock()
	defer ns.notificationsMu.RUnlock()

	unread := make([]RealTimeNotification, 0)
	for _, notification := range ns.notifications {
		if !notification.Read {
			unread = append(unread, notification)
		}
	}

	return unread
}

// MarkAsRead marks a notification as read
func (ns *NotificationSystem) MarkAsRead(notificationID string) {
	ns.notificationsMu.Lock()
	defer ns.notificationsMu.Unlock()

	for i, notification := range ns.notifications {
		if notification.ID == notificationID {
			ns.notifications[i].Read = true
			break
		}
	}
}

// Clear removes all notifications
func (ns *NotificationSystem) Clear() {
	ns.notificationsMu.Lock()
	defer ns.notificationsMu.Unlock()
	ns.notifications = make([]RealTimeNotification, 0)
}

// Shutdown shuts down the notification system
func (ns *NotificationSystem) Shutdown() {
	ns.cancel()
	close(ns.notificationChan)
}

// generateNotificationID generates a unique notification ID
func generateNotificationID() string {
	return fmt.Sprintf("notif-%d", time.Now().UnixNano())
}
