package settings

import (
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/integration"
)

const (
	CommitAgent   = 0
	PRTitleAgent  = 1
	PRBodyAgent   = 2
	StartingAgent = 3
)

type Settings struct {
	Environment []EnvironmentVariable    `json:"environment"`
	GitHubToken string                   `json:"githubToken"`
	AIProviders []AIProviderSettings     `json:"aiProviders"`
	Agents      []agent.AgentOptions     `json:"agents"`
	SystemAgent SystemAgentSettings      `json:"systemAgent"`
	Workflows   []agent.WorkflowSettings `json:"workflows"`
	Integration integration.Settings     `json:"integration"`
}

type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TriggerSettings struct {
	Integration string `json:"integration"`
	Event       string `json:"event"`
	Data        any    `json:"data"`
}

type AIProviderSettings struct {
	Name     string `json:"name"`
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
	SystemPrompt    string `json:"systemPrompt"`
}
