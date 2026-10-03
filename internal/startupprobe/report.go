// Package startupprobe defines the bounded, read-only Host startup handshake.
package startupprobe

import (
	"errors"
	"path/filepath"
	"strings"
	"winone/internal/catalog"
)

// Report proves the launched Host reached its probe entry, not UI or worker health.
type Report struct {
	UIReady             bool               `json:"uiReady,omitempty"`
	ConfigurationSHA256 string             `json:"configurationSHA256,omitempty"`
	Protocol            int                `json:"protocol"`
	Nonce               string             `json:"nonce"`
	PID                 int                `json:"pid"`
	Executable          string             `json:"executable"`
	Modules             []catalog.Manifest `json:"modules"`
}

// Validate binds the response to the process created by the caller.
func (r Report) Validate(pid int, executable, nonce string) error {
	if r.Protocol != 1 || r.PID != pid || r.Nonce != nonce || nonce == "" ||
		!filepath.IsAbs(r.Executable) || !strings.EqualFold(filepath.Clean(r.Executable), filepath.Clean(executable)) {
		return errors.New("startup probe identity mismatch")
	}
	expected := map[string]bool{"always-on-top": false, "capture": false, "wintray": false}
	if len(r.Modules) != len(expected) {
		return errors.New("startup probe module list incomplete")
	}
	for _, module := range r.Modules {
		seen, exists := expected[module.ID]
		if !exists || seen {
			return errors.New("startup probe module list invalid")
		}
		if err := module.Validate(); err != nil {
			return err
		}
		expected[module.ID] = true
	}
	return nil
}
