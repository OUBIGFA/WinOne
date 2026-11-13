//go:build windows

package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Use the bundled interpreter, but a deterministic protocol peer. This tests
// real pipes, cancellation and process ownership without benchmarking models.
func ocrWorkerFixture(t *testing.T, idle time.Duration) (*LocalOCR, context.CancelFunc) {
	t.Helper()
	runtime := os.Getenv("WINONE_OCR_TEST_RUNTIME")
	if runtime == "" {
		t.Skip("set WINONE_OCR_TEST_RUNTIME to the prepared OCR runtime")
	}
	root := t.TempDir()
	python := filepath.Join(root, "capture", "Ocr", "python")
	if err := os.MkdirAll(python, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"python.exe", "python3.dll", "python313.dll", "python313.zip", "python313._pth", "vcruntime140.dll", "vcruntime140_1.dll"} {
		source, destination := filepath.Join(runtime, "python", name), filepath.Join(python, name)
		if _, err := os.Stat(source); os.IsNotExist(err) {
			continue
		}
		// Own the image file as well as the process. Windows cannot unlink a
		// hardlink while another build is running the source interpreter.
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	script := `import json, os, pathlib, struct, sys, time
while True:
    header = sys.stdin.buffer.read(4)
    if not header: break
    length = struct.unpack('<I', header)[0]
    if length == 0:
        print(json.dumps({'prepared': True}), flush=True)
        continue
    payload = sys.stdin.buffer.read(length).decode()
    if payload.startswith('wait:'):
        pathlib.Path(payload[5:]).write_text(str(os.getpid()))
        time.sleep(60)
    if payload == 'crash': os._exit(17)
    if payload == 'oversized':
        print('x' * (2 * 1024 * 1024), flush=True)
    elif payload == 'blank': print(json.dumps({'text': ''}), flush=True)
    else: print(json.dumps({'text': str(os.getpid()) + ':' + payload}), flush=True)
`
	if err := os.WriteFile(filepath.Join(root, "capture", "Ocr", "recognize.py"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	service := NewLocalOCR(ctx, Translator{Executable: filepath.Join(root, "shared", "ai", "WinOne.AI.exe"), DataDirectory: filepath.Join(root, "data")})
	service.idleTimeout = idle
	t.Cleanup(cancel)
	return service, cancel
}

func workerReply(t *testing.T, service *LocalOCR, text string) int {
	t.Helper()
	result, err := service.Recognize(context.Background(), []byte(text))
	if err != nil || !strings.HasSuffix(result, ":"+text) {
		t.Fatalf("OCR response %q: %v", result, err)
	}
	pid, err := strconv.Atoi(strings.SplitN(result, ":", 2)[0])
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitOCRExit(t *testing.T, pid int) {
	t.Helper()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 3000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("OCR process %d survived: %d %v", pid, status, err)
	}
}

func TestLocalOCRReusesOneWorkerAndReleasesItOnHostExit(t *testing.T) {
	service, cancel := ocrWorkerFixture(t, time.Minute)
	pid := workerReply(t, service, "first")
	if next := workerReply(t, service, "second"); next != pid {
		t.Fatalf("worker restarted: %d -> %d", pid, next)
	}
	cancel()
	waitOCRExit(t, pid)
	if _, err := service.Recognize(context.Background(), []byte("late")); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed Host restarted OCR: %v", err)
	}
}

func TestLocalOCRStandbyPreparesBeforeUseAndSurvivesIdle(t *testing.T) {
	service, _ := ocrWorkerFixture(t, 20*time.Millisecond)
	if err := <-service.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	service.gate <- struct{}{}
	pid := service.worker.process.cmd.Process.Pid
	<-service.gate
	if ready, reported := service.Status(); !ready || reported != pid {
		t.Fatal("warmup did not publish readiness")
	}
	if next := workerReply(t, service, "first after preparation"); next != pid {
		t.Fatal("first OCR did not reuse the prepared process")
	}
	time.Sleep(100 * time.Millisecond) // Beyond the fixture's ad-hoc idle lifetime.
	if next := workerReply(t, service, "after idle"); next != pid {
		t.Fatal("standby OCR was retired during idle")
	}
	if err := service.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if next := workerReply(t, service, "after resume"); next != pid {
		t.Fatal("resume replaced a healthy worker")
	}
	if err := <-service.SetStandby(false); err != nil {
		t.Fatal(err)
	}
	waitOCRExit(t, pid)
	if ready, reported := service.Status(); ready || reported != 0 {
		t.Fatal("disabled OCR still reported ready")
	}
	if err := service.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.gate <- struct{}{}
	defer func() { <-service.gate }()
	if service.worker != nil {
		t.Fatal("disabled module was restarted by a late resume")
	}
}

func TestLocalOCRIdleMaintenanceRecoversBeforeTheNextRecognition(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	if err := <-service.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	_, original := service.Status()
	service.gate <- struct{}{}
	process := service.worker.process
	if err := process.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-process.finished
	<-service.gate
	if err := service.PrepareIfIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	replacement := waitOCRReady(t, service, original)
	if actual := workerReply(t, service, "after background recovery"); actual != replacement {
		t.Fatal("recognition did not reuse the recovered worker")
	}
	if err := <-service.SetStandby(false); err != nil {
		t.Fatal(err)
	}
	if err := service.PrepareIfIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ready, pid := service.Status(); ready || pid != 0 {
		t.Fatal("maintenance revived disabled local OCR")
	}
}

func waitOCRReady(t *testing.T, service *LocalOCR, previous int) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ready, pid := service.Status(); ready && pid != 0 && pid != previous {
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background recovery left OCR unprepared")
	return 0
}

func TestLocalOCRStandbyRecoversOnExitWithoutWaitingForMaintenance(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	if err := <-service.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	_, original := service.Status()
	service.gate <- struct{}{}
	process := service.worker.process
	if err := process.cmd.Process.Kill(); err != nil {
		<-service.gate
		t.Fatal(err)
	}
	<-process.finished
	<-service.gate
	// There is no Host timer or recognition request in this test.
	replacement := waitOCRReady(t, service, original)
	if actual := workerReply(t, service, "after immediate recovery"); actual != replacement {
		t.Fatal("first recognition did not reuse the automatically prepared worker")
	}
	if err := <-service.SetStandby(false); err != nil {
		t.Fatal(err)
	}
	waitOCRExit(t, replacement)
	if ready, pid := service.Status(); ready || pid != 0 {
		t.Fatal("intentional shutdown restarted OCR")
	}
}

func TestLocalOCRRepeatedCrashesWaitForMaintenance(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	if err := <-service.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	_, original := service.Status()
	killWorker := func() {
		service.gate <- struct{}{}
		defer func() { <-service.gate }()
		process := service.worker.process
		if err := process.cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		<-process.finished
	}
	killWorker()
	replacement := waitOCRReady(t, service, original)
	killWorker()
	// A second crash shortly after recovery must not create an unbounded
	// restart loop. The ordinary maintenance path can still repair it.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ready, pid := service.Status(); ready || pid != 0 {
			t.Fatal("repeated crashes immediately restarted OCR again")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := service.PrepareIfIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered := waitOCRReady(t, service, replacement)
	if actual := workerReply(t, service, "after repeated crashes"); actual != recovered {
		t.Fatal("maintenance did not leave a reusable worker")
	}
	if err := <-service.SetStandby(false); err != nil {
		t.Fatal(err)
	}
	waitOCRExit(t, recovered)
}

func TestLocalOCRIdleMaintenanceDoesNotQueueBehindActiveWork(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	if err := <-service.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	service.gate <- struct{}{}
	done := make(chan error, 1)
	go func() { done <- service.PrepareIfIdle(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		<-service.gate
		t.Fatal("background preparation waited behind an active request")
	}
	<-service.gate
}

func TestLocalOCRLateResultAfterDisableIsNotRetained(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Hour)
	if err := <-service.SetStandby(false); err != nil {
		t.Fatal(err)
	}
	pid := workerReply(t, service, "already captured result")
	waitOCRExit(t, pid)
	service.gate <- struct{}{}
	defer func() { <-service.gate }()
	if service.worker != nil || service.idle != nil {
		t.Fatal("disabled module retained a late OCR request")
	}
}

func TestLocalOCRLatestStandbyIntentWins(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	service.gate <- struct{}{}
	on := service.SetStandby(true)
	off := service.SetStandby(false)
	<-service.gate
	if err := <-on; err != nil {
		t.Fatal(err)
	}
	if err := <-off; err != nil {
		t.Fatal(err)
	}
	service.gate <- struct{}{}
	defer func() { <-service.gate }()
	if service.worker != nil {
		t.Fatal("queued warmup resurrected a disabled module")
	}
}

func TestLocalOCRReleasesIdleWorkerAndStartsAgain(t *testing.T) {
	service, _ := ocrWorkerFixture(t, 50*time.Millisecond)
	pid := workerReply(t, service, "first")
	waitOCRExit(t, pid)
	workerReply(t, service, "after idle")
}

func TestLocalOCRRecoversAfterWorkerFailure(t *testing.T) {
	for _, failure := range []string{"crash", "oversized"} {
		t.Run(failure, func(t *testing.T) {
			service, _ := ocrWorkerFixture(t, time.Minute)
			pid := workerReply(t, service, "first")
			if _, err := service.Recognize(context.Background(), []byte(failure)); err == nil {
				t.Fatal("invalid response accepted")
			}
			waitOCRExit(t, pid)
			workerReply(t, service, "recovered")
		})
	}
}

func TestLocalOCRCancellationStopsActiveWorkerButNotQueuedRequests(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	marker := filepath.Join(t.TempDir(), "started")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := service.Recognize(ctx, []byte("wait:"+marker)); done <- err }()
	deadline := time.Now().Add(3 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil {
			pid, _ = strconv.Atoi(string(data))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("worker never received the request")
	}
	queued, stopQueued := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stopQueued()
	if _, err := service.Recognize(queued, []byte("queued")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued cancellation: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active request survived cancellation")
	}
	waitOCRExit(t, pid)
	workerReply(t, service, "recovered")
}

func TestLocalOCRRejectsInvalidInputsBeforeStarting(t *testing.T) {
	service := NewLocalOCR(context.Background(), Translator{})
	for _, value := range [][]byte{nil, []byte("image")} {
		if _, err := service.Recognize(context.Background(), value); err == nil {
			t.Fatal(fmt.Sprintf("accepted invalid input with %d bytes", len(value)))
		}
	}
}
