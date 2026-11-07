//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Clipboard bytes stay in an owner-private, short-lived file. Only the native
// process which has attached to Chat can return the actual OS acknowledgement;
// putting a message in the event inbox alone is not a successful copy.
func (g *chatGhosts) clipboard(ctx context.Context, kind string, params json.RawMessage) (any, error) {
	var input struct{ Path string }
	if json.Unmarshal(params, &input) != nil || !filepath.IsAbs(input.Path) {
		return nil, errors.New("剪贴板文件路径无效")
	}
	source, err := filepath.EvalSymlinks(input.Path)
	if err != nil {
		return nil, err
	}
	folder := "cindy-media"
	if kind == "clipboard-png" {
		folder = "clipboard-staging"
	}
	root, err := filepath.EvalSymlinks(filepath.Join(g.control.directory, "chat", "ghosts", folder))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, source)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("剪贴板文件不属于当前插件宿主")
	}
	stat, err := os.Stat(source)
	if err != nil || !stat.Mode().IsRegular() {
		return nil, errors.New("剪贴板源文件不可读")
	}
	g.control.mu.Lock()
	available := len(g.control.owners) > 0
	g.control.mu.Unlock()
	if !available {
		return nil, errors.New("没有可挂靠的宿主窗口")
	}
	request, _ := json.Marshal(map[string]string{"kind": kind, "path": source})
	answer, err := g.ask(ctx, request)
	if err != nil {
		return nil, err
	}
	var receipt struct {
		Copied  bool
		Message string
	}
	if json.Unmarshal(answer, &receipt) != nil || !receipt.Copied {
		if receipt.Message != "" {
			return nil, errors.New(receipt.Message)
		}
		return nil, errors.New("写入剪贴板未获确认")
	}
	return map[string]bool{"copied": true}, nil
}
