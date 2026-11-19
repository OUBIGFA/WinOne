package aiservices

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed header_presets.json
var headerPresetJSON []byte

type headerPreset struct {
	Name    string      `json:"name"`
	Headers [][2]string `json:"headers"`
}

var headerPresets = func() []headerPreset {
	var presets []headerPreset
	if err := json.Unmarshal(headerPresetJSON, &presets); err != nil {
		panic(err)
	}
	return presets
}()

// The original applyHeaderPreset fills empty values and appends absent names.
// Explicit user values, spelling, order and JSON editing state stay intact.
func applyHeaderPreset(rows *[]*headerRow, preset headerPreset) {
	kept := (*rows)[:0]
	for _, row := range *rows {
		if strings.TrimSpace(row.Name) != "" || strings.TrimSpace(row.Value) != "" {
			kept = append(kept, row)
		}
	}
	*rows = kept
	for _, header := range preset.Headers {
		var had *headerRow
		for _, row := range *rows {
			if strings.EqualFold(strings.TrimSpace(row.Name), header[0]) {
				had = row
				break
			}
		}
		if had == nil {
			*rows = append(*rows, &headerRow{Name: header[0], Value: header[1]})
		} else if strings.TrimSpace(had.Value) == "" {
			had.Value = header[1]
		}
	}
}
