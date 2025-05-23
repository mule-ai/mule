package system

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (s *System) getModels(data any) (any, error) {
	provider, ok := data.(string)
	if !ok {
		provider = ""
	}
	response := ModelsResponse{
		Providers: []ProviderModels{},
	}

	if provider != "" {
		models, err := s.getModelsForProvider(provider)
		if err != nil {
			return nil, err
		}
		response.Providers = append(response.Providers, ProviderModels{
			Name:   provider,
			Models: models,
		})
	} else {
		for _, provider := range s.providers {
			response.Providers = append(response.Providers, ProviderModels{
				Name:   provider.Name,
				Models: provider.Models(),
			})
		}
	}
	responseString, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, err
	}
	return string(responseString), nil
}

func (s *System) getModelsForProvider(provider string) ([]string, error) {
	providerString := strings.ToLower(provider)
	for _, provider := range s.providers {
		if provider.Name == providerString {
			return provider.Models(), nil
		}
	}
	return nil, fmt.Errorf("provider not found")
}

func (s *System) getProviders(data any) (any, error) {
	response := ProvidersResponse{
		Providers: []string{},
	}
	for _, provider := range s.providers {
		response.Providers = append(response.Providers, provider.Name)
	}
	responseString, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, err
	}
	return string(responseString), nil
}
