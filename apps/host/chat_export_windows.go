//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"winone/internal/chat"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/processidentity"
)

// The parent Chat handler has already verified the native desktop peer and
// acquired the maintenance guard. Hold a dedicated identity handle until save.
func (c *chatControl) exportArchive(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "chat_export", Message: err.Error()}}
	}
	var options chat.ArchiveOptions
	if err := configjson.DecodeObject(r.Params, &options); err != nil {
		return fail(err)
	}
	owner, err := processidentity.Open(r.PeerPID, c.desktop)
	if err != nil {
		return fail(errors.New("请从本安装的 WinOne 窗口导出任务"))
	}
	defer owner.Close()
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Minute)
	defer cancel()
	go func() { _ = owner.Wait(ctx); cancel() }()
	result, err := c.engine.ExportArchive(ctx, options)
	if err != nil {
		return fail(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return fail(err)
	}
	return host.Response{ID: r.ID, Result: raw}
}
