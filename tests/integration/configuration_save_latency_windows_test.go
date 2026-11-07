//go:build windows

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/transport"
	"winone/modules/wintray/preferences"
)

// Explicit, owned Host RPC measurement. All modules and every shortcut are
// disabled; no actual program, login trigger or real profile is touched. The
// same production settings.save/queue/read-back path serves the native page.
func TestHostConfigurationSave100ProgramsMeasuresActualCompletion(t *testing.T) {
	executable := os.Getenv("WINONE_CONFIG_SAVE_HOST_EXE")
	if executable == "" || os.Getenv("WINONE_CONFIG_SAVE_SYSTEM") != "1" {
		t.Skip("explicit repository-private Host save measurement")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Clean(absolute), string(filepath.Separator)+"_temp"+string(filepath.Separator)) {
		t.Fatal("measurement requires a repository-owned candidate Host")
	}
	binary, err := os.ReadFile(absolute)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(binary)
	root := t.TempDir()
	global := `{"schemaVersion":1,"theme":"light","language":"zh-CN","captureShortcut":"","pinShortcut":"","openShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":"","quickAssistantShortcut":"","startup":{"runAtLogon":false,"startMinimized":true}}`
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(global), 0600); err != nil {
		t.Fatal(err)
	}
	modules := map[string]any{}
	for _, module := range catalog.Builtins() {
		modules[module.ID] = map[string]any{"enabled": false, "pendingStop": false}
	}
	state, _ := json.Marshal(map[string]any{"schemaVersion": 4, "revision": 0, "modules": modules})
	if err := os.WriteFile(filepath.Join(root, "module-state.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-save-latency-%d`, time.Now().UnixNano())
	command := exec.Command(absolute, "--pipe", pipe, "--data-dir", root)
	diagnostic, err := os.Create(filepath.Join(root, "host-stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = diagnostic
	if err := command.Start(); err != nil {
		diagnostic.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait(); _ = diagnostic.Close() })
	call := func(method string, input any, out any) error {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		response, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: fmt.Sprint(time.Now().UnixNano()), Method: method, Params: data}, 5*time.Second)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s: %s", response.Error.Code, response.Error.Message)
		}
		return json.Unmarshal(response.Result, out)
	}
	var current struct {
		Revision string
		Settings preferences.Settings
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err = call("wintray.settings.get", nil, &current); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("owned Host startup: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	programs := make([]map[string]any, 100)
	for i := range programs {
		programs[i] = map[string]any{"id": fmt.Sprintf("program-%d", i), "name": fmt.Sprintf("Program %d", i), "exePath": filepath.Join(root, fmt.Sprintf("fixture-%d.exe", i)), "runOnStartup": false, "trayBehavior": map[string]any{}, "schedule": map[string]any{"autoExitMinutes": 30, "frequencyDays": 1, "frequencyRuns": 1}}
	}
	bytes, _ := json.Marshal(programs)
	if err = json.Unmarshal(bytes, &current.Settings.ManagedApps); err != nil {
		t.Fatal(err)
	}
	var completed struct {
		State  string
		Result struct {
			Revision string
			Settings preferences.Settings
		}
	}
	measurements := make([]float64, 0, 21)
	for i := range 22 {
		current.Settings.ManagedApps[0].Name = fmt.Sprintf("Owner saved %d", i)
		began := time.Now()
		if err = call("wintray.settings.save", map[string]any{"revision": current.Revision, "settings": current.Settings}, &completed); err != nil {
			t.Fatal(err)
		}
		if completed.State != "completed" || completed.Result.Revision == current.Revision {
			t.Fatalf("save was merely accepted, not completed: %+v", completed)
		}
		// Match the native page's saved read-back before reporting completion.
		if err = call("wintray.settings.get", nil, &current); err != nil {
			t.Fatal(err)
		}
		if len(current.Settings.ManagedApps) != 100 || current.Settings.ManagedApps[0].Name != fmt.Sprintf("Owner saved %d", i) {
			t.Fatal("saved draft did not round-trip")
		}
		if i > 0 {
			measurements = append(measurements, float64(time.Since(began))/float64(time.Millisecond))
		}
	}
	ordered := append([]float64(nil), measurements...)
	sort.Float64s(ordered)
	total := 0.0
	for _, milliseconds := range ordered {
		total += milliseconds
	}
	metrics, err := json.Marshal(map[string]any{"hostSHA256": hex.EncodeToString(hash[:]), "programs": 100, "runs": len(ordered), "scope": "actual Host RPC, durable transaction and native-page read-back; modules disabled; excludes physical click-to-photon and system startup reconciliation", "meanMs": total / float64(len(ordered)), "medianMs": ordered[len(ordered)/2], "p95Ms": ordered[(95*len(ordered)+99)/100-1], "samplesMs": measurements})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SAVE_COMPLETION_METRICS %s", metrics)
}
