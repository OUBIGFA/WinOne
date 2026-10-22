//go:build windows

package selection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

type Selection struct {
	Status     string       `json:"status"`
	Text       string       `json:"text,omitempty"`
	Rectangles [][4]float64 `json:"rectangles,omitempty"`
}

var readerSlot = make(chan struct{}, 1)

// ReadSelection isolates potentially unresponsive accessibility providers in a
// one-shot process. Its input is withheld until kill-on-close ownership is set.
const defaultReaderTimeout = 1500 * time.Millisecond

func ReadSelection(ctx context.Context, executable string, event Event) (Selection, error) {
	return readSelection(ctx, executable, event, defaultReaderTimeout)
}

func readSelection(ctx context.Context, executable string, event Event, timeout time.Duration) (Selection, error) {
	if !filepath.IsAbs(executable) {
		return Selection{}, errors.New("划词读取程序路径无效")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// Rapid keyboard selection replaces pending work without accumulating
	// accessibility processes. Waiting for the slot is part of the deadline.
	select {
	case readerSlot <- struct{}{}:
		defer func() { <-readerSlot }()
	case <-ctx.Done():
		return Selection{}, ctx.Err()
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return Selection{}, err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return Selection{}, err
	}
	cmd := exec.CommandContext(ctx, executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	input, err := cmd.StdinPipe()
	if err != nil {
		return Selection{}, err
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return Selection{}, err
	}
	cmd.Stderr = io.Discard
	if os.Getenv("WINONE_SELECTION_DIAGNOSTICS") == "1" {
		cmd.Stderr = os.Stderr
	}
	if err = cmd.Start(); err != nil {
		return Selection{}, err
	}
	// Cancellation closes the job, covering children even if COM never returns.
	done := make(chan struct{})
	cancellationDone := make(chan struct{})
	defer func() { close(done); <-cancellationDone }()
	go func() {
		defer close(cancellationDone)
		select {
		case <-ctx.Done():
			windows.TerminateJobObject(job, 1)
		case <-done:
		}
	}()
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, handle)
		windows.CloseHandle(handle)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return Selection{}, err
	}
	request := map[string]any{"window": uint64(event.Window), "pid": event.ProcessID}
	if !event.Keyboard {
		request["point"] = event.Point
		if event.OriginValid {
			request["origin"] = event.Origin
		}
	}
	if err = json.NewEncoder(input).Encode(request); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return Selection{}, err
	}
	input.Close()
	data, readErr := io.ReadAll(io.LimitReader(output, (1<<20)+1))
	if readErr != nil || len(data) > 1<<20 {
		cmd.Process.Kill()
		cmd.Wait()
		return Selection{}, errors.New("划词读取响应无效")
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return Selection{}, ctx.Err()
	}
	if err != nil {
		return Selection{}, err
	}
	var result Selection
	if json.Unmarshal(data, &result) != nil || len(result.Text) > 65536 || len(result.Rectangles) > 4096 {
		return Selection{}, errors.New("划词读取响应无效")
	}
	switch result.Status {
	case "selected":
		if result.Text == "" {
			return Selection{}, errors.New("划词读取返回空内容")
		}
	case "empty", "sensitive", "stale", "unsupported", "unrelated", "too_large", "unavailable":
		if result.Text != "" {
			return Selection{}, errors.New("划词读取响应无效")
		}
	default:
		return Selection{}, errors.New("划词读取状态无效")
	}
	// Opt-in diagnostics expose the outcome only, never the selected text.
	if os.Getenv("WINONE_SELECTION_DIAGNOSTICS") == "1" {
		fmt.Fprintf(os.Stderr, "selection response: status=%s\n", result.Status)
	}
	return result, nil
}
