//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/previewtransfer"
	"winone/internal/profiles"
	"winone/internal/protocol"
)

func TestPreviewGenerationRejectsOverlapButAllowsCleanup(t *testing.T) {
	entered, resume := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	handle := limitProfilePreviewGeneration(func(request host.Request) host.Response {
		if request.Method == "import.preview.start" && calls.Add(1) == 1 {
			close(entered)
			<-resume
		}
		return host.Response{ID: request.ID}
	})
	done := make(chan host.Response, 1)
	go func() { done <- handle(host.Request{Method: "import.preview.start"}) }()
	<-entered
	defer func() { close(resume); <-done }()
	for _, method := range []string{"import.preview", "import.preview.start"} {
		response := handle(host.Request{ID: "busy", Method: method})
		if response.Error == nil || response.Error.Code != "profile_busy" || response.ID != "busy" {
			t.Fatalf("overlapping generation accepted: %+v", response)
		}
	}
	for _, method := range []string{"import.preview.read", "import.preview.release"} {
		if response := handle(host.Request{Method: method}); response.Error != nil {
			t.Fatal(response.Error)
		}
	}
}

func TestPreviewGenerationCanRetryAfterFailure(t *testing.T) {
	calls := 0
	handle := limitProfilePreviewGeneration(func(request host.Request) host.Response {
		calls++
		return host.Response{Error: &host.RPCError{Code: "profile"}}
	})
	for range 2 {
		handle(host.Request{Method: "import.preview.start"})
	}
	if calls != 2 {
		t.Fatal("failed generation retained busy state")
	}
}

func TestLargeProfilePreviewUsesBoundedVerifiedChunks(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, aotConfiguration(directory, get), captureConfiguration(directory, get), wintrayConfiguration(directory, get), selectionConfiguration(directory, nil), quickAssistantConfiguration(directory, nil), strokeConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	states, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	archive := profiles.Archive{CreatedBy: "WinOne-test", Preferences: profiles.Preferences{Name: "large", Theme: "system", Language: "zh-CN", Enabled: map[string]bool{}, Shortcuts: map[string]string{}}, Modules: map[string]profiles.ModuleSettings{}, Paths: map[string]profiles.PathReference{}}
	random := rand.New(rand.NewSource(73))
	for i := 0; i < 4000; i++ {
		label := make([]byte, 256)
		for n := range label {
			label[n] = byte('a' + random.Intn(26))
		}
		archive.Paths[fmt.Sprintf("path.%04d", i)] = profiles.PathReference{Kind: "directory", Label: string(label)}
	}
	data, err := profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "large.winone")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"path": path, "bindings": map[string]string{}})
	handler := profileHandler(store, states)
	if old := handler(host.Request{Version: 1, ID: "old", Method: "import.preview", Params: params}); old.Error == nil {
		t.Fatal("oversized legacy response was allowed")
	}
	response := handler(host.Request{Version: 1, ID: "start", Method: "import.preview.start", Params: params})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var descriptor previewtransfer.Descriptor
	if err = json.Unmarshal(response.Result, &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.Size <= protocol.MaxFrameSize {
		t.Fatal("fixture did not exceed a frame", descriptor.Size)
	}
	output, err := previewtransfer.Receive(context.Background(), descriptor, func(offset int) (previewtransfer.Chunk, error) {
		params, _ := json.Marshal(map[string]any{"token": descriptor.Token, "offset": offset})
		response := handler(host.Request{Version: 1, ID: "chunk", Method: "import.preview.read", Params: params})
		if response.Error != nil {
			return previewtransfer.Chunk{}, fmt.Errorf("%s", response.Error.Message)
		}
		wire, _ := json.Marshal(response)
		if len(wire) >= protocol.MaxFrameSize {
			t.Fatal("chunk exceeded pipe limit")
		}
		var chunk previewtransfer.Chunk
		err := json.Unmarshal(response.Result, &chunk)
		return chunk, err
	})
	if err != nil {
		t.Fatal(err)
	}
	var preview profileFilePreview
	if err = json.Unmarshal(output, &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Paths) != 4000 || preview.Paths[0].Label != archive.Paths["path.0000"].Label {
		t.Fatal("preview paths changed")
	}
	if store.CommittedDocument().Revision != preview.ConfigurationRevision {
		t.Fatal("preview mutated configuration")
	}
	params, _ = json.Marshal(map[string]string{"token": descriptor.Token})
	handler(host.Request{Version: 1, ID: "release", Method: "import.preview.release", Params: params})
}

func TestExportWritesAllCommittedModulesWithoutStartingWorkers(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, aotConfiguration(directory, get), captureConfiguration(directory, get), wintrayConfiguration(directory, get), selectionConfiguration(directory, nil), quickAssistantConfiguration(directory, nil), strokeConfiguration(directory, get), timedPowerConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	states, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = states.SetIntent(0, "capture", false); err != nil {
		t.Fatal(err)
	}
	if _, err = states.FinishStop("capture"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "portable.winone")
	params, _ := json.Marshal(map[string]string{"path": path, "name": "公司与家里"})
	response := profileHandler(store, states)(host.Request{Version: 1, ID: "export", Method: "export", Params: params})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := profiles.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.Modules) != len(catalog.Builtins()) || archive.Preferences.Enabled["capture"] || archive.Preferences.Name != "公司与家里" || archive.Preferences.Shortcuts["always-on-top.toggle"] != "Alt+Q" || archive.Preferences.Shortcuts["quick-assistant.show"] != "Ctrl+Shift+Q" {
		t.Fatal(archive.Preferences)
	}
	if len(archive.Paths) != 1 || archive.Paths["capture.output"].Kind != "directory" {
		t.Fatal(archive.Paths)
	}
}
