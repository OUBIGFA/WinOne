//go:build windows

package recycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecycleDecisionNeverAllowsPermanentDeletion(t *testing.T) {
	if preDeleteDecision(context.Background(), 0) == 0 {
		t.Fatal("permanent deletion was allowed")
	}
	if preDeleteDecision(context.Background(), 0x80) != 0 {
		t.Fatal("recyclable item was rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if preDeleteDecision(ctx, 0x80) == 0 {
		t.Fatal("cancelled operation was allowed")
	}
}

func TestDirectoryMovesIntoRecycleBinWithContents(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit recycle-bin integration required")
	}
	path := filepath.Join(t.TempDir(), "owned-recycle-fixture")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "proof.txt"), []byte("recoverable fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	receipt, err := Directory(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Recycled || receipt.Path == "" {
		t.Fatal("no recycle-bin receipt", receipt)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("source directory remains", err)
	}
	if data, err := os.ReadFile(filepath.Join(receipt.Path, "proof.txt")); err != nil || string(data) != "recoverable fixture" {
		t.Fatal("recycled content is not recoverable", receipt, err)
	}
	recovered, err := FindDirectory(context.Background(), path)
	if err != nil || recovered.Path != receipt.Path {
		t.Fatal("lost receipt could not be recovered", recovered, err)
	}
	t.Logf("owned directory retained in recycle bin: %s", receipt.Path)
	left := 0
	liveSinks.Range(func(_, _ any) bool { left++; return true })
	if left != 0 {
		t.Fatal("COM progress sinks remained after completion", left)
	}
}

func TestFileMovesIntoRecycleBinAfterFinalCheck(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit recycle-bin integration required")
	}
	path := filepath.Join(t.TempDir(), "owned-log-fixture.log")
	if err := os.WriteFile(path, []byte("recoverable log"), 0600); err != nil {
		t.Fatal(err)
	}
	checked := false
	receipt, err := CheckedFile(context.Background(), path, func() error { checked = true; return nil })
	if err != nil || !checked || !receipt.Recycled {
		t.Fatal(receipt, err)
	}
	if data, err := os.ReadFile(receipt.Path); err != nil || string(data) != "recoverable log" {
		t.Fatal("recycled file is not recoverable", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("original remains", err)
	}
}

func TestFinalDirectoryCheckAbortsBeforeDeletion(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit recycle-bin integration required")
	}
	path := filepath.Join(t.TempDir(), "preserved-fixture")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := CheckedDirectory(context.Background(), path, func() error { called = true; return errors.New("fixture changed") })
	if err == nil || !called {
		t.Fatal("final ownership check was not enforced", err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("rejected target disappeared", err)
	}
}
