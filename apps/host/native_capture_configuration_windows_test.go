//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"testing"
	"time"

	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/transport"
)

// Explicit integration test: no tray, startup tasks or user's settings are used.
// The child verifies this process owns the real Host configuration pipe.
func TestNativeCaptureConfigurationRoundTrip(t *testing.T) {
	probe, directory := os.Getenv("WINONE_NATIVE_CONFIG_PROBE"), os.Getenv("WINONE_NATIVE_CONFIG_DATA")
	if probe == "" || directory == "" {
		t.Skip("set isolated native configuration probe and data directory")
	}
	if !filepath.IsAbs(probe) || !filepath.IsAbs(directory) {
		t.Fatal("integration paths must be absolute")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("choose a new integration data directory")
	}
	adapter := func() configuration.ModuleAdapter {
		return captureConfiguration(directory, func() *modules.Lazy { return nil })
	}
	store, err := configuration.OpenHost(directory, adapter())
	if err != nil {
		t.Fatal(err)
	}
	raw, revision, err := store.Module("capture")
	if err != nil {
		t.Fatal(err)
	}
	var initial capturePreferences
	if err = json.Unmarshal(raw, &initial); err != nil {
		t.Fatal(err)
	}
	initial.OutputDirectory = filepath.Join(directory, "images")
	initial.FileNameTemplate = "work-{date}"
	initial.CopyToClipboard = false
	raw, _ = json.Marshal(initial)
	if _, revision, err = store.SaveModule("capture", revision, raw); err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	service.RegisterModule("capture", func() host.Module { return host.Module{ID: "capture", State: "disabled"} }, captureConfigurationHandler(store, func(r host.Request) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "module_disabled", Message: "worker should not start"}}
	}))
	queue := host.NewConfigurationQueue(service)
	pipe := fmt.Sprintf(`\\.\pipe\WinOneNativeConfiguration-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	serving := make(chan error, 1)
	go func() { serving <- transport.Serve(ctx, listener, queue) }()
	defer func() {
		cancel()
		if err := <-serving; err != nil {
			t.Error(err)
		}
	}()
	command := exec.CommandContext(ctx, probe, "--native-host-roundtrip", pipe, strconv.Itoa(os.Getpid()), filepath.Join(directory, "modules", "capture"))
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("native configuration client: %v\n%s", err, output)
	}
	var result struct {
		Saves, HostProcess int
		Completed          bool
	}
	if err = json.Unmarshal(output, &result); err != nil || !result.Completed || result.Saves != 3 || result.HostProcess != os.Getpid() {
		t.Fatalf("invalid native result: %s (%v)", output, err)
	}
	reopened, err := configuration.OpenHost(directory, adapter())
	if err != nil {
		t.Fatal(err)
	}
	raw, afterRevision, err := reopened.Module("capture")
	if err != nil {
		t.Fatal(err)
	}
	var after capturePreferences
	if err = json.Unmarshal(raw, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.ToolbarTools, []string{"mosaic", "arrow"}) || after.OutputDirectory != initial.OutputDirectory || after.FileNameTemplate != initial.FileNameTemplate || after.CopyToClipboard || afterRevision != revision+3 {
		t.Fatalf("reopened configuration lost preferences or unrelated fields: %+v revision %d", after, afterRevision)
	}
	projection, err := os.ReadFile(filepath.Join(directory, "modules", "capture", "managed-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var managed capturePreferences
	if err = json.Unmarshal(projection, &managed); err != nil || !reflect.DeepEqual(managed.ToolbarTools, after.ToolbarTools) || managed.Revision != afterRevision {
		t.Fatal("worker projection differs from confirmed Host configuration", err)
	}
	if err = os.WriteFile(filepath.Join(directory, "native-roundtrip.json"), output, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("native client completed three real Host transactions; reopened configuration and worker projection agree")
}
