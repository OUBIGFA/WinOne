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

// RecognizeLocal runs the packaged native OCR worker and pinned model inventory.
// It consumes the owned PNG in memory and releases the worker when the call ends.
func (t Translator) RecognizeLocal(ctx context.Context, image []byte) (string, error) {
	if len(image) == 0 || len(image) > 32<<20 {
		return "", errors.New("OCR 图片为空或过大")
	}
	if !filepath.IsAbs(t.Executable) || !filepath.IsAbs(t.DataDirectory) {
		return "", errors.New("OCR 运行路径无效")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(t.Executable), "..", "..", "capture", "Ocr"))
	executable := filepath.Join(root, "..", "WinOne.Ocr.exe")
	for _, path := range []string{executable, filepath.Join(root, "rapidocrlite.dll")} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return "", errors.New("OCR 文件缺失，请修复安装")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--ocr-worker", root)
	cmd.Dir = root
	cmd.Env = isolatedEnvironment(t.DataDirectory)
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
