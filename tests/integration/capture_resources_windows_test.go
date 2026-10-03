//go:build windows

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
	"winone/internal/host"
	"winone/internal/transport"
)

type captureMemorySample struct {
	WorkingSetBytes     uint64 `json:"workingSetBytes"`
	PeakWorkingSetBytes uint64 `json:"peakWorkingSetBytes"`
	PrivateBytes        uint64 `json:"privateBytes"`
	Handles             uint32 `json:"handles"`
	GdiObjects          uint32 `json:"gdiObjects"`
	UserObjects         uint32 `json:"userObjects"`
}

var captureMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
var captureResourceKernel = windows.NewLazySystemDLL("kernel32.dll")
var captureGuiResources = windows.NewLazySystemDLL("user32.dll").NewProc("GetGuiResources")

func readCaptureGui(handle windows.Handle, kind uintptr) (uint32, error) {
	// GetGuiResources can legitimately return zero; read its last-error value on
	// the same OS thread so an unavailable counter is never reported as zero.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	captureResourceKernel.NewProc("SetLastError").Call(0)
	value, _, err := captureGuiResources.Call(uintptr(handle), kind)
	if value == 0 && err != windows.ERROR_SUCCESS {
		return 0, err
	}
	return uint32(value), nil
}

// PROCESS_MEMORY_COUNTERS_EX contains SIZE_T fields after two DWORDs.
// A process handle pins identity throughout a run, avoiding PID reuse races.
func readCaptureMemory(handle windows.Handle) (captureMemorySample, error) {
	var counters struct {
		Size           uint32
		Faults         uint32
		PeakWorkingSet uintptr
		WorkingSet     uintptr
		PeakPaged      uintptr
		Paged          uintptr
		PeakNonPaged   uintptr
		NonPaged       uintptr
		Pagefile       uintptr
		PeakPagefile   uintptr
		Private        uintptr
	}
	counters.Size = uint32(unsafe.Sizeof(counters))
	result, _, err := captureMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if result == 0 {
		return captureMemorySample{}, err
	}
	var handles uint32
	if ok, _, err := captureResourceKernel.NewProc("GetProcessHandleCount").Call(uintptr(handle), uintptr(unsafe.Pointer(&handles))); ok == 0 {
		return captureMemorySample{}, err
	}
	gdi, err := readCaptureGui(handle, 0)
	if err != nil {
		return captureMemorySample{}, err
	}
	user, err := readCaptureGui(handle, 1)
	if err != nil {
		return captureMemorySample{}, err
	}
	return captureMemorySample{WorkingSetBytes: uint64(counters.WorkingSet), PeakWorkingSetBytes: uint64(counters.PeakWorkingSet), PrivateBytes: uint64(counters.Private), Handles: handles, GdiObjects: gdi, UserObjects: user}, nil
}

type capturePreviewMeasurement struct {
	ElapsedMilliseconds     float64             `json:"elapsedMilliseconds"`
	CPUTimeMilliseconds     float64             `json:"cpuTimeMilliseconds"`
	SampledPeakPrivateBytes uint64              `json:"sampledPeakPrivateBytes"`
	Final                   captureMemorySample `json:"final"`
}

func captureCPUTime(handle windows.Handle) (float64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
	return float64(ticks) / 10000, nil
}

type capturePreviewCase struct {
	Width    int                         `json:"width"`
	Height   int                         `json:"height"`
	Baseline captureMemorySample         `json:"baseline"`
	Runs     []capturePreviewMeasurement `json:"runs"`
}

// This opt-in probe measures only the real capture worker. Fixture generation
// and PNG checks run in the test process and are excluded from its counters.
func TestCapturePreviewResourceBenchmark(t *testing.T) {
	output := os.Getenv("WINONE_CAPTURE_BENCHMARK_OUTPUT")
	if output == "" {
		t.Skip("set WINONE_CAPTURE_BENCHMARK_OUTPUT for the resource probe")
	}
	iterations := 5
	if value := os.Getenv("WINONE_CAPTURE_BENCHMARK_ITERATIONS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			t.Fatal("benchmark iterations must be between 1 and 100")
		}
		iterations = parsed
	}
	exe := os.Getenv("WINONE_CAPTURE_EXE")
	warmup := 0
	if value := os.Getenv("WINONE_CAPTURE_BENCHMARK_WARMUP"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > 100 {
			t.Fatal("benchmark warmup must be between 0 and 100")
		}
		warmup = parsed
	}
	if exe == "" {
		t.Fatal("set WINONE_CAPTURE_EXE")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("benchmark output must be a new file")
	}
	file, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	_, err = io.Copy(digest, file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	binaryHash := hex.EncodeToString(digest.Sum(nil))
	// The .NET apphost EXE may remain byte-identical when managed code changes.
	// Fingerprint the assembly as well so reports identify the executed code.
	assembly, err := os.Open(filepath.Join(filepath.Dir(exe), strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))+".dll"))
	if err != nil {
		t.Fatal(err)
	}
	digest.Reset()
	_, err = io.Copy(digest, assembly)
	assembly.Close()
	if err != nil {
		t.Fatal(err)
	}
	assemblyHash := hex.EncodeToString(digest.Sum(nil))
	report := struct {
		SchemaVersion             int                  `json:"schemaVersion"`
		Scope                     string               `json:"scope"`
		OutputMode                string               `json:"outputMode"`
		BinarySHA256              string               `json:"binarySha256"`
		AssemblySHA256            string               `json:"assemblySha256"`
		LogicalCPUs               int                  `json:"logicalCpus"`
		PollIntervalMilliseconds  int                  `json:"pollIntervalMilliseconds"`
		TieredCompilationDisabled bool                 `json:"tieredCompilationDisabled"`
		HighQoS                   bool                 `json:"highQoS"`
		WarmupIterations          int                  `json:"warmupIterations"`
		Cases                     []capturePreviewCase `json:"cases"`
	}{SchemaVersion: 1, Scope: "capture-worker-only", OutputMode: "original-resolution", BinarySHA256: binaryHash, AssemblySHA256: assemblyHash, LogicalCPUs: runtime.NumCPU(), PollIntervalMilliseconds: 10, TieredCompilationDisabled: os.Getenv("WINONE_CAPTURE_BENCHMARK_DISABLE_TIERING") == "1", HighQoS: os.Getenv("WINONE_CAPTURE_BENCHMARK_HIGH_QOS") == "1", WarmupIterations: warmup}
	for _, size := range []image.Point{{X: 1920, Y: 1080}, {X: 3840, Y: 2160}} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			report.Cases = append(report.Cases, measureCapturePreview(t, exe, size, iterations, warmup))
		})
	}
	if t.Failed() {
		return
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	destination, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = destination.Write(data); err != nil {
		destination.Close()
		t.Fatal(err)
	}
	if err = destination.Close(); err != nil {
		t.Fatal(err)
	}
}

func measureCapturePreview(t *testing.T, exe string, size image.Point, iterations, warmup int) capturePreviewCase {
	t.Helper()
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.png")
	source := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			at := y*source.Stride + x*4
			source.Pix[at] = byte(x % 256)
			source.Pix[at+1] = byte(y % 256)
			source.Pix[at+2] = byte((x + y) % 256)
			source.Pix[at+3] = 255
		}
	}
	file, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, source); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	source = nil
	id := strings.Repeat("1", 32)
	metadata, err := json.Marshal(map[string]any{"id": id, "path": sourcePath, "width": size.X, "height": size.Y})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "last-capture.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-preview-benchmark-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if os.Getenv("WINONE_CAPTURE_BENCHMARK_DISABLE_TIERING") == "1" {
		cmd.Env = append(os.Environ(), "DOTNET_TieredCompilation=0")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	access := uint32(windows.PROCESS_QUERY_INFORMATION | windows.PROCESS_VM_READ)
	if os.Getenv("WINONE_CAPTURE_BENCHMARK_HIGH_QOS") == "1" {
		access |= windows.PROCESS_SET_INFORMATION
	}
	handle, err := windows.OpenProcess(access, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if os.Getenv("WINONE_CAPTURE_BENCHMARK_HIGH_QOS") == "1" {
		// Microsoft documents this as HighQoS. Only this disposable benchmark
		// child is changed; the application and system defaults stay untouched.
		state := struct{ Version, ControlMask, StateMask uint32 }{1, 1, 0}
		ok, _, setErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetProcessInformation").Call(uintptr(handle), 4, uintptr(unsafe.Pointer(&state)), unsafe.Sizeof(state))
		if ok == 0 {
			t.Fatal(setErr)
		}
	}
	call := func(method string, params any, target any) error {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "resource-probe", Method: method, Params: data})
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		return json.Unmarshal(response.Result, target)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var state any
		if call("snapshot", nil, &state) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not become ready")
		}
		time.Sleep(25 * time.Millisecond)
	}
	baseline, err := readCaptureMemory(handle)
	if err != nil {
		t.Fatal(err)
	}
	result := capturePreviewCase{Width: size.X, Height: size.Y, Baseline: baseline}
	document := map[string]any{"version": 1, "width": size.X, "height": size.Y, "marks": []map[string]any{
		{"kind": "blur", "x": size.X / 8, "y": size.Y / 8, "width": size.X / 2, "height": size.Y / 2, "effectSize": 24},
		{"kind": "mosaic", "x": size.X / 2, "y": size.Y / 2, "width": size.X / 3, "height": size.Y / 3, "effectSize": 24},
		{"kind": "text", "x": 20, "y": 20, "width": 500, "height": 80, "text": "WinOne preview", "fontSize": 32},
	}}
	for iteration := 0; iteration < iterations+warmup; iteration++ {
		if iteration == warmup && warmup > 0 {
			baseline, err = readCaptureMemory(handle)
			if err != nil {
				t.Fatal(err)
			}
			result.Baseline = baseline
		}
		cpuBefore, err := captureCPUTime(handle)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		var job struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Error  string `json:"error"`
			Result struct {
				ID       string `json:"id"`
				SourceID string `json:"sourceId"`
			} `json:"result"`
		}
		if err = call("edit.preview", map[string]any{"expectedId": id, "document": document}, &job); err != nil {
			t.Fatal(err)
		}
		measurement := capturePreviewMeasurement{}
		deadline = time.Now().Add(45 * time.Second)
		for {
			sample, err := readCaptureMemory(handle)
			if err != nil {
				t.Fatal(err)
			}
			if sample.PrivateBytes > measurement.SampledPeakPrivateBytes {
				measurement.SampledPeakPrivateBytes = sample.PrivateBytes
			}
			measurement.Final = sample
			if job.State != "running" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("preview timed out")
			}
			time.Sleep(10 * time.Millisecond)
			if err = call("jobs.get", map[string]string{"id": job.ID}, &job); err != nil {
				t.Fatal(err)
			}
		}
		measurement.ElapsedMilliseconds = float64(time.Since(started).Microseconds()) / 1000
		cpuAfter, err := captureCPUTime(handle)
		if err != nil {
			t.Fatal(err)
		}
		measurement.CPUTimeMilliseconds = cpuAfter - cpuBefore
		if job.State != "completed" || job.Result.SourceID != id || len(job.Result.ID) != 32 {
			t.Fatalf("preview failed: %+v", job)
		}
		file, err = os.Open(filepath.Join(directory, "previews", job.Result.ID+".png"))
		if err != nil {
			t.Fatal(err)
		}
		config, decodeErr := png.DecodeConfig(file)
		file.Close()
		if decodeErr != nil || config.Width != size.X || config.Height != size.Y {
			t.Fatalf("invalid preview size: %+v %v", config, decodeErr)
		}
		if iteration >= warmup {
			result.Runs = append(result.Runs, measurement)
		}
	}
	current, err := os.ReadFile(filepath.Join(directory, "last-capture.json"))
	if err != nil || string(current) != string(metadata) {
		t.Fatal("benchmark preview changed source metadata", err)
	}
	previews, err := os.ReadDir(filepath.Join(directory, "previews"))
	if err != nil || len(previews) != 1 {
		t.Fatal("preview files accumulated", err, len(previews))
	}
	return result
}

func TestCaptureMemoryProbeReadsLiveProcess(t *testing.T) {
	sample, err := readCaptureMemory(windows.CurrentProcess())
	if err != nil {
		t.Fatal(err)
	}
	if sample.WorkingSetBytes == 0 || sample.PrivateBytes == 0 || sample.PeakWorkingSetBytes < sample.WorkingSetBytes || sample.Handles == 0 {
		t.Fatal("invalid native memory counters", sample)
	}
	if _, err = readCaptureMemory(0); err == nil {
		t.Fatal("invalid process handle accepted")
	}
}
