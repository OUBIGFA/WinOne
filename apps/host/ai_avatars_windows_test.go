//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/protocol"
)

func avatarRequest(s *aiService, method string, peer uint32, params any) host.Response {
	data, _ := json.Marshal(params)
	return s.handle(host.Request{ID: "avatar", Method: method, PeerPID: peer, Params: data})
}

func TestAvatarUploadBelongsToOnePeerAndRequiresContiguousChunks(t *testing.T) {
	s := newAIService(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "WinOne.exe"))
	response := avatarRequest(s, "icons.store", 101, map[string]any{"data": []byte("first"), "size": 100})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var pending struct {
		Upload string `json:"upload"`
	}
	if err := json.Unmarshal(response.Result, &pending); err != nil || pending.Upload == "" {
		t.Fatalf("no upload receipt: %s, %v", response.Result, err)
	}
	for _, args := range []struct {
		peer   uint32
		offset int
	}{{102, 5}, {101, 4}, {101, 6}} {
		r := avatarRequest(s, "icons.store", args.peer, map[string]any{"data": []byte("next"), "size": 100, "offset": args.offset, "upload": pending.Upload})
		if r.Error == nil {
			t.Fatal("accepted another peer or a missing/duplicate chunk")
		}
	}
	response = avatarRequest(s, "icons.store", 101, map[string]any{"data": []byte("next"), "size": 100, "offset": 5, "upload": pending.Upload})
	if response.Error != nil {
		t.Fatal("valid continuation was lost", response.Error)
	}
}

func TestAvatarReadFitsFramesAndRefusesChangedContent(t *testing.T) {
	s := newAIService(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "WinOne.exe"))
	// A maximum-size SVG crosses the frame limit once base64 encoded. The
	// transfer must return the complete original bytes across bounded replies.
	data := append([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), bytes.Repeat([]byte(" "), (1<<20)-46)...)
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:8]) + ".svg"
	directory := filepath.Join(s.translator.DataDirectory, "config", "magpie", "icons")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	var received []byte
	for len(received) < len(data) {
		r := avatarRequest(s, "icons.read", 101, map[string]any{"icon": "file:" + name, "offset": len(received)})
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		frame, _ := json.Marshal(r)
		if len(frame) >= protocol.MaxFrameSize {
			t.Fatal("avatar exceeded pipe frame")
		}
		var chunk struct {
			Data     []byte `json:"data"`
			Size     int    `json:"size"`
			MimeType string `json:"mimeType"`
		}
		if err := json.Unmarshal(r.Result, &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Size != len(data) || len(chunk.Data) == 0 || chunk.MimeType != "image/svg+xml" {
			t.Fatalf("invalid avatar chunk: size=%d mime=%q", chunk.Size, chunk.MimeType)
		}
		received = append(received, chunk.Data...)
	}
	if !bytes.Equal(received, data) {
		t.Fatal("avatar changed during transfer")
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte(`<svg/>`), 0600); err != nil {
		t.Fatal(err)
	}
	if r := avatarRequest(s, "icons.read", 101, map[string]any{"icon": "file:" + name}); r.Error == nil {
		t.Fatal("read picture whose content no longer matches its identity")
	}
	if r := avatarRequest(s, "icons.read", 101, map[string]any{"icon": "file:../ai-settings.json"}); r.Error == nil {
		t.Fatal("read outside the avatar directory")
	}
}
