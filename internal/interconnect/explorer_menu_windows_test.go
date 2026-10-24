//go:build windows

package interconnect

import (
	"crypto/rand"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExplorerMenuUsesOnlyIsolatedFileAndDirectoryVerbs(t *testing.T) {
	if os.Getenv("WINONE_INTERCONNECT_SYSTEM_TESTS") != "1" {
		t.Skip("explicit system interaction tests only")
	}
	base := `Software\WinOneInterconnectTests\` + rand.Text() + `\Classes`
	t.Cleanup(func() { removeTestRegistry(t, filepath.Dir(base)) })
	exe := filepath.Join(t.TempDir(), "WinOne.exe")
	state, err := configureExplorerMenu(base, exe, true)
	if err != nil || !state.Enabled {
		t.Fatal(state, err)
	}
	t.Cleanup(func() { _, _ = configureExplorerMenu(base, exe, false) })
	for _, entry := range menuEntries {
		key, err := registry.OpenKey(registry.CURRENT_USER, base+`\`+entry+`\command`, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		value, _, err := key.GetStringValue("")
		key.Close()
		if err != nil {
			t.Fatal(err)
		}
		arguments, err := windows.DecomposeCommandLine(value)
		if err != nil || !slices.Equal(arguments, []string{exe, "--interconnect", "--source", "%1"}) {
			t.Fatal(value, err)
		}
	}
	state, err = configureExplorerMenu(base, exe, false)
	if err != nil || state.Enabled || state.Partial {
		t.Fatal(state, err)
	}
}

func TestExplorerMenuDoesNotOverwriteAnUnownedEntry(t *testing.T) {
	if os.Getenv("WINONE_INTERCONNECT_SYSTEM_TESTS") != "1" {
		t.Skip("explicit system interaction tests only")
	}
	base := `Software\WinOneInterconnectTests\` + rand.Text() + `\Classes`
	t.Cleanup(func() { removeTestRegistry(t, filepath.Dir(base)) })
	path := base + `\` + menuEntries[0]
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	defer registry.DeleteKey(registry.CURRENT_USER, path)
	if err := key.SetStringValue("", "keep user menu"); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "WinOne.exe")
	if _, err := configureExplorerMenu(base, exe, true); err == nil {
		t.Fatal("unowned menu overwritten")
	}
	value, _, err := key.GetStringValue("")
	if err != nil || value != "keep user menu" {
		t.Fatal(value, err)
	}
}

func removeTestRegistry(t *testing.T, path string) {
	t.Helper()
	if !strings.HasPrefix(path, `Software\WinOneInterconnectTests\`) {
		t.Fatal("test registry cleanup escaped its namespace")
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.READ)
	if err != nil {
		return
	}
	children, err := key.ReadSubKeyNames(-1)
	key.Close()
	if err != nil {
		t.Error(err)
		return
	}
	for _, child := range children {
		removeTestRegistry(t, path+`\`+child)
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
		t.Error(err)
	}
}
