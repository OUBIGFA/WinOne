package resultui

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// Explicit read-only sampling of this test process's own renderer child.
// Ordinary tests neither launch native workers nor inspect other user apps.
func TestDiagramRendererResources(t *testing.T) {
	exe := os.Getenv("WINONE_DIAGRAM_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_DIAGRAM_TEST_EXE")
	}
	source := "flowchart TD\n"
	for i := 0; i < 24; i++ {
		source += string(rune('A'+i)) + "[处理节点] --> " + string(rune('A'+i+1)) + "\n"
	}
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := renderLocalDiagram(context.Background(), exe, t.TempDir(), source, false)
		done <- err
	}()
	type counters struct {
		Size, Faults                                                                                                          uint32
		PeakWorkingSet, WorkingSet, PeakPagedPool, PagedPool, PeakNonPagedPool, NonPagedPool, Pagefile, PeakPagefile, Private uintptr
	}
	memory := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	var handle windows.Handle
	var peakWS, peakCommit uintptr
	samples := 0
	defer func() {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			if samples == 0 {
				t.Fatal("renderer exited without a memory observation")
			}
			report := map[string]any{"elapsedMs": float64(time.Since(start).Microseconds()) / 1000, "samples": samples, "peakWorkingSetBytes": peakWS, "peakCommitBytes": peakCommit, "scope": "one owned native renderer; peak working set includes shared pages"}
			t.Logf("renderer resource sample: %v", report)
			if output := os.Getenv("WINONE_DIAGRAM_METRICS_FILE"); output != "" {
				data, _ := json.MarshalIndent(report, "", "  ")
				if err := os.WriteFile(output, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			return
		default:
		}
		if handle == 0 {
			snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
			if err == nil {
				entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
				for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
					if entry.ParentProcessID == uint32(os.Getpid()) && strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(exe)) {
						handle, _ = windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, entry.ProcessID)
						break
					}
				}
				windows.CloseHandle(snapshot)
			}
		}
		if handle != 0 {
			value := counters{}
			value.Size = uint32(unsafe.Sizeof(value))
			ok, _, _ := memory.Call(uintptr(handle), uintptr(unsafe.Pointer(&value)), uintptr(value.Size))
			if ok != 0 {
				samples++
				peakWS = max(peakWS, value.PeakWorkingSet)
				peakCommit = max(peakCommit, value.PeakPagefile)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
}
