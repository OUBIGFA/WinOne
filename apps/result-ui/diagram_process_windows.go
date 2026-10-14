package resultui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var diagramSlot = make(chan struct{}, 1)

// A one-shot native renderer isolates malformed diagrams and font/layout work
// from input handling. It starts only for a complete Mermaid block.
func renderLocalDiagram(ctx context.Context, executable, cache, source string, dark bool) (*ui.Bitmap, error) {
	if len(source) > 65536 {
		return nil, errors.New("Mermaid 源码过长，请拆分图表")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	select {
	case diagramSlot <- struct{}{}:
		defer func() { <-diagramSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	command := exec.CommandContext(ctx, executable)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if cache != "" {
		command.Env = append(os.Environ(), "XDG_CACHE_HOME="+cache)
	}
	data, _ := json.Marshal(map[string]any{"source": source, "dark": dark})
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	defer input.Close()
	command.Stderr = io.Discard
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = command.Start(); err != nil {
		return nil, errors.New("图表组件无法启动，请修复安装")
	}
	// Bind before sending the diagram. A crashed result host must not leave a
	// renderer or its descendants alive, even when its cancellation code cannot run.
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		windows.CloseHandle(process)
	}
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}
	if _, err = input.Write(data); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}
	input.Close()
	body, readErr := io.ReadAll(io.LimitReader(output, (24<<20)+1))
	if readErr != nil || len(body) > 24<<20 {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, errors.New("图表输出无效或过大")
	}
	waitErr := command.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var result struct {
		PNG   []byte `json:"png"`
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &result) != nil || waitErr != nil {
		return nil, errors.New("图表渲染失败，请检查 Mermaid 源码")
	}
	if result.Error != "" {
		return nil, errors.New("图表渲染失败：" + result.Error)
	}
	size, err := png.DecodeConfig(bytes.NewReader(result.PNG))
	if err != nil || int64(size.Width)*int64(size.Height) > 4_100_000 {
		return nil, errors.New("图表图片无效或过大")
	}
	return ui.DecodeBitmap(result.PNG)
}
func (c *Controller) renderDiagram(source string, dark bool, done func(*ui.Bitmap, error)) {
	go func() {
		exe, err := os.Executable()
		if err != nil {
			c.update(func() { done(nil, err) })
			return
		}
		cache, _ := os.UserCacheDir()
		cache = filepath.Join(cache, "WinOne", "cache", "diagrams")
		if configured := os.Getenv("WINONE_DIAGRAM_CACHE"); configured != "" {
			cache = configured
		}
		bitmap, err := renderLocalDiagram(c.ctx, filepath.Join(filepath.Dir(exe), "WinOne.Diagrams.exe"), cache, source, dark)
		c.update(func() {
			follow := c.Panel.AtEnd()
			done(bitmap, err)
			if follow {
				c.Panel.FollowEnd()
			}
		})
	}()
}
