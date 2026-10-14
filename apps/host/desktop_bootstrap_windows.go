//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"winone/internal/adminstartup"
	"winone/internal/capturehandoff"
	"winone/internal/configjson"
	"winone/internal/logintask"
	"winone/internal/maintenance"
)

// The Rust entrypoint delegates only the existing Windows authorization
// operations. No caller-provided executable, script or installation is run.
func desktopBootstrap(args []string, executable string, input io.Reader, output io.Writer) (bool, error) {
	if len(args) == 0 || (args[0] != "--desktop-bootstrap" && args[0] != "--desktop-helper") {
		return false, nil
	}
	desktop := filepath.Join(filepath.Dir(executable), "WinOne.exe")
	if args[0] == "--desktop-helper" {
		if handled, err := adminstartup.RunAuthorizationHelper(args[1:], desktop); handled {
			return true, err
		}
		if handled, err := adminstartup.RunAuthorizedStartHelper(args[1:], desktop); handled {
			return true, err
		}
		return true, errors.New("不支持的桌面辅助操作")
	}
	if len(args) != 1 {
		return true, errors.New("桌面启动参数无效")
	}
	data, err := io.ReadAll(io.LimitReader(input, 16<<10+1))
	if err != nil {
		return true, err
	}
	if len(data) > 16<<10 {
		return true, errors.New("桌面启动信息过长")
	}
	var request struct {
		Pipe      string `json:"pipe"`
		Directory string `json:"dataDirectory"`
		Standard  bool   `json:"standardSession"`
		Draft     string `json:"draftId"`
	}
	if err := configjson.DecodeObject(data, &request); err != nil {
		return true, err
	}
	if request.Draft != "" {
		if err := capturehandoff.ValidateDraftID(request.Draft); err != nil {
			return true, err
		}
	}
	if !filepath.IsAbs(request.Directory) || !strings.HasPrefix(request.Pipe, `\\.\pipe\WinOne-`) || strings.ContainsRune(request.Pipe, 0) {
		return true, errors.New("桌面启动路径无效")
	}
	lease, err := maintenance.AcquireRuntimeForInstallation(request.Directory, filepath.Dir(executable))
	if err != nil {
		return true, err
	}
	defer lease.Close()
	redirected, startErr := adminstartup.Try(context.Background(), request.Directory, request.Pipe, desktop, logintask.InteractiveLaunch, request.Standard, request.Draft)
	warning := ""
	if startErr != nil {
		warning = startErr.Error()
	}
	return true, json.NewEncoder(output).Encode(struct {
		Version    int    `json:"version"`
		Redirected bool   `json:"redirected"`
		Warning    string `json:"warning"`
		Standard   bool   `json:"standardSession"`
	}{1, redirected, warning, request.Standard || startErr != nil})
}
