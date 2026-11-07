package learning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var verifiedResources sync.Map

// VerifyDictionary checks the packaged digest once per process. The installer
// also verifies its asset manifest; runtime never silently substitutes a web API.
func VerifyDictionary(directory string) (string, error) {
	path := filepath.Join(directory, "ecdict.db")
	if _, ok := verifiedResources.Load(path); ok {
		return path, nil
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return "", errors.New("离线词典资源缺失，请修复 WinOne 安装")
	}
	var info struct {
		Format int    `json:"format"`
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(manifest, &info) != nil || info.Format != 1 || len(info.SHA256) != 64 {
		return "", errors.New("离线词典清单无效")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err = io.Copy(sum, file); err != nil {
		return "", err
	}
	if hex.EncodeToString(sum.Sum(nil)) != info.SHA256 {
		return "", errors.New("离线词典校验失败，请修复 WinOne 安装")
	}
	verifiedResources.Store(path, true)
	return path, nil
}
