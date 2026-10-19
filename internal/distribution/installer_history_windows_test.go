//go:build windows

package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerHistoryDoesNotBlockAfterDependencyUpgrade(t *testing.T) {
	root := t.TempDir()
	installation := filepath.Join(root, "Programs", "WinOne")
	if err := os.MkdirAll(installation, 0700); err != nil { t.Fatal(err) }
	request := InstallerUpdate{Installation:installation, Profile:filepath.Join(root,"profile"), RegistryKey:`Software\WinOne-History-Test`, ShortcutPath:filepath.Join(root,"WinOne.lnk")}
	base := installation + ".update"
	for i, phase := range []string{"committed", "rolled-back"} {
		dir := base
		if i != 0 { dir += "-1" }
		if err := os.Mkdir(dir, 0700); err != nil { t.Fatal(err) }
		owner, _ := json.Marshal(installerUpdateOwner{request.Installation, request.Profile, request.RegistryKey, request.ShortcutPath})
		if err := os.WriteFile(filepath.Join(dir,"owner.json"), owner, 0600); err != nil { t.Fatal(err) }
		plan := journalPlan()
		plan.Files[0].Path = "capture/Ocr/python/Lib/site-packages/removed-version.dist-info/METADATA"
		plan.Files[0].PreviousPath = plan.Files[0].Path
		journal, err := CreateUpgradeJournal(filepath.Join(dir,"journal.json"), plan)
		if err != nil { t.Fatal(err) }
		state := journal.Snapshot(); state.Phase = phase
		if phase == "committed" {
			state.FileStates[0] = "applied"
			for key := range state.MetadataStates { state.MetadataStates[key] = "applied" }
		}
		raw, _ := json.Marshal(state)
		if err := os.WriteFile(filepath.Join(dir,"journal.json"), raw, 0600); err != nil { t.Fatal(err) }
	}
	selected, err := installerWorkDirectory(request, base)
	if err != nil || selected != base+"-2" { t.Fatalf("completed history blocked reinstall: %s %v",selected,err) }
	// Do not erase history to make installation work, and do not ignore a
	// genuine interrupted write marker just because all known records ended.
	if _, err := os.Stat(filepath.Join(base,"journal.json")); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(installation,".winone-pending"), []byte("unmatched"), 0600); err != nil { t.Fatal(err) }
	if _, err := installerWorkDirectory(request, base); err == nil { t.Fatal("unmatched pending marker was ignored") }
}
