//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"winone/internal/ai"
)

// Media calls reuse the actual long-lived Magpie gateway and provider catalog.
// The private bridge transports only a verified local result path, not tens of
// megabytes of base64 through Runner's bounded JSON-lines protocol.
func (g *chatGhosts) mediaRouting(ctx context.Context) (*ai.RoutingProcess, func(), error) {
	s := g.control.ai
	name := ghostRequestID()
	s.routing.mu.Lock()
	if s.routing.viewers == nil {
		s.routing.viewers = map[string]bool{}
	}
	s.routing.viewers[name] = true
	s.routing.mu.Unlock()
	process, err := s.startRouting()
	release := func() { s.releaseRoutingViewer(name) }
	if err != nil {
		release()
		return nil, nil, err
	}
	return process, release, nil
}
func (g *chatGhosts) mediaCatalog(ctx context.Context) (any, error) {
	configuration, err := g.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	process, release, err := g.mediaRouting(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	raw, err := process.MediaModels(ctx, func(selected ai.Selection) bool { return configuration.ValidateSelection(selected) == nil })
	if err != nil {
		return nil, err
	}
	var listed struct {
		Data []struct{ ID, Name, Kind string }
	}
	if err = json.Unmarshal(raw, &listed); err != nil {
		return nil, err
	}
	defaults := ""
	if configuration.MagpieSettings != nil {
		defaults = configuration.MagpieSettings.ImageGen
	}
	catalogs := map[string][]map[string]any{"images": {}, "videos": {}, "embeds": {}}
	for _, row := range listed.Data {
		provider, model, found := strings.Cut(row.ID, "/")
		if !found {
			continue
		}
		kind := ""
		switch row.Kind {
		case "image":
			kind = "images"
		case "video":
			kind = "videos"
		case "embedding", "embed":
			kind = "embeds"
		}
		if kind == "" && defaults == row.ID {
			kind = "images"
		}
		if row.Kind == "" || row.Kind == "chat" || row.Kind == "embedding" {
			// Embedding endpoints often share the provider's enabled text list
			// without capability metadata. These are candidates, not a promise;
			// the provider's real response remains the authority at execution.
			catalogs["embeds"] = append(catalogs["embeds"], map[string]any{"id": row.ID, "name": row.Name, "providerId": provider, "model": model})
		}
		if kind != "" {
			catalogs[kind] = append(catalogs[kind], map[string]any{"id": row.ID, "name": row.Name, "providerId": provider, "model": model})
		}
	}
	result := map[string]any{}
	for kind, models := range catalogs {
		var chosen any
		if len(models) > 0 {
			selected := models[0]["id"]
			for _, model := range models {
				if model["id"] == defaults {
					selected = model["id"]
					break
				}
			}
			chosen = map[string]any{"default": selected}
		}
		result[kind] = map[string]any{"models": models, "defaults": chosen}
	}
	return result, nil
}
func (g *chatGhosts) saveMediaResult(data []byte, mime string) (any, error) {
	if len(data) == 0 {
		return nil, errors.New("媒体服务没有返回内容")
	}
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif", "video/mp4": ".mp4", "video/webm": ".webm"}[mime]
	if ext == "" {
		return nil, errors.New("媒体服务返回了不支持的文件类型")
	}
	dir := filepath.Join(g.control.directory, "chat", "ghosts", "generated-media")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file := filepath.Join(dir, ghostRequestID()+ext)
	if err := os.WriteFile(file, data, 0600); err != nil {
		return nil, err
	}
	return map[string]string{"path": file, "mimeType": mime}, nil
}
func (g *chatGhosts) gatewayMedia(ctx context.Context, process *ai.RoutingProcess, method, address string, input any) ([]byte, http.Header, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1"+address, strings.NewReader(string(body)))
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	output := &ghostResponse{header: http.Header{}, status: 200}
	configuration, err := g.control.aiConfiguration()
	if err != nil {
		return nil, nil, err
	}
	models := []string{}
	for _, provider := range configuration.Providers {
		for _, model := range provider.Models {
			models = append(models, provider.ID+"/"+model)
		}
	}
	process.ServeMedia(output, request, models)
	if output.status/100 != 2 {
		return nil, nil, errors.New("媒体请求失败：HTTP " + http.StatusText(output.status) + " " + string(output.body.Bytes()[:min(output.body.Len(), 400)]))
	}
	return output.body.Bytes(), output.header, nil
}
func (g *chatGhosts) pluginMedia(ctx context.Context, kind string, raw json.RawMessage) (any, error) {
	var input struct {
		Prompt, Model, ProviderID, AspectRatio, Ratio, Resolution, RefMode string
		FPS                                                                int
		ImagePaths                                                         []string
		Duration                                                           int
		Audio                                                              *bool
		ImagePath                                                          string
		Texts                                                              []string
		Dimensions                                                         int
		InputType                                                          string
		TimeoutMs                                                          int64
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	id := input.Model
	if input.ProviderID != "" && !strings.HasPrefix(id, input.ProviderID+"/") {
		id = input.ProviderID + "/" + id
	}
	provider, model, found := strings.Cut(id, "/")
	if !found {
		return nil, errors.New("请选择具有供应商来源的媒体模型")
	}
	configuration, err := g.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	if err = configuration.ValidateSelection(ai.Selection{Provider: provider, Model: model}); err != nil {
		return nil, err
	}
	timeout := 30 * time.Minute
	if kind == "embed-text" {
		timeout = time.Duration(input.TimeoutMs) * time.Millisecond
		if timeout <= 0 || timeout > 2*time.Minute {
			timeout = 90 * time.Second
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	process, release, err := g.mediaRouting(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	references := []map[string]string{}
	for _, file := range input.ImagePaths {
		ref, err := readOneBotImage(file, 50<<20)
		if err != nil {
			return nil, err
		}
		references = append(references, map[string]string{"image_url": "data:" + ref["mimeType"] + ";base64," + ref["base64"]})
	}
	if kind == "embed-text" {
		request := map[string]any{"model": id, "input": input.Texts}
		if input.InputType != "" {
			request["input_type"] = input.InputType
		}
		if input.Dimensions > 0 {
			request["dimensions"] = input.Dimensions
		}
		raw, _, err := g.gatewayMedia(ctx, process, "POST", "/v1/embeddings", request)
		if err != nil {
			return nil, err
		}
		var response struct {
			Data []struct {
				Index     int
				Embedding []float64
			}
			Model string
		}
		if err = json.Unmarshal(raw, &response); err != nil {
			return nil, err
		}
		vectors := make([][]float64, len(input.Texts))
		if len(response.Data) != len(vectors) {
			return nil, errors.New("向量服务返回数量与输入不一致")
		}
		for _, item := range response.Data {
			if item.Index < 0 || item.Index >= len(vectors) || vectors[item.Index] != nil || len(item.Embedding) == 0 {
				return nil, errors.New("向量服务返回格式无效")
			}
			for _, value := range item.Embedding {
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, errors.New("向量服务返回非有限值")
				}
			}
			vectors[item.Index] = item.Embedding
		}
		return map[string]any{"embeddings": vectors, "modelUsed": id}, nil
	}
	if kind == "image-generate" {
		request := map[string]any{"model": id, "prompt": input.Prompt, "n": 1, "response_format": "b64_json"}
		if input.AspectRatio != "" {
			request["size"] = input.AspectRatio
		}
		route := "/v1/images/generations"
		if len(references) > 0 {
			route = "/v1/images/edits"
			request["images"] = references
		}
		raw, _, err := g.gatewayMedia(ctx, process, "POST", route, request)
		if err != nil {
			return nil, err
		}
		var response struct {
			Data []struct {
				B64  string `json:"b64_json"`
				URL  string
				MIME string `json:"mime_type"`
			}
		}
		if err = json.Unmarshal(raw, &response); err != nil || len(response.Data) == 0 {
			return nil, errors.New("图片服务没有返回图片")
		}
		image := response.Data[0]
		if image.B64 == "" {
			return nil, errors.New("媒体服务没有返回可验证的图片字节")
		}
		data, err := base64.StdEncoding.DecodeString(image.B64)
		if err != nil {
			return nil, err
		}
		mime := http.DetectContentType(data)
		if !strings.HasPrefix(mime, "image/") {
			return nil, errors.New("图片服务返回的数据不是图片")
		}
		return g.saveMediaResult(data, mime)
	}
	if kind != "video-generate" {
		return nil, errors.New("未知媒体请求")
	}
	request := map[string]any{"model": id, "prompt": input.Prompt}
	if input.Duration > 0 {
		request["seconds"] = input.Duration
	}
	if input.AspectRatio != "" {
		request["size"] = input.AspectRatio
	}
	if input.Ratio != "" {
		request["size"], request["ratio"] = input.Ratio, input.Ratio
	}
	if input.Resolution != "" {
		request["resolution"] = input.Resolution
	}
	if input.FPS > 0 {
		request["fps"] = input.FPS
	}
	if input.Audio != nil {
		request["audio"] = *input.Audio
	}
	if len(references) > 0 {
		if input.RefMode == "first_and_last_frame" {
			request["input_reference"] = references[0]
			if len(references) > 1 {
				request["last_frame"] = references[1]
			}
		} else {
			request["reference_images"] = references
		}
	}
	if input.ImagePath != "" {
		ref, err := readOneBotImage(input.ImagePath, 50<<20)
		if err != nil {
			return nil, err
		}
		request["input_reference"] = "data:" + ref["mimeType"] + ";base64," + ref["base64"]
	}
	raw, _, err = g.gatewayMedia(ctx, process, "POST", "/v1/videos", request)
	if err != nil {
		return nil, err
	}
	var response struct {
		ID, Status string
		Error      any
	}
	if err = json.Unmarshal(raw, &response); err != nil || response.ID == "" || strings.ContainsAny(response.ID, "/\\?\x00") {
		return nil, errors.New("视频服务没有返回有效任务标识")
	}
	for {
		switch response.Status {
		case "completed", "succeeded":
			data, header, err := g.gatewayMedia(ctx, process, "GET", "/v1/videos/"+response.ID+"/content", nil)
			if err != nil {
				return nil, err
			}
			mime := strings.Split(header.Get("Content-Type"), ";")[0]
			if mime == "" {
				mime = http.DetectContentType(data)
			}
			return g.saveMediaResult(data, mime)
		case "failed", "cancelled":
			return nil, errors.New("视频生成失败")
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		raw, _, err = g.gatewayMedia(ctx, process, "GET", "/v1/videos/"+response.ID, nil)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &response); err != nil {
			return nil, err
		}
	}
}
