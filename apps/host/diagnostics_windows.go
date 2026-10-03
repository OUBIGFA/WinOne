//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

var hostEventMu sync.Mutex

// Preserve independent runtime warnings; log failures are diagnostic, not a
// reason to hide a restart/permission failure or prevent the Host from running.
func hostRuntimeWarnings(restartWarning, administratorWarning string, logFailure error, locale string) string {
	messages := []string{}
	for _, message := range []string{restartWarning, administratorWarning} {
		if message != "" {
			messages = append(messages, message)
		}
	}
	if logFailure != nil {
		prefix := "启动诊断日志未能写入："
		if locale == "en-US" {
			prefix = "The startup diagnostic log could not be written: "
		}
		messages = append(messages, prefix+logFailure.Error())
	}
	return strings.Join(messages, "\n")
}

// Host events retain bounded error details, not serialized settings.
// Reset before the next record would exceed 1 MiB; module logs remain separate.
func recordHostEvent(directory, event string, failure error) error {
	// Menu actions and lifecycle events can finish on different goroutines.
	// Serialize rotation, seek and append as one operation to retain every event.
	hostEventMu.Lock()
	defer hostEventMu.Unlock()
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
		return errors.New("日志目录不能是链接")
	}
	logs, err := root.OpenRoot("logs")
	if err != nil {
		return err
	}
	defer logs.Close()
	info, err = logs.Lstat("host.log")
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("日志目标不是普通文件")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	detail := ""
	if failure != nil {
		detail = failure.Error()
		if len(detail) > 8192 {
			detail = strings.ToValidUTF8(detail[:8192], "�")
		}
	}
	data, err := json.Marshal(struct {
		Time  string `json:"time"`
		Event string `json:"event"`
		Error string `json:"error,omitempty"`
	}{time.Now().Format(time.RFC3339), event, detail})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := logs.OpenFile("host.log", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
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
	if _, err = file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}
