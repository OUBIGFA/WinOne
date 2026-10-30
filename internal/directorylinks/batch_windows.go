//go:build windows

package directorylinks

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BatchRequest struct {
	Sources   []string `json:"sources"`
	Directory string   `json:"directory"`
	Kind      string   `json:"kind"`
	Relative  bool     `json:"relative"`
	LinkName  string   `json:"linkName,omitempty"`
}
type BatchPlan struct {
	ID    string `json:"id"`
	Items []Plan `json:"items"`
}
type ExecutionResult struct {
	ID        string `json:"id"`
	Attempted bool   `json:"attempted"`
	Error     string `json:"error,omitempty"`
}
type BatchResult struct {
	Records   []Record `json:"records"`
	Completed int      `json:"completed"`
	Total     int      `json:"total"`
}

// PreviewBatch validates the complete batch before any filesystem mutation,
// including case-insensitive collisions from different source folders.
func (m *Manager) PreviewBatch(r BatchRequest) (BatchPlan, error) {
	if len(r.Sources) == 0 || len(r.Sources) > 200 {
		return BatchPlan{}, errors.New("每次请选择 1–200 个源文件或文件夹")
	}
	// The merged creation form retains the single-entry naming capability.
	// Reject an ambiguous custom name for multiple sources; Preview still owns
	// path validation, collisions and source/destination identity checks.
	if r.LinkName != "" && len(r.Sources) != 1 {
		return BatchPlan{}, errors.New("自定义链接名称仅适用于一个源项目")
	}
	seen := map[string]bool{}
	plans := []Plan{}
	for _, source := range r.Sources {
		name := filepath.Base(strings.TrimRight(strings.TrimSpace(source), `\/`))
		if r.LinkName != "" {
			name = r.LinkName
		}
		key := strings.ToLower(name)
		if seen[key] {
			return BatchPlan{}, fmt.Errorf("多个源项目会创建同名入口：%s", name)
		}
		seen[key] = true
		p, err := m.Preview(Request{Source: source, Directory: r.Directory, LinkName: name, Kind: r.Kind, Relative: r.Relative})
		if err != nil {
			return BatchPlan{}, fmt.Errorf("%s：%w", source, err)
		}
		plans = append(plans, p)
		m.mu.Lock()
		delete(m.plans, p.ID)
		m.mu.Unlock()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.batches == nil {
		m.batches = map[string][]Plan{}
	}
	if len(m.batches) >= 32 {
		m.batches = map[string][]Plan{}
	}
	id := rand.Text()
	m.batches[id] = plans
	return BatchPlan{ID: id, Items: plans}, nil
}

// ExecuteBatch matches the reviewed upstream workflow: stop on the first
// failure and report which entries were attempted, never conceal a partial run.
func ExecuteBatch(plans []Plan) []ExecutionResult {
	results := make([]ExecutionResult, len(plans))
	stopped := false
	for i, p := range plans {
		results[i].ID = p.ID
		if stopped {
			results[i].Error = "此前项目未完成，本项未执行"
			continue
		}
		results[i].Attempted = true
		if err := Execute(p); err != nil {
			results[i].Error = err.Error()
			stopped = true
		}
	}
	return results
}

func (m *Manager) CreateBatchWith(ctx context.Context, token string, execute func([]Plan) ([]ExecutionResult, error)) (BatchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	plans, ok := m.batches[token]
	if m.migrating {
		return BatchResult{}, errMigrationBusy
	}
	if !ok {
		return BatchResult{}, errors.New("批量预览已过期，请重新预览")
	}
	delete(m.batches, token)
	if len(m.records)+len(plans) > maxRecords {
		return BatchResult{}, errors.New("链接历史已满")
	}
	for _, p := range plans {
		delete(m.plans, p.ID)
		expiry, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
		if err != nil || time.Now().After(expiry) {
			return BatchResult{}, errors.New("预览已过期")
		}
		if err = verifyPlan(p); err != nil {
			return BatchResult{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return BatchResult{}, err
	}
	start := len(m.records)
	for _, p := range plans {
		m.records = append(m.records, Record{ID: rand.Text(), Name: p.Name, LinkPath: p.LinkPath, TargetPath: p.Source, Kind: p.Kind, ObjectType: p.ObjectType, TargetText: p.TargetText, Relative: p.Relative, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "creating", Reason: "creating", TargetIdentity: p.SourceIdentity})
	}
	if err := m.save(); err != nil {
		m.records = m.records[:start]
		return BatchResult{}, err
	}
	results, runErr := execute(plans)
	if len(results) != len(plans) {
		runErr = errors.Join(runErr, errors.New("批量创建结果数量不匹配"))
	}
	completed := 0
	for i, p := range plans {
		r := m.records[start+i]
		valid := i < len(results) && results[i].ID == p.ID
		if !valid {
			r.Status, r.Reason = "unknown", "created-unverified"
			if runErr != nil {
				r.Detail = runErr.Error()
			}
		} else if !results[i].Attempted {
			r.Status, r.Reason, r.Detail = "failed", "not-attempted", results[i].Error
		} else if results[i].Error != "" {
			r.Status, r.Reason, r.Detail = "failed", "create-failed", results[i].Error
		} else {
			entry, err := openState(p.LinkPath, false, false)
			if err != nil {
				r.Status, r.Reason, r.Detail = "unknown", "created-unverified", err.Error()
			} else {
				r.EntryIdentity = entry.identity
				entry.close()
				r = checked(r)
				if r.Status == "active" {
					completed++
				}
			}
		}
		r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
		m.records[start+i] = r
	}
	result := BatchResult{Records: append([]Record{}, m.records[start:]...), Completed: completed, Total: len(plans)}
	return result, errors.Join(runErr, m.save())
}

// ExportPlan produces reviewable commands without executing them. Escaping is
// specific to a batch file; it is not reused for runtime native API calls.
func (m *Manager) ExportPlan(token, directory string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	plans := m.batches[token]
	if p, ok := m.plans[token]; ok {
		plans = []Plan{p}
	}
	if len(plans) == 0 {
		return "", errors.New("请先预览创建计划")
	}
	directory, err := localPath(directory)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.Join(errors.New("请选择现有导出文件夹"), err)
	}
	quote := func(s string) string { return `"` + strings.ReplaceAll(s, "%", "%%") + `"` }
	var text strings.Builder
	text.WriteString("@echo off\r\nsetlocal DisableDelayedExpansion\r\nchcp 65001 >NUL\r\nREM WinOne reviewed link plan. Recheck destinations before running.\r\n")
	for _, p := range plans {
		kind := ""
		if p.Kind == "hardlink" {
			kind = " /H"
		} else if p.Kind == "junction" {
			kind = " /J"
		} else if p.ObjectType == "directory" {
			kind = " /D"
		}
		text.WriteString("mklink" + kind + " " + quote(p.LinkPath) + " " + quote(p.TargetText) + "\r\nif errorlevel 1 exit /b %errorlevel%\r\n")
	}
	text.WriteString("exit /b 0\r\n")
	path := filepath.Join(directory, "WinOne-links-"+rand.Text()+".cmd")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.WriteString(text.String())
	if err = errors.Join(writeErr, f.Close()); err != nil {
		return "", err
	}
	return path, nil
}
