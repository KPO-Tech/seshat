package agent

import internalagent "github.com/KPO-Tech/seshat/internal/agent"

type (
	AgentDefinition   = internalagent.AgentDefinition
	AgentSource       = internalagent.AgentSource
	AgentRegistry     = internalagent.AgentRegistry
	AsyncAgent        = internalagent.AsyncAgent
	AsyncAgentManager = internalagent.AsyncAgentManager
)

func NewAgentRegistry() *AgentRegistry {
	return internalagent.NewAgentRegistry()
}

// DefaultAsyncManager returns the process-wide manager backing the
// spawn_agent/wait_agent/send_agent_message/close_agent tool family. Host
// applications (e.g. seshat-backend) use it to expose management operations —
// like cancelling a runaway or unwanted background agent — through their own
// API, without duplicating the engine's agent-lifecycle bookkeeping.
func DefaultAsyncManager() *AsyncAgentManager {
	return internalagent.GetDefaultAsyncManager()
}
