//go:build windows

package modulepackage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/settings"
)

// SettingsProjection is read-only to the worker. The Host owns this file and
// its revision; a running worker must acknowledge settings.reload synchronously.
type SettingsProjection struct {
	SchemaVersion int             `json:"schemaVersion"`
	Revision      uint64          `json:"revision"`
	Values        json.RawMessage `json:"values"`
}

// ProbeRecovery is a coordinator-only, unbound lifecycle check. A verified
// worker sees committed settings, answers its identity probe, restores resources
// via shutdown, and exits before any package recovery is finalized.
func (r *RuntimeModule) ProbeRecovery() error {
	_, err := r.probe(false)
	return err
}

// ProbeCandidate checks an unbound replacement. The first result proves that
// no new process was ever created, or that shutdown and exit were confirmed;
// it is independent of whether the candidate's health check succeeded.
func (r *RuntimeModule) ProbeCandidate() (restored bool, err error) { return r.probe(true) }

func (r *RuntimeModule) probe(allowNeverStarted bool) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bound || r.unloaded {
		return false, errors.New("recovery probe requires an unbound runtime")
	}
	if err := r.prepareFiles(); err != nil {
		return allowNeverStarted && !r.worker.HasStarted() && r.files == nil && r.dependencies == nil, err
	}
	r.worker.Resume()
	response := r.worker.Handle(host.Request{Version: 1, ID: "package-recovery-probe", Method: "snapshot"})
	var result error
	if response.Error != nil {
		result = errors.New(response.Error.Message)
	}
	if allowNeverStarted && !r.worker.HasStarted() {
		closeErr := r.worker.Close()
		filesErr := r.releaseFiles()
		return closeErr == nil && filesErr == nil, errors.Join(result, closeErr, filesErr)
	}
	if err := r.worker.CloseConfirmed(); err != nil {
		return false, errors.Join(result, err)
	}
	filesErr := r.releaseFiles()
	result = errors.Join(result, filesErr)
	if result == nil {
		r.worker.Resume()
		r.closing = false
	}
	return filesErr == nil, result
}

func (r *RuntimeModule) ProjectSettings(raw json.RawMessage, revision uint64) error {
	if len(raw) > MaxSettingsBytes {
		return errors.New("module settings exceed size limit")
	}
	if err := configjson.ValidateObject(raw); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unloaded {
		return errors.New("module runtime was unloaded")
	}
	if err := r.currentTrust(); err != nil {
		return err
	}
	if r.dependencies != nil {
		if err := r.dependencies.Recheck(r.ctx); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(r.options.DataDirectory, 0700); err != nil {
		return err
	}
	projection := SettingsProjection{SchemaVersion: r.manifest.metadata.Module.SettingsSchemaVersion, Revision: revision, Values: raw}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return err
	}
	if err = settings.WriteAtomic(filepath.Join(r.options.DataDirectory, "settings.json"), encoded); err != nil {
		return err
	}
	if !r.worker.Running() {
		return nil
	}
	response := r.worker.Handle(host.Request{Version: 1, ID: "configuration-reload", Method: "settings.reload"})
	if response.Error != nil {
		return errors.New(response.Error.Message)
	}
	var ack struct {
		Revision *uint64 `json:"revision"`
	}
	if err = json.Unmarshal(response.Result, &ack); err != nil || ack.Revision == nil || *ack.Revision != revision {
		return errors.New("module did not acknowledge the applied settings revision")
	}
	return nil
}
