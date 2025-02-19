package settings

import "github.com/mule-ai/mule/pkg/agent"

type Settings struct {
	GitHubToken string               `json:"githubToken"`
	AIProviders []AIProviderSettings `json:"aiProviders"`
	Agents      []agent.AgentOptions `json:"agents"`
}

type AIProviderSettings struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	Server   string `json:"server"`
}
