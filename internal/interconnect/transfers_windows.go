//go:build windows

package interconnect

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"winone/internal/configjson"
	"winone/internal/settings"
)

// Transfers supervises task processes and keeps history/receipts. It does not
// implement a file-sharing protocol. Sender-only store receipts stay on disk
// and are stripped from snapshots returned to the desktop.
type Transfers struct {
	mu               sync.Mutex
	saveMu           sync.Mutex
	opMu             sync.Mutex
	executable, data string
	jobs             map[string]*transferJob
	closing          bool
	saveError        string
	notify           func(mode, status string) error
}
type transferRecord struct {
	ID      string         `json:"id"`
	Created time.Time      `json:"created"`
	Spec    map[string]any `json:"spec"`
	State   map[string]any `json:"state"`
}
type transferJob struct {
	mu      sync.Mutex
	record  transferRecord
	process *worker
	done    chan struct{}
}

func NewTransfers(executable, data string) (*Transfers, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(data) {
		return nil, errors.New("传输运行路径无效")
	}
	m := &Transfers{executable: executable, data: data, jobs: map[string]*transferJob{}}
	content, err := os.ReadFile(filepath.Join(data, "tasks.json"))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var saved struct {
		Tasks []transferRecord `json:"tasks"`
	}
	if err = configjson.DecodeObject(content, &saved); err != nil {
		return nil, err
	}
	for _, record := range saved.Tasks {
		if record.ID == "" || strings.ContainsAny(record.ID, "/\\.") || record.State == nil || record.Spec == nil {
			return nil, errors.New("传输记录损坏")
		}
		if record.State["status"] == "running" {
			record.State["status"] = "interrupted"
			record.State["error"] = "应用已退出，请重新开始传输"
			delete(record.State, "prompt")
		}
		done := make(chan struct{})
		close(done)
		m.jobs[record.ID] = &transferJob{record: record, done: done}
	}
	return m, nil
}
func (m *Transfers) all() []*transferJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]*transferJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}
func (m *Transfers) records(public bool) ([]transferRecord, error) {
	m.mu.Lock()
	saveError := m.saveError
	m.mu.Unlock()
	records := []transferRecord{}
	for _, job := range m.all() {
		job.mu.Lock()
		data, err := json.Marshal(job.record)
		job.mu.Unlock()
		if err != nil {
			return nil, err
		}
		var record transferRecord
		if err = json.Unmarshal(data, &record); err != nil {
			return nil, err
		}
		if public {
			if saveError != "" {
				record.State["persistenceError"] = saveError
			}
			delete(record.Spec, "uploadToken")
			delete(record.Spec, "dataDir")
			if result, ok := record.State["result"].(map[string]any); ok {
				delete(result, "uploadToken")
			}
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Created.After(records[j].Created) })
	return records, nil
}
func (m *Transfers) persist() (err error) {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.saveError = ""
		if err != nil {
			m.saveError = "保存传输记录失败: " + err.Error()
		}
	}()
	records, err := m.records(false)
	if err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"tasks": records})
	if err != nil {
		return err
	}
	return settings.WriteAtomic(filepath.Join(m.data, "tasks.json"), data)
}
func (m *Transfers) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if method == "transfer_files" {
		var request struct {
			Paths []string `json:"paths"`
		}
		if err := configjson.DecodeObject(params, &request); err != nil {
			return nil, err
		}
		paths := request.Paths
		if len(paths) > 200 {
			return nil, errors.New("每次最多选择 200 项")
		}
		files := []map[string]any{}
		for _, path := range paths {
			if !filepath.IsAbs(path) {
				return nil, errors.New("需要绝对路径")
			}
			info, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			files = append(files, map[string]any{"path": path, "name": info.Name(), "size": info.Size(), "type": "application/octet-stream"})
		}
		return json.Marshal(files)
	}
	if method == "transfer_list" {
		m.mu.Lock()
		retry := m.saveError != ""
		m.mu.Unlock()
		// Retry a failed history write on refresh, while preserving the real
		// operation result and reporting any remaining persistence error.
		if retry {
			_ = m.persist()
		}
		records, err := m.records(true)
		if err != nil {
			return nil, err
		}
		return json.Marshal(records)
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if method == "transfer_start" {
		var spec map[string]any
		if err := configDecode(params, &spec); err != nil {
			return nil, err
		}
		return m.start(ctx, spec)
	}
	var command struct {
		ID       string   `json:"id"`
		PromptID uint64   `json:"promptId"`
		Answer   string   `json:"answer"`
		Paths    []string `json:"paths,omitempty"`
		ItemID   string   `json:"itemId,omitempty"`
		Text     string   `json:"text,omitempty"`
		Path     string   `json:"path,omitempty"`
		Replace  bool     `json:"replace,omitempty"`
	}
	if err := configjson.DecodeObject(params, &command); err != nil {
		return nil, err
	}
	m.mu.Lock()
	job := m.jobs[command.ID]
	m.mu.Unlock()
	if job == nil {
		return nil, errors.New("传输任务不存在")
	}
	switch method {
	case "share_code_restart", "share_code_stop", "share_code_answer", "share_add_text", "share_save_as":
		job.mu.Lock()
		defer job.mu.Unlock()
		if job.process == nil || job.record.Spec["mode"] != "share_server" {
			return nil, errors.New("共享未运行")
		}
		var value any
		switch method {
		case "share_code_answer":
			value = map[string]any{"id": command.PromptID, "answer": command.Answer}
		case "share_add_text":
			value = command.Text
		case "share_save_as":
			value = map[string]any{"id": command.ItemID, "path": command.Path, "replace": command.Replace}
		}
		return job.process.call(ctx, strings.TrimPrefix(method, "share_"), value)
	case "transfer_text", "transfer_text_open":
		job.mu.Lock()
		data, _ := job.record.Spec["dataDir"].(string)
		result, _ := job.record.State["result"].(map[string]any)
		available := job.record.Spec["mode"] == "receive" && job.record.State["status"] == "completed" && result["textPath"] != nil
		job.mu.Unlock()
		if !available || !filepath.IsAbs(data) {
			return nil, errors.New("没有可用的接收文本")
		}
		path := filepath.Join(data, "received-text.txt")
		if method == "transfer_text_open" {
			return json.RawMessage("null"), windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(path), nil, nil, windows.SW_SHOWNORMAL)
		}
		stat, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if stat.Size() > 1<<20 {
			return nil, errors.New("文本超过 1 MiB，请打开文本后复制全文")
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return json.Marshal(string(text))
	case "share_add", "share_remove":
		job.mu.Lock()
		defer job.mu.Unlock()
		if job.process == nil || job.record.Spec["mode"] != "share_server" {
			return nil, errors.New("共享未运行")
		}
		if method == "share_add" {
			return job.process.call(ctx, "add", command.Paths)
		}
		return job.process.call(ctx, "remove", command.ItemID)
	case "transfer_restart":
		job.mu.Lock()
		if job.process != nil {
			job.mu.Unlock()
			return nil, errors.New("任务仍在运行")
		}
		mode, _ := job.record.Spec["mode"].(string)
		if mode != "share_server" && mode != "tunnel_server" && mode != "share_client" && mode != "tunnel_client" {
			job.mu.Unlock()
			return nil, errors.New("请通过再次使用重新提交传输")
		}
		data, marshalErr := json.Marshal(job.record.Spec)
		job.mu.Unlock()
		if marshalErr != nil {
			return nil, marshalErr
		}
		var spec map[string]any
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
		for _, other := range m.all() {
			other.mu.Lock()
			conflict := other.process != nil && other.record.Spec["dataDir"] == spec["dataDir"]
			other.mu.Unlock()
			if conflict {
				return nil, errors.New("此服务已在另一任务中恢复")
			}
		}
		spec["_resumeData"] = spec["dataDir"]
		spec["resume"] = true
		return m.start(ctx, spec)
	case "transfer_stop":
		var stopErr error
		job.mu.Lock()
		if job.process != nil {
			_, err := job.process.call(ctx, "stop", nil)
			closeErr := job.process.close()
			job.process = nil
			job.record.State["status"] = "cancelled"
			delete(job.record.State, "prompt")
			job.mu.Unlock()
			stopErr = errors.Join(err, closeErr)
		} else {
			job.mu.Unlock()
		}
		return json.RawMessage("null"), errors.Join(stopErr, m.persist())
	case "transfer_answer":
		job.mu.Lock()
		defer job.mu.Unlock()
		if job.process == nil {
			return nil, errors.New("传输已结束")
		}
		return job.process.call(ctx, "answer", map[string]any{"id": command.PromptID, "answer": command.Answer})
	case "transfer_remove":
		job.mu.Lock()
		active := job.process != nil
		result, _ := job.record.State["result"].(map[string]any)
		hasReceipt := result != nil && result["uploadToken"] != "" && result["uploadToken"] != nil
		job.mu.Unlock()
		if active {
			return nil, errors.New("请先取消传输")
		}
		if hasReceipt {
			return nil, errors.New("请先撤销托管分享，保留撤销凭据")
		}
		m.mu.Lock()
		delete(m.jobs, command.ID)
		m.mu.Unlock()
		if err := m.persist(); err != nil {
			m.mu.Lock()
			m.jobs[command.ID] = job
			m.mu.Unlock()
			return nil, err
		}
		return json.RawMessage("null"), nil
	case "share_revoke":
		job.mu.Lock()
		result, _ := job.record.State["result"].(map[string]any)
		code, token := result["code"], result["uploadToken"]
		job.mu.Unlock()
		if token == nil || token == "" {
			return nil, errors.New("未找到分享撤销凭据")
		}
		return m.start(ctx, map[string]any{"mode": "store_revoke", "code": code, "uploadToken": token, "revokeTask": command.ID})
	default:
		return nil, errors.New("不支持的传输操作")
	}
}
func configDecode(params []byte, spec *map[string]any) error {
	if err := configjson.DecodeObject(params, spec); err != nil {
		return err
	}
	for _, name := range []string{"_resumeData", "resume", "key", "psk", "shareURL", "revokeTask", "uploadToken"} {
		if _, ok := (*spec)[name]; ok {
			return errors.New("请求包含内部字段")
		}
	}
	mode, _ := (*spec)["mode"].(string)
	switch mode {
	case "send", "receive", "store_send", "store_inspect", "store_receive", "share_server", "share_client", "tunnel_server", "tunnel_client":
	default:
		return errors.New("不支持的传输类型")
	}
	return nil
}
func (m *Transfers) start(ctx context.Context, spec map[string]any) (json.RawMessage, error) {
	id := rand.Text()
	spec["dataDir"] = filepath.Join(m.data, "tasks", id)
	if existing, ok := spec["_resumeData"]; ok {
		spec["dataDir"] = existing
		delete(spec, "_resumeData")
	}
	if mode := spec["mode"]; mode == "receive" || mode == "store_receive" {
		if output, _ := spec["outputDir"].(string); output == "" {
			spec["outputDir"] = filepath.Join(m.data, "received")
		}
	}
	// Private fields are consumed by the Host, not passed as engine options.
	engineSpec := map[string]any{}
	for key, value := range spec {
		if key != "revokeTask" {
			engineSpec[key] = value
		}
	}
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return nil, errors.New("互联助手正在退出")
	}
	executable := m.executable
	mode, _ := spec["mode"].(string)
	if strings.HasPrefix(mode, "share_") || strings.HasPrefix(mode, "tunnel_") {
		executable = filepath.Join(filepath.Dir(executable), "WinOne.Interconnect.Peer.exe")
	}
	p, err := startWorker(executable)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	job := &transferJob{record: transferRecord{ID: id, Created: time.Now(), Spec: spec, State: map[string]any{"status": "running"}}, process: p, done: make(chan struct{})}
	m.jobs[id] = job
	m.mu.Unlock()
	if err = m.persist(); err != nil {
		job.mu.Lock()
		p.retire()
		p.close()
		job.process = nil
		job.record.State = map[string]any{"status": "failed", "error": err.Error()}
		job.mu.Unlock()
		close(job.done)
		return nil, err
	}
	// Save the task location before any operation can publish data or create
	// remote receipts. The child has not received a start command before this.
	if _, err = p.call(ctx, "start", engineSpec); err != nil {
		job.mu.Lock()
		p.retire()
		p.close()
		job.process = nil
		job.record.State = map[string]any{"status": "failed", "error": err.Error()}
		job.mu.Unlock()
		close(job.done)
		return nil, errors.Join(err, m.persist())
	}
	go m.monitor(job)
	return json.Marshal(map[string]string{"id": id})
}
func (m *Transfers) monitor(job *transferJob) {
	defer close(job.done)
	for {
		job.mu.Lock()
		p := job.process
		if p == nil {
			job.mu.Unlock()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		raw, err := p.call(ctx, "snapshot", nil)
		cancel()
		state := map[string]any{}
		if err == nil {
			err = configjson.DecodeObject(raw, &state)
		}
		if err != nil {
			state = map[string]any{"status": "failed", "error": err.Error()}
		}
		job.record.State = state
		finished := state["status"] != "running"
		if finished {
			if closeErr := p.close(); closeErr != nil {
				state["cleanupError"] = closeErr.Error()
			}
			job.process = nil
		}
		revokeID, _ := job.record.Spec["revokeTask"].(string)
		completed := state["status"] == "completed"
		job.mu.Unlock()
		if finished {
			mode, _ := job.record.Spec["mode"].(string)
			if m.notify != nil && (mode == "send" || mode == "receive" || mode == "store_send" || mode == "store_receive") && (completed || state["status"] == "failed") {
				if notifyErr := m.notify(mode, state["status"].(string)); notifyErr != nil {
					job.mu.Lock()
					job.record.State["notificationError"] = notifyErr.Error()
					job.mu.Unlock()
				}
			}
			if completed && revokeID != "" {
				m.mu.Lock()
				original := m.jobs[revokeID]
				m.mu.Unlock()
				if original != nil {
					original.mu.Lock()
					result, _ := original.record.State["result"].(map[string]any)
					if result != nil {
						delete(result, "uploadToken")
						result["revoked"] = true
					}
					original.mu.Unlock()
				}
			}
			_ = m.persist() // persist records the error for refresh/retry.
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
func (m *Transfers) Close() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	m.closing = true
	m.mu.Unlock()
	var group sync.WaitGroup
	failures := make(chan error, len(m.all()))
	for _, job := range m.all() {
		group.Add(1)
		go func(job *transferJob) {
			defer group.Done()
			job.mu.Lock()
			if job.process != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err := job.process.call(ctx, "stop", nil)
				cancel()
				closeErr := job.process.close()
				job.process = nil
				job.record.State["status"] = "cancelled"
				delete(job.record.State, "prompt")
				if err = errors.Join(err, closeErr); err != nil {
					failures <- err
				}
			}
			job.mu.Unlock()
			<-job.done
		}(job)
	}
	group.Wait()
	close(failures)
	var err error
	for failure := range failures {
		err = errors.Join(err, failure)
	}
	if len(m.all()) > 0 {
		err = errors.Join(err, m.persist())
	}
	return err
}
