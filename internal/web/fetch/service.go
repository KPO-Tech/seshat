package fetch

import webcore "github.com/EngineerProjects/nexus-engine/internal/web"

var _ webcore.FetchService = (*Service)(nil)

// Service implements the reusable fetch core used by tool wrappers and future runtime integrations.
type Service struct {
	config         *Config
	httpClient     HTTPClient
	browserManager webcore.BrowserManager
	artifactStore  webcore.ArtifactStore
	cache          *Cache
	decisionCache  *DecisionCache
	resolver       webcore.HostResolver
	renderPool     *renderSessionPool
}

// NewService creates a new shared fetch service with local defaults for HTTP and caching.
func NewService(config *Config) *Service {
	if config == nil {
		config = DefaultConfig()
	}

	service := &Service{
		config:         config,
		httpClient:     config.HTTPClient,
		browserManager: config.BrowserManager,
		artifactStore:  config.ArtifactStore,
		cache:          config.Cache,
		decisionCache:  config.DecisionCache,
		resolver:       config.Resolver,
	}
	if service.httpClient == nil {
		service.httpClient = DefaultHTTPClient(service.resolver)
	}
	if service.cache == nil {
		service.cache = DefaultCache()
	}
	if service.decisionCache == nil {
		service.decisionCache = DefaultDecisionCache()
	}
	if service.resolver == nil {
		service.resolver = webcore.DefaultResolver()
	}
	if config.RenderPoolEnabled && service.browserManager != nil {
		service.renderPool = newRenderSessionPool(config.RenderPoolTTL, config.RenderPoolMaxSessions, service.browserManager)
	}
	return service
}
