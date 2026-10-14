//go:build windows

package startupprobe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type desktopFailure struct {
	SchemaVersion                int       `json:"schemaVersion"`
	Stage                        string    `json:"stage"`
	RecordedAt                   time.Time `json:"recordedAt"`
	UIProcessID                  int       `json:"uiProcessId"`
	BrowserProcessID             uint32    `json:"browserProcessId"`
	BrowserCreated               uint64    `json:"browserCreated"`
	UIElapsedMs                  int64     `json:"uiElapsedMs"`
	BrowserWaitMs                int64     `json:"browserWaitMs"`
	RemainingBeforeBrowserWaitMs int64     `json:"remainingBeforeBrowserWaitMs"`
	RuntimeDiagnostic            string    `json:"runtimeDiagnostic,omitempty"`
	Error                        string    `json:"error"`
}

// Only validated process identity and bounded runtime output enter this record.
// Do not persist the handshake nonce, user configuration or environment values.
func recordDesktopFailure(scratch string, report Report, uiElapsed, waited, remaining time.Duration, runtimeText string, failure error) (string, error) {
	file, err := os.CreateTemp(scratch, "desktop-probe-failure-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	value := desktopFailure{SchemaVersion: 1, Stage: "browser-exit", RecordedAt: time.Now().UTC(), UIProcessID: report.PID, BrowserProcessID: report.BrowserProcessID, BrowserCreated: report.BrowserCreated, UIElapsedMs: uiElapsed.Milliseconds(), BrowserWaitMs: waited.Milliseconds(), RemainingBeforeBrowserWaitMs: remaining.Milliseconds(), RuntimeDiagnostic: runtimeText, Error: failure.Error()}
	err = json.NewEncoder(file).Encode(value)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	return filepath.Clean(path), err
}
