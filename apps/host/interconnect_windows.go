//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fyne.io/systray"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/interconnect"
	"winone/internal/processidentity"
)

func registerInterconnect(service *host.Service, data, executable, pipe string, language func() string) (func() error, error) {
	manager, err := interconnect.New(filepath.Join(filepath.Dir(executable), "shared", "interconnect", "WinOne.Interconnect.Lan.exe"), filepath.Join(data, "tools", "interconnect", "lan"))
	if err != nil {
		failure := err.Error()
		service.RegisterModule("interconnect", func() host.Module {
			return host.Module{ID: "interconnect", Name: "互联助手", State: "faulted", Detail: failure}
		}, func(request host.Request) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "interconnect_settings", Message: failure}}
		})
		return func() error { return nil }, nil
	}
	desktop := filepath.Join(filepath.Dir(executable), "WinOne.exe")
	var pendingMu sync.Mutex
	pending := interconnectIntent{Paths: []string{}}
	queue := func(paths []string, receive bool) error {
		if len(paths) > 200 {
			return errors.New("每次最多选择 200 项")
		}
		for _, path := range paths {
			if !filepath.IsAbs(path) || len(path) > 32760 || strings.ContainsAny(path, "\x00\r\n") {
				return errors.New("需要有效的本机绝对路径")
			}
		}
		pendingMu.Lock()
		defer pendingMu.Unlock()
		if len(pending.Paths)+len(paths) > 200 {
			return errors.New("请先处理待发送文件")
		}
		pending.Requested = true
		pending.Receive = receive
		pending.Paths = append(pending.Paths, paths...)
		return nil
	}
	entry := &interconnectEntry{open: func(paths []string, receive bool) error {
		if err := queue(paths, receive); err != nil {
			return err
		}
		return launchDesktop(desktop, pipe, data, "")
	}}
	manager.SetNotifier(func(mode, status string) error {
		title, text := "WinOne · 互联助手", "传输已完成，请在互联助手查看结果。"
		if status == "failed" {
			text = "传输失败，请在互联助手查看原因。"
		}
		if language() == "en-US" {
			title = "WinOne · Interconnect"
			text = "Transfer completed. Open Interconnect to view the result."
			if status == "failed" {
				text = "Transfer failed. Open Interconnect to view the error."
			}
		}
		return systray.ShowMessage(title, text)
	})
	service.RegisterModule("interconnect", func() host.Module { return host.Module{ID: "interconnect", Name: "互联助手", State: "ready"} }, func(request host.Request) host.Response {
		fail := func(err error) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "interconnect", Message: err.Error()}}
		}
		// A different same-user process cannot use an elevated Host as a file proxy.
		if request.Method == "open" {
			var intent interconnectIntent
			if err := configjson.DecodeObject(request.Params, &intent); err != nil {
				return fail(err)
			}
			if err := queue(intent.Paths, intent.Receive); err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: json.RawMessage(`{"queued":true}`)}
		}
		peer, err := processidentity.Open(request.PeerPID, desktop)
		if err != nil {
			return fail(err)
		}
		peer.Close()
		var value any
		handled := true
		switch request.Method {
		case "pending":
			pendingMu.Lock()
			value = pending
			pending = interconnectIntent{Paths: []string{}}
			pendingMu.Unlock()
		case "menu.get":
			value, err = interconnect.ExplorerMenu(desktop)
		case "menu.set":
			var enabled struct {
				Enabled bool `json:"enabled"`
			}
			err = configjson.DecodeObject(request.Params, &enabled)
			if err == nil {
				value, err = interconnect.ConfigureExplorerMenu(desktop, enabled.Enabled)
			}
		case "general.get":
			value = manager.General()
		case "general.save":
			var general interconnect.General
			err = configjson.DecodeObject(request.Params, &general)
			if err == nil {
				err = manager.SaveGeneral(general)
				value = manager.General()
			}
		case "entry.get":
			value = entry.state()
		case "entry.show":
			err = entry.show(language() == "en-US")
			value = entry.state()
		case "entry.hide":
			err = entry.close()
			value = entry.state()
		default:
			handled = false
		}
		if handled {
			if err != nil {
				return fail(err)
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return fail(err)
			}
			return host.Response{ID: request.ID, Result: raw}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := manager.Call(ctx, request.Method, request.Params)
		if err != nil {
			return fail(err)
		}
		if len(result) == 0 {
			result = json.RawMessage("null")
		}
		return host.Response{ID: request.ID, Result: result}
	})
	return func() error { return errors.Join(entry.shutdown(), manager.Close()) }, nil
}

type interconnectIntent struct {
	Requested bool     `json:"requested,omitempty"`
	Paths     []string `json:"paths"`
	Receive   bool     `json:"receive,omitempty"`
}
