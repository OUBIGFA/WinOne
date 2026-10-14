//go:build windows

package modulepackage

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/modulestate"
)

func replacementStage(t *testing.T, registry *Registry, preparation ...string) (*StagedPackage, StageOptions) {
	t.Helper()
	manifest, publisher, key := signedTestManifest(t)
	manifest.Metadata.Version = "1.3.0"
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range map[string][]byte{"module.json": raw, "module.json.minisig": minisign.Sign(key, raw), "bin/worker.exe": []byte("inert worker"), "LICENSE.txt": []byte("test license")} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "replacement.zip")
	if err = os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	options := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: hex.EncodeToString(digest[:])}
	var stage *StagedPackage
	if len(preparation) == 0 {
		stage, err = StageArchive(context.Background(), path, options)
	} else {
		stage, err = registry.StageArchive(context.Background(), path, options, preparation[0])
	}
	if err != nil {
		t.Fatal(err)
	}
	return stage, options
}

func changeRequest(t *testing.T, registry *Registry, state *modulestate.Store, replacement *ReplacementStage) ChangeRequest {
	t.Helper()
	snapshot, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return ChangeRequest{ID: "change-1", RegistryRevision: snapshot.Revision, StateRevision: state.Snapshot().Revision, Current: snapshot.Records["example.tool"], Replacement: replacement}
}

func stopChangeState(t *testing.T, state *modulestate.Store) {
	t.Helper()
	if _, err := state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
		t.Fatal(err)
	}
	if _, err := state.FinishStop("example.tool"); err != nil {
		t.Fatal(err)
	}
}

func TestPackageChangeRemovalPersistsAndRetainsPreferences(t *testing.T) {
	ctx := context.Background()
	registry, stage, options := runtimeFixture(t)
	settingsPath := filepath.Join(options.DataDirectory, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"keep":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	request := changeRequest(t, registry, options.State, nil)
	prepared, err := registry.BeginChange(ctx, request, options.State)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Pending == nil || prepared.Pending.Phase != "prepared" {
		t.Fatal(prepared)
	}
	publisher, _ := options.Trust("example")
	if _, err = registry.Resolve(ctx, "example.tool", publisher, options.Environment); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("pending removal allowed loading", err)
	}
	reopened, err := OpenRegistry(registry.directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.ApplyChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	finished, err := reopened.FinishChange(ctx, request.ID, options.State, nil)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Pending != nil || finished.LastChange == nil || finished.LastChange.Outcome != "committed" || len(finished.Records) != 0 {
		t.Fatal(finished)
	}
	if options.State.Snapshot().Modules["example.tool"].PackageSHA256 != "" {
		t.Fatal("removed package is still bound")
	}
	if data, err := os.ReadFile(settingsPath); err != nil || string(data) != `{"keep":true}` {
		t.Fatal("preferences changed", err)
	}
	if _, err = os.Stat(filepath.Join(stage.Directory(), "bin", "worker.exe")); err != nil {
		t.Fatal("metadata transaction deleted payload", err)
	}
	if retry, err := reopened.BeginChange(ctx, request, options.State); err != nil || retry.Revision != finished.Revision {
		t.Fatal("removal replay changed registry", err)
	}
	approval := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	reinstalled, err := reopened.PrepareModule(ctx, filepath.Base(stage.Directory()), approval, finished.Revision, options.State)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = options.State.SetIntent(options.State.Snapshot().Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	if retry, err := reopened.BeginChange(ctx, request, options.State); err != nil || retry.Revision != reinstalled.RegistryRevision || !options.State.Snapshot().Modules["example.tool"].Enabled {
		t.Fatal("old uninstall retry changed a reinstalled package", err)
	}
}

func TestAppliedRemovalCanRestoreItsPreviousBinding(t *testing.T) {
	ctx := context.Background()
	registry, stage, options := runtimeFixture(t)
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ApplyChange(ctx, request.ID, options.State, nil); err != nil {
		t.Fatal(err)
	}
	publisher, err := options.Trust("example")
	if err != nil {
		t.Fatal(err)
	}
	previous := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	if _, err = registry.RestoreChange(ctx, request.ID, options.State, previous); err != nil {
		t.Fatal("applied removal could not restore its empty binding", err)
	}
	if _, err = registry.FinishChange(ctx, request.ID, options.State, &previous); err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil || snapshot.Pending != nil || snapshot.LastChange.Outcome != "restored" || snapshot.Records["example.tool"] != request.Current {
		t.Fatal("removal recovery lost registration", err)
	}
	if options.State.Snapshot().Modules["example.tool"].PackageSHA256 != request.Current.ManifestSHA256 {
		t.Fatal("previous binding was not restored")
	}
}

func TestPreparedRemovalRejectsUnexpectedEmptyBinding(t *testing.T) {
	ctx := context.Background()
	registry, stage, options := runtimeFixture(t)
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := options.State.ReplacePackageBinding("example.tool", request.Current.ManifestSHA256, ""); err != nil {
		t.Fatal(err)
	}
	publisher, _ := options.Trust("example")
	approval := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: stage.Manifest().Digest()}
	if _, err := registry.RestoreChange(ctx, request.ID, options.State, approval); err == nil {
		t.Fatal("out-of-order removal binding was accepted before apply")
	}
	snapshot, err := registry.Snapshot()
	if err != nil || snapshot.Pending == nil || snapshot.Pending.Phase != "prepared" {
		t.Fatal("rejected binding changed the journal", err)
	}
}

func TestPackageChangeDirectionSurvivesFailedBindingWrite(t *testing.T) {
	for _, restoring := range []bool{false, true} {
		t.Run(fmt.Sprintf("restoring-%v", restoring), func(t *testing.T) {
			ctx := context.Background()
			registry, old, options := runtimeFixture(t)
			statePath := filepath.Join(t.TempDir(), "state.json")
			state, err := modulestate.Open(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = state.BindPackage("example.tool", old.Manifest().Digest()); err != nil {
				t.Fatal(err)
			}
			after, approval := replacementStage(t, registry)
			request := changeRequest(t, registry, state, &ReplacementStage{ID: filepath.Base(after.Directory()), Options: approval})
			if _, err = registry.BeginChange(ctx, request, state); err != nil {
				t.Fatal(err)
			}
			phase, version, digest := "applied", "1.3.0", approval.ExpectedManifestSHA256
			transition := func(store *modulestate.Store) (RegistrySnapshot, error) {
				return registry.ApplyChange(ctx, request.ID, store, &approval)
			}
			finishApproval := approval
			if restoring {
				if _, err = registry.ApplyChange(ctx, request.ID, state, &approval); err != nil {
					t.Fatal(err)
				}
				publisher, _ := options.Trust("example")
				previous := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: old.Manifest().Digest()}
				transition = func(store *modulestate.Store) (RegistrySnapshot, error) {
					return registry.RestoreChange(ctx, request.ID, store, previous)
				}
				phase, version, digest = "restoring", "1.2.0", old.Manifest().Digest()
				finishApproval = previous
			}
			before := state.Snapshot()
			path, err := windows.UTF16PtrFromString(statePath)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := transition(state)
			if err = windows.CloseHandle(lock); err != nil {
				t.Fatal(err)
			}
			if writeErr == nil {
				t.Fatal("locked binding write succeeded")
			}
			reopened, err := OpenRegistry(registry.directory)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := reopened.Snapshot()
			if err != nil || journal.Pending.Phase != phase || journal.Records["example.tool"].Version != version {
				t.Fatal("direction was not persisted before binding", journal, err)
			}
			resumed, err := modulestate.Open(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if resumed.Snapshot().Revision != before.Revision {
				t.Fatal("failed binding write changed intent")
			}
			result, err := transition(resumed)
			if err != nil || result.Revision != journal.Revision {
				t.Fatal("binding recovery duplicated the registry transition", err)
			}
			if resumed.Snapshot().Modules["example.tool"].PackageSHA256 != digest {
				t.Fatal("binding resumed in the wrong direction")
			}
			if _, err = reopened.FinishChange(ctx, request.ID, resumed, &finishApproval); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPackageChangeReplacementRequiresRestoredEnableIntent(t *testing.T) {
	ctx := context.Background()
	registry, _, options := runtimeFixture(t)
	if _, err := options.State.SetIntent(options.State.Snapshot().Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	after, approval := replacementStage(t, registry)
	request := changeRequest(t, registry, options.State, &ReplacementStage{ID: filepath.Base(after.Directory()), Options: approval})
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ApplyChange(ctx, request.ID, options.State, &approval); err == nil {
		t.Fatal("active package was switched")
	}
	stopChangeState(t, options.State)
	applied, err := registry.ApplyChange(ctx, request.ID, options.State, &approval)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Pending.Phase != "applied" || applied.Records["example.tool"].Version != "1.3.0" {
		t.Fatal(applied)
	}
	if _, err = registry.Resolve(ctx, "example.tool", approval.Publisher, approval.Environment); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("unchecked replacement became public", err)
	}
	if _, err = registry.FinishChange(ctx, request.ID, options.State, &approval); err == nil {
		t.Fatal("upgrade lost enabled preference")
	}
	if _, err = options.State.SetIntent(options.State.Snapshot().Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	finished, err := registry.FinishChange(ctx, request.ID, options.State, &approval)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Pending != nil || !options.State.Snapshot().Modules["example.tool"].Enabled {
		t.Fatal("enable preference was not retained")
	}
	if _, err = registry.Resolve(ctx, "example.tool", approval.Publisher, approval.Environment); err != nil {
		t.Fatal(err)
	}
}

func TestPackageChangeFailedRegistryWriteLeavesBindingUntouched(t *testing.T) {
	ctx := context.Background()
	registry, _, options := runtimeFixture(t)
	after, approval := replacementStage(t, registry)
	request := changeRequest(t, registry, options.State, &ReplacementStage{ID: filepath.Base(after.Directory()), Options: approval})
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(registry.directory, registryFileName))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := registry.ApplyChange(ctx, request.ID, options.State, &approval)
	if err = windows.CloseHandle(lock); err != nil {
		t.Fatal(err)
	}
	if writeErr == nil {
		t.Fatal("locked registry replacement succeeded")
	}
	if options.State.Snapshot().Modules["example.tool"].PackageSHA256 != request.Current.ManifestSHA256 {
		t.Fatal("binding changed before the transition direction was durable")
	}
	reopened, err := OpenRegistry(registry.directory)
	if err != nil {
		t.Fatal(err)
	}
	before, err := reopened.Snapshot()
	if err != nil || before.Pending.Phase != "prepared" || before.Records["example.tool"].Version != "1.2.0" {
		t.Fatal(before, err)
	}
	if _, err = reopened.ApplyChange(ctx, request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.FinishChange(ctx, request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
}

func TestPackageChangeRollbackAndCancellation(t *testing.T) {
	ctx := context.Background()
	registry, old, options := runtimeFixture(t)
	after, approval := replacementStage(t, registry)
	request := changeRequest(t, registry, options.State, &ReplacementStage{ID: filepath.Base(after.Directory()), Options: approval})
	if _, err := registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ApplyChange(ctx, request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	publisher, _ := options.Trust("example")
	previous := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: old.Manifest().Digest()}
	if _, err := registry.RestoreChange(ctx, request.ID, options.State, previous); err != nil {
		t.Fatal(err)
	}
	finished, err := registry.FinishChange(ctx, request.ID, options.State, &previous)
	if err != nil {
		t.Fatal(err)
	}
	if finished.LastChange.Outcome != "restored" || finished.Records["example.tool"].Version != "1.2.0" {
		t.Fatal(finished)
	}
	request = changeRequest(t, registry, options.State, nil)
	request.ID = "cancel-2"
	if _, err = registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.CancelChange(ctx, request.ID, options.State); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.Resolve(ctx, "example.tool", publisher, options.Environment); err != nil {
		t.Fatal(err)
	}
}

func TestPackageChangeCandidatePermitIsBoundAndCopied(t *testing.T) {
	ctx := context.Background()
	registry, _, options := runtimeFixture(t)
	old, err := registry.LoadModule(ctx, "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	after, approval := replacementStage(t, registry)
	request := changeRequest(t, registry, options.State, &ReplacementStage{ID: filepath.Base(after.Directory()), Options: approval})
	if _, err = registry.BeginChange(ctx, request, options.State); err != nil {
		t.Fatal(err)
	}
	if err = old.Unload(); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.ApplyChange(ctx, request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	options.Trust = func(string) (Publisher, error) { return approval.Publisher, nil }
	if _, err = registry.LoadModule(ctx, "example.tool", options); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("normal load bypassed pending verification", err)
	}
	options.Change = &ChangePermit{ID: request.ID, RegistryRevision: request.RegistryRevision + 1}
	if _, err = registry.LoadModule(ctx, "example.tool", options); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("another operation epoch was accepted", err)
	}
	options.Change.RegistryRevision = request.RegistryRevision
	candidate, err := registry.LoadModule(ctx, "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := candidate.Close(); err != nil {
			t.Error(err)
		}
	})
	options.Change.RegistryRevision++
	if err = candidate.Resume(); err != nil {
		t.Fatal("caller changed the candidate's stored permit", err)
	}
	if _, err = registry.FinishChange(ctx, request.ID, options.State, &approval); err != nil {
		t.Fatal(err)
	}
	if reply := options.Service.Handle(host.Request{Version: 1, ID: "disabled", Method: "example.tool.echo"}); reply.Error == nil {
		t.Fatal("completed upgrade enabled a disabled module")
	}
}
