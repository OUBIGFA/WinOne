package chat

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/settings"
)

// Preferences belong to WinOne. A project's instructions may describe work,
// but cannot enable executable extensions or change the user's MCP servers.
type Preferences struct {
	TitleModel         ai.Selection `json:"titleModel,omitempty"`
	WorkspaceDirectory string       `json:"workspaceDirectory"`
	SkillDirectories   []string     `json:"skillDirectories"`
	PluginDirectories  []string     `json:"pluginDirectories"`
	MCPServers         []MCPServer  `json:"mcpServers"`
	Automations        []Automation `json:"automations"`
}
type MCPServer struct {
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
}
type Automation struct {
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	Prompt             string       `json:"prompt"`
	ProjectID          string       `json:"projectId,omitempty"`
	Directory          string       `json:"directory,omitempty"`
	Model              ai.Selection `json:"model"`
	Permission         string       `json:"permission"`
	IntervalMinutes    int          `json:"intervalMinutes"`
	Enabled            bool         `json:"enabled"`
	NextRunAt          string       `json:"nextRunAt,omitempty"`
	LastRunAt          string       `json:"lastRunAt,omitempty"`
	LastConversationID string       `json:"lastConversationId,omitempty"`
	Status             string       `json:"status,omitempty"`
	Error              string       `json:"error,omitempty"`
}

func clonePreferences(value Preferences) Preferences {
	// Maps and slices must not alias the persisted preferences exposed to UI.
	raw, _ := json.Marshal(value)
	var copy Preferences
	_ = json.Unmarshal(raw, &copy)
	if copy.SkillDirectories == nil {
		copy.SkillDirectories = []string{}
	}
	if copy.PluginDirectories == nil {
		copy.PluginDirectories = []string{}
	}
	if copy.MCPServers == nil {
		copy.MCPServers = []MCPServer{}
	}
	if copy.Automations == nil {
		copy.Automations = []Automation{}
	}
	return copy
}
func (s *Store) loadPreferences() error {
	s.preferences = clonePreferences(Preferences{WorkspaceDirectory: filepath.Join(s.root, "workspaces")})
	raw, err := os.ReadFile(filepath.Join(s.root, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("Chat 设置文件过大")
	}
	if err = configjson.DecodeObject(raw, &s.preferences); err != nil {
		return fmt.Errorf("Chat 设置无法读取，原文件已保留：%w", err)
	}
	if err = validatePreferences(s.preferences); err != nil {
		return err
	}
	s.preferences = clonePreferences(s.preferences)
	return nil
}
func (s *Store) Preferences() Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePreferences(s.preferences)
}

var serverName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func validatePreferences(value Preferences) error {
	if (value.TitleModel.Provider == "") != (value.TitleModel.Model == "") || len(value.TitleModel.Provider) > 256 || len(value.TitleModel.Model) > 256 {
		return errors.New("标题模型无效")
	}
	if !filepath.IsAbs(value.WorkspaceDirectory) || strings.ContainsRune(value.WorkspaceDirectory, 0) {
		return errors.New("请选择默认工作文件夹")
	}
	if len(value.SkillDirectories) > 64 || len(value.PluginDirectories) > 64 || len(value.MCPServers) > 64 || len(value.Automations) > 100 {
		return errors.New("Chat 设置项目过多")
	}
	for _, directory := range append(slices.Clone(value.SkillDirectories), value.PluginDirectories...) {
		if !filepath.IsAbs(directory) || strings.ContainsRune(directory, 0) {
			return errors.New("技能和插件目录必须是绝对路径")
		}
	}
	names := map[string]bool{}
	for _, server := range value.MCPServers {
		key := strings.ReplaceAll(server.Name, "-", "_")
		if !serverName.MatchString(server.Name) || names[key] {
			return errors.New("MCP 服务名称无效或重复")
		}
		names[key] = true
		if (server.Command == "") == (server.URL == "") {
			return errors.New("MCP 服务需设置一个命令或 HTTP 地址")
		}
		if strings.ContainsAny(server.Command, "\x00\r\n") || len(server.Args) > 128 || server.Timeout < 0 || server.Timeout > 600 {
			return errors.New("MCP 命令或超时无效")
		}
		if server.Cwd != "" && !filepath.IsAbs(server.Cwd) {
			return errors.New("MCP 工作目录必须是绝对路径")
		}
		if server.URL != "" {
			u, err := url.Parse(server.URL)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
				return errors.New("MCP 地址必须是有效的 HTTP 或 HTTPS 地址")
			}
		}
		for _, values := range []map[string]string{server.Env, server.Headers} {
			for key, value := range values {
				if strings.ContainsAny(key+value, "\x00\r\n") || strings.HasPrefix(value, "!") {
					return errors.New("MCP 环境变量和请求头不能执行命令或含换行")
				}
			}
		}
	}
	ids := map[string]bool{}
	for _, a := range value.Automations {
		if !identifier.MatchString(a.ID) || ids[a.ID] || strings.TrimSpace(a.Name) == "" || len(a.Name) > 160 || strings.TrimSpace(a.Prompt) == "" || len(a.Prompt) > 100000 || a.IntervalMinutes < 1 || a.IntervalMinutes > 525600 || !permissionValid(a.Permission) {
			return errors.New("自动化任务名称、提示词、周期或权限无效")
		}
		ids[a.ID] = true
		if a.Directory != "" && !filepath.IsAbs(a.Directory) {
			return errors.New("自动化工作目录必须是绝对路径")
		}
	}
	return nil
}
func (s *Store) savePreferencesLocked(value Preferences) error {
	if err := validatePreferences(value); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("Chat 设置过大")
	}
	if err = settings.WriteAtomic(filepath.Join(s.root, "settings.json"), raw); err != nil {
		return err
	}
	s.preferences = clonePreferences(value)
	return nil
}
func (s *Store) SavePreferences(value Preferences) (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range value.Automations {
		a := &value.Automations[i]
		if a.ID == "" {
			a.ID = rand.Text()
		}
		old := slices.IndexFunc(s.preferences.Automations, func(prior Automation) bool { return prior.ID == a.ID })
		if old >= 0 {
			prior := s.preferences.Automations[old]
			a.LastRunAt, a.LastConversationID, a.Status, a.Error = prior.LastRunAt, prior.LastConversationID, prior.Status, prior.Error
			if prior.Enabled == a.Enabled && prior.IntervalMinutes == a.IntervalMinutes {
				a.NextRunAt = prior.NextRunAt
			} else {
				a.NextRunAt = ""
			}
		}
		if a.Enabled && a.NextRunAt == "" {
			a.NextRunAt = time.Now().UTC().Add(time.Duration(a.IntervalMinutes) * time.Minute).Format(time.RFC3339)
		}
	}
	if err := validatePreferences(value); err != nil {
		return Preferences{}, err
	}
	// Check supplied directories without moving existing conversations or loading
	// code. Removable directories may be absent when loading previously saved settings.
	if filepath.Clean(value.WorkspaceDirectory) == filepath.Join(s.root, "workspaces") {
		if err := os.MkdirAll(value.WorkspaceDirectory, 0700); err != nil {
			return Preferences{}, err
		}
	}
	for _, directory := range append(append([]string{value.WorkspaceDirectory}, value.SkillDirectories...), value.PluginDirectories...) {
		if _, err := existingDirectory(directory); err != nil {
			return Preferences{}, err
		}
	}
	for _, a := range value.Automations {
		if a.ProjectID != "" && !slices.ContainsFunc(s.data.Projects, func(p Project) bool { return p.ID == a.ProjectID }) {
			return Preferences{}, errors.New("自动化项目不存在")
		}
	}
	if err := s.savePreferencesLocked(value); err != nil {
		return Preferences{}, err
	}
	return clonePreferences(value), nil
}
