//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestUsageExportWritesLargeCSVAndReturnsOnlyCompactReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chosen.csv")
	if err := os.WriteFile(path, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	csv := "model,input,output\n" + strings.Repeat("模型,2000,1000\n", 100000)
	if len(csv) <= 1<<20 {
		t.Fatal("fixture must exceed desktop frame")
	}
	value, err := writeUsageExport(context.Background(), path, ai.UsageQuery{Period: "7d", Provider: "selected", Offset: 15}, func(_ context.Context, query ai.UsageQuery) (json.RawMessage, error) {
		if !query.Export || query.Offset != 0 || query.Provider != "selected" {
			t.Fatalf("filter changed: %+v", query)
		}
		raw, err := json.Marshal(map[string]any{"csv": csv, "total": 100000})
		return raw, err
	}, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, append([]byte{0xEF, 0xBB, 0xBF}, []byte(csv)...)) {
		t.Fatal("CSV was truncated or lost UTF-8 text")
	}
	receipt, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if value.State != "saved" || value.Rows != 100000 || value.Path != path || len(receipt) > 1024 {
		t.Fatalf("invalid receipt: %s", receipt)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary export file remained")
	}
}

func TestUsageExportFailurePreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chosen.csv")
	if err := os.WriteFile(path, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	read := func(context.Context, ai.UsageQuery) (json.RawMessage, error) {
		return json.RawMessage(`{"csv":"model,input\nnew,2\n","total":1}`), nil
	}
	_, err := writeUsageExport(context.Background(), path, ai.UsageQuery{Period: "7d"}, read, func() error { return errors.New("owner exited") })
	if err == nil {
		t.Fatal("closed owner was allowed to write")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "previous" {
		t.Fatal("failed export replaced previous file")
	}
	for _, target := range []string{"relative.csv", filepath.Join(t.TempDir(), "not-csv.txt")} {
		if _, err := writeUsageExport(context.Background(), target, ai.UsageQuery{}, func(context.Context, ai.UsageQuery) (json.RawMessage, error) {
			t.Fatal("invalid target generated CSV")
			return nil, nil
		}, func() error { return nil }); err == nil {
			t.Fatal("invalid path accepted")
		}
	}
}

func TestUsageExportRejectsArbitraryPipePeer(t *testing.T) {
	root := t.TempDir()
	service := newAIService(context.Background(), root, filepath.Join(root, "WinOne.Host.exe"))
	path := filepath.Join(root, "never.csv")
	params, _ := json.Marshal(map[string]any{"query": map[string]string{"period": "7d"}, "path": path})
	for _, pid := range []uint32{0, uint32(os.Getpid())} {
		response := service.handle(host.Request{ID: "export", Method: "usage.export.save", PeerPID: pid, Params: params})
		if response.Error == nil {
			t.Fatal("non-WinOne pipe client accepted")
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("rejected client wrote a file")
	}
}
