package agent

import (
	"context"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Cached Agent Runner Integration
// ---------------------------------------------------------------------------

// CachedAgentConfig extends RunConfig with caching options
type CachedAgentConfig struct {
	*RunConfig

	// Cache is the tool results cache to use
	Cache *ToolResultCache

	// EnableCache enables/disables caching for this agent
	EnableCache bool

	// CacheInvalidationRules defines rules for when to invalidate cache
	CacheInvalidationRules *CacheInvalidationRules
}

// CacheInvalidationRules defines when to invalidate cache entries
type CacheInvalidationRules struct {
	// InvalidateOnFileChange invalidates cache when files are modified
	InvalidateOnFileChange bool

	// InvalidateOnNewFile invalidates cache when new files are created
	InvalidateOnNewFile bool

	// InvalidateOnToolCall invalidates cache when specific tools are called
	InvalidateOnToolCall []string

	// InvalidateInterval invalidates cache at regular intervals
	InvalidateInterval int // minutes, 0 = no interval invalidation
}

// DefaultCacheInvalidationRules returns default cache invalidation rules
func DefaultCacheInvalidationRules() *CacheInvalidationRules {
	return &CacheInvalidationRules{
		InvalidateOnFileChange: true,
		InvalidateOnNewFile:    true,
		InvalidateOnToolCall:   []string{"write_file", "edit", "bash"},
		InvalidateInterval:     0,
	}
}

// CachedAgentRunner wraps an agent runner with caching capabilities
type CachedAgentRunner struct {
	*Runner
	cache  *ToolResultCache
	config *CachedAgentConfig
	stats  CachedAgentStats
}

// CachedAgentStats provides statistics for cached agent execution
type CachedAgentStats struct {
	// TotalToolCalls is the total number of tool calls made
	TotalToolCalls int64

	// CachedToolCalls is the number of tool calls that were served from cache
	CachedToolCalls int64

	// CacheHitRate is the percentage of tool calls served from cache
	CacheHitRate float64

	// CacheSavings is the estimated time saved (in milliseconds) from cache hits
	CacheSavings int64
}

// NewCachedAgentRunner creates a new cached agent runner
func NewCachedAgentRunner(engine *engine.Engine, cache *ToolResultCache) *CachedAgentRunner {
	return &CachedAgentRunner{
		Runner: NewRunner(engine),
		cache:  cache,
		config: &CachedAgentConfig{
			RunConfig:              &RunConfig{},
			Cache:                  cache,
			EnableCache:            true,
			CacheInvalidationRules: DefaultCacheInvalidationRules(),
		},
	}
}

// SetCachedConfig sets the cached agent configuration
func (r *CachedAgentRunner) SetCachedConfig(config *CachedAgentConfig) {
	r.config = config
	r.config.Cache = r.cache
}

// SetCacheInvalidationRules sets custom cache invalidation rules
func (r *CachedAgentRunner) SetCacheInvalidationRules(rules *CacheInvalidationRules) {
	r.config.CacheInvalidationRules = rules
}

// EnableCache enables or disables caching
func (r *CachedAgentRunner) EnableCache(enabled bool) {
	r.config.EnableCache = enabled
}

// RunCachedAgent runs an agent with caching enabled
func (r *CachedAgentRunner) RunCachedAgent(ctx context.Context, config *CachedAgentConfig) (*RunResult, *CachedAgentStats, error) {
	// Ensure config has cache reference
	if config.Cache == nil {
		config.Cache = r.cache
	}
	if config.EnableCache {
		config.Cache = r.cache
	}

	// Set cache invalidation rules
	if config.CacheInvalidationRules == nil {
		config.CacheInvalidationRules = DefaultCacheInvalidationRules()
	}

	// Add caching interceptor to engine
	// Note: In a real implementation, we'd need to wrap the engine's submit method
	// For now, we'll just use the cache for the final result

	// Run the agent with caching
	result, err := r.RunAgentAdvanced(ctx, config.RunConfig)
	if err != nil {
		return nil, nil, err
	}

	// Update cache statistics
	r.stats.TotalToolCalls = int64(result.ToolUses)
	if r.cache != nil {
		cacheStats := r.cache.GetStats()
		r.stats.CachedToolCalls = cacheStats.Hits
		r.stats.CacheHitRate = cacheStats.HitRate
	}

	return result, &r.stats, nil
}

// GetCachedStats returns the current cached agent statistics
func (r *CachedAgentRunner) GetCachedStats() CachedAgentStats {
	return r.stats
}

// GetCacheStats returns the underlying cache statistics
func (r *CachedAgentRunner) GetCacheStats() ToolResultCacheStats {
	if r.cache == nil {
		return ToolResultCacheStats{}
	}
	return r.cache.GetStats()
}

// InvalidateCache invalidates cache based on rules
func (r *CachedAgentRunner) InvalidateCache() {
	if r.cache == nil || r.config.CacheInvalidationRules == nil {
		return
	}

	// Apply invalidation rules — interval-based invalidation not yet implemented.

	// Tool-based invalidation
	if len(r.config.CacheInvalidationRules.InvalidateOnToolCall) > 0 {
		for _, toolName := range r.config.CacheInvalidationRules.InvalidateOnToolCall {
			r.cache.InvalidateByTool(toolName)
		}
	}
}

// HandleToolResult processes a tool result with caching
func (r *CachedAgentRunner) HandleToolResult(ctx context.Context, toolName string, params map[string]any, result *types.ToolResult) {
	if r.cache == nil || !r.config.EnableCache {
		return
	}

	// Check if we should cache this result
	if r.cache.ShouldCache(toolName, result) {
		err := r.cache.Set(toolName, params, result)
		if err != nil {
			fmt.Printf("[cached-runner] Failed to cache result for tool %s: %v\n", toolName, err)
		}
	}

	// Handle cache invalidation based on rules
	r.handleCacheInvalidation(toolName, params, result)
}

// handleCacheInvalidation applies cache invalidation rules
func (r *CachedAgentRunner) handleCacheInvalidation(toolName string, params map[string]any, result *types.ToolResult) {
	rules := r.config.CacheInvalidationRules
	if rules == nil {
		return
	}

	// Invalidate on specific tool calls
	for _, invalidationTool := range rules.InvalidateOnToolCall {
		if toolName == invalidationTool {
			r.cache.InvalidateByTool(invalidationTool)
			break
		}
	}

	// Invalidate on file changes/modifications
	if rules.InvalidateOnFileChange || rules.InvalidateOnNewFile {
		r.handleFileBasedInvalidation(toolName, result)
	}
}

// handleFileBasedInvalidation invalidates cache entries based on file operations
func (r *CachedAgentRunner) handleFileBasedInvalidation(toolName string, result *types.ToolResult) {
	fileModifyTools := []string{"write_file", "edit_file"}
	fileReadTools := []string{"read_file", "glob", "grep"}

	rules := r.config.CacheInvalidationRules
	if rules == nil {
		return
	}

	// Check if this is a file modification tool
	for _, modifyTool := range fileModifyTools {
		if toolName == modifyTool {
			if rules.InvalidateOnFileChange {
				// Invalidate cache entries that depend on this file
				r.cache.InvalidateByTool("read_file")
				r.cache.InvalidateByTool("glob")
				r.cache.InvalidateByTool("grep")
			}
			return
		}
	}

	// Check if this is a file read tool that should trigger invalidation
	for _, readTool := range fileReadTools {
		if toolName == readTool {
			// InvalidateOnNewFile: file-state tracking not yet implemented.
			return
		}
	}
}

// ToolExecutorFn is the function signature for actual tool execution on a cache miss.
type ToolExecutorFn func(ctx context.Context, toolName string, params map[string]any) (*types.ToolResult, error)

// CacheInterceptor wraps tool execution with a read-through cache.
// On a cache hit, the stored result is returned immediately.
// On a cache miss, executor is called, the result is cached (if eligible), and returned.
type CacheInterceptor struct {
	cache    *ToolResultCache
	executor ToolExecutorFn
}

// NewCacheInterceptor creates a new cache interceptor.
// executor is the function that actually runs the tool on a cache miss.
// Passing nil for executor is valid — cache hits still work; misses return an error.
func NewCacheInterceptor(cache *ToolResultCache, executor ToolExecutorFn) *CacheInterceptor {
	return &CacheInterceptor{
		cache:    cache,
		executor: executor,
	}
}

// ExecuteTool returns a cached result when available; otherwise calls the executor,
// stores the result if eligible, and returns it.
func (i *CacheInterceptor) ExecuteTool(ctx context.Context, toolName string, params map[string]any) (*types.ToolResult, error) {
	if cachedResult, found := i.cache.Get(toolName, params); found {
		return cachedResult, nil
	}

	if i.executor == nil {
		return nil, fmt.Errorf("cache miss for tool %q and no executor configured", toolName)
	}

	result, err := i.executor(ctx, toolName, params)
	if err != nil {
		return nil, err
	}

	// Cache on success.
	if i.cache.ShouldCache(toolName, result) {
		_ = i.cache.Set(toolName, params, result) // best-effort
	}

	return result, nil
}

// PreExecuteTool is called before a tool is executed
func (i *CacheInterceptor) PreExecuteTool(ctx context.Context, toolName string, params map[string]any) {
	// Check cache before execution
	if _, found := i.cache.Get(toolName, params); found {
		// We have a cached result, return it
		// In a real implementation, we'd have a way to signal this to the caller
		fmt.Printf("[cache-interceptor] Cache hit for tool %s\n", toolName)
	}
}

// PostExecuteTool is called after a tool is executed
func (i *CacheInterceptor) PostExecuteTool(ctx context.Context, toolName string, params map[string]any, result *types.ToolResult) {
	// Cache the result if appropriate
	if i.cache.ShouldCache(toolName, result) {
		err := i.cache.Set(toolName, params, result)
		if err != nil {
			fmt.Printf("[cache-interceptor] Failed to cache result: %v\n", err)
		}
	}
}
