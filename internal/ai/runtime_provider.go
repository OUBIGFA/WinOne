package ai

// RuntimeProvider is private request data, never a settings response. Each
// auxiliary call uses this snapshot and the same per-provider account home.
type RuntimeProvider struct {
	Provider Provider
	Key      string
	Keys     []RuntimeKey
}
type RuntimeKey struct{ ID, Key string }
type auxiliaryKey struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type auxiliaryProvider struct {
	AccountKind string                   `json:"accountKind,omitempty"` // Public account type for provider-specific middleware, never auth.
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Icon        string                   `json:"icon,omitempty"`
	Chat        string                   `json:"chat,omitempty"`
	Responses   string                   `json:"responses,omitempty"`
	Anthropic   string                   `json:"anthropic,omitempty"`
	Key         string                   `json:"key"`
	KeyName     string                   `json:"keyName,omitempty"`
	Keys        []auxiliaryKey           `json:"keys,omitempty"`
	Models      []string                 `json:"models"`
	Searches    bool                     `json:"searches,omitempty"`
	Metadata    map[string]ModelMetadata `json:"metadata,omitempty"`
}
