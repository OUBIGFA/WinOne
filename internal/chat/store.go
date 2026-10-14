// Package chat stores WinOne's project/navigation metadata. Pi owns conversation
// history and agent execution; this package never rewrites Pi session entries.
package chat

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/settings"
)

type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Permission string `json:"permission,omitempty"`
}
type Conversation struct {
	ID               string       `json:"id"`
	ProjectID        string       `json:"projectId,omitempty"`
	Title            string       `json:"title"`
	TitleManual      bool         `json:"titleManual,omitempty"`
	Directory        string       `json:"directory"`
	SessionDirectory string       `json:"sessionDirectory"`
	SessionFile      string       `json:"sessionFile,omitempty"`
	HasMessages      bool         `json:"hasMessages,omitempty"`
	Model            ai.Selection `json:"model"`
	Permission       string       `json:"permission"`
	ThinkingLevel    string       `json:"thinkingLevel,omitempty"`
	FastMode         bool         `json:"fastMode,omitempty"`
	Archived         bool         `json:"archived"`
	Pinned           bool         `json:"pinned"`
	CreatedAt        string       `json:"createdAt"`
	UpdatedAt        string       `json:"updatedAt"`
}
type Catalog struct {
	Version       int            `json:"version"`
	Projects      []Project      `json:"projects"`
	Conversations []Conversation `json:"conversations"`
}
type Draft struct {
	Text        string       `json:"text"`
	PendingID   string       `json:"pendingId,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}
type Store struct {
	reviewMutation sync.Mutex
	mu             sync.Mutex
	path, root     string
	data           Catalog
	preferences    Preferences
	resources      map[string]map[string]string // Exact Worker-loaded resource aliases and resolved targets.
	reviewFiles    map[string]map[string]string // Exact successful file-tool results on the active branch.
}

var identifier = regexp.MustCompile(`^[A-Z2-7]{26}$`)
var reservedName = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\..*)?$`)

func Open(directory string) (*Store, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("Chat 保存目录必须是绝对路径")
	}
	s := &Store{root: directory, path: filepath.Join(directory, "index.json"), data: Catalog{Version: 1, Projects: []Project{}, Conversations: []Conversation{}}}
	if err := s.loadPreferences(); err != nil {
		return nil, err
	}
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, errors.New("Chat 索引过大")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&s.data); err != nil {
		return nil, fmt.Errorf("Chat 索引无法读取，原文件已保留：%w", err)
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("Chat 索引含有多余或未完成的数据，原文件已保留")
	}
	// Migrate the unpublished prototype's edit mode without granting commands.
	for i := range s.data.Conversations {
		if s.data.Conversations[i].Permission == "edit" {
			s.data.Conversations[i].Permission = "ask"
		}
	}
	if err = validateCatalog(s.data); err != nil {
		return nil, err
	}
	return s, nil
}
func clone(value Catalog) Catalog {
	value.Projects = slices.Clone(value.Projects)
	value.Conversations = slices.Clone(value.Conversations)
	if value.Projects == nil {
		value.Projects = []Project{}
	}
	if value.Conversations == nil {
		value.Conversations = []Conversation{}
	}
	return value
}
func (s *Store) Snapshot() Catalog { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.data) }
func permissionValid(value string) bool {
	return value == "ask" || value == "auto" || value == "bypassPermissions"
}
func validateCatalog(value Catalog) error {
	if value.Version != 1 || len(value.Projects) > 256 || len(value.Conversations) > 2000 {
		return errors.New("Chat 索引版本或大小无效")
	}
	projects := map[string]bool{}
	conversations := map[string]bool{}
	for _, p := range value.Projects {
		if !identifier.MatchString(p.ID) || projects[p.ID] || strings.TrimSpace(p.Name) == "" || len(p.Name) > 160 || !filepath.IsAbs(p.Path) {
			return errors.New("Chat 项目资料无效")
		}
		projects[p.ID] = true
	}
	for _, c := range value.Conversations {
		if !identifier.MatchString(c.ID) || conversations[c.ID] || c.ProjectID != "" && !projects[c.ProjectID] || strings.TrimSpace(c.Title) == "" || len(c.Title) > 320 || !filepath.IsAbs(c.Directory) || !filepath.IsAbs(c.SessionDirectory) || !permissionValid(c.Permission) {
			return errors.New("Chat 对话资料无效")
		}
		if c.SessionFile != "" && !within(c.SessionDirectory, c.SessionFile) {
			return errors.New("Pi 会话文件不在已登记的保存目录")
		}
		conversations[c.ID] = true
	}
	return nil
}
func within(root, target string) bool {
	if !filepath.IsAbs(target) {
		return false
	}
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func existingDirectory(directory string) (string, error) {
	if !filepath.IsAbs(directory) {
		return "", errors.New("请选择本地文件夹")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("所选位置不是文件夹")
	}
	return resolved, nil
}
func (s *Store) change(apply func(*Catalog) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.data)
	if err := apply(&next); err != nil {
		return err
	}
	if err := validateCatalog(next); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err = settings.WriteAtomic(s.path, data); err != nil {
		return err
	}
	s.data = next
	return nil
}
func (s *Store) AddProject(directory, name string, permission ...string) (Project, error) {
	resolved, err := existingDirectory(directory)
	if err != nil {
		return Project{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(resolved)
	}
	project := Project{ID: rand.Text(), Name: strings.TrimSpace(name), Path: resolved}
	if len(permission) > 0 && permissionValid(permission[0]) {
		project.Permission = permission[0]
	}
	err = s.change(func(value *Catalog) error {
		for _, old := range value.Projects {
			if strings.EqualFold(old.Path, resolved) {
				project = old
				return nil
			}
		}
		value.Projects = append(value.Projects, project)
		return nil
	})
	return project, err
}
func (s *Store) CreateProject(parent, name string, permission ...string) (Project, error) {
	parent, err := existingDirectory(parent)
	if err != nil {
		return Project{}, err
	}
	if name == "" || name != strings.TrimSpace(name) || strings.ContainsAny(name, `<>:"/\|?*`+"\x00") || strings.HasSuffix(name, ".") || reservedName.MatchString(name) || len(name) > 160 {
		return Project{}, errors.New("项目名称无效")
	}
	directory := filepath.Join(parent, name)
	if err = os.Mkdir(directory, 0700); err != nil {
		return Project{}, err
	}
	project, err := s.AddProject(directory, name, permission...)
	if err != nil {
		return Project{}, fmt.Errorf("文件夹已创建，但项目登记未保存。请重新打开 %s：%w", directory, err)
	}
	return project, nil
}
func (s *Store) RemoveProject(id string) error {
	return s.change(func(value *Catalog) error {
		index := slices.IndexFunc(value.Projects, func(p Project) bool { return p.ID == id })
		if index < 0 {
			return errors.New("项目不存在")
		}
		value.Projects = slices.Delete(value.Projects, index, index+1)
		for i := range value.Conversations {
			if value.Conversations[i].ProjectID == id {
				value.Conversations[i].ProjectID = ""
			}
		}
		return nil
	})
}
func (s *Store) RenameProject(id, name string) (Project, error) {
	var project Project
	err := s.change(func(value *Catalog) error {
		index := slices.IndexFunc(value.Projects, func(p Project) bool { return p.ID == id })
		if index < 0 {
			return errors.New("项目不存在")
		}
		value.Projects[index].Name = strings.TrimSpace(name)
		project = value.Projects[index]
		return nil
	})
	return project, err
}
func (s *Store) NewConversation(projectID, directory, title string, model ai.Selection, saveDirectory ...string) (Conversation, error) {
	if len(saveDirectory) > 1 {
		return Conversation{}, errors.New("只能选择一个会话保存位置")
	}
	var savedRoot string
	if len(saveDirectory) == 1 && saveDirectory[0] != "" {
		var err error
		savedRoot, err = existingDirectory(saveDirectory[0])
		if err != nil {
			return Conversation{}, err
		}
	}
	id := rand.Text()
	if strings.TrimSpace(title) == "" {
		title = "Chat"
	}
	c := Conversation{ID: id, ProjectID: projectID, Title: strings.TrimSpace(title), Permission: "ask", Model: model, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	c.UpdatedAt = c.CreatedAt
	err := s.change(func(value *Catalog) error {
		if projectID != "" {
			index := slices.IndexFunc(value.Projects, func(p Project) bool { return p.ID == projectID })
			if index < 0 {
				return errors.New("项目不存在")
			}
			directory = value.Projects[index].Path
			if permissionValid(value.Projects[index].Permission) {
				c.Permission = value.Projects[index].Permission
			}
			c.SessionDirectory = filepath.Join(s.root, "sessions", id)
		}
		if directory == "" {
			directory = filepath.Join(s.preferences.WorkspaceDirectory, id)
			if err := os.MkdirAll(directory, 0700); err != nil {
				return err
			}
		}
		resolved, err := existingDirectory(directory)
		if err != nil {
			return err
		}
		c.Directory = resolved
		if savedRoot != "" {
			c.SessionDirectory = filepath.Join(savedRoot, ".winone-chat", id)
		}
		// Explicit projectless locations own both task files and the Pi history.
		if c.SessionDirectory == "" {
			c.SessionDirectory = filepath.Join(resolved, ".winone-chat", id)
		}
		if err = os.MkdirAll(c.SessionDirectory, 0700); err != nil {
			return err
		}
		value.Conversations = append(value.Conversations, c)
		return nil
	})
	return c, err
}
func (s *Store) Conversation(id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Conversations {
		if c.ID == id {
			return c, nil
		}
	}
	return Conversation{}, errors.New("对话不存在")
}
func (s *Store) UpdateConversation(id string, apply func(*Conversation) error) (Conversation, error) {
	var result Conversation
	err := s.change(func(value *Catalog) error {
		index := slices.IndexFunc(value.Conversations, func(c Conversation) bool { return c.ID == id })
		if index < 0 {
			return errors.New("对话不存在")
		}
		if err := apply(&value.Conversations[index]); err != nil {
			return err
		}
		value.Conversations[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result = value.Conversations[index]
		return nil
	})
	return result, err
}
func (s *Store) BindSession(id, file string) (Conversation, error) {
	return s.UpdateConversation(id, func(c *Conversation) error {
		if !within(c.SessionDirectory, file) {
			return errors.New("Pi 返回了未授权的会话保存位置")
		}
		resolved, err := filepath.EvalSymlinks(filepath.Dir(file))
		if err != nil {
			return err
		}
		expected, err := filepath.EvalSymlinks(c.SessionDirectory)
		if err != nil {
			return err
		}
		if !within(expected, filepath.Join(resolved, filepath.Base(file))) {
			return errors.New("会话保存位置越过了目录边界")
		}
		c.SessionFile = file
		return nil
	})
}
func (s *Store) ReadDraft(id string) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.draftLocked(id)
}
func (s *Store) draftLocked(id string) (Draft, error) {
	for _, c := range s.data.Conversations {
		if c.ID == id {
			file, err := os.Open(filepath.Join(c.SessionDirectory, "draft.json"))
			if errors.Is(err, os.ErrNotExist) {
				return Draft{}, nil
			}
			if err != nil {
				return Draft{}, err
			}
			defer file.Close()
			raw, err := io.ReadAll(io.LimitReader(file, 128*1024+1))
			if err != nil {
				return Draft{}, err
			}
			if len(raw) > 128*1024 {
				return Draft{}, errors.New("Chat 草稿过大")
			}
			var draft Draft
			if err = json.Unmarshal(raw, &draft); err != nil {
				return Draft{}, err
			}
			return draft, nil
		}
	}
	return Draft{}, errors.New("对话不存在")
}

// The desktop serializes draft writes. A Pi acknowledgement can clear only
// its own pending input, never text the user has started typing afterwards.
func (s *Store) SaveDraft(id string, draft Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveDraftLocked(id, draft)
}
func (s *Store) saveDraftLocked(id string, draft Draft) error {
	if len(draft.Text) > 100000 || len(draft.PendingID) > 128 {
		return errors.New("Chat 草稿过长")
	}
	for _, c := range s.data.Conversations {
		if c.ID == id {
			data, err := json.Marshal(draft)
			paths := make([]string, 0, len(draft.Attachments))
			for _, attachment := range draft.Attachments {
				paths = append(paths, attachment.Path)
			}
			if err := validateConversationAttachments(c, paths); err != nil {
				return err
			}
			if err != nil {
				return err
			}
			return settings.WriteAtomic(filepath.Join(c.SessionDirectory, "draft.json"), data)
		}
	}
	return errors.New("对话不存在")
}
func (s *Store) ConfirmInput(id, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft, err := s.draftLocked(id)
	if err != nil {
		return err
	}
	if draft.PendingID == requestID {
		return s.saveDraftLocked(id, Draft{})
	}
	return nil
}
