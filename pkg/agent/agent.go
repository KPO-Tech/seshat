package agent

import internalagent "github.com/EngineerProjects/nexus-engine/internal/agent"

type (
	AgentDefinition = internalagent.AgentDefinition
	AgentRegistry   = internalagent.AgentRegistry
)

func NewAgentRegistry() *AgentRegistry {
	return internalagent.NewAgentRegistry()
}
