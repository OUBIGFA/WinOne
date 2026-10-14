//go:build windows

package chat

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type reviewSessionEntry struct {
	CustomType string `json:"customType"`
	Data       struct {
		ID string `json:"id"`
	} `json:"data"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Message  struct {
		Role       string          `json:"role"`
		ToolCallID string          `json:"toolCallId"`
		IsError    bool            `json:"isError"`
		Content    json.RawMessage `json:"content"`
		Details    struct {
			Patch string               `json:"patch"`
			Files []recordedFileChange `json:"files"`
		} `json:"details"`
	} `json:"message"`
}
type reviewToolCall struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	} `json:"arguments"`
}

// SessionReview is a record of actual successful file tools on Pi's active
// branch. Ordinary chat folders need no Git initialization. Each file shows its
// latest recorded edit; write results without a before-image show saved content
// explicitly, never a fabricated deletion count or a guessed new-file status.
func (s *Store) SessionReview(id string) (ReviewSnapshot, map[string]string, error) {
	return s.sessionReview(id, "")
}

// TurnReview retains the original turn boundary even after later turns edit
// the same file. Empty or foreign identities must not fall back to all changes.
func (s *Store) TurnReview(id, turnID string) (ReviewSnapshot, map[string]string, error) {
	if turnID == "" {
		return ReviewSnapshot{}, nil, errors.New("请选择要查看的工作轮次")
	}
	return s.sessionReview(id, turnID)
}

func (s *Store) sessionReview(id, turnID string) (ReviewSnapshot, map[string]string, error) {
	c, err := s.Conversation(id)
	if err != nil {
		return ReviewSnapshot{}, nil, err
	}
	result := ReviewSnapshot{Root: c.Directory, Scope: "working", Source: "session", Files: []ReviewFile{}}
	patches := map[string]string{}
	captures := map[string]*capturedTurnFile{}
	// Recompute grants from the current branch, so a retry cannot retain access
	// to a file referenced only by an abandoned branch.
	grants := map[string]string{}
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.reviewFiles == nil {
			s.reviewFiles = map[string]map[string]string{}
		}
		s.reviewFiles[id] = grants
	}()
	if c.SessionFile == "" {
		return result, patches, nil
	}
	if !within(c.SessionDirectory, c.SessionFile) {
		return result, nil, errors.New("会话路径越界")
	}
	root, err := resolvePreviewPath(c.SessionDirectory)
	if err != nil {
		return result, nil, err
	}
	path, err := resolvePreviewPath(c.SessionFile)
	if err != nil {
		return result, nil, err
	}
	if !within(root, path) {
		return result, nil, errors.New("会话路径越界")
	}
	file, err := os.Open(path)
	if err != nil {
		return result, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return result, nil, err
	}
	if info.Size() > 128<<20 {
		return result, nil, errors.New("会话文件过大")
	}
	entries := map[string]reviewSessionEntry{}
	leaf := ""
	scanner := bufio.NewScanner(io.LimitReader(file, 128<<20))
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	for scanner.Scan() {
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		var entry reviewSessionEntry
		if err = json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return result, nil, errors.New("会话正在写入或格式无效，请稍后刷新")
		}
		if entry.Type == "session" {
			continue
		}
		if entry.ID == "" {
			return result, nil, errors.New("会话消息缺少 Pi 标识")
		}
		entries[entry.ID] = entry
		leaf = entry.ID
	}
	if err = scanner.Err(); err != nil {
		return result, nil, err
	}
	branch := []reviewSessionEntry{}
	seen := map[string]bool{}
	for leaf != "" {
		entry, ok := entries[leaf]
		if !ok || seen[leaf] {
			return result, nil, errors.New("会话分支无效")
		}
		seen[leaf] = true
		branch = append(branch, entry)
		leaf = entry.ParentID
	}
	slices.Reverse(branch)
	calls := map[string]reviewToolCall{}
	add := func(filePath, patch, status string, capture *recordedFileChange) {
		if strings.TrimSpace(filePath) == "" {
			return
		}
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(c.Directory, filePath)
		}
		displayPath := filePath
		if within(c.Directory, filePath) {
			displayPath, _ = filepath.Rel(c.Directory, filePath)
		}
		displayPath = filepath.ToSlash(displayPath)
		if turnID != "" {
			mergeTurnCapture(captures, displayPath, filePath, capture)
		}
		if _, exists := patches[displayPath]; !exists {
			result.Files = append(result.Files, ReviewFile{Path: displayPath, Status: status})
		}
		patches[displayPath] = patch
		if target, e := resolvePreviewPath(filePath); e == nil {
			if info, e := os.Stat(target); e == nil && info.Mode().IsRegular() {
				grants[strings.ToLower(filepath.Clean(filePath))] = target
			}
		}
	}
	currentTurn, pendingTurn := "", ""
	foundTurn := turnID == ""
	for _, entry := range branch {
		if entry.CustomType == "winone.input" {
			pendingTurn = entry.Data.ID
		}
		if entry.Message.Role == "user" {
			currentTurn = pendingTurn
			pendingTurn = ""
			if currentTurn == "" {
				currentTurn = entry.ID
			}
			if currentTurn == turnID || entry.ID == turnID {
				foundTurn = true
				currentTurn = turnID
			}
		}
		if turnID != "" && currentTurn != turnID {
			continue
		}
		if entry.Message.Role == "assistant" {
			var parts []reviewToolCall
			if json.Unmarshal(entry.Message.Content, &parts) == nil {
				for _, part := range parts {
					if part.Type == "toolCall" && (part.Name == "edit" || part.Name == "write" || part.Name == "powershell") {
						calls[part.ID] = part
					}
				}
			}
		}
		if entry.Message.Role != "toolResult" || entry.Message.IsError {
			continue
		}
		call, ok := calls[entry.Message.ToolCallID]
		if !ok {
			continue
		}
		delete(calls, entry.Message.ToolCallID)
		if entry.Message.Details.Files != nil {
			for _, file := range entry.Message.Details.Files {
				add(file.Path, file.Patch, file.Status, &file)
			}
			continue
		}
		patch := entry.Message.Details.Patch
		if patch == "" && call.Name == "write" && call.Arguments.Content != nil {
			patch = *call.Arguments.Content
		}
		add(call.Arguments.Path, patch, "M", nil)
	}
	if !foundTurn {
		return result, nil, errors.New("此工作轮次不在当前会话分支中")
	}
	if turnID != "" {
		result.captured = captures
		result.IsReversible = len(result.Files) > 0
		for index, file := range result.Files {
			capture := captures[file.Path]
			if capture == nil || !capture.Valid {
				result.IsReversible = false
				continue
			}
			oldPath := "a/" + file.Path
			if !capture.BeforeExists {
				oldPath = "/dev/null"
				result.Files[index].Status = "A"
			}
			patch, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: diffLines(capture.BeforeText), B: diffLines(capture.AfterText), FromFile: oldPath, ToFile: "b/" + file.Path, Context: 3})
			if err != nil {
				return result, nil, err
			}
			patches[file.Path] = patch
		}
		result.WorkspaceState = turnWorkspaceState(result)
	}
	hash := sha256.New()
	for _, file := range result.Files {
		hash.Write([]byte(file.Path))
		hash.Write([]byte(patches[file.Path]))
		if capture := captures[file.Path]; capture != nil {
			raw, _ := json.Marshal(capture)
			hash.Write(raw)
		}
	}
	result.Revision = hex.EncodeToString(hash.Sum(nil))
	return result, patches, nil
}
