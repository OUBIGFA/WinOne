//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"winone/internal/ai"
)

// Use WinOne's saved image route and long-lived Magpie media service. This is
// private Worker IPC: no provider key, arbitrary URL or renderer-chosen path.
func (b *chatOneBot) portraitCatalog() (any, error) {
	config, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	result := []map[string]string{}
	if config.MagpieSettings == nil || config.MagpieSettings.ImageGen == "" {
		return result, nil
	}
	id := config.MagpieSettings.ImageGen
	provider, model, ok := strings.Cut(id, "/")
	if ok && config.ValidateSelection(ai.Selection{Provider: provider, Model: model}) == nil {
		result = append(result, map[string]string{"id": id, "provider_id": provider})
	}
	return result, nil
}

type oneBotMediaResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *oneBotMediaResponse) Header() http.Header { return w.header }
func (w *oneBotMediaResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *oneBotMediaResponse) Write(data []byte) (int, error) {
	if w.body.Len()+len(data) > 48<<20 {
		return 0, errors.New("头像生成结果过大")
	}
	w.WriteHeader(200)
	return w.body.Write(data)
}
func (b *chatOneBot) portraitRequest(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct{ Model, Prompt, Job string }
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	tool := "generate_image"
	arguments := map[string]string{"prompt": input.Prompt}
	if input.Job != "" {
		if !strings.HasPrefix(input.Job, "img_") || len(input.Job) > 128 {
			return nil, errors.New("图像任务标识无效")
		}
		tool = "generation_result"
		arguments = map[string]string{"id": input.Job}
	} else {
		config, err := b.control.aiConfiguration()
		if err != nil {
			return nil, err
		}
		if config.MagpieSettings == nil || input.Model == "" || config.MagpieSettings.ImageGen != input.Model || strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 16000 {
			return nil, errors.New("头像模型已变化，请重新生成")
		}
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1/mcp/magpie/image", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	result := &oneBotMediaResponse{header: make(http.Header)}
	b.control.ai.serveChat(result, request)
	if result.status != http.StatusOK {
		return nil, errors.New("头像生成服务未完成请求")
	}
	if !json.Valid(result.body.Bytes()) {
		return nil, errors.New("头像生成服务返回无效结果")
	}
	return json.RawMessage(result.body.Bytes()), nil
}
func (b *chatOneBot) creationResources() (any, error) {
	if b.control.store == nil {
		return nil, errors.New("能力目录尚未就绪")
	}
	prefs := b.control.store.Preferences()
	return map[string]any{"cwd": filepath.Join(b.control.directory, "onebot"), "agentDir": filepath.Join(b.control.directory, "chat", "agents", "onebot-catalog"), "skillDirectories": prefs.SkillDirectories, "pluginDirectories": prefs.PluginDirectories, "mcpServers": prefs.MCPServers}, nil
}
func openWechatAuthorization(raw json.RawMessage) (any, error) {
	var input struct{ URL string }
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	address, err := url.Parse(input.URL)
	if err != nil || address.Scheme != "https" || address.User != nil || !(address.Hostname() == "weixin.qq.com" || strings.HasSuffix(address.Hostname(), ".weixin.qq.com")) {
		return nil, errors.New("微信授权地址无效")
	}
	verb, _ := windows.UTF16PtrFromString("open")
	target, _ := windows.UTF16PtrFromString(input.URL)
	if err := windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
