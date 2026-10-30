//go:build windows

package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"winone/internal/ai"
)

// IM's synchronous host.secrets contract is fulfilled by an encrypted KV in the
// Bun worker. Its master key is itself Windows-user protected by the same DPAPI
// implementation as WinOne AI credentials. It never enters a desktop response.
func oneBotMasterKey(directory string) (string, error) {
	file := filepath.Join(directory, "im-master-key.bin")
	if info, err := os.Lstat(file); err == nil {
		if !info.Mode().IsRegular() || info.Size() > 4096 {
			return "", errors.New("OneBot 安全存储文件无效")
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		key, err := ai.UnprotectLocalData(raw)
		if err != nil {
			return "", err
		}
		defer clear(key)
		if len(key) != 32 {
			return "", errors.New("OneBot 安全存储密钥无效")
		}
		return base64.StdEncoding.EncodeToString(key), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	defer clear(key)
	sealed, err := ai.ProtectLocalData(key)
	if err != nil {
		return "", err
	}
	output, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return oneBotMasterKey(directory)
	}
	if err != nil {
		return "", err
	}
	_, writeErr := output.Write(sealed)
	syncErr := output.Sync()
	closeErr := output.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
