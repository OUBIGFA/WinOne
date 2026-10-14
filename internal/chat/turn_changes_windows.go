//go:build windows

package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"winone/internal/settings"
)

type recordedFileChange struct {
	Path, Status, Patch string
	Captured            bool    `json:"captured"`
	BeforeExists        bool    `json:"beforeExists"`
	BeforeText          *string `json:"beforeText"`
	AfterText           *string `json:"afterText"`
	ResolvedPath        string  `json:"resolvedPath"`
}
type capturedTurnFile struct {
	Path, ResolvedPath, BeforeText, AfterText string
	BeforeExists, Valid, Updated              bool
}

func mergeTurnCapture(captures map[string]*capturedTurnFile, key, path string, record *recordedFileChange) {
	current := captures[key]
	valid := record != nil && record.Captured && record.BeforeText != nil && record.AfterText != nil && record.ResolvedPath != ""
	if current == nil {
		current = &capturedTurnFile{Path: path, Valid: valid}
		captures[key] = current
		if valid {
			current.BeforeExists = record.BeforeExists
			current.BeforeText = *record.BeforeText
			current.ResolvedPath = record.ResolvedPath
		}
	}
	if !valid {
		current.Valid = false
		return
	}
	if current.Updated && (!record.BeforeExists || current.AfterText != *record.BeforeText) {
		current.Valid = false
	}
	if !strings.EqualFold(current.ResolvedPath, record.ResolvedPath) {
		current.Valid = false
	}
	current.AfterText = *record.AfterText
	current.Updated = true
}
func diffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	} else {
		lines[len(lines)-1] += "\n"
	}
	return lines
}

// Check the same resolved object recorded by the successful tool. A retargeted
// junction must never redirect undo to an unrelated file.
func turnFileMatches(file *capturedTurnFile, exists bool, text string) bool {
	resolved, err := resolvePreviewPath(file.Path)
	if errors.Is(err, os.ErrNotExist) && !exists {
		parent, e := resolvePreviewPath(filepath.Dir(file.Path))
		return e == nil && strings.EqualFold(filepath.Join(parent, filepath.Base(file.Path)), file.ResolvedPath)
	}
	if err != nil || !exists || !strings.EqualFold(resolved, file.ResolvedPath) {
		return false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len([]byte(text))) {
		return false
	}
	body, err := os.ReadFile(resolved)
	return err == nil && string(body) == text
}
func turnWorkspaceState(snapshot ReviewSnapshot) string {
	if !snapshot.IsReversible {
		return "partial"
	}
	applied, undone := true, true
	for _, file := range snapshot.captured {
		applied = applied && turnFileMatches(file, true, file.AfterText)
		undone = undone && turnFileMatches(file, file.BeforeExists, file.BeforeText)
	}
	if applied {
		return "applied"
	}
	if undone {
		return "undone"
	}
	return "conflict"
}

// ApplyTurnReview accepts only a saved turn and its capture revision. Expected
// contents are checked for the entire set before writing and again per file.
// Added files go through the caller's checked recycle operation on undo.
func (s *Store) ApplyTurnReview(ctx context.Context, id, turnID, revision, action string, recycleFile func(string, func() error) error) (ReviewSnapshot, error) {
	s.reviewMutation.Lock()
	defer s.reviewMutation.Unlock()
	if action != "undo" && action != "reapply" {
		return ReviewSnapshot{}, errors.New("未知文件恢复操作")
	}
	snapshot, _, err := s.TurnReview(id, turnID)
	if err != nil {
		return snapshot, err
	}
	if revision == "" || revision != snapshot.Revision {
		return snapshot, errors.New("文件改动记录已变化，请刷新后重试")
	}
	if !snapshot.IsReversible {
		return snapshot, errors.New("本轮缺少完整的修改前后内容，无法恢复")
	}
	expectedState := "applied"
	if action == "reapply" {
		expectedState = "undone"
	}
	if snapshot.WorkspaceState != expectedState {
		return snapshot, errors.New("文件在本轮后已被修改，已保留当前内容；请先核对差异")
	}
	type change struct {
		file                 *capturedTurnFile
		oldExists, newExists bool
		oldText, newText     string
	}
	plan := make([]change, 0, len(snapshot.Files))
	for _, file := range snapshot.Files {
		capture := snapshot.captured[file.Path]
		item := change{capture, true, capture.BeforeExists, capture.AfterText, capture.BeforeText}
		if action == "reapply" {
			item.oldExists, item.newExists = item.newExists, item.oldExists
			item.oldText, item.newText = item.newText, item.oldText
		}
		if !item.newExists && recycleFile == nil {
			return snapshot, errors.New("文件回收操作不可用")
		}
		plan = append(plan, item)
	}
	apply := func(item change) error {
		check := func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !turnFileMatches(item.file, item.oldExists, item.oldText) {
				return fmt.Errorf("文件已变化：%s", item.file.Path)
			}
			return nil
		}
		if err := check(); err != nil {
			return err
		}
		if !item.newExists {
			return recycleFile(item.file.Path, check)
		}
		if !item.oldExists {
			file, err := os.OpenFile(item.file.ResolvedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			_, writeErr := file.WriteString(item.newText)
			return errors.Join(writeErr, file.Close())
		}
		info, err := os.Stat(item.file.ResolvedPath)
		if err != nil {
			return err
		}
		if err = settings.WriteAtomic(item.file.ResolvedPath, []byte(item.newText)); err != nil {
			return err
		}
		return os.Chmod(item.file.ResolvedPath, info.Mode().Perm())
	}
	for index, item := range plan {
		if err = apply(item); err != nil {
			// Roll back only contents this action itself wrote. A concurrent edit is
			// never overwritten merely to claim an all-or-nothing result.
			for prior := index - 1; prior >= 0; prior-- {
				undo := plan[prior]
				undo.oldExists, undo.newExists = undo.newExists, undo.oldExists
				undo.oldText, undo.newText = undo.newText, undo.oldText
				err = errors.Join(err, apply(undo))
			}
			return snapshot, err
		}
	}
	snapshot.WorkspaceState = turnWorkspaceState(snapshot)
	return snapshot, nil
}
