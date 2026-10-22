//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"winone/internal/chat"
	"winone/internal/configjson"
)

func (c *chatControl) mediaAction(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	var input struct {
		ID      string            `json:"id"`
		URL     string            `json:"url"`
		Name    string            `json:"name"`
		Files   []chat.Attachment `json:"files"`
		Replace string            `json:"replace"`
	}
	if err := configjson.DecodeObject(raw, &input); err != nil {
		return nil, err
	}
	if _, err := c.store.Conversation(input.ID); err != nil {
		return nil, err
	}
	server, err := c.previewServer()
	if err != nil {
		return nil, err
	}
	finish := func(file chat.Attachment) (any, error) {
		preview, err := server.Open(input.ID, file.Path)
		if err != nil {
			return nil, err
		}
		return map[string]any{"attachment": file, "preview": preview}, nil
	}
	switch method {
	case "media.attach":
		return nil, c.store.AppendMediaDraft(input.ID, input.Files, input.Replace)
	case "media.upload", "attachments.upload":
		address, err := server.OpenUpload(input.ID, func(ctx context.Context, data []byte) (any, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if method == "attachments.upload" {
				file, err := c.store.ImportAttachment(input.ID, input.Name, data)
				return map[string]any{"attachment": file}, err
			}
			file, err := c.store.ImportMedia(input.ID, input.Name, data)
			if err != nil {
				return nil, err
			}
			return finish(file)
		})
		return map[string]string{"url": address}, err
	case "media.source":
		file, err := c.store.ImportRemoteMedia(ctx, input.ID, input.URL)
		if err != nil {
			return nil, err
		}
		return finish(file)
	default:
		return nil, errors.New("未知图片操作")
	}
}
