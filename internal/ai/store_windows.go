//go:build windows

package ai

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
	"winone/internal/settings"
)

type Store struct {
	mu             sync.Mutex
	path           string
	value          Preferences
	protected      string
	configuration  *Configuration
	keys           map[string]string
	credentialKeys map[string]map[string]string
	searchKeys     map[string]string
	keyRounds      map[string]*keyRound
	now            func() time.Time
}
type stored struct {
	SchemaVersion           int                          `json:"schemaVersion,omitempty"`
	Preferences             Preferences                  `json:"preferences"`
	ProtectedKey            string                       `json:"protectedKey,omitempty"`
	Configuration           *Configuration               `json:"configuration,omitempty"`
	ProtectedKeys           map[string]string            `json:"protectedKeys,omitempty"`
	ProtectedCredentialKeys map[string]map[string]string `json:"protectedCredentialKeys,omitempty"`
	ProtectedSearchKeys     map[string]string            `json:"protectedSearchKeys,omitempty"`
}

const maximumSettingsBytes = 1024 * 1024

func Open(path string) (*Store, error) {
	s := &Store{path: path, value: DefaultPreferences()}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximumSettingsBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumSettingsBytes {
		return nil, errors.New("AI 设置文件过大")
	}
	var document stored
	if err = json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.SchemaVersion > 2 {
		return nil, errors.New("AI 配置来自更新的版本")
	}
	if document.Configuration != nil {
		if err = validateConfiguration(*document.Configuration); err != nil {
			return nil, err
		}
		s.configuration = document.Configuration
		s.keys = document.ProtectedKeys
		s.credentialKeys = document.ProtectedCredentialKeys
		s.searchKeys = document.ProtectedSearchKeys
		for i := range s.configuration.Providers {
			s.configuration.Providers[i].HasKey = s.keys[s.configuration.Providers[i].ID] != ""
		}
		s.value = projectConfiguration(*s.configuration)
		s.protected = document.ProtectedKey
		return s, nil
	}
	if err = validate(document.Preferences); err != nil {
		return nil, err
	}
	s.value = document.Preferences
	s.protected = document.ProtectedKey
	return s, nil
}
func validate(p Preferences) error {
	if p.AgentBackend != "" && (p.AuthMode != "account" || p.AgentBackend != "builtin" && p.AgentBackend != "plugin") {
		return errors.New("账号连接方式无效")
	}
	if p.OCREngine != "" && p.OCREngine != "local" && p.OCREngine != "ai" {
		return errors.New("请选择有效的 OCR 引擎")
	}
	if p.AuthMode != "" && p.AuthMode != "api-key" && p.AuthMode != "account" {
		return errors.New("请选择有效的登录方式")
	}
	if p.Engine != "google" && p.Engine != "llm" {
		return errors.New("请选择有效的翻译引擎")
	}
	if !ValidTranslationTarget(p.Target) {
		return errors.New("请选择有效的目标语言")
	}
	switch p.Protocol {
	case "chat", "responses", "anthropic":
	default:
		return errors.New("请选择有效的 AI 协议")
	}
	if len(p.Prompt) > 16000 || len(p.Model) > 256 || len(p.BaseURL) > 2048 {
		return errors.New("AI 设置过长")
	}
	if p.Engine == "llm" || p.OCREngine == "ai" {
		if p.AuthMode == "account" {
			if p.Agent == "" || len(p.Agent) > 80 || strings.ContainsAny(p.Agent, "/\\\x00") {
				return errors.New("请选择智能体账号")
			}
			if strings.TrimSpace(p.Model) == "" {
				return errors.New("请输入模型名称")
			}
			return nil
		}
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
			return errors.New("AI 地址须为 HTTPS；本机服务可用 HTTP")
		}
		for _, suffix := range []string{"/chat/completions", "/responses", "/messages"} {
			if strings.HasSuffix(strings.TrimRight(u.Path, "/"), suffix) {
				return errors.New("请填写 API 基础地址，例如 https://example.com/v1")
			}
		}
		if strings.TrimSpace(p.Model) == "" {
			return errors.New("请输入模型名称")
		}
	}
	return nil
}
func (s *Store) Snapshot() Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.value
	p.HasKey = s.protected != ""
	return p
}
func (s *Store) Save(p Preferences, key *string) (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configuration != nil {
		return s.value, errors.New("请在 AI 服务中编辑多个服务及任务分配")
	}
	if p.Revision != s.value.Revision {
		return s.value, settings.ErrConflict
	}
	if err := validate(p); err != nil {
		return s.value, err
	}
	sealed := s.protected
	if key != nil {
		if len(*key) > 16384 {
			return s.value, errors.New("密钥过长")
		}
		sealed = ""
		if *key != "" {
			data, err := protect([]byte(*key), false)
			if err != nil {
				return s.value, err
			}
			sealed = base64.StdEncoding.EncodeToString(data)
		}
	}
	p.Revision++
	p.HasKey = sealed != ""
	bytes, err := json.Marshal(stored{Preferences: p, ProtectedKey: sealed})
	if err != nil {
		return s.value, err
	}
	if err = settings.WriteAtomic(s.path, bytes); err != nil {
		return s.value, err
	}
	s.value = p
	s.protected = sealed
	return p, nil
}
func (s *Store) ForRequest() (Preferences, string, error) {
	return s.ForTask("translate", nil)
}

// Generic module calls use the shared chat route independently of translation.
func (s *Store) ForModelRequest() (Preferences, string, error) {
	return s.ForTask("chat", nil)
}

// DPAPI binds the credential to this Windows user. It is never returned by
// settings reads and does not enter portable configuration or capture drafts.
func protect(data []byte, decrypt bool) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, 1, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, 1, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	value := unsafe.Slice(out.Data, int(out.Size))
	copied := append([]byte(nil), value...)
	clear(value)
	return copied, nil
}
