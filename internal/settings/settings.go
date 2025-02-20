package settings

import (
	"github.com/mule-ai/mule/pkg/agent"
)

const (
	CommitAgent   = 0
	PRTitleAgent  = 1
	PRBodyAgent   = 2
	StartingAgent = 3
)

type Settings struct {
	GitHubToken string               `json:"githubToken"`
	AIProviders []AIProviderSettings `json:"aiProviders"`
	Agents      []agent.AgentOptions `json:"agents"`
	SystemAgent SystemAgentSettings  `json:"systemAgent"`
}

type AIProviderSettings struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	Server   string `json:"server"`
}

type SystemAgentSettings struct {
	ProviderName    string `json:"providerName"`
	Model           string `json:"model"`
	CommitTemplate  string `json:"commitTemplate"`
	PRTitleTemplate string `json:"prTitleTemplate"`
	PRBodyTemplate  string `json:"prBodyTemplate"`
}
