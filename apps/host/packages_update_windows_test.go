//go:build windows

package main

import (
	"aead.dev/minisign"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func TestInstalledPackagePreviewOffersNewerVersionWithoutChangingInstallation(t *testing.T) {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old, publisher := hostPackageArchiveFixtureSigned(t, []byte("old inert worker"), public, private, "1.0.0")
	p, store, _ := packagePreviewFixtureFromArchive(t, old, publisher)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	if err := p.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	approved := readyPackagePreview(t, p, store, old)
	raw, _ := json.Marshal(map[string]string{"id": approved.ID})
	if reply := p.installPreview(host.Request{PeerPID: 11, Params: raw}, store, service, control); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	original := store.CommittedDocument()
	for _, version := range []string{"1.1.0", "1.0.0", "0.9.0"} {
		t.Run(version, func(t *testing.T) {
			archive, _ := hostPackageArchiveFixtureSigned(t, []byte("new inert worker"), public, private, version)
			raw, _ := json.Marshal(map[string]string{"path": archive})
			reply := p.handlePreview(host.Request{PeerPID: 11, Method: "packagePreview.start", Params: raw}, store)
			if reply.Error != nil {
				t.Fatal(reply.Error)
			}
			var view struct {
				ID, State, Operation, PreviousVersion string
				CanInstall                            bool
				Error                                 string
			}
			if err := json.Unmarshal(reply.Result, &view); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for view.State == "checking" {
				if time.Now().After(deadline) {
					t.Fatal("preview timed out")
				}
				time.Sleep(10 * time.Millisecond)
				raw, _ = json.Marshal(map[string]string{"id": view.ID})
				reply = p.handlePreview(host.Request{PeerPID: 11, Method: "packagePreview.get", Params: raw}, store)
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				if err := json.Unmarshal(reply.Result, &view); err != nil {
					t.Fatal(err)
				}
			}
			if version == "1.1.0" {
				if !view.CanInstall || view.Operation != "update" || view.PreviousVersion != "1.0.0" {
					t.Fatalf("new version was not offered: %+v", view)
				}
			} else if view.CanInstall || view.Error == "" {
				t.Fatal("same or older version was accepted")
			}
			if store.CommittedDocument().Revision != original.Revision || p.Running() {
				t.Fatal("preview changed settings or started a worker")
			}
		})
	}
}

func TestUpdateConfirmationRejectsExpiredCanceledForeignAndChangedApprovals(t *testing.T) {
	for _, mode := range []string{"expired", "canceled", "foreign", "source", "configuration"} {
		t.Run(mode, func(t *testing.T) {
			public, private, err := minisign.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			old, publisher := hostPackageArchiveFixtureSigned(t, []byte("inert old worker"), public, private, "1.0.0")
			next, _ := hostPackageArchiveFixtureSigned(t, []byte("inert new worker"), public, private, "1.1.0")
			p, store, _ := packagePreviewFixtureFromArchive(t, old, publisher)
			service := host.New(store)
			control := modules.NewControl(p.state, nil, service.FeatureCatalog)
			if err = p.bind(service, control, store); err != nil {
				t.Fatal(err)
			}
			first := readyPackagePreview(t, p, store, old)
			raw, _ := json.Marshal(map[string]string{"id": first.ID})
			if installed := p.installPreview(host.Request{PeerPID: 11, Params: raw}, store, service, control); installed.Error != nil {
				t.Fatal(installed.Error)
			}
			approved := readyPackagePreview(t, p, store, next)
			raw, _ = json.Marshal(map[string]string{"id": approved.ID})
			owner := uint32(11)
			switch mode {
			case "expired":
				p.previews[approved.ID].expires = time.Now().Add(-time.Second)
			case "canceled":
				p.handlePreview(host.Request{PeerPID: 11, Method: "packagePreview.cancel", Params: raw}, store)
			case "foreign":
				owner = 12
			case "source":
				if err = os.WriteFile(next, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "configuration":
				settings := store.Snapshot()
				settings.Theme = "dark"
				if _, err = store.Save(settings); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			before, err := registry.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			revision := p.state.Snapshot().Revision
			if reply := p.updatePreview(host.Request{PeerPID: owner, Params: raw}, store, service, control); reply.Error == nil {
				t.Fatal("invalid confirmation accepted")
			}
			after, err := registry.Snapshot()
			if err != nil || after.Revision != before.Revision || after.Pending != nil || p.state.Snapshot().Revision != revision || p.Running() {
				t.Fatal("rejected update mutated the installation", err)
			}
		})
	}
}

func TestUpdatePreviewKeepsSettingsWhenCandidateCannotAcceptThem(t *testing.T) {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old, publisher := hostPackageArchiveFixtureSigned(t, []byte("inert old worker"), public, private, "1.0.0")
	p, store, _ := packagePreviewFixtureFromArchive(t, old, publisher)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	if err = p.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	first := readyPackagePreview(t, p, store, old)
	raw, _ := json.Marshal(map[string]string{"id": first.ID})
	if installed := p.installPreview(host.Request{PeerPID: 11, Params: raw}, store, service, control); installed.Error != nil {
		t.Fatal(installed.Error)
	}
	if _, _, err = store.SaveModule("example.tool", store.CommittedDocument().Revision, json.RawMessage(`{"size":7}`)); err != nil {
		t.Fatal(err)
	}
	for _, options := range []hostPackageArchiveOptions{
		{Version: "1.1.0", SchemaVersion: 2},
		{Version: "1.1.0", Schema: `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"size":{"type":"integer","maximum":5}},"required":["size"],"additionalProperties":false}`},
	} {
		candidate, _ := hostPackageArchiveFixtureDefined(t, []byte("inert new worker"), public, private, options)
		preview := checkedPackagePreview(t, p, store, candidate)
		if preview.CanInstall || preview.Error == "" {
			t.Fatal("incompatible existing configuration accepted")
		}
		value, _, err := store.Module("example.tool")
		if err != nil || string(value) != `{"size":7}` {
			t.Fatal("preview changed stored configuration", err)
		}
	}
}
