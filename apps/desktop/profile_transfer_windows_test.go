//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/previewtransfer"
	"winone/internal/transport"
)

type previewPipeFixture struct {
	store      *previewtransfer.Store
	data       []byte
	descriptor previewtransfer.Descriptor
	failure    string
}

func (f *previewPipeFixture) Handle(request host.Request) host.Response {
	var result any
	var err error
	var args struct {
		Token  string `json:"token"`
		Offset int    `json:"offset"`
	}
	if err = json.Unmarshal(request.Params, &args); err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Message: err.Error()}}
	}
	switch request.Method {
	case "profiles.import.preview.start":
		f.descriptor, err = f.store.Own(f.data)
		result = f.descriptor
	case "profiles.import.preview.read":
		if f.failure == "read" {
			return host.Response{ID: request.ID, Error: &host.RPCError{Message: "read failed"}}
		}
		result, err = f.store.Read(args.Token, args.Offset)
		if f.failure == "checksum" && err == nil {
			chunk := result.(previewtransfer.Chunk)
			chunk.Data = append([]byte(nil), chunk.Data...)
			chunk.Data[0] ^= 1
			result = chunk
		}
	case "profiles.import.preview.release":
		f.store.Release(args.Token)
		result = map[string]bool{"released": true}
	default:
		err = fmt.Errorf("unexpected method %s", request.Method)
	}
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Message: err.Error()}}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Message: err.Error()}}
	}
	return host.Response{ID: request.ID, Result: data}
}

func TestProfilePreviewCrossesRealPipeAndReleasesBuffer(t *testing.T) {
	for _, failure := range []string{"", "read", "checksum", "json"} {
		t.Run("failure="+failure, func(t *testing.T) {
			testProfilePreviewRelease(t, failure)
		})
	}
}

func testProfilePreviewRelease(t *testing.T, failure string) {
	data, err := json.Marshal(map[string]string{"value": strings.Repeat("配置", 200000)})
	if err != nil {
		t.Fatal(err)
	}
	if failure == "json" {
		data = []byte("invalid json")
	}
	fixture := &previewPipeFixture{store: previewtransfer.NewStore(time.Minute), data: data, failure: failure}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-preview-transfer-%d`, time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- transport.Serve(ctx, listener, fixture) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	app := &App{pipe: pipe}
	output, err := app.Profiles("import.preview", `{}`)
	if failure == "" && (err != nil || output != string(data)) {
		t.Fatal("large preview failed over pipe", err)
	}
	if failure != "" && (err == nil || output != "") {
		t.Fatalf("failed transfer returned output, error=%v", err)
	}
	if _, err = fixture.store.Read(fixture.descriptor.Token, 0); err == nil {
		t.Fatal("desktop retained completed transfer")
	}
}
