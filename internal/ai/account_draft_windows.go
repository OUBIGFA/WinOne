package ai

// AccountDraft describes only a connection selected in the settings form. It
// never changes global routes or commits the provider's remaining preferences.
type AccountDraft struct {
	Agent   string `json:"agent"`
	Backend string `json:"agentBackend,omitempty"`
}

func (d AccountDraft) Provider(id string) (Provider, error) {
	p := Provider{ID: id, Name: "Account", AuthMode: "account", Protocol: "chat", Agent: d.Agent, AgentBackend: d.Backend, Models: []string{}}
	err := validateConfiguration(Configuration{Engine: "llm", OCREngine: "local", Target: "auto", Providers: []Provider{p}, Tasks: map[string]Selection{}})
	return p, err
}
