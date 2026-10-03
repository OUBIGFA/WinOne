//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"winone/internal/host"
	"winone/internal/modules"
)

type moduleEndpoint interface {
	Status() host.Module
	Handle(host.Request) host.Response
	Enabled() bool
}

func startWinTray(ctx context.Context, worker *modules.Lazy) error {
	worker.Resume()
	for {
		response := worker.Handle(host.Request{Version: 1, ID: "module-enable", Method: "snapshot"})
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
		var status struct {
			State string `json:"initializationState"`
			Error string `json:"initializationError"`
		}
		if err := json.Unmarshal(response.Result, &status); err != nil {
			return err
		}
		switch status.State {
		case "ready":
			return nil
		case "failed":
			if status.Error != "" {
				return errors.New(status.Error)
			}
			return errors.New("启动与托盘初始化失败")
		case "starting":
		default:
			return errors.New("组件缺少初始化状态，请更新完整组件包")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// stopWinTray uses its recovery-only startup path if no normal worker is running.
// The product's login trigger stays available to other enabled modules.
func stopWinTray(ctx context.Context, worker *modules.Lazy) error {
	return releaseWinTray(ctx, worker, "deactivate")
}

// Keep the Host adapter while sharing the protocol with upgrade preparation.
func releaseWinTray(ctx context.Context, worker interface {
	Resume()
	Handle(host.Request) host.Response
	Close() error
}, method string) error {
	if method != "deactivate" && method != "uninstall.prepare" {
		return errors.New("unsupported release operation")
	}
	return modules.ReleaseWinTray(ctx, worker, method == "uninstall.prepare")
}
