package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
	"github.com/spf13/viper"
)

// Config contains host-level runtime settings shared by CLI and server entrypoints.
type Config struct {
	RuntimeRoot       string  `mapstructure:"runtime_root" yaml:"runtime_root,omitempty"`
	Cwd               string  `mapstructure:"cwd" yaml:"cwd,omitempty"`
	Model             string  `mapstructure:"model" yaml:"model,omitempty"`
	MaxTokens         int     `mapstructure:"max_tokens" yaml:"max_tokens,omitempty"`
	Temperature       float64 `mapstructure:"temperature" yaml:"temperature,omitempty"`
	MCPEnabled        bool    `mapstructure:"mcp_enabled" yaml:"mcp_enabled,omitempty"`
	SkillsEnabled     bool    `mapstructure:"skills_enabled" yaml:"skills_enabled,omitempty"`
	Debug             bool    `mapstructure:"debug" yaml:"debug,omitempty"`
	APIKey            string  `mapstructure:"api_key" yaml:"api_key,omitempty"`
	DBPath            string  `mapstructure:"db_path" yaml:"db_path,omitempty"`
	SessionDBPath     string  `mapstructure:"session_db_path" yaml:"session_db_path,omitempty"`
	ProviderBaseURL   string  `mapstructure:"provider_base_url" yaml:"provider_base_url,omitempty"`
	ProviderRegion    string  `mapstructure:"provider_region" yaml:"provider_region,omitempty"`
	ProviderProjectID string  `mapstructure:"provider_project_id" yaml:"provider_project_id,omitempty"`
	ProviderResource  string  `mapstructure:"provider_resource" yaml:"provider_resource,omitempty"`
	AdminEmail        string  `mapstructure:"admin_email" yaml:"admin_email,omitempty"`
	AdminPassword     string  `mapstructure:"admin_password" yaml:"admin_password,omitempty"`
	AdminPasswordHash string  `mapstructure:"admin_password_hash" yaml:"admin_password_hash,omitempty"`

	// Database driver configuration (multi-driver support)
	DBDriver      string `mapstructure:"db_driver" yaml:"db_driver,omitempty"`
	DBDSN         string `mapstructure:"db_dsn" yaml:"db_dsn,omitempty"`
	DBAutoMigrate bool   `mapstructure:"db_auto_migrate" yaml:"db_auto_migrate,omitempty"`

	// Embedder configuration for the RAG / knowledge domain
	EmbedderBaseURL  string `mapstructure:"embedder_base_url" yaml:"embedder_base_url,omitempty"`
	EmbedderAPIKey   string `mapstructure:"embedder_api_key" yaml:"embedder_api_key,omitempty"`
	EmbedderModel    string `mapstructure:"embedder_model" yaml:"embedder_model,omitempty"`
	EmbedderProvider string `mapstructure:"embedder_provider" yaml:"embedder_provider,omitempty"`

	// Vector store backend (sqlite|pgvector|qdrant|chroma|memory). Defaults to sqlite.
	VectorBackend           string `mapstructure:"vector_backend" yaml:"vector_backend,omitempty"`
	QdrantHost              string `mapstructure:"qdrant_host" yaml:"qdrant_host,omitempty"`
	QdrantPort              int    `mapstructure:"qdrant_port" yaml:"qdrant_port,omitempty"`
	QdrantAPIKey            string `mapstructure:"qdrant_api_key" yaml:"qdrant_api_key,omitempty"`
	QdrantPrefix            string `mapstructure:"qdrant_prefix" yaml:"qdrant_prefix,omitempty"`
	PgVectorCreateExtension bool   `mapstructure:"pgvector_create_extension" yaml:"pgvector_create_extension,omitempty"`
	PgVectorDSN             string `mapstructure:"pgvector_dsn" yaml:"pgvector_dsn,omitempty"`
	PgVectorIndexMethod     string `mapstructure:"pgvector_index_method" yaml:"pgvector_index_method,omitempty"`
	PgVectorHNSWM           int    `mapstructure:"pgvector_hnsw_m" yaml:"pgvector_hnsw_m,omitempty"`
	PgVectorHNSWEF          int    `mapstructure:"pgvector_hnsw_ef_construction" yaml:"pgvector_hnsw_ef_construction,omitempty"`
	PgVectorIVFFlatLists    int    `mapstructure:"pgvector_ivfflat_lists" yaml:"pgvector_ivfflat_lists,omitempty"`
	ChromaURL               string `mapstructure:"chroma_url" yaml:"chroma_url,omitempty"`
	ChromaAPIKey            string `mapstructure:"chroma_api_key" yaml:"chroma_api_key,omitempty"`
	ChromaTenant            string `mapstructure:"chroma_tenant" yaml:"chroma_tenant,omitempty"`
	ChromaDatabase          string `mapstructure:"chroma_database" yaml:"chroma_database,omitempty"`

	// Storage configuration
	StorageProvider         string `mapstructure:"storage_provider" yaml:"storage_provider,omitempty"`
	StorageLocalPath        string `mapstructure:"storage_local_path" yaml:"storage_local_path,omitempty"`
	S3Endpoint              string `mapstructure:"s3_endpoint" yaml:"s3_endpoint,omitempty"`
	S3Bucket                string `mapstructure:"s3_bucket" yaml:"s3_bucket,omitempty"`
	S3AccessKeyID           string `mapstructure:"s3_access_key_id" yaml:"s3_access_key_id,omitempty"`
	S3SecretAccessKey       string `mapstructure:"s3_secret_access_key" yaml:"s3_secret_access_key,omitempty"`
	S3Region                string `mapstructure:"s3_region" yaml:"s3_region,omitempty"`
	S3KeyPrefix             string `mapstructure:"s3_key_prefix" yaml:"s3_key_prefix,omitempty"`
	StorageGCEnabled        bool   `mapstructure:"storage_gc_enabled" yaml:"storage_gc_enabled,omitempty"`
	StorageGCInterval       string `mapstructure:"storage_gc_interval" yaml:"storage_gc_interval,omitempty"`
	StorageGCLimit          int    `mapstructure:"storage_gc_limit" yaml:"storage_gc_limit,omitempty"`
	StorageGCNamespaces     string `mapstructure:"storage_gc_namespaces" yaml:"storage_gc_namespaces,omitempty"`
	BrowserRemoteControlURL string `mapstructure:"browser_remote_control_url" yaml:"browser_remote_control_url,omitempty"`
	BrowserExecutablePath   string `mapstructure:"browser_executable_path" yaml:"browser_executable_path,omitempty"`
	DoclingURL              string `mapstructure:"docling_url" yaml:"docling_url,omitempty"`

	// Search tool integration keys — loaded from the credentials DB at runtime,
	// never stored in the YAML config file.
	TavilyAPIKey      string `mapstructure:"-" yaml:"-"`
	ExaAPIKey         string `mapstructure:"-" yaml:"-"`
	JinaAPIKey        string `mapstructure:"-" yaml:"-"`
	WebSearchProvider string `mapstructure:"web_search_provider" yaml:"web_search_provider,omitempty"`

	// API rate limiting — max requests per minute per authenticated user (0 = disabled)
	RateLimitPerMinute int `mapstructure:"rate_limit_per_minute" yaml:"rate_limit_per_minute,omitempty"`

	// User management flags
	EnableSignup    bool   `mapstructure:"enable_signup" yaml:"enable_signup,omitempty"`
	DefaultUserRole string `mapstructure:"default_user_role" yaml:"default_user_role,omitempty"`
	EnableAPIKeys   bool   `mapstructure:"enable_api_keys" yaml:"enable_api_keys,omitempty"`

	// Skill repository collections — comma-separated git URLs cloned at startup.
	// Example: NEXUS_SKILL_REPOS=https://github.com/romainsimon/paperasse
	SkillRepos string `mapstructure:"skill_repos" yaml:"skill_repos,omitempty"`

	// FeaturedSkillRepos — comma-separated git URLs shown as installable catalog entries in the UI.
	// Example: NEXUS_FEATURED_SKILL_REPOS=https://github.com/romainsimon/paperasse
	FeaturedSkillRepos string `mapstructure:"featured_skill_repos" yaml:"featured_skill_repos,omitempty"`

	// TrustedProxies — comma-separated CIDR ranges (or bare IPs) of reverse proxies whose
	// X-Forwarded-For / X-Real-Ip headers should be trusted for IP resolution.
	// Leave empty (the default) for a desktop deployment without a reverse proxy.
	// Example: NEXUS_TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12
	TrustedProxies string `mapstructure:"trusted_proxies" yaml:"trusted_proxies,omitempty"`

	// SkillRepoHosts — comma-separated list of git hosting domains allowed when installing
	// a skill repo via the API. Defaults to github.com, gitlab.com, bitbucket.org, codeberg.org.
	// Example: NEXUS_SKILL_REPO_HOSTS=github.com,mygitlab.company.com
	SkillRepoHosts string `mapstructure:"skill_repo_hosts" yaml:"skill_repo_hosts,omitempty"`

	// DefaultSkillRepo — git URL of the official Nexus skills collection cloned
	// silently in the background on first boot. Defaults to the canonical nexus-skills
	// repo. Set to "none" to disable automatic install.
	// Example: NEXUS_DEFAULT_SKILL_REPO=https://github.com/EngineerProjects/nexus-skills
	DefaultSkillRepo string `mapstructure:"default_skill_repo" yaml:"default_skill_repo,omitempty"`
}

// DefaultConfig returns the shared defaults used by host applications.
func DefaultConfig() Config {
	return Config{
		RuntimeRoot:             "",
		Cwd:                     ".",
		Model:                   "glm-4.5",
		MaxTokens:               4096,
		Temperature:             0.7,
		MCPEnabled:              true,
		SkillsEnabled:           true,
		Debug:                   false,
		APIKey:                  "",
		DBPath:                  "",
		SessionDBPath:           "",
		ProviderBaseURL:         "",
		ProviderRegion:          "",
		ProviderProjectID:       "",
		ProviderResource:        "",
		AdminEmail:              "",
		AdminPassword:           "",
		AdminPasswordHash:       "",
		DBAutoMigrate:           true,
		PgVectorCreateExtension: true,
		PgVectorIndexMethod:     "hnsw",
		PgVectorHNSWM:           16,
		PgVectorHNSWEF:          64,
		PgVectorIVFFlatLists:    100,
		StorageGCEnabled:        true,
		StorageGCInterval:       "1h",
		StorageGCLimit:          512,
		EnableSignup:            true,
		DefaultUserRole:         "member",
		EnableAPIKeys:           true,
		StorageGCNamespaces: strings.Join([]string{
			"artifacts/web",
			"artifacts/browser/screenshots",
			"artifacts/browser/downloads",
		}, ","),
	}
}

// Load returns a fully initialized shared runtime config.
func Load() (Config, error) {
	config := DefaultConfig()
	return config, LoadInto(&config)
}

// LoadInto refreshes a caller-owned config from env files, env vars, and config files.
func LoadInto(config *Config) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	*config = DefaultConfig()
	loadEnvFile(".env")    //nolint:errcheck // best-effort .env loading
	loadEnvFile("../.env") //nolint:errcheck // best-effort .env loading

	v := viper.New()
	v.SetConfigName(".nexus")
	v.SetConfigType("yaml")
	v.AddConfigPath("$HOME")
	v.AddConfigPath(".")
	v.SetEnvPrefix("NEXUS")

	v.BindEnv("runtime_root", runtimepath.EnvRuntimeRoot)
	v.BindEnv("cwd", "NEXUS_CWD")
	v.BindEnv("model", "NEXUS_MODEL")
	v.BindEnv("debug", "NEXUS_DEBUG")
	v.BindEnv("api_key", "NEXUS_API_KEY")
	v.BindEnv("db_path", "NEXUS_DB_PATH")
	v.BindEnv("session_db_path", "NEXUS_SESSION_DB_PATH")
	v.BindEnv("provider_base_url", "NEXUS_PROVIDER_BASE_URL")
	v.BindEnv("provider_region", "NEXUS_PROVIDER_REGION")
	v.BindEnv("provider_project_id", "NEXUS_PROVIDER_PROJECT_ID")
	v.BindEnv("provider_resource", "NEXUS_PROVIDER_RESOURCE")
	v.BindEnv("admin_email", "NEXUS_ADMIN_EMAIL")
	v.BindEnv("admin_password", "NEXUS_ADMIN_PASSWORD")
	v.BindEnv("admin_password_hash", "NEXUS_ADMIN_PASSWORD_HASH")
	v.BindEnv("web_search_provider", "WEB_SEARCH_PROVIDER")

	v.BindEnv("storage_provider", "NEXUS_STORAGE_PROVIDER")
	v.BindEnv("storage_local_path", "NEXUS_STORAGE_LOCAL_PATH")
	v.BindEnv("s3_endpoint", "NEXUS_S3_ENDPOINT")
	v.BindEnv("s3_bucket", "NEXUS_S3_BUCKET")
	v.BindEnv("s3_access_key_id", "NEXUS_S3_ACCESS_KEY_ID")
	v.BindEnv("s3_secret_access_key", "NEXUS_S3_SECRET_ACCESS_KEY")
	v.BindEnv("s3_region", "NEXUS_S3_REGION")
	v.BindEnv("s3_key_prefix", "NEXUS_S3_KEY_PREFIX")
	v.BindEnv("storage_gc_enabled", "NEXUS_STORAGE_GC_ENABLED")
	v.BindEnv("storage_gc_interval", "NEXUS_STORAGE_GC_INTERVAL")
	v.BindEnv("storage_gc_limit", "NEXUS_STORAGE_GC_LIMIT")
	v.BindEnv("storage_gc_namespaces", "NEXUS_STORAGE_GC_NAMESPACES")
	v.BindEnv("browser_remote_control_url", "NEXUS_BROWSER_REMOTE_CONTROL_URL")
	v.BindEnv("browser_executable_path", "NEXUS_BROWSER_EXECUTABLE_PATH")
	v.BindEnv("db_driver", "NEXUS_DB_DRIVER")
	v.BindEnv("db_dsn", "NEXUS_DB_DSN")
	v.BindEnv("db_auto_migrate", "NEXUS_DB_AUTO_MIGRATE")
	v.SetDefault("db_driver", "sqlite")
	v.SetDefault("db_auto_migrate", true)

	v.BindEnv("embedder_base_url", "RAG_EMBEDDING_URL")
	v.BindEnv("embedder_api_key", "RAG_EMBEDDING_API_KEY")
	v.BindEnv("embedder_model", "RAG_EMBEDDING_MODEL")
	v.BindEnv("embedder_provider", "RAG_EMBEDDING_PROVIDER")

	v.BindEnv("vector_backend", "NEXUS_VECTOR_BACKEND")
	v.BindEnv("qdrant_host", "QDRANT_HOST")
	v.BindEnv("qdrant_port", "QDRANT_PORT")
	v.BindEnv("qdrant_api_key", "QDRANT_API_KEY")
	v.BindEnv("qdrant_prefix", "QDRANT_PREFIX")
	v.BindEnv("pgvector_create_extension", "NEXUS_PGVECTOR_CREATE_EXTENSION")
	v.BindEnv("pgvector_dsn", "NEXUS_PGVECTOR_DSN")
	v.BindEnv("pgvector_index_method", "NEXUS_PGVECTOR_INDEX_METHOD")
	v.BindEnv("pgvector_hnsw_m", "NEXUS_PGVECTOR_HNSW_M")
	v.BindEnv("pgvector_hnsw_ef_construction", "NEXUS_PGVECTOR_HNSW_EF_CONSTRUCTION")
	v.BindEnv("pgvector_ivfflat_lists", "NEXUS_PGVECTOR_IVFFLAT_LISTS")
	v.BindEnv("chroma_url", "CHROMA_URL")
	v.BindEnv("chroma_api_key", "CHROMA_API_KEY")
	v.BindEnv("chroma_tenant", "CHROMA_TENANT")
	v.BindEnv("chroma_database", "CHROMA_DATABASE")

	v.BindEnv("enable_signup", "NEXUS_ENABLE_SIGNUP")
	v.BindEnv("default_user_role", "NEXUS_DEFAULT_USER_ROLE")
	v.BindEnv("enable_api_keys", "NEXUS_ENABLE_API_KEYS")
	v.BindEnv("skill_repos", "NEXUS_SKILL_REPOS")
	v.BindEnv("featured_skill_repos", "NEXUS_FEATURED_SKILL_REPOS")
	v.BindEnv("trusted_proxies", "NEXUS_TRUSTED_PROXIES")
	v.BindEnv("skill_repo_hosts", "NEXUS_SKILL_REPO_HOSTS")
	v.SetDefault("enable_signup", true)
	v.SetDefault("default_user_role", "member")
	v.SetDefault("enable_api_keys", true)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config: %w", err)
		}
	}

	if err := v.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	config.RuntimeRoot = runtimepath.ResolveRoot(config.RuntimeRoot)
	ApplyRuntimeEnv(*config)
	return nil
}

// ApplySearchKeys sets search-provider env vars from config fields so that
// the search tool sub-packages pick them up via os.Getenv.
func ApplySearchKeys(config Config) {
	setEnvIfPresent := func(name, value string) {
		if value != "" && os.Getenv(name) == "" {
			os.Setenv(name, value)
		}
	}
	setEnvIfPresent("TAVILY_API_KEY", config.TavilyAPIKey)
	setEnvIfPresent("EXA_API_KEY", config.ExaAPIKey)
	setEnvIfPresent("JINA_API_KEY", config.JinaAPIKey)
	setEnvIfPresent("WEB_SEARCH_PROVIDER", config.WebSearchProvider)
}

// EffectiveDBPath returns the legacy local SQLite path used by CLI/runtime code.
// For backend hosts, prefer EffectiveBackendSQLitePath and EffectiveSessionDBPath.
func EffectiveDBPath(config Config) string {
	if config.DBPath != "" {
		return config.DBPath
	}
	return runtimepath.BackendDBPath(config.RuntimeRoot)
}

// EffectiveBackendSQLitePath returns the SQLite file used when the backend DB
// driver itself is SQLite.
func EffectiveBackendSQLitePath(config Config) string {
	return EffectiveDBPath(config)
}

// EffectiveSessionDBPath returns the SQLite file used for runtime session/core
// persistence. It is independent from the backend DB driver so a Postgres
// backend can still keep the runtime session store in SQLite.
func EffectiveSessionDBPath(config Config) string {
	if config.SessionDBPath != "" {
		return config.SessionDBPath
	}
	return EffectiveDBPath(config)
}

func EffectiveRuntimeRoot(config Config) string {
	return runtimepath.ResolveRoot(config.RuntimeRoot)
}

func EffectiveStorageLocalPath(config Config) string {
	if trimmed := strings.TrimSpace(config.StorageLocalPath); trimmed != "" {
		return filepath.Clean(runtimepath.ExpandTilde(trimmed))
	}
	return runtimepath.StorageDir(config.RuntimeRoot)
}

// EffectiveAPIKeyAndProvider resolves the configured provider credentials in priority order.
func EffectiveAPIKeyAndProvider(config Config) (string, sdk.APIProvider) {
	if preferred := preferredProvider(config); preferred != "" {
		return ResolveAPIKey(config, preferred), preferred
	}

	for _, provider := range AvailableProviders() {
		if key := resolveAPIKeyFromEnv(provider.Name); key != "" {
			return key, provider.Name
		}
	}

	if config.APIKey != "" {
		return config.APIKey, inferProviderFromAPIKey(config.APIKey)
	}

	return "", DetectProviderFromModel(config.Model)
}

// DetectProviderFromModel infers the provider from the requested model name.
func DetectProviderFromModel(model string) sdk.APIProvider {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return ""
	}

	if provider := ProviderForModel(model); provider != "" {
		return provider
	}

	switch {
	case strings.HasPrefix(model, "glm"):
		return sdk.APIProviderZAi
	case strings.HasPrefix(model, "@cf/"):
		return sdk.APIProviderWorkersAI
	case strings.HasPrefix(model, "anthropic/"),
		strings.HasPrefix(model, "openai/"),
		strings.HasPrefix(model, "deepseek/"),
		strings.HasPrefix(model, "qwen/"),
		strings.HasPrefix(model, "meta-llama/"):
		return sdk.APIProviderOpenRouter
	case strings.Contains(model, "claude"):
		return sdk.APIProviderAnthropic
	case strings.Contains(model, "gpt") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"):
		return sdk.APIProviderOpenAI
	case strings.Contains(model, "gemini"):
		return sdk.APIProviderGemini
	case strings.HasPrefix(model, "minimax-"), strings.HasPrefix(model, "m2"):
		return sdk.APIProviderMiniMax
	case strings.Contains(model, ":") && (strings.Contains(model, "qwen") ||
		strings.Contains(model, "deepseek") ||
		strings.Contains(model, "llama") ||
		strings.Contains(model, "mistral") ||
		strings.Contains(model, "codellama") ||
		strings.Contains(model, "phi")):
		return sdk.APIProviderOllama
	default:
		return ""
	}
}

// ParseModelIdentifier normalizes optional provider prefixes like "openai:gpt-4o".
func ParseModelIdentifier(raw string) sdk.ModelIdentifier {
	value := strings.TrimSpace(raw)
	if value == "" {
		return sdk.DefaultClientConfig().Model
	}

	provider := sdk.APIProviderAnthropic
	model := value
	if prefix, rest, ok := strings.Cut(value, ":"); ok && strings.TrimSpace(rest) != "" {
		if resolved, ok := parseProvider(prefix); ok {
			provider = resolved
			model = strings.TrimSpace(rest)
		}
	}

	return sdk.ModelIdentifier{
		Provider: provider,
		Model:    model,
	}
}

// HasExplicitProviderPrefix reports whether a model string explicitly carries a recognized provider prefix.
func HasExplicitProviderPrefix(raw string) bool {
	value := strings.TrimSpace(raw)
	prefix, rest, ok := strings.Cut(value, ":")
	if !ok || strings.TrimSpace(rest) == "" {
		return false
	}
	_, ok = parseProvider(prefix)
	return ok
}

func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			// Only set if not already present in the process environment,
			// so that vars exported by start.sh take priority over .env files.
			if key != "" && os.Getenv(key) == "" {
				os.Setenv(key, strings.TrimSpace(parts[1]))
			}
		}
	}
	return nil
}

func preferredProvider(config Config) sdk.APIProvider {
	raw := strings.TrimSpace(config.Model)
	if raw == "" {
		return ""
	}
	if HasExplicitProviderPrefix(raw) {
		return ParseModelIdentifier(raw).Provider
	}
	return DetectProviderFromModel(raw)
}

func inferProviderFromAPIKey(key string) sdk.APIProvider {
	switch {
	case strings.HasPrefix(key, "sk-ant-"):
		return sdk.APIProviderAnthropic
	case strings.HasPrefix(key, "sk-or-"):
		return sdk.APIProviderOpenRouter
	case strings.HasPrefix(key, "sk-"):
		return sdk.APIProviderOpenAI
	default:
		return sdk.APIProviderZAi
	}
}
