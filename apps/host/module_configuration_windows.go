//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"sort"
	"winone/internal/configjson"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
)

func nativeModuleLifecycle(state *modulestate.Store, source func() map[string]*modules.Gate, isolation ...func() map[string]bool) configuration.ModuleLifecycle {
	blocked := func() map[string]bool {
		if len(isolation) > 0 && isolation[0] != nil {
			return isolation[0]()
		}
		return nil
	}
	current := func() (map[string]*modules.Gate, []string) {
		gates := source()
		ids := make([]string, 0, len(gates))
		for id := range gates {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return gates, ids
	}
	gates, _ := current()
	initial := map[string]bool{}
	for id, record := range state.Snapshot().Modules {
		if gates[id] == nil {
			continue
		}
		initial[id] = record.Enabled
	}
	for id, intent := range blocked() {
		initial[id] = intent
	}
	return configuration.ModuleLifecycle{
		Initial: initial,
		Before: func(target map[string]bool) error {
			gates, ids := current()
			isolated := blocked()
			for id, record := range state.Snapshot().Modules {
				if _, skip := isolated[id]; skip {
					continue
				}
				if gates[id] == nil && (record.Enabled || record.PendingStop || target[id]) {
					return errors.New("模块生命周期未登记")
				}
			}
			for _, id := range ids {
				if _, skip := isolated[id]; skip {
					continue
				}
				gate := gates[id]
				if gate == nil {
					return errors.New("模块生命周期未登记")
				}
				current := state.Snapshot()
				// A failed disable must first finish returning its resources, even when
				// rollback wants the module enabled again afterwards.
				if !target[id] || current.Modules[id].PendingStop {
					if err := gate.SetEnabled(current.Revision, false); err != nil {
						return err
					}
				}
			}
			return nil
		},
		After: func(target map[string]bool) error {
			gates, ids := current()
			isolated := blocked()
			for id, enabled := range target {
				if _, skip := isolated[id]; skip {
					continue
				}
				if enabled && gates[id] == nil {
					return errors.New("模块生命周期未登记")
				}
			}
			for _, id := range ids {
				if _, skip := isolated[id]; skip {
					continue
				}
				if target[id] {
					if err := gates[id].SetEnabled(state.Snapshot().Revision, true); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

func moduleConfigurationHandler(store *configuration.HostStore, state *modulestate.Store, control *modules.Control, packageRecovery ...func(host.Request) host.Response) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_configuration", Message: err.Error()}}
		}
		var result any
		switch request.Method {
		case "recoverPackage", "packagePreview.start", "packagePreview.get", "packagePreview.cancel", "installPackage", "updatePackage", "packageRemoval.preview", "removePackage", "cleanupPackages":
			if len(packageRecovery) != 1 || packageRecovery[0] == nil {
				return fail(errors.New("模块包恢复入口未就绪"))
			}
			return packageRecovery[0](request)
		case "resetWinTray.preview":
			preview, err := previewWinTrayReset(store, state)
			if err != nil {
				return fail(err)
			}
			result = preview
		case "resetWinTray":
			var args struct {
				ConfigurationRevision uint64 `json:"configurationRevision"`
				ModuleRevision        uint64 `json:"moduleRevision"`
				Confirmed             bool   `json:"confirmed"`
			}
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			if err := resetWinTray(store, state, args.ConfigurationRevision, args.ModuleRevision, args.Confirmed); err != nil {
				return fail(err)
			}
			result = modules.ControlJob{State: "completed"}
		case "snapshot":
			snapshot := struct {
				modules.ControlSnapshot
				ConfigurationError string          `json:"configurationError"`
				Cleanup            json.RawMessage `json:"cleanup,omitempty"`
				Packages           json.RawMessage `json:"packages,omitempty"`
			}{ControlSnapshot: control.Snapshot()}
			if err := store.RecoveryError(); err != nil {
				snapshot.ConfigurationError = err.Error()
			}
			if len(packageRecovery) == 1 && packageRecovery[0] != nil {
				query := request
				query.Method = "cleanupStatus"
				status := packageRecovery[0](query)
				if status.Error != nil {
					snapshot.Cleanup, _ = json.Marshal(packageCleanupStatus{Pending: true, Error: status.Error.Message})
				} else {
					snapshot.Cleanup = status.Result
				}
				query.Method = "packageInventory"
				inventory := packageRecovery[0](query)
				if inventory.Error != nil {
					snapshot.Packages, _ = json.Marshal(packageInventoryView{Error: inventory.Error.Message})
				} else {
					snapshot.Packages = inventory.Result
				}
			}
			result = snapshot
		case "setEnabled":
			var args struct {
				ID       string `json:"id"`
				Enabled  *bool  `json:"enabled"`
				Revision uint64 `json:"revision"`
			}
			if err := configjson.DecodeObject(request.Params, &args); err != nil {
				return fail(err)
			}
			if args.Enabled == nil {
				return fail(errors.New("缺少模块开关"))
			}
			if state.Snapshot().Revision != args.Revision {
				return fail(configuration.ErrConflict)
			}
			document := store.CommittedDocument()
			if _, exists := document.ModuleEnabled[args.ID]; !exists {
				return fail(errors.New("模块未登记"))
			}
			document.ModuleEnabled[args.ID] = *args.Enabled
			if _, err := store.CommitDocument(document); err != nil {
				return fail(err)
			}
			result = modules.ControlJob{State: "completed"}
		case "recoverConfiguration":
			if err := store.Recover(); err != nil {
				return fail(err)
			}
			result = modules.ControlJob{State: "completed"}
		default:
			return control.Handle(request)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: data}
	}
}
