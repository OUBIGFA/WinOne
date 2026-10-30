//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"winone/internal/chat"
)

// The desktop selects the upstream opaque project ID. Only this Host supplies
// registered paths/names to the Worker, so a crafted catalog cannot link an
// arbitrary renderer-supplied directory. Existing unavailable links remain
// readable/removable through the original workbench service.
func (b *chatOneBot) projectCall(ctx context.Context, runner *chat.Runner, method string, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		BotID  string `json:"botId"`
		ID     string `json:"id"`
		Linked *bool  `json:"linked"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	catalog, err := b.projectCatalog()
	if err != nil {
		return nil, err
	}
	params := map[string]any{"botId": input.BotID, "catalog": catalog}
	workerMethod := "projects.query"
	if method == "projects.set" {
		if input.Linked == nil {
			return nil, errors.New("缺少项目关联状态")
		}
		workerMethod = "projects.link"
		params["id"], params["linked"] = input.ID, *input.Linked
	}
	return runner.Call(ctx, workerMethod, params)
}

// UI and Pi use the same registered directory facts. The Pi bridge injects
// this snapshot alongside the session/turn grant, outside model arguments.
func (b *chatOneBot) projectCatalog() ([]map[string]any, error) {
	if b.control.store == nil {
		return nil, errors.New("WinOne 项目目录尚未就绪")
	}
	catalog := make([]map[string]any, 0)
	for _, project := range b.control.store.Snapshot().Projects {
		info, err := os.Stat(project.Path)
		catalog = append(catalog, map[string]any{"path": project.Path, "name": project.Name, "exists": err == nil && info.IsDir()})
	}
	return catalog, nil
}
