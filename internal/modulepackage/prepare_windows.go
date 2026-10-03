//go:build windows

package modulepackage

import (
	"context"
	"errors"
	"winone/internal/modulestate"
)

type PreparedModule struct {
	Package          RegisteredPackage
	RegistryRevision uint64
	StateRevision    uint64
}

// PrepareModule leaves a retryable inventory entry if intent persistence fails.
// The package gate requires the matching digest binding, so the intermediate
// state cannot activate code. UI/runtime/configuration adapter binding follows.
// state must be the Host's authoritative live store. An offline helper must
// first exclude the running Host, rather than create a second in-memory writer.
func (r *Registry) PrepareModule(ctx context.Context, id string, options StageOptions, expectedRevision uint64, state *modulestate.Store) (PreparedModule, error) {
	if state == nil {
		return PreparedModule{}, errors.New("module state store is required")
	}
	snapshot, err := r.RegisterStage(ctx, id, options, expectedRevision)
	if err != nil {
		return PreparedModule{}, err
	}
	result := PreparedModule{RegistryRevision: snapshot.Revision}
	for _, record := range snapshot.Records {
		if record.StageID == id {
			result.Package = record
			break
		}
	}
	if result.Package.ModuleID == "" {
		return result, errors.New("registered module could not be located")
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	intent, err := state.BindPackage(result.Package.ModuleID, result.Package.ManifestSHA256)
	result.StateRevision = intent.Revision
	return result, err
}
