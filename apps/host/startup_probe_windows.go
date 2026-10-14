//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/startupprobe"
)

// This entry is deliberately read-only: it cannot acquire runtime resources,
// read a profile, register shortcuts, or run login programs during maintenance.
func writeStartupProbe(output io.Writer, nonce string) error {
	return writeProbeReport(output, nonce, "")
}

// Configuration bytes arrive over the owned process stdin; no filename or
// live profile is opened, and adapters contribute validation only.
func writeConfigurationProbe(output io.Writer, input io.Reader, nonce string) error {
	data, err := io.ReadAll(io.LimitReader(input, (8<<20)+1))
	if err != nil {
		return err
	}
	if err = configuration.CheckUpgrade(data, aotConfiguration("", nil), captureConfiguration("", nil), wintrayConfiguration("", nil, nil), selectionConfiguration("", nil), quickAssistantConfiguration("", nil), strokeConfiguration("", nil)); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	return writeProbeReport(output, nonce, hex.EncodeToString(sum[:]))
}

func writeProbeReport(output io.Writer, nonce, configurationSHA string) error {
	return writeProfileReport(output, nonce, configurationSHA, "", nil)
}

// This separate entry may read the explicit profile's package/control files,
// but cannot enter normal startup, acquire resources, or modify the profile.
func writeProfileProbe(output io.Writer, input io.Reader, nonce, directory string) error {
	if nonce == "" || len(nonce) > 128 {
		return errors.New("invalid profile probe nonce")
	}
	data, err := io.ReadAll(io.LimitReader(input, (8<<20)+1))
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 8<<20 {
		return errors.New("invalid profile probe input")
	}
	snapshot, err := startupprobe.ReadProfileSnapshot(directory)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, snapshot.Configuration) {
		return errors.New("profile probe configuration changed")
	}
	trust, err := moduleTrust(modulePublishers)
	if err != nil {
		return err
	}
	issues, err := checkProfileUpgrade(context.Background(), directory, snapshot, packageEnvironment(), trust)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	return writeProfileReport(output, nonce, hex.EncodeToString(sum[:]), snapshot.Digest(), issues)
}

func writeProfileReport(output io.Writer, nonce, configurationSHA, profileSHA string, issues []startupprobe.ProfileIssue) error {
	if nonce == "" || len(nonce) > 128 {
		return errors.New("invalid startup probe nonce")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	report := startupprobe.Report{Protocol: 1, Nonce: nonce, PID: os.Getpid(), Executable: executable, Modules: catalog.Builtins(), ConfigurationSHA256: configurationSHA}
	report.ProfileSHA256 = profileSHA
	report.ProfileIssueCount = len(issues)
	if len(issues) > 32 {
		issues = issues[:32]
	}
	report.ProfileIssues = issues
	if err = report.Validate(os.Getpid(), executable, nonce); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(report)
}
