//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// PrivacyStatus is upstream pii.Status plus its independent local on/off
// preference. It never carries request text, credentials or model contents.
type PrivacyStatus struct {
	On          bool   `json:"on"`
	Supported   bool   `json:"supported"`
	Installed   bool   `json:"installed"`
	Size        int64  `json:"size"`
	Downloading bool   `json:"downloading,omitempty"`
	Done        int64  `json:"done,omitempty"`
	Total       int64  `json:"total,omitempty"`
	Error       string `json:"error,omitempty"`
	Running     bool   `json:"running,omitempty"`
	Device      string `json:"device,omitempty"`
	GPU         bool   `json:"gpu,omitempty"`
	LoadMS      int    `json:"loadMs,omitempty"`
	GPUError    string `json:"gpuError,omitempty"`
	Failed      string `json:"failed,omitempty"`
}

// Upstream stores this machine's opt-in separately, like routing preferences.
// Checking an absent profile must not start a process or download anything.
func (t Translator) PrivacyEnabled() (bool, error) {
	root := t.RootDataDirectory
	if root == "" {
		root = t.DataDirectory
	}
	raw, err := os.ReadFile(filepath.Join(root, "privacy", "config", "magpie", "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("本地隐私模型设置无法读取：%w", err)
	}
	var setting struct {
		On bool `json:"redactModel"`
	}
	if len(raw) > 4<<20 || json.Unmarshal(raw, &setting) != nil || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return false, errors.New("本地隐私模型设置无效，请恢复配置后重试")
	}
	return setting.On, nil
}

// StartPrivacy reuses the owned HTTP/pipe transport but has no routing or
// provider API surface. Account/settings refreshes cannot stop its download.
func (t Translator) StartPrivacy(ctx context.Context) (*RoutingProcess, error) {
	root := t.RootDataDirectory
	if root == "" {
		root = t.DataDirectory
	}
	t.DataDirectory = filepath.Join(root, "privacy")
	t.RootDataDirectory = root
	t.PrivacyService = nil
	return t.startService(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, nil, "privacy.serve")
}

func (r *RoutingProcess) PrivacyModel(ctx context.Context, action string) (PrivacyStatus, error) {
	var result PrivacyStatus
	method := http.MethodGet
	var body io.Reader
	if action != "" {
		switch action {
		case "on", "off", "cancel", "remove":
		default:
			return result, errors.New("本地隐私模型操作无效")
		}
		raw, _ := json.Marshal(map[string]string{"do": action})
		body = bytes.NewReader(raw)
		method = http.MethodPost
	}
	request, err := http.NewRequestWithContext(ctx, method, r.URL+"/api/privacy-model", body)
	if err != nil {
		return result, err
	}
	request.Header.Set("Authorization", "Bearer "+r.Key)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return result, fmt.Errorf("本地隐私模型服务无法连接：%w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return result, err
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		if failure.Error != "" {
			return result, fmt.Errorf("本地隐私模型操作失败：%s", failure.Error)
		}
		return result, fmt.Errorf("本地隐私模型操作失败（HTTP %d）", response.StatusCode)
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, errors.New("本地隐私模型返回无效状态")
	}
	return result, nil
}
