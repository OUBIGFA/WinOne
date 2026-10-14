//go:build windows

package main

import (
	"encoding/json"
	"winone/internal/configjson"
	"winone/internal/diagnostics"
	"winone/internal/host"
)

// Diagnostics are a Host service, never a module call: disabled or failed
// engines must not be activated just because their logs are being inspected.
func registerDiagnostics(service *host.Service, directory string) {
	service.RegisterModule("diagnostics", func() host.Module {
		return host.Module{ID: "diagnostics", State: "ready"}
	}, func(request host.Request) host.Response {
		fail := func(code, message string) host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: code, Message: message}}
		}
		if request.Method != "read" && request.Method != "folder" {
			return fail("unknown_method", "不支持的日志操作")
		}
		var params struct {
			Module string `json:"module"`
		}
		if err := configjson.DecodeObject(request.Params, &params); err != nil {
			return fail("invalid", err.Error())
		}
		var result any
		var err error
		if request.Method == "read" {
			result, err = diagnostics.Read(directory, params.Module)
		} else {
			var path string
			path, err = diagnostics.Folder(directory, params.Module)
			result = map[string]string{"path": path}
		}
		if err != nil {
			return fail("diagnostics", err.Error())
		}
		data, err := json.Marshal(result)
		if err != nil {
			return fail("diagnostics", err.Error())
		}
		return host.Response{ID: request.ID, Result: data}
	})
}
