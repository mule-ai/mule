package system

type ModelsResponse struct {
	Providers []ProviderModels `json:"providers"`
}

type ProviderModels struct {
	Name   string   `json:"name"`
	Models []string `json:"models"`
}

type ProvidersResponse struct {
	Providers []string `json:"providers"`
}
