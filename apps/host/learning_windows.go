//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"
	"winone/internal/ankiengine"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/learning"
	"winone/internal/processidentity"
)

// registerLearning shares one collection worker between settings and result
// windows. Result calls retain their session ownership checks before dispatch.
func registerLearning(service *host.Service, ctx context.Context, data, executable, pipe string, results *resultWindows) func() error {
	bundle := filepath.Dir(executable)
	desktop := filepath.Join(bundle, "WinOne.exe")
	engine := ankiengine.New(ankiengine.BundleOptions(bundle, data))
	var pendingMu sync.Mutex
	var pending json.RawMessage
	type change struct {
		Sequence uint64          `json:"sequence"`
		Owner    uint32          `json:"owner"`
		Event    json.RawMessage `json:"event"`
	}
	var sequence uint64
	var changes []change
	publish := func(owner uint32, events []json.RawMessage) {
		pendingMu.Lock()
		defer pendingMu.Unlock()
		for _, event := range events {
			sequence++
			changes = append(changes, change{sequence, owner, append(json.RawMessage(nil), event...)})
		}
		if len(changes) > 128 {
			changes = append([]change(nil), changes[len(changes)-128:]...)
		}
	}
	queueEvents := func(events []json.RawMessage) error {
		publish(0, events)
		for _, event := range events {
			var info struct{ Type string }
			if json.Unmarshal(event, &info) == nil && info.Type == "navigate" {
				pendingMu.Lock()
				pending = append(json.RawMessage(nil), event...)
				pendingMu.Unlock()
				if err := launchDesktop(desktop, pipe, data, ""); err != nil {
					return err
				}
			}
		}
		return nil
	}
	connect, connectErr := ankiengine.NewConnect(engine, filepath.Join(data, "learning"), queueEvents)
	transfers := &learning.RPCTransfers{}
	var handler func(host.Request) host.Response
	handler = func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "learning", Message: err.Error()}}
		}
		var input struct {
			ID     string          `json:"id,omitempty"`
			Action string          `json:"action,omitempty"`
			Method string          `json:"method,omitempty"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if request.Method == "call" || request.Method == "ready" {
			if err := configjson.DecodeObject(request.Params, &input); err != nil {
				return fail(err)
			}
		} else if request.Method == "upload" || request.Method == "read" || request.Method == "release" {
			if err := json.Unmarshal(request.Params, &input); err != nil {
				return fail(err)
			}
		}
		if input.ID != "" {
			if err := results.authorize(input.ID, request.PeerPID); err != nil {
				return fail(err)
			}
		} else {
			peer, err := processidentity.Open(request.PeerPID, desktop)
			if err != nil {
				return fail(err)
			}
			peer.Close()
		}
		if request.Method == "upload" || request.Method == "read" || request.Method == "release" {
			var part learning.RPCPart
			if err := configjson.DecodeObject(request.Params, &part); err != nil {
				return fail(err)
			}
			switch request.Method {
			case "upload":
				data, token, err := transfers.Append(request.PeerPID, part)
				if err != nil {
					return fail(err)
				}
				if data != nil {
					request.Method = "call"
					request.Params = data
					return handler(request)
				}
				body, _ := json.Marshal(map[string]string{"token": token})
				return host.Response{ID: request.ID, Result: body}
			case "read":
				value, err := transfers.Read(request.PeerPID, part)
				if err != nil {
					return fail(err)
				}
				body, _ := json.Marshal(value)
				return host.Response{ID: request.ID, Result: body}
			case "release":
				transfers.Release(request.PeerPID, part.Token)
				return host.Response{ID: request.ID, Result: json.RawMessage(`{"released":true}`)}
			}
		}
		if request.Method == "ready" {
			if err := engine.Available(); err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: json.RawMessage(`{"ready":true}`)}
		}
		if request.Method == "cancel" {
			cancelled, err := engine.CancelCurrent(request.PeerPID)
			if err != nil {
				return fail(err)
			}
			body, _ := json.Marshal(map[string]bool{"requested": cancelled})
			return host.Response{ID: request.ID, Result: body}
		}
		if request.Method == "connect.get" || request.Method == "connect.save" {
			if connectErr != nil {
				return fail(connectErr)
			}
			state := connect.State()
			if request.Method == "connect.save" {
				var settings ankiengine.ConnectSettings
				if err := configjson.DecodeObject(request.Params, &settings); err != nil {
					return fail(err)
				}
				var err error
				state, err = connect.Configure(settings)
				if err != nil {
					return fail(err)
				}
			}
			body, err := json.Marshal(state)
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: body}
		}
		if request.Method == "pending" {
			pendingMu.Lock()
			value := pending
			pending = nil
			pendingMu.Unlock()
			body, _ := json.Marshal(map[string]any{"requested": len(value) > 0, "options": value})
			return host.Response{ID: request.ID, Result: body}
		}
		if request.Method == "events" {
			engine.Touch()
			var input struct {
				After uint64 `json:"after"`
			}
			if err := configjson.DecodeObject(request.Params, &input); err != nil {
				return fail(err)
			}
			pendingMu.Lock()
			var events []change
			for _, event := range changes {
				if event.Sequence > input.After {
					events = append(events, event)
				}
			}
			body, err := json.Marshal(map[string]any{"sequence": sequence, "events": events})
			pendingMu.Unlock()
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: body}
		}
		if request.Method != "call" {
			return fail(errors.New("未知英语学习操作"))
		}
		method := input.Method
		var params any = input.Params
		if input.Action != "" {
			method = "legacy"
			params = map[string]any{"action": input.Action, "params": input.Params}
		}
		if method == "" {
			return fail(errors.New("缺少英语学习操作"))
		}
		callCtx, cancel := context.WithTimeout(ctx, 150*time.Second)
		defer cancel()
		var value json.RawMessage
		events, err := engine.CallOwned(callCtx, request.PeerPID, method, params, &value)
		publish(request.PeerPID, events)
		if err != nil {
			return fail(err)
		}
		for _, event := range events {
			// Settings owns its native view; only result windows request an
			// external navigation. Do not replay settings actions on focus.
			if input.ID == "" {
				continue
			}
			var info struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(event, &info) != nil || info.Type != "navigate" {
				continue
			}
			pendingMu.Lock()
			pending = append(json.RawMessage(nil), event...)
			pendingMu.Unlock()
			if input.ID != "" {
				if err := launchDesktop(desktop, pipe, data, ""); err != nil {
					return fail(err)
				}
			}
		}
		body, err := transfers.Offer(request.PeerPID, value)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: request.ID, Result: body}
	}
	service.RegisterModule("learning", func() host.Module { return host.Module{ID: "learning", Name: "英语学习", State: "ready"} }, handler)
	return func() error {
		transfers.Close()
		var err error
		if connect != nil {
			err = connect.Close()
		}
		return errors.Join(err, engine.Close())
	}
}
