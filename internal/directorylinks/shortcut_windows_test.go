//go:build windows

package directorylinks

import (
	"context"
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func createShortcutFixture(t *testing.T, path, target string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if e, ok := err.(*ole.OleError); !ok || e.Code() != 1 {
			t.Fatal(err)
		}
	}
	defer ole.CoUninitialize()
	object, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Release()
	shell, err := object.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Release()
	shortcut, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		t.Fatal(err)
	}
	defer shortcut.Clear()
	result, err := oleutil.PutProperty(shortcut.ToIDispatch(), "TargetPath", target)
	if err != nil {
		t.Fatal(err)
	}
	result.Clear()
	result, err = oleutil.CallMethod(shortcut.ToIDispatch(), "Save")
	if err != nil {
		t.Fatal(err)
	}
	result.Clear()
}

func TestScanReadsExistingShortcutWithoutExecutingOrChangingIt(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "target.txt")
	os.WriteFile(source, []byte("source stays"), 0600)
	path := filepath.Join(root, "existing.lnk")
	createShortcutFixture(t, path, source)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Scan(context.Background(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 || result.Records[0].Kind != "shortcut" || result.Records[0].Status != "active" || result.Records[0].TargetPath != source {
		t.Fatal(result)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("shortcut inspection changed it", err)
	}
	if err = os.Rename(source, source+".gone"); err != nil {
		t.Fatal(err)
	}
	row, err := m.Check(context.Background(), result.Records[0].ID)
	if err != nil || row.Status != "broken" {
		t.Fatal(row, err)
	}
}
