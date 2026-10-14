// Package startupprobe defines the bounded, read-only Host startup handshake.
package startupprobe

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"winone/internal/catalog"
)

type ProfileIssue struct {
	ModuleID string `json:"moduleId"`
	Reason   string `json:"reason"`
}

// Report proves the launched Host reached its probe entry, not UI or worker health.
type Report struct {
	UIReady             bool               `json:"uiReady,omitempty"`
	BrowserProcessID    uint32             `json:"browserProcessId,omitempty"`
	BrowserCreated      uint64             `json:"browserCreated,omitempty"`
	ConfigurationSHA256 string             `json:"configurationSHA256,omitempty"`
	ProfileSHA256       string             `json:"profileSHA256,omitempty"`
	ProfileIssues       []ProfileIssue     `json:"profileIssues,omitempty"`
	ProfileIssueCount   int                `json:"profileIssueCount,omitempty"`
	Protocol            int                `json:"protocol"`
	Nonce               string             `json:"nonce"`
	PID                 int                `json:"pid"`
	Executable          string             `json:"executable"`
	Modules             []catalog.Manifest `json:"modules"`
}

// Validate binds the response to the process created by the caller.
func (r Report) Validate(pid int, executable, nonce string) error {
	if r.ProfileSHA256 != "" {
		raw, err := hex.DecodeString(r.ProfileSHA256)
		if err != nil || len(raw) != 32 || r.ProfileSHA256 != strings.ToLower(r.ProfileSHA256) || r.ConfigurationSHA256 == "" {
			return errors.New("profile probe identity invalid")
		}
	} else if len(r.ProfileIssues) > 0 || r.ProfileIssueCount != 0 {
		return errors.New("profile issues lack profile identity")
	}
	if len(r.ProfileIssues) > 32 || r.ProfileIssueCount < len(r.ProfileIssues) || r.ProfileIssueCount > 4096 {
		return errors.New("invalid profile issue count")
	}
	seenIssues := map[string]bool{}
	for _, issue := range r.ProfileIssues {
		if !catalog.ValidOptionalID(issue.ModuleID) || seenIssues[issue.ModuleID] || issue.Reason == "" || len(issue.Reason) > 512 {
			return errors.New("invalid profile issue")
		}
		seenIssues[issue.ModuleID] = true
	}
	if (r.BrowserProcessID == 0) != (r.BrowserCreated == 0) || r.BrowserProcessID != 0 && (!r.UIReady || int(r.BrowserProcessID) == r.PID) {
		return errors.New("desktop probe browser identity invalid")
	}
	if r.Protocol != 1 || r.PID != pid || r.Nonce != nonce || nonce == "" ||
		!filepath.IsAbs(r.Executable) || !strings.EqualFold(filepath.Clean(r.Executable), filepath.Clean(executable)) {
		return errors.New("startup probe identity mismatch")
	}
	expected := map[string]bool{"always-on-top": false, "capture": false, "wintray": false}
	// Previously installed three-module builds remain valid rollback targets.
	if len(r.Modules) >= 4 {
		expected["selection"] = false
	}
	if len(r.Modules) >= 5 {
		expected["quick-assistant"] = false
	}
	if len(r.Modules) == 6 {
		expected["stroke"] = false
	}
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
