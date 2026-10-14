//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RecognizeLocal reuses the packaged HushSnap OCR script and model inventory.
// It consumes the owned PNG in memory and releases Python when the call ends.
func (t Translator) RecognizeLocal(ctx context.Context, image []byte) (string, error) {
	if len(image) == 0 || len(image) > 32<<20 {
		return "", errors.New("OCR 图片为空或过大")
	}
	if !filepath.IsAbs(t.Executable) || !filepath.IsAbs(t.DataDirectory) {
		return "", errors.New("OCR 运行路径无效")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(t.Executable), "..", "..", "capture", "Ocr"))
	python, script := filepath.Join(root, "python", "python.exe"), filepath.Join(root, "recognize.py")
	for _, path := range []string{python, script} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return "", errors.New("OCR 文件缺失，请修复安装")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-B", script, filepath.Join(root, "models"))
	cmd.Dir = root
	cmd.Env = append(isolatedEnvironment(t.DataDirectory), "PYTHONUTF8=1")
	process, err := startOwnedProcess(ctx, cmd)
	if err != nil {
		return "", errors.New("OCR 无法启动，请修复安装")
	}
	if _, err = process.input.Write(image); err != nil {
		process.closeJob()
		_ = process.finish()
		return "", err
	}
	process.input.Close()
	data, readErr := io.ReadAll(io.LimitReader(process.output, 1024*1024+1))
	if readErr != nil || len(data) > 1024*1024 {
		process.closeJob()
		_ = process.finish()
		return "", errors.New("OCR 响应无法读取或过大")
	}
	err = process.finish()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var result struct {
		Text string `json:"text"`
	}
	if err != nil || json.Unmarshal(data, &result) != nil {
		return "", errors.New("OCR 识别失败，请重试")
	}
	if strings.TrimSpace(result.Text) == "" {
		return "", errors.New("未识别到文字，请重新选择区域")
	}
	return result.Text, nil
}
