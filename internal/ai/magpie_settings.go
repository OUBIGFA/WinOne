package ai

// Only account-provider execution mode remains user configurable. Model
// metadata is assembled from retained providers for the local gateway.
// Old advanced fields are ignored on read and never reach a worker.
type MagpieSettings struct {
	ClaudePrintMode bool `json:"claudePrintMode,omitempty"`
	NoStats         bool `json:"noStats"`
	NoUsageStats    bool `json:"noUsageStats"`
}

func (s *MagpieSettings) clone() *MagpieSettings {
	out := &MagpieSettings{NoStats: true, NoUsageStats: true}
	if s != nil {
		out.ClaudePrintMode = s.ClaudePrintMode
	}
	return out
}
func (s *MagpieSettings) validate() error { return nil }
