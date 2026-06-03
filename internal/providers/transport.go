package providers

import providertransport "github.com/EngineerProjects/nexus-engine/internal/providers/transport"

type Transport = providertransport.Transport
type BedrockTransport = providertransport.BedrockTransport
type FoundryTransport = providertransport.FoundryTransport
type VertexTransport = providertransport.VertexTransport

func NewTransport(apiKey string, config *Config) (Transport, error) {
	return providertransport.NewTransport(apiKey, providerTransportConfig(config))
}

func NewBedrockTransport(apiKey string, config *Config) (*BedrockTransport, error) {
	return providertransport.NewBedrockTransport(apiKey, providerTransportConfig(config))
}

func NewFoundryTransport(apiKey string, config *Config) (*FoundryTransport, error) {
	return providertransport.NewFoundryTransport(apiKey, providerTransportConfig(config))
}

func NewVertexTransport(apiKey string, config *Config) (*VertexTransport, error) {
	return providertransport.NewVertexTransport(apiKey, providerTransportConfig(config))
}

func providerTransportConfig(config *Config) *providertransport.Config {
	if config == nil {
		return &providertransport.Config{}
	}
	return &providertransport.Config{
		Provider:  config.Provider,
		BaseURL:   config.BaseURL,
		Region:    config.Region,
		ProjectID: config.ProjectID,
	}
}
