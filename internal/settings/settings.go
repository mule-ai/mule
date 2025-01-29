package settings

type Settings struct {
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	APIKey      string `json:"apiKey"`
	Server      string `json:"server"`
	GitHubToken string `json:"githubToken"`
}
