package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/yetone/magpie/internal/provider"
)

// Quota/check-in/warm-up readers resolve settings from the isolated account
// profile themselves. Save the current Host choices through upstream's account
// serializer, which strips discovered tokens and leaves the sign-in untouched.
func prepareAccountPreferences(ctx context.Context, in request) error {
	if in.ProviderPreferences == nil {
		return nil
	}
	p, err := findAccount(ctx, in.Agent)
	if err != nil {
		return err
	}
	if err = applyProviderPreferences(p, in); err != nil {
		return err
	}
	p.Headers = in.Headers
	if in.Models != nil {
		p.Models = in.Models
	}
	return provider.Save(*p)
}

// Apply only the shared, typed writable options; endpoint, authentication,
// account discovery and model identities remain owned by the selected path.
// Decoding into the actual upstream Provider preserves its field semantics
// and keeps newly added options out of bespoke request/routing implementations.
func applyProviderPreferences(p *provider.Provider, in request) error {
	if in.ProviderPreferences == nil {
		return nil
	}
	if err := in.ProviderPreferences.validate(); err != nil {
		return err
	}
	if why := provider.CheckRateWindows(toRateWindows(in.ProviderPreferences.RateWindows)); why != "" {
		return errors.New(why)
	}
	// Include zero values when applying a complete Host snapshot. Otherwise a
	// cleared option would silently retain the old isolated account preference.
	fields := map[string]any{}
	value := reflect.ValueOf(*in.ProviderPreferences)
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = value.Field(i).Interface()
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, p)
}

// Only the type changes; the upstream check is the authority for these windows.
func toRateWindows(in []RateWindow) []provider.RateWindow {
	out := make([]provider.RateWindow, len(in))
	for i, w := range in {
		out[i] = provider.RateWindow{Days: w.Days, Start: w.Start, End: w.End, Rate: w.Rate}
	}
	return out
}
