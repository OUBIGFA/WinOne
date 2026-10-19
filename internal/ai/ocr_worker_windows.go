//go:build windows

package ai

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type ocrWorker struct {
	process *aiProcess
	output  *bufio.Scanner
}

// LocalOCR owns one serialized OCR worker for the Host. An enabled local OCR
// module keeps model sessions ready; ad-hoc callers retain the bounded idle
// lifetime. No screenshot or recognized text is retained while waiting.
// A cancelled request kills its worker; a waiting request cannot kill another.
type LocalOCR struct {
	lifetime    context.Context
	runtime     Translator
	gate        chan struct{}
	worker      *ocrWorker
	idle        *time.Timer
	generation  uint64
	idleTimeout time.Duration
	standby     atomic.Bool
	managed     atomic.Bool // A disabled managed module must not keep an ad-hoc idle lease.
	ready       atomic.Bool
	processID   atomic.Int64
}

func NewLocalOCR(lifetime context.Context, runtime Translator) *LocalOCR {
	s := &LocalOCR{lifetime: lifetime, runtime: runtime, gate: make(chan struct{}, 1), idleTimeout: time.Minute}
	context.AfterFunc(lifetime, func() {
		s.gate <- struct{}{}
		defer func() { <-s.gate }()
		s.retire()
	})
	return s
}

// Status is a read-only readiness signal; observing it never starts a worker.
// It lets diagnostics distinguish background initialization from first-use work.
func (s *LocalOCR) Status() (ready bool, pid int) {
	return s.lifetime.Err() == nil && s.ready.Load(), int(s.processID.Load())
}

// SetStandby publishes intent before scheduling work, so a late warmup cannot
// resurrect a disabled module. Completion is buffered; callers may observe it
// without making the settings or startup thread wait for model initialization.
func (s *LocalOCR) SetStandby(enabled bool) <-chan error {
	s.managed.Store(true)
	previous := s.standby.Swap(enabled)
	done := make(chan error, 1)
	if enabled && previous {
		done <- nil
		return done
	}
	go func() {
		if enabled {
			done <- s.Prepare(s.lifetime)
			return
		}
		select {
		case s.gate <- struct{}{}:
			defer func() { <-s.gate }()
			if !s.standby.Load() {
				s.retire()
			}
			done <- nil
		case <-s.lifetime.Done():
			done <- s.lifetime.Err()
		}
	}()
	return done
}

// Prepare runs a tiny generated-image inference, including both lazy model
// sessions. Calling it again after resume faults their pages back in without
// reading the desktop, clipboard, credentials or any user's image.
func (s *LocalOCR) Prepare(ctx context.Context) error {
	_, err := s.exchange(ctx, nil, true)
	return err
}

// retire is called with gate held. The Job Object also covers abrupt Host exit;
// waiting here reaps the interpreter before another worker can replace it.
func (s *LocalOCR) retire() {
	s.ready.Store(false)
	s.processID.Store(0)
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	if s.worker == nil {
		return
	}
	p := s.worker.process
	s.worker = nil
	p.input.Close()
	p.closeJob()
	<-p.finished
}

func (s *LocalOCR) start() error {
	if s.worker != nil {
		select {
		case <-s.worker.process.finished:
			s.retire()
		default:
			return nil
		}
	}
	t := s.runtime
	root := filepath.Clean(filepath.Join(filepath.Dir(t.Executable), "..", "..", "capture", "Ocr"))
	python, script := filepath.Join(root, "python", "python.exe"), filepath.Join(root, "recognize.py")
	for _, path := range []string{python, script} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return errors.New("OCR 文件缺失，请修复安装")
		}
	}
	cmd := exec.CommandContext(s.lifetime, python, "-B", script, filepath.Join(root, "models"), "--serve")
	cmd.Dir = root
	cmd.Env = append(isolatedEnvironment(t.DataDirectory), "PYTHONUTF8=1")
	p, err := startOwnedProcess(s.lifetime, cmd)
	if err != nil {
		return errors.New("OCR 无法启动，请修复安装")
	}
	output := bufio.NewScanner(p.output)
	output.Buffer(make([]byte, 4096), 1024*1024)
	s.worker = &ocrWorker{process: p, output: output}
	s.processID.Store(int64(p.cmd.Process.Pid))
	go func() {
		_ = p.cmd.Wait()
		s.ready.Store(false)
		s.processID.Store(0)
		p.closeJob()
		close(p.finished)
	}()
	return nil
}

func (s *LocalOCR) Recognize(ctx context.Context, image []byte) (string, error) {
	return s.exchange(ctx, image, false)
}

func (s *LocalOCR) exchange(ctx context.Context, image []byte, prepare bool) (string, error) {
	if !prepare && (len(image) == 0 || len(image) > 32<<20) {
		return "", errors.New("OCR 图片为空或过大")
	}
	if !filepath.IsAbs(s.runtime.Executable) || !filepath.IsAbs(s.runtime.DataDirectory) {
		return "", errors.New("OCR 运行路径无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.lifetime.Done():
		return "", s.lifetime.Err()
	}
	defer func() { <-s.gate }()
	if err := s.lifetime.Err(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if prepare && !s.standby.Load() {
		return "", nil
	}
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	s.generation++
	if err := s.start(); err != nil {
		return "", err
	}
	worker, generation := s.worker, s.generation
	// Each timer carries the lease it was created for. A timer already firing
	// when the next request starts must not retire that newly active request.
	defer func() {
		if s.worker != worker || s.standby.Load() {
			return
		}
		if s.managed.Load() {
			// A result captured before disabling may finish after the stop job.
			// Let it finish, then release; never re-arm a minute of residency.
			s.retire()
			return
		}
		s.idle = time.AfterFunc(s.idleTimeout, func() {
			s.gate <- struct{}{}
			defer func() { <-s.gate }()
			if s.worker == worker && s.generation == generation && !s.standby.Load() {
				s.retire()
			}
		})
	}()
	cancelled := make(chan struct{})
	stopCancellation := context.AfterFunc(ctx, func() { worker.process.closeJob(); close(cancelled) })
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(image)))
	_, err := worker.process.input.Write(header[:])
	if err == nil {
		_, err = worker.process.input.Write(image)
	}
	read := err == nil && worker.output.Scan()
	// Do not hand the worker to the next request while its predecessor's
	// cancellation callback may still be closing that same process.
	if !stopCancellation() {
		<-cancelled
	}
	if failure := ctx.Err(); failure != nil {
		s.retire()
		return "", failure
	}
	if failure := s.lifetime.Err(); failure != nil {
		s.retire()
		return "", failure
	}
	if !read {
		s.retire()
		return "", errors.New("OCR 响应无法读取或过大，请重试")
	}
	var result struct {
		Text     *string `json:"text"`
		Prepared bool    `json:"prepared"`
		Error    string  `json:"error"`
	}
	if json.Unmarshal(worker.output.Bytes(), &result) != nil || result.Error == "" && (prepare && !result.Prepared || !prepare && result.Text == nil) {
		s.retire()
		return "", errors.New("OCR 响应无效，请重试")
	}
	if result.Error != "" {
		if prepare {
			s.retire()
			return "", errors.New("OCR 预热失败，请修复安装或重试")
		}
		return "", errors.New("OCR 识别失败，请重试")
	}
	s.ready.Store(true)
	if prepare {
		return "", nil
	}
	if strings.TrimSpace(*result.Text) == "" {
		return "", errors.New("未识别到文字，请重新选择区域")
	}
	return *result.Text, nil
}
