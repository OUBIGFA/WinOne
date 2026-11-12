package distribution

import "testing"

// Old installed manifests still have to be read to replace their files. The
// same artifacts must never enter a newly built package after module removal.
func TestRetiredPayloadOnlyAcceptedForHistoricalInstallations(t *testing.T) {
	for _, name := range []string{
		"shared/chat/worker.js", "shared/chat/onebot/worker.js",
		"shared/chat/libraryDbWorker.js", "shared/chat/ghost.js",
		"WinOne.Plugin.exe", "WinOne.PDF.exe", "pdfium.dll", "ghostty-vt.dll",
		"shared/ai/plugins/redact/index.mjs", "shared/ai/runtime/photon_rs_bg.wasm",
	} {
		if allowed(name) {
			t.Errorf("retired payload is allowed in a new package: %s", name)
		}
		if !allowedHistorical(name) {
			t.Errorf("old installed payload cannot be read for upgrade: %s", name)
		}
	}
}
