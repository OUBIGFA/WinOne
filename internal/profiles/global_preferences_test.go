package profiles

import "testing"

func TestPortableGlobalPreferencesRejectInvalidValues(t *testing.T) {
	for _, value := range []string{"not-a-page", "../chat"} {
		archive := sampleArchive()
		archive.Preferences.DefaultPage = &value
		if _, err := Encode(archive); err == nil {
			t.Fatalf("invalid default page accepted: %s", value)
		}
	}
	for _, value := range []string{"A", "Alt+F12", "Ctrl+Alt+Unknown"} {
		archive := sampleArchive()
		archive.Preferences.OpenShortcut = &value
		if _, err := Encode(archive); err == nil {
			t.Fatalf("invalid open shortcut accepted: %s", value)
		}
	}
}

func TestArchivePlanIncludesGlobalOpenShortcutConflicts(t *testing.T) {
	archive := sampleArchive()
	value := "alt+ctrl+s"
	archive.Preferences.OpenShortcut = &value
	plan, err := PrepareArchive(archive, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range plan.ShortcutIssues {
		if issue.Kind == "conflict" && (issue.Action == "application.open" || issue.Other == "application.open") {
			return
		}
	}
	t.Fatalf("global shortcut conflict absent: %+v", plan.ShortcutIssues)
}
