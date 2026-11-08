package ai

// RuntimeProvider is private request data, never a settings response. Each
// auxiliary call uses this snapshot and the same per-provider account home.
type RuntimeProvider struct {
	Provider Provider
	Key      string
	Keys     []RuntimeKey
}
type RuntimeKey struct {
	ID, Key string
	KeyOptions
}
type auxiliaryKey struct {
	KeyOptions
	Name string `json:"name"`
	Key  string `json:"key"`
}

type auxiliaryProvider struct {
	ProviderPreferences
	LimitRestOff   bool                     `json:"limitRestOff,omitempty"`
	AccountKind    string                   `json:"accountKind,omitempty"` // Public account type for provider-specific middleware, never auth.
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Icon           string                   `json:"icon,omitempty"`
	Chat           string                   `json:"chat,omitempty"`
	Responses      string                   `json:"responses,omitempty"`
	Anthropic      string                   `json:"anthropic,omitempty"`
	Key            string                   `json:"key"`
	KeyName        string                   `json:"keyName,omitempty"`
	KeyProtocol    string                   `json:"keyProtocol,omitempty"`
	KeyWeight      int                      `json:"keyWeight,omitempty"`
	KeyPlan        string                   `json:"keyPlan,omitempty"`
	Keys           []auxiliaryKey           `json:"keys,omitempty"`
	Models         []string                 `json:"models"`
	Searches       bool                     `json:"searches,omitempty"`
	Metadata       map[string]ModelMetadata `json:"metadata,omitempty"`
	ModelNames     map[string]string        `json:"modelNames,omitempty"`
	DecisionSource *decisionSource          `json:"decisionSource,omitempty"`
	NativeSource   map[string]any           `json:"nativeSource,omitempty"`
	NativeKeys     []auxiliaryKey           `json:"nativeKeys,omitempty"`
	KeyIDs         map[string]string        `json:"keyIDs,omitempty"`
}

// Credential-free descriptor lets the upstream runtime decide API support.
type decisionSource struct {
	ProviderPreferences *ProviderPreferences `json:"providerPreferences,omitempty"`
	Protocol            string               `json:"protocol"`
	BaseURL             string               `json:"baseUrl"`
	Preset              string               `json:"preset,omitempty"`
	PresetRegion        string               `json:"presetRegion,omitempty"`
}
