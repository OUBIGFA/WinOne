//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"
	"winone/internal/chat"
	"winone/internal/configjson"
)

func (c *chatControl) fileSearchAction(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	var input struct {
		ID            string `json:"id"`
		Path          string `json:"path"`
		Query         string `json:"query"`
		CaseSensitive bool   `json:"caseSensitive"`
		MaxMatches    int    `json:"maxMatches"`
		SearchID      string `json:"searchId"`
	}
	if err := configjson.DecodeObject(raw, &input); err != nil {
		return nil, err
	}
	if _, err := c.store.Conversation(input.ID); err != nil {
		return nil, err
	}
	key := input.ID + ":" + input.SearchID
	if method == "files.cancel" {
		if cancel, ok := c.fileSearches.LoadAndDelete(key); ok {
			cancel.(context.CancelFunc)()
		}
		return nil, nil
	}
	directory, err := c.store.Files(input.ID, input.Path)
	if err != nil {
		return nil, err
	}
	server, err := c.previewServer()
	if err != nil {
		return nil, err
	}
	executable := filepath.Join(filepath.Dir(c.desktop), "shared", "chat", "rg.exe")
	read := func(readCtx context.Context) ([]byte, error) {
		readCtx, cancel := context.WithTimeout(readCtx, 30*time.Second)
		defer cancel()
		c.fileSearches.Store(key, context.CancelFunc(cancel))
		defer c.fileSearches.Delete(key)
		var value any
		var err error
		switch method {
		case "files.list":
			value, err = c.store.Files(input.ID, input.Path)
		case "files.index":
			value, err = chat.ReadFileIndex(readCtx, executable, directory.Directory)
		case "files.search":
			value, err = chat.SearchWorkspaceFiles(readCtx, executable, directory.Directory, input.Query, input.CaseSensitive, input.MaxMatches)
		default:
			return nil, errors.New("未知文件浏览操作")
		}
		if err != nil {
			return nil, err
		}
		return json.Marshal(value)
	}
	address, err := server.OpenContent(input.ID, "files.json", "application/json; charset=utf-8", read)
	return map[string]string{"url": address, "directory": directory.Directory}, err
}
