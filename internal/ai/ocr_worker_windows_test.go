//go:build windows

package ai

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The test executable can act as a deterministic peer of the real worker
// protocol. Pipes, cancellation, crash recovery and Job Objects remain real.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--ocr-worker" {
		for {
			var length uint32
			if err := binary.Read(os.Stdin, binary.LittleEndian, &length); err != nil {
				return
			}
			if length == 0 {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"prepared": true, "device": "gpu"})
				continue
			}
			data := make([]byte, length)
			if _, err := io.ReadFull(os.Stdin, data); err != nil {
				os.Exit(1)
			}
			payload := string(data)
			if strings.HasPrefix(payload, "wait:") {
				if err := os.WriteFile(strings.TrimPrefix(payload, "wait:"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
					os.Exit(1)
				}
				time.Sleep(time.Minute)
			}
			switch payload {
			case "crash":
				os.Exit(17)
			case "oversized":
				fmt.Println(strings.Repeat("x", 2*1024*1024))
			case "blank":
				_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"text": ""})
			default:
				_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"text": strconv.Itoa(os.Getpid()) + ":" + payload, "device": "gpu"})
			}
		}
	}
	os.Exit(m.Run())
}

func ocrWorkerFixture(t *testing.T, idle time.Duration) (*LocalOCR, context.CancelFunc) {
	t.Helper()
	root := t.TempDir()
	runtime := filepath.Join(root, "capture", "Ocr")
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "capture", "WinOne.Capture.exe"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "rapidocrlite.dll"), nil, 0600); err != nil {
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

// Readiness is logged once per worker (not on every 30 s maintenance), and
// each recognition logs its device, size and duration, never the text.
func TestLocalOCRObservesReadinessOnceAndEachRecognition(t *testing.T) {
	service, _ := ocrWorkerFixture(t, time.Minute)
	var events []string
	service.Observe = func(event, message string) { events = append(events, event+" "+message) }
	if err := <-service.SetStandby(true); err != nil {
		t.Fatal(err)
	}
	if err := service.PrepareIfIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	workerReply(t, service, "secret words")
	if len(events) != 2 || !strings.HasPrefix(events[0], "ocr.ready device=gpu ms=") || !strings.HasPrefix(events[1], "ocr.timing device=gpu size=unknown bytes=12 ms=") {
		t.Fatalf("events: %q", events)
	}
	for _, event := range events {
		if strings.Contains(event, "secret") {
			t.Fatalf("recognized text reached the log: %q", event)
		}
	}
}

func TestPNGSizeReadsHeaderOnly(t *testing.T) {
	header := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52, 0, 0, 0x0d, 0x70, 0, 0, 0x05, 0xa0}
	if got := pngSize(header); got != "3440x1440" {
		t.Fatal(got)
	}
	if got := pngSize([]byte("not a png at all, really long enough")); got != "unknown" {
		t.Fatal(got)
	}
}
