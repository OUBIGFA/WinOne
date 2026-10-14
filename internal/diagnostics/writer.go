package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Event is the shared format for Host, worker and native-engine diagnostics.
// Callers supply bounded errors/messages, never serialized configuration.
type Event struct {
	Time    string `json:"time"`
	Source  string `json:"source,omitempty"`
	Event   string `json:"event"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func logText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) > 8192 {
		end := 8192
		for !utf8.RuneStart(value[end]) {
			end--
		}
		value = value[:end]
	}
	return value
}

// Record opens the one product log only for the duration of an append. The
// cross-process lock also protects rotation when an elevated worker writes.
func Record(directory string, event Event) error {
	if directory == "" || event.Source == "" || len(event.Source) > 64 || event.Event == "" || len(event.Event) > 128 {
		return errors.New("invalid diagnostic event")
	}
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339Nano)
	}
	event.Message = logText(event.Message)
	event.Error = logText(event.Error)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return withLogLock(directory, func() (failure error) {
		root, err := os.OpenRoot(directory)
		if err != nil {
			return err
		}
		defer root.Close()
		if err = root.Mkdir("logs", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := root.Lstat("logs")
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("log directory must not be a link")
		}
		logs, err := root.OpenRoot("logs")
		if err != nil {
			return err
		}
		defer logs.Close()
		info, err = logs.Lstat("host.log")
		if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("log must be a regular file")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		file, err := logs.OpenFile("host.log", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		defer func() { failure = errors.Join(failure, file.Close()) }()
		info, err = file.Stat()
		if err != nil {
			return err
		}
		if info.Size()+int64(len(data)) > 1<<20 {
			if err = file.Truncate(0); err != nil {
				return err
			}
		}
		if _, err = file.Seek(0, io.SeekEnd); err != nil {
			return err
		}
		if n, err := file.Write(data); err != nil {
			return err
		} else if n != len(data) {
			return io.ErrShortWrite
		}
		return file.Sync()
	})
}

// ModuleDataRoot recognizes the Host's canonical modules/<id> layout. A
// standalone module fixture instead owns its supplied directory in full.
func ModuleDataRoot(directory, id string) string {
	directory = filepath.Clean(directory)
	if filepath.Base(directory) == id && filepath.Base(filepath.Dir(directory)) == "modules" {
		return filepath.Dir(filepath.Dir(directory))
	}
	return directory
}

type Writer struct {
	mu                sync.Mutex
	directory, source string
	pending           []byte
	closed            bool
	failure           error
}

func NewModuleWriter(directory, id string) (*Writer, error) {
	if directory == "" || id == "" || len(id) > 64 {
		return nil, errors.New("invalid module log destination")
	}
	return &Writer{directory: ModuleDataRoot(directory, id), source: id}, nil
}

// Write accepts fragmented stdout/stderr without keeping an unbounded line or
// a permanently open file. Long lines are split at complete UTF-8 boundaries.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.failure != nil {
		return 0, w.failure
	}
	total := len(p)
	for len(p) > 0 {
		n := min(len(p), 8192-len(w.pending))
		segment := p[:n]
		if end := bytes.IndexByte(segment, '\n'); end >= 0 {
			w.pending = append(w.pending, segment[:end]...)
			p = p[end+1:]
			if err := w.flush(len(w.pending)); err != nil {
				return total - len(p), err
			}
			continue
		}
		w.pending = append(w.pending, segment...)
		p = p[n:]
		if len(w.pending) == 8192 {
			cut := len(w.pending)
			start := cut - 1
			for start > 0 && !utf8.RuneStart(w.pending[start]) {
				start--
			}
			if !utf8.FullRune(w.pending[start:]) {
				cut = start
			}
			if cut == 0 {
				cut = len(w.pending)
			}
			if err := w.flush(cut); err != nil {
				return total - len(p), err
			}
		}
	}
	return total, nil
}
func (w *Writer) flush(n int) error {
	message := strings.TrimSuffix(string(w.pending[:n]), "\r")
	if message != "" {
		if err := Record(w.directory, Event{Source: w.source, Event: "output", Message: message}); err != nil {
			w.failure = err
			return err
		}
	}
	w.pending = append(w.pending[:0], w.pending[n:]...)
	return nil
}
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.failure
	}
	w.closed = true
	if w.failure != nil {
		return w.failure
	}
	return w.flush(len(w.pending))
}
