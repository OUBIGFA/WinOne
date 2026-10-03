package preferences

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"winone/modules/wintray/internal/config"
)

type ImportPreview struct {
	Settings config.Settings `json:"settings"`
	Warnings []string        `json:"warnings"`
	History  json.RawMessage `json:"history,omitempty"`
}

// PreviewImport reads only the explicitly selected file; saving uses the existing transaction.
func PreviewImport(path string) (ImportPreview, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".json") {
		return ImportPreview{}, errors.New("请选择 WinTray 的 JSON 配置文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return ImportPreview{}, err
	}
	defer f.Close()
	const limit = 4 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return ImportPreview{}, err
	}
	if len(data) > limit {
		return ImportPreview{}, errors.New("配置文件超过 4 MB")
	}
	decoded, err := config.DecodeImport(data)
	if err != nil {
		return ImportPreview{}, err
	}
	if err = config.Validate(decoded); err != nil {
		return ImportPreview{}, err
	}
	warnings := []string{"导入后将替换当前程序列表，点击保存后生效。"}
	var original config.Settings
	if err = json.Unmarshal(data, &original); err != nil {
		return ImportPreview{}, err
	}
	originalJSON, _ := json.Marshal(original)
	decodedJSON, _ := json.Marshal(decoded)
	if !bytes.Equal(originalJSON, decodedJSON) {
		warnings = append(warnings, "已按旧版本兼容规则转换，请核对各项设置。")
	}
	for _, app := range decoded.ManagedApps {
		if _, err = os.Stat(app.ExePath); errors.Is(err, os.ErrNotExist) {
			warnings = append(warnings, "找不到程序："+app.Name)
		} else if err != nil {
			return ImportPreview{}, err
		}
	}
	var history json.RawMessage
	historyFile, historyErr := os.Open(filepath.Join(filepath.Dir(path), "startup-history.json"))
	if historyErr == nil {
		defer historyFile.Close()
		history, err = io.ReadAll(io.LimitReader(historyFile, limit+1))
		if err != nil {
			return ImportPreview{}, err
		}
		if len(history) > limit {
			return ImportPreview{}, errors.New("运行历史超过 4 MB")
		}
		if _, err = config.DecodeHistory(history); err != nil {
			return ImportPreview{}, err
		}
		warnings = append(warnings, "保存时合并运行历史，保留运行次数限制。")
	} else if !errors.Is(historyErr, os.ErrNotExist) {
		return ImportPreview{}, historyErr
	}
	preview := ImportPreview{Settings: decoded, Warnings: warnings, History: history}
	encoded, err := json.Marshal(preview)
	if err != nil {
		return ImportPreview{}, err
	}
	// Leave room for the response envelope and later edited settings within the 1 MB pipe frame.
	if len(encoded) > 900*1024 {
		return ImportPreview{}, errors.New("配置与运行历史超过单次导入上限（900 KB）")
	}
	return preview, nil
}
