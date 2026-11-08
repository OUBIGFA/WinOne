package ai

import "fmt"

// CodexAsyncSnippet is an explicit export for the external client's config.
// WinOne never edits that file. The model provider table must be the one the
// client uses for WinOne, as required by upstream codex_async.go (Codex 0.163+).
func (s MagpieClientSettings) CodexAsyncSnippet() string {
	if !s.CodexAsyncCompact {
		return ""
	}
	percent := s.CodexAsyncCompactPercent
	if percent == 0 {
		percent = 65
	}
	return fmt.Sprintf("[features.async_compaction]\nenabled = true\nmodel_async_compact_threshold_percent = %d\n\n[model_providers.winone.capabilities]\nremote_compaction = \"v2\"\n", percent)
}
