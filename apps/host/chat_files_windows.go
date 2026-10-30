//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"strings"
	"winone/internal/chat"
)

func (c *chatControl) previewServer() (*chat.PreviewServer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.previews == nil {
		ctx := c.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		server, err := chat.NewPreviewServer(ctx, c.store)
		if err != nil {
			return nil, err
		}
		c.previews = server
	}
	return c.previews, nil
}

func (c *chatControl) fileAction(method string, raw json.RawMessage) (any, error) {
	var input struct {
		ID       string   `json:"id"`
		Path     string   `json:"path"`
		Paths    []string `json:"paths"`
		Revision string   `json:"revision"`
		Text     string   `json:"text"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	switch method {
	case "file.stat":
		paths := input.Paths
		if len(paths) == 0 && input.Path != "" {
			paths = []string{input.Path}
		}
		// Refresh exact successful-tool grants once for the whole batch, not
		// once per missing historical file. Missing/deleted files are normal.
		results, err := c.store.StatFiles(input.ID, paths)
		if err != nil {
			return nil, err
		}
		needsGrants := false
		for _, item := range results {
			needsGrants = needsGrants || item.Error != ""
		}
		if needsGrants {
			if _, _, err := c.store.SessionReview(input.ID); err != nil {
				return nil, err
			}
			results, err = c.store.StatFiles(input.ID, paths)
		}
		return map[string]any{"files": results}, err
	case "file.preview":
		if _, err := c.store.ResolvePreviewFile(input.ID, input.Path); err != nil {
			// An explicitly approved file tool can produce a file outside CWD.
			// Only exact successful records on this branch may grant its preview.
			if _, _, recordErr := c.store.SessionReview(input.ID); recordErr != nil {
				return nil, recordErr
			}
		}
		server, err := c.previewServer()
		if err != nil {
			return nil, err
		}
		return server.Open(input.ID, input.Path)
	case "file.save":
		return c.store.SavePreviewFile(input.ID, input.Path, input.Revision, input.Text)
	case "file.create":
		return c.store.CreatePreviewFile(input.ID, input.Path)
	case "file.open", "file.reveal", "file.resolve":
		path, err := c.store.ResolveFileAction(input.ID, input.Path)
		if err != nil {
			// Successful file tools can grant an exact outside-workspace file.
			// Reuse that recorded grant, never a renderer-supplied directory.
			if _, _, reviewErr := c.store.SessionReview(input.ID); reviewErr != nil {
				return nil, reviewErr
			}
			path, err = c.store.ResolveFileAction(input.ID, input.Path)
		}
		if err != nil {
			return nil, err
		}
		if method == "file.resolve" {
			return map[string]string{"path": path}, nil
		}
		return nil, openVerifiedChatPath(path, method == "file.reveal")
	}
	return nil, errors.New("未知文件操作")
}

// Only explicit desktop actions reach the shell after the owning store resolves
// their path. Group attachments share these same native actions with Chat.
func openVerifiedChatPath(path string, reveal bool) error {
	verb, _ := windows.UTF16PtrFromString("open")
	if reveal {
		exe, _ := windows.UTF16PtrFromString("explorer.exe")
		args, err := windows.UTF16PtrFromString("/select,\"" + strings.ReplaceAll(path, "\"", "") + "\"")
		if err != nil {
			return err
		}
		return windows.ShellExecute(0, verb, exe, args, nil, windows.SW_SHOWNORMAL)
	}
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}
