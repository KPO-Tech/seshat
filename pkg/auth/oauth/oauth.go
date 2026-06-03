package oauth

import internaloauth "github.com/EngineerProjects/nexus-engine/internal/auth/oauth"

type (
	Client             = internaloauth.Client
	Config             = internaloauth.Config
	DeviceCodeResponse = internaloauth.DeviceCodeResponse
	Token              = internaloauth.Token
	TokenResponse      = internaloauth.TokenResponse
	TokenType          = internaloauth.TokenType
)

func DefaultOpenAIConfig() *Config {
	return internaloauth.DefaultOpenAIConfig()
}

func NewClient(config *Config) *Client {
	return internaloauth.NewClient(config)
}
