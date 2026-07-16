package agent

import internalagent "github.com/KPO-Tech/seshat/internal/agent"

type (
	AgentDefinition = internalagent.AgentDefinition
	AgentRegistry   = internalagent.AgentRegistry
)

func NewAgentRegistry() *AgentRegistry {
	return internalagent.NewAgentRegistry()
}
