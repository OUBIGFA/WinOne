//go:build windows

package stroke

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

type workerFixture struct {
	cmd       *exec.Cmd
	pipe      string
	done      chan error
	directory string
}

func startWorkerFixture(t *testing.T, p Preferences) *workerFixture {
	t.Helper()
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	directory := os.Getenv("WINONE_STROKE_HELPER_DIRECTORY")
	if directory == "" {
		directory = t.TempDir()
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "managed-settings.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-Stroke-Fixture-%d-%d`, os.Getpid(), time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory, "--parent", strconv.Itoa(os.Getpid()), "--fixture")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fixture := &workerFixture{cmd: cmd, pipe: pipe, directory: directory, done: make(chan error, 1)}
	go func() { fixture.done <- cmd.Wait() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_, _ = transport.Call(ctx, pipe, host.Request{Version: 1, ID: "cleanup", Method: "shutdown"})
		select {
		case <-fixture.done:
		case <-time.After(time.Second):
			_ = cmd.Process.Kill()
			<-fixture.done
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
		cancel()
		if err == nil && response.Error == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not become ready", err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	return fixture
}

// The helper is intentionally terminated by its test parent. Its cleanup does
// not run, so the worker must observe the real owning-process handle itself.
func TestStrokeOwnerHelper(t *testing.T) {
	if os.Getenv("WINONE_STROKE_OWNER_HELPER") != "1" {
		t.Skip("owned helper")
	}
	p := Defaults()
	p.ActionPackages = []ActionPackage{{ID: "fixture", Name: "Fixture", Actions: []Action{{ID: "long", Name: "Long", Gesture: "left", Enabled: true, Code: "Thread.Sleep(Timeout.Infinite);"}}}}
	p.Scopes = []Scope{{ID: "fixture", Name: "Fixture", Kind: "global", Packages: []string{"fixture"}}}
	f := startWorkerFixture(t, p)
	f.call(t, "activate", nil)
	f.call(t, "fixture.execute", map[string]any{"name": "fixture.long"})
	var state struct{ ProcessID, ActionProcessID int }
	deadline := time.Now().Add(5 * time.Second)
	for {
		_ = json.Unmarshal(f.call(t, "snapshot", nil), &state)
		if state.ActionProcessID > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("executor did not start")
		}
		time.Sleep(15 * time.Millisecond)
	}
	_ = json.NewEncoder(os.Stdout).Encode(state)
	var wait [1]byte
	_, _ = os.Stdin.Read(wait[:])
}

func TestStrokeParentLossStopsWorkerAndItsExecutor(t *testing.T) {
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	helper := exec.Command(os.Args[0], "-test.run=^TestStrokeOwnerHelper$")
	helper.Env = append(os.Environ(), "WINONE_STROKE_OWNER_HELPER=1", "WINONE_STROKE_HELPER_DIRECTORY="+t.TempDir())
	helper.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	stdout, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := helper.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	helper.Stderr = os.Stderr
	if err = helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = helper.Process.Kill() }()
	scan := bufio.NewScanner(stdout)
	var state struct{ ProcessID, ActionProcessID int }
	if !scan.Scan() {
		t.Fatal("owner helper failed")
	}
	if err = json.Unmarshal(scan.Bytes(), &state); err != nil {
		t.Fatal(err, string(scan.Bytes()))
	}
	var handles []windows.Handle
	for _, id := range []int{state.ProcessID, state.ActionProcessID} {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(id))
		if err != nil {
			t.Fatal(err)
		}
		path := make([]uint16, 32768)
		size := uint32(len(path))
		if err = windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err != nil || !strings.EqualFold(windows.UTF16ToString(path[:size]), exe) {
			windows.CloseHandle(handle)
			t.Fatal("unexpected process identity", err)
		}
		handles = append(handles, handle)
		defer windows.CloseHandle(handle)
	}
	if err = helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()
	for _, handle := range handles {
		status, err := windows.WaitForSingleObject(handle, 4000)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(handle, 1)
			t.Fatal("parent loss left a Stroke process alive", status, err)
		}
	}
}
func (f *workerFixture) call(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := transport.Call(ctx, f.pipe, host.Request{Version: 1, ID: method, Method: method, Params: raw})
	if err != nil {
		t.Fatal(method, err)
	}
	if response.Error != nil {
		t.Fatal(method, response.Error.Message)
	}
	return response.Result
}

func TestStrokeWorkerCompilesAllOriginalDefaultsAndRecordsVectors(t *testing.T) {
	f := startWorkerFixture(t, Defaults())
	f.call(t, "activate", nil)
	var state struct {
		Running, MouseHook   bool
		ConfigurationManaged bool
	}
	if err := json.Unmarshal(f.call(t, "snapshot", nil), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Running || state.MouseHook || !state.ConfigurationManaged {
		t.Fatal("fixture activated a real hook or missed managed settings", state)
	}
	var record struct{ ID string }
	_ = json.Unmarshal(f.call(t, "record.start", nil), &record)
	var result struct {
		Vectors [][]int
		Match   struct {
			ID    string
			Score int
		}
	}
	_ = json.Unmarshal(f.call(t, "record.read", map[string]any{"id": record.ID, "points": []Point{{100, 0}, {0, 0}}}), &result)
	if len(result.Vectors) != 128 || result.Match.ID != "left" || result.Match.Score != 128 {
		t.Fatal("recording bypassed Stroke recognition", result.Match, len(result.Vectors))
	}
	var newer struct{ ID string }
	_ = json.Unmarshal(f.call(t, "record.start", nil), &newer)
	f.call(t, "record.cancel", map[string]any{"id": record.ID})
	f.call(t, "record.read", map[string]any{"id": newer.ID, "points": []Point{{100, 0}, {0, 0}}})
	f.call(t, "record.cancel", map[string]any{"id": newer.ID})
}

func TestStrokeLoadsAnExplicitDLLPlugin(t *testing.T) {
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	plugin := filepath.Join(filepath.Dir(exe), "tests", "FixturePlugin.dll")
	if _, err := os.Stat(plugin); err != nil {
		t.Fatal("build the plugin fixture with -Test", err)
	}
	marker := filepath.Join(t.TempDir(), "plugin.txt")
	p := Defaults()
	p.Namespaces = append(p.Namespaces, "P = StrokeFixture.Plugin")
	p.Assemblies = append(p.Assemblies, plugin)
	p.ActionPackages = []ActionPackage{{ID: "plugin", Name: "Plugin", Actions: []Action{{ID: "call", Name: "Call", Gesture: "left", Enabled: true, Code: fmt.Sprintf("File.WriteAllText(%q, P.Value + \" 测试\");", marker)}}}}
	p.Scopes = []Scope{{ID: "fixture", Name: "Fixture", Kind: "global", Packages: []string{"plugin"}}}
	f := startWorkerFixture(t, p)
	f.call(t, "activate", nil)
	f.call(t, "fixture.execute", map[string]any{"name": "plugin.call"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if text, err := os.ReadFile(marker); err == nil {
			if string(text) != "plugin-loaded 测试" {
				t.Fatal(string(text))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin was not loaded", string(f.call(t, "snapshot", nil)))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStrokeLoadsPluginCompiledAgainstOfficialBinaries(t *testing.T) {
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	plugin := filepath.Join(filepath.Dir(exe), "tests", "FixtureLegacyPlugin.dll")
	if _, err := os.Stat(plugin); err != nil {
		t.Skip("build -Test with -LegacyReferenceDirectory to verify binary compatibility")
	}
	marker := filepath.Join(t.TempDir(), "legacy.txt")
	p := Defaults()
	p.Namespaces = append(p.Namespaces, "StrokeLegacyFixture")
	p.Assemblies = append(p.Assemblies, plugin)
	p.ActionPackages = []ActionPackage{{ID: "legacy", Name: "Legacy", Actions: []Action{{ID: "plugin", Name: "Plugin", Gesture: "left", Enabled: true, Code: fmt.Sprintf("File.WriteAllText(%q, Plugin.Press().ToString());", marker)}}}}
	p.Scopes = []Scope{{ID: "fixture", Name: "Fixture", Kind: "global", Packages: []string{"legacy"}}}
	f := startWorkerFixture(t, p)
	f.call(t, "activate", nil)
	f.call(t, "fixture.execute", map[string]any{"name": "legacy.plugin"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if value, err := os.ReadFile(marker); err == nil {
			if string(value) != strconv.Itoa(f.cmd.Process.Pid) {
				t.Fatal("Base context was lost across Task.Run", string(value))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old binary plugin did not run", string(f.call(t, "snapshot", nil)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	var state struct{ FixtureInputs int }
	_ = json.Unmarshal(f.call(t, "snapshot", nil), &state)
	if state.FixtureInputs != 2 {
		t.Fatal("legacy Base input bypassed the owned channel", state)
	}
}

func TestStrokeShutdownLeavesScriptLaunchedProgramAlive(t *testing.T) {
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	childExe := filepath.Join(filepath.Dir(exe), "tests", "FixtureChild.exe")
	marker := filepath.Join(t.TempDir(), "child.txt")
	p := Defaults()
	p.ActionPackages = []ActionPackage{{ID: "launch", Name: "Launch", Actions: []Action{{ID: "child", Name: "Child", Gesture: "left", Enabled: true, Code: fmt.Sprintf("Base.Run(%q, %q);", childExe, `"`+marker+`"`)}}}}
	p.Scopes = []Scope{{ID: "fixture", Name: "Fixture", Kind: "global", Packages: []string{"launch"}}}
	f := startWorkerFixture(t, p)
	f.call(t, "activate", nil)
	f.call(t, "fixture.execute", map[string]any{"name": "launch.child"})
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, err := os.ReadFile(marker); err == nil {
			pid, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("independent fixture did not start", string(f.call(t, "snapshot", nil)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	path := make([]uint16, 32768)
	size := uint32(len(path))
	if err = windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err != nil || !strings.EqualFold(windows.UTF16ToString(path[:size]), childExe) {
		t.Fatal("independent fixture identity mismatch", err)
	}
	defer func() { _ = windows.TerminateProcess(handle, 0); _, _ = windows.WaitForSingleObject(handle, 2000) }()
	f.call(t, "shutdown", nil)
	select {
	case err := <-f.done:
		if err != nil {
			t.Fatal(err)
		}
		f.done <- nil
	case <-time.After(4 * time.Second):
		t.Fatal("worker did not exit")
	}
	if result, err := windows.WaitForSingleObject(handle, 0); err != nil || result != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("shutdown killed a script-launched independent program", result, err)
	}
}

func TestStrokeLongActionSharesBaseStateAndShutdownReturnsOwnedInputs(t *testing.T) {
	p := Defaults()
	p.ScriptThreadCount = 8
	marker := filepath.Join(t.TempDir(), "state.txt")
	ready := filepath.Join(t.TempDir(), "ready.txt")
	p.ActionPackages = []ActionPackage{{ID: "test", Name: "Test", Actions: []Action{{ID: "long", Name: "Long", Gesture: "left", Enabled: true, Code: fmt.Sprintf("Base.Data[\"shared\"] = 41; Base.KeyDown(Keys.ControlKey); File.WriteAllText(%q, \"ready\"); Thread.Sleep(Timeout.Infinite);", ready)}, {ID: "shared", Name: "Shared", Gesture: "right", Enabled: true, Code: fmt.Sprintf("if ((int)Base.Data[\"shared\"] != 41) throw new Exception(\"state lost\"); File.WriteAllText(%q, \"shared\");", marker)}}}}
	p.Scopes = []Scope{{ID: "fixture", Name: "Fixture", Kind: "global", Enabled: false, Packages: []string{"test"}}}
	f := startWorkerFixture(t, p)
	f.call(t, "activate", nil)
	f.call(t, "fixture.execute", map[string]any{"name": "test.long"})
	awaitFile := func(path string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(path); err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("action did not reach marker", string(f.call(t, "snapshot", nil)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	awaitFile(ready)
	f.call(t, "fixture.execute", map[string]any{"name": "test.shared"})
	awaitFile(marker)
	var status struct{ ScriptWorkers, ScriptThreads, FixtureInputs int }
	_ = json.Unmarshal(f.call(t, "snapshot", nil), &status)
	if status.ScriptWorkers != 1 || status.ScriptThreads < 2 || status.FixtureInputs != 1 {
		t.Fatal("shared executor/STA/input ownership failed", status)
	}
	start := time.Now()
	var stopped struct {
		Restored      bool
		FixtureInputs int
	}
	_ = json.Unmarshal(f.call(t, "shutdown", nil), &stopped)
	if !stopped.Restored || stopped.FixtureInputs != 2 {
		t.Fatal("shutdown did not return the held Control key", stopped)
	}
	select {
	case err := <-f.done:
		if err != nil {
			t.Fatal(err)
		}
		f.done <- nil
	case <-time.After(4 * time.Second):
		t.Fatal("long action blocked shutdown")
	}
	if time.Since(start) > 4*time.Second {
		t.Fatal("shutdown exceeded deadline")
	}
}

func TestStrokeExecutorCrashReturnsCompletedActionsHeldKey(t *testing.T) {
	p := Defaults()
	p.ActionPackages = []ActionPackage{{ID: "crash", Name: "Crash", Actions: []Action{{ID: "background", Name: "Background", Gesture: "left", Enabled: true, Code: `Base.KeyDown(Keys.ControlKey); new Thread(delegate(){ Thread.Sleep(100); throw new Exception("fixture"); }).Start();`}, {ID: "next", Name: "Next", Gesture: "right", Enabled: true, Code: `Base.PressKey(Keys.F14);`}}}}
	p.Scopes = []Scope{{ID: "fixture", Name: "Fixture", Kind: "global", Packages: []string{"crash"}}}
	f := startWorkerFixture(t, p)
	f.call(t, "activate", nil)
	f.call(t, "fixture.execute", map[string]any{"name": "crash.background"})
	var state struct{ FixtureInputs, ScriptWorkers, Queued int }
	deadline := time.Now().Add(5 * time.Second)
	for {
		_ = json.Unmarshal(f.call(t, "snapshot", nil), &state)
		if state.FixtureInputs == 2 && state.ScriptWorkers == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("executor crash left a held key", state)
		}
		time.Sleep(25 * time.Millisecond)
	}
	f.call(t, "fixture.execute", map[string]any{"name": "crash.next"})
	deadline = time.Now().Add(5 * time.Second)
	for {
		_ = json.Unmarshal(f.call(t, "snapshot", nil), &state)
		if state.FixtureInputs == 4 && state.Queued == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a new executor could not recover", state)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
