//go:build windows

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

// GatewayRequest keeps the raw protocol while reusing owned processes and the
// same credentials as in-product requests. Once output starts, never retry it.
func (t Translator) GatewayRequest(ctx context.Context, p Preferences, key, path string, body json.RawMessage, headers http.Header, w http.ResponseWriter) (partial bool, err error) {
	return t.protocolRequest(ctx, p, key, path, body, headers, w, "gateway")
}

// ChatRequest preserves Pi tool payloads through Magpie while attributing usage
// to WinOne. The caller, never the incoming HTTP body, chooses the usage source.
func (t Translator) ChatRequest(ctx context.Context, p Preferences, key, path string, body json.RawMessage, headers http.Header, w http.ResponseWriter) (partial bool, err error) {
	return t.protocolRequest(ctx, p, key, path, body, headers, w, "winone")
}

func (t Translator) protocolRequest(ctx context.Context, p Preferences, key, path string, body json.RawMessage, headers http.Header, w http.ResponseWriter, source string) (partial bool, err error) {
	release, err := t.acquireRequest(ctx, p, key)
	if err != nil {
		return false, err
	}
	defer release()
	process, err := t.start(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		if process != nil {
			process.closeJob()
			_ = process.finish()
		}
	}()
	input := map[string]any{"operation": "gateway", "providerId": p.ProviderID, "protocol": p.Protocol, "baseUrl": p.BaseURL, "key": key, "model": p.Model, "agent": p.Agent, "agentBackend": p.AgentBackend, "preset": p.Preset, "presetRegion": p.PresetRegion, "gatewayPath": path, "gatewayBody": body, "gatewayHeaders": headers}
	if p.GlobalGateway {
		input["operation"] = "magpie.gateway"
	}
	input["usageSource"] = source
	input["headers"] = p.Headers
	input["modelMetadata"] = p.ModelMetadata
	input["magpieSettings"] = p.MagpieSettings
	auxiliary, stopAuxiliary, err := t.auxiliaryBridge(ctx, p.Auxiliaries, p.MagpieSettings)
	if err != nil {
		return false, err
	}
	defer stopAuxiliary()
	input["auxiliaryProviders"] = auxiliary
	if err = json.NewEncoder(process.input).Encode(input); err != nil {
		return false, err
	}
	process.input.Close()
	scan := bufio.NewScanner(io.LimitReader(process.output, 96<<20))
	scan.Buffer(make([]byte, 4096), 128<<10)
	status := 0
	done := false
	total := 0
	for scan.Scan() {
		var event struct {
			Type    string      `json:"type"`
			Status  int         `json:"status"`
			Headers http.Header `json:"headers"`
			Data    []byte      `json:"data"`
		}
		if done || json.Unmarshal(scan.Bytes(), &event) != nil {
			return partial, errors.New("本地网关响应无效")
		}
		switch event.Type {
		case "headers":
			if status != 0 || event.Status < 200 || event.Status > 599 {
				return partial, errors.New("本地网关响应状态无效")
			}
			status = event.Status
			if status >= 400 {
				return false, &AttemptFailure{Status: status, RetryAfter: gatewayRetryAfter(event.Headers.Get("Retry-After"), time.Now())}
			}
			for _, name := range []string{"Content-Type", "Cache-Control", "X-Request-Id"} {
				if value := event.Headers.Get(name); value != "" {
					w.Header().Set(name, value)
				}
			}
			w.WriteHeader(status)
			partial = true
		case "data":
			total += len(event.Data)
			if status == 0 || total > 64<<20 {
				return partial, errors.New("本地网关响应过大或无效")
			}
			if _, err = w.Write(event.Data); err != nil {
				return partial, err
			}
			if flush, ok := w.(http.Flusher); ok {
				flush.Flush()
			}
		case "done":
			done = true
		default:
			return partial, errors.New("AI 服务未能完成网关请求")
		}
	}
	if scan.Err() != nil || !done || status == 0 {
		return partial, errors.New("本地网关输出中断")
	}
	err = process.finish()
	process = nil
	return partial, err
}

// HTTP Retry-After permits either seconds or a date. Keep the existing key
// cooldown policy for both forms instead of retrying a dated limit early.
func gatewayRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(max(0, min(seconds, 86400))) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, min(date.Sub(now), 24*time.Hour))
	}
	return 0
}
