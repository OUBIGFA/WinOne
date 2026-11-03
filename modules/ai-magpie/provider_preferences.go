package main

import (
	"encoding/json"
	"errors"

	"github.com/yetone/magpie/internal/provider"
)

// Apply only the shared, typed writable options; endpoint, authentication,
// account discovery and model identities remain owned by the selected path.
// Decoding into the actual upstream Provider preserves its field semantics
// and keeps newly added options out of bespoke request/routing implementations.
func applyProviderPreferences(p *provider.Provider, in request) error {
	if in.ProviderPreferences == nil { return nil }
	if err := in.ProviderPreferences.validate(); err != nil { return err }
	if why := provider.CheckRateWindows(toRateWindows(in.ProviderPreferences.RateWindows)); why != "" { return errors.New(why) }
	data, err := json.Marshal(in.ProviderPreferences)
	if err != nil { return err }
	return json.Unmarshal(data, p)
}

// Only the type changes; the upstream check is the authority for these windows.
func toRateWindows(in []RateWindow) []provider.RateWindow {
	out := make([]provider.RateWindow, len(in))
	for i, w := range in { out[i] = provider.RateWindow{Days:w.Days, Start:w.Start, End:w.End, Rate:w.Rate} }
	return out
}
