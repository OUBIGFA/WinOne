package ai

import "errors"

// Only account-provider execution mode remains user configurable. Model
// metadata is assembled from retained providers for the local gateway.
// Old advanced fields are ignored on read and never reach a worker.
type MagpieSettings struct {
	ClaudePrintMode    bool   `json:"claudePrintMode,omitempty"`
	ClaudeNoExtraUsage bool   `json:"claudeNoExtraUsage,omitempty"`
	CodexClient        string `json:"codexClient,omitempty"`
	// The public gateway fills these from its machine-local settings. They
	// are not copied from portable AI provider preferences.
	BareModelIDs     bool `json:"bareModelIDs,omitempty"`
	BareModelIDsOnly bool `json:"bareModelIDsOnly,omitempty"`
	NoStats          bool `json:"noStats"`
	NoUsageStats     bool `json:"noUsageStats"`
}

func (s *MagpieSettings) clone() *MagpieSettings {
	out := &MagpieSettings{NoStats: true, NoUsageStats: true}
	if s != nil {
		out.ClaudePrintMode = s.ClaudePrintMode
		out.ClaudeNoExtraUsage = s.ClaudeNoExtraUsage
		out.CodexClient = s.CodexClient
	}
	return out
}
func (s *MagpieSettings) validate() error {
	if s != nil && s.CodexClient != "" && s.CodexClient != "desktop" {
		return errors.New("Codex 请求来源必须为 CLI 或桌面应用")
	}
	return nil
}
