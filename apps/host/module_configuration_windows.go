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

func nativeModuleLifecycle(state *modulestate.Store, gates map[string]*modules.Gate) configuration.ModuleLifecycle {
	initial := map[string]bool{}
	ids := []string{}
	for id, record := range state.Snapshot().Modules {
		if gates[id] == nil {
			continue
		}
		initial[id] = record.Enabled
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return configuration.ModuleLifecycle{
		Initial: initial,
		Before: func(target map[string]bool) error {
			for id, record := range state.Snapshot().Modules {
				if gates[id] == nil && (record.Enabled || record.PendingStop || target[id]) {
					return errors.New("模块生命周期未登记")
				}
			}
			for _, id := range ids {
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
			for _, id := range ids {
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

func moduleConfigurationHandler(store *configuration.HostStore, state *modulestate.Store, control *modules.Control) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_configuration", Message: err.Error()}}
		}
		var result any
		switch request.Method {
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
				ConfigurationError string `json:"configurationError"`
			}{ControlSnapshot: control.Snapshot()}
			if err := store.RecoveryError(); err != nil {
				snapshot.ConfigurationError = err.Error()
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
