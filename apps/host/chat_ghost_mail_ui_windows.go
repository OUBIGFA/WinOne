//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Reader bodies follow the original 1 MiB MailMessage contract. The native
// pipe can carry that data, but the Bun worker's JSON-lines protocol cannot
// repeat a whole document plus the SDK. Borrow only owner-private temporary
// files for this one transform; no public path, grant or second queue is made.
func (g *chatGhosts) mailUi(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Op, URL string
		Reading bool
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.Op == "image" {
		raw, _ := json.Marshal(map[string]string{"url": input.URL})
		return g.call(ctx, "media-preview", raw)
	}
	if input.Op == "file" {
		return nil, errors.New("邮件暂存路径只能由本机宿主生成")
	}
	runner, err := g.ensure(ctx)
	if err != nil {
		return nil, err
	}
	if !input.Reading || (input.Op != "document" && input.Op != "model") {
		return runner.Call(ctx, "mail-ui", params)
	}
	root := filepath.Join(g.control.directory, "chat", "ghosts", "native-mail-ui")
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(root, "view-*.json")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer os.Remove(name)
	defer os.Remove(name + ".result")
	_, err = file.Write(params)
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	raw, err := runner.Call(ctx, "mail-ui", map[string]string{"op": "file", "path": name})
	if err != nil {
		return nil, err
	}
	var result struct{ Path string }
	if json.Unmarshal(raw, &result) != nil || result.Path != name+".result" {
		return nil, errors.New("邮件呈现结果不属于当前请求")
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, errors.New("邮件呈现结果无效")
	}
	return json.RawMessage(data), nil
}
