package search

import (
	internalsearch "github.com/EngineerProjects/nexus-engine/internal/web/search"
	internalproviders "github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
)

type (
	Config  = internalsearch.Config
	Input   = internalsearch.Input
	Output  = internalsearch.Output
	Service = internalsearch.Service
)

func NewService() *Service {
	return internalsearch.NewService()
}

func NewServiceWithConfig(config *Config) *Service {
	return internalsearch.NewServiceWithConfig(config)
}

func FinalizeOutput(input Input, output internalproviders.ProviderOutput) Output {
	return internalsearch.FinalizeOutput(input, output)
}

func IsEnabled() bool {
	return internalsearch.IsEnabled()
}

func GetProviderMode() string {
	return internalsearch.GetProviderMode()
}

func GetConfiguredProviders() []string {
	return internalsearch.GetConfiguredProviders()
}
