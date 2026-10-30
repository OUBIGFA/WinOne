package app

import (
	"testing"
)

const testPipe = `\\.\pipe\WinOne-test`

func TestLaunchOptionsKeepProfilesAndRejectMissingDuplicateOrRemoteValues(t *testing.T) {
	o, err := ParseOptions([]string{"--data-dir", `C:\Profiles\Other`, "--open-settings", "--standard-session"}, testPipe, `C:\Local`)
	if err != nil || o.Directory != `C:\Profiles\Other` || !o.OpenSettings || !o.Standard {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{{"--pipe"}, {"--data-dir", "relative"}, {"--pipe", `\\server\pipe\WinOne`}, {"--open-settings", "--open-settings"}, {"--data-dir", "--open-settings"}} {
		if _, err := ParseOptions(args, testPipe, `C:\Local`); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
}

func TestExplorerLinkPrefillPreservesPathsAndOptions(t *testing.T) {
	o, err := ParseOptions([]string{"--directory-links", "--source", `C:\Documents\notes.txt`, "--link-type", "hardlink", "--link-targets=absolute", "--retain-script"}, testPipe, `C:\Local`)
	if err != nil || !o.DirectoryLinks || !o.OpenSettings || len(o.LinkSources) != 1 || o.LinkOptions["kind"] != "hardlink" || o.LinkOptions["relative"] != false || o.LinkOptions["retainScript"] != true {
		t.Fatalf("%+v %v", o, err)
	}
	if _, err := ParseOptions([]string{"--source", `C:\file`}, testPipe, `C:\Local`); err == nil {
		t.Fatal("source without an entry was accepted")
	}
}

func TestCaptureEntryRequiresCanonicalDraft(t *testing.T) {
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	o, err := ParseOptions([]string{"--capture-draft", id}, testPipe, `C:\Local`)
	if err != nil || o.CaptureDraft != id {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{{"--capture-draft", "../draft"}, {"--capture-draft", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, {"--capture-draft", id, "--open-settings"}} {
		if _, err := ParseOptions(args, testPipe, `C:\Local`); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
}

func TestWakeRequestsRejectFeatureCommands(t *testing.T) {
	if settings, ok := wakeRequest([]string{"--open-settings", "--pipe", "local", "--data-dir", "profile"}); !ok || !settings {
		t.Fatal("open request rejected")
	}
	if settings, ok := wakeRequest(nil); !ok || settings {
		t.Fatal("plain wake misread")
	}
	for _, args := range [][]string{{"--pipe"}, {"--capture-draft", "abc"}, {"--unknown"}} {
		if _, ok := wakeRequest(args); ok {
			t.Errorf("accepted %q", args)
		}
	}
}
