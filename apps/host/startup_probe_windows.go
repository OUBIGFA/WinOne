//go:build windows

package main

import (
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
	if err = configuration.CheckUpgrade(data, aotConfiguration("", nil), captureConfiguration("", nil), wintrayConfiguration("", nil, nil)); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	return writeProbeReport(output, nonce, hex.EncodeToString(sum[:]))
}

func writeProbeReport(output io.Writer, nonce, configurationSHA string) error {
	if nonce == "" || len(nonce) > 128 {
		return errors.New("invalid startup probe nonce")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	report := startupprobe.Report{Protocol: 1, Nonce: nonce, PID: os.Getpid(), Executable: executable, Modules: catalog.Builtins(), ConfigurationSHA256: configurationSHA}
	if err = report.Validate(os.Getpid(), executable, nonce); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(report)
}
