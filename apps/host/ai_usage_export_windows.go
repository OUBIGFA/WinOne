//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/processidentity"
	"winone/internal/settings"
)

// Only the same installation's native settings window may supply the path it
// obtained from SaveFileDialog. A normal pipe client cannot write arbitrary files.
func (s *aiService) handleUsageExport(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai_export", Message: err.Error()}}
	}
	if !strings.EqualFold(filepath.Base(s.desktopExecutable), "WinOne.exe") {
		return fail(errors.New("导出窗口身份无效"))
	}
	owner, err := processidentity.Open(r.PeerPID, s.desktopExecutable)
	if err != nil {
		return fail(errors.New("请从本安装的 WinOne 窗口导出用量"))
	}
	defer owner.Close()
	var params struct {
		Query ai.UsageQuery `json:"query"`
		Path  string        `json:"path"`
	}
	if err := configjson.DecodeObject(r.Params, &params); err != nil {
		return fail(err)
	}
	runtime, err := s.magpieTranslator()
	if err != nil {
		return fail(err)
	}
	value, err := writeUsageExport(s.ctx, params.Path, params.Query, runtime.Usage, func() error {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		exited, err := owner.Exited()
		if err != nil {
			return err
		}
		if exited {
			return errors.New("导出窗口已关闭")
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fail(err)
	}
	return host.Response{ID: r.ID, Result: raw}
}

type usageExportSaved struct {
	State string `json:"state"`
	Path  string `json:"path"`
	Rows  int    `json:"rows"`
}

func writeUsageExport(ctx context.Context, path string, query ai.UsageQuery, read func(context.Context, ai.UsageQuery) (json.RawMessage, error), alive func() error) (usageExportSaved, error) {
	var empty usageExportSaved
	if !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') || !strings.EqualFold(filepath.Ext(path), ".csv") {
		return empty, errors.New("请选择 CSV 文件保存位置")
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || !parent.IsDir() {
		return empty, errors.New("CSV 保存目录不存在")
	}
	query.Export = true
	query.Offset = 0
	raw, err := read(ctx, query)
	if err != nil {
		return empty, err
	}
	var result struct {
		CSV   *string `json:"csv"`
		Total int     `json:"total"`
	}
	if json.Unmarshal(raw, &result) != nil || result.CSV == nil || *result.CSV == "" || len(*result.CSV) > 64<<20 || result.Total < 0 {
		return empty, errors.New("CSV 导出结果无效")
	}
	if err := alive(); err != nil {
		return empty, err
	}
	// The full payload stays between the AI worker and Host. Only this compact
	// receipt crosses the 1 MiB desktop frame, after a synced atomic replacement.
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(*result.CSV)...)
	if err := settings.WriteAtomic(path, data); err != nil {
		return empty, errors.New("CSV 保存失败：" + err.Error())
	}
	return usageExportSaved{State: "saved", Path: path, Rows: result.Total}, nil
}
