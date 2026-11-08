package ai

// AccountDraft describes only a connection selected in the settings form. It
// never changes global routes or commits the provider's remaining preferences.
type AccountDraft struct {
	Preferences     *ProviderPreferences `json:"providerPreferences,omitempty"`
	Models          []string             `json:"models,omitempty"`
	Headers         map[string]string    `json:"headers,omitempty"`
	LimitRestOff    bool                 `json:"limitRestOff,omitempty"`
	AdditionalTools bool                 `json:"additionalTools,omitempty"`
	Agent           string               `json:"agent"`
	Backend         string               `json:"agentBackend,omitempty"`
}

func (d AccountDraft) Provider(id string) (Provider, error) {
	p := Provider{ID: id, Name: "Account", AuthMode: "account", Protocol: "chat", Agent: d.Agent, AgentBackend: d.Backend, Models: []string{}}
	if d.Preferences != nil {
		p.ProviderPreferences = d.Preferences.clone()
	}
	p.Models, p.Headers, p.LimitRestOff, p.AdditionalTools = d.Models, d.Headers, d.LimitRestOff, d.AdditionalTools
	err := validateConfiguration(Configuration{Engine: "llm", OCREngine: "local", Target: "auto", Providers: []Provider{p}, Tasks: map[string]Selection{}})
	return p, err
}
