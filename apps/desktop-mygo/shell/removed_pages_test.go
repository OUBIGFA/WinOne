package shell

import "testing"

func TestCatalogContainsOnlyRetainedPages(t *testing.T) {
	want := []string{"overview", "capture", "selection", "quick-assistant", "wintray", "convenience", "interconnect", "ai-services", "settings"}
	got := Catalog()
	if len(got) != len(want) {
		t.Fatalf("page count = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("page %d = %s, want %s", i, got[i].ID, id)
		}
	}
	if !spec("interconnect").footer || spec("interconnect").separated || !spec("ai-services").separated {
		t.Error("Interconnect must sit in the footer immediately above the AI services divider")
	}
	for _, id := range []string{"chat", "onebot", "flash-chat"} {
		if spec(id).id != "overview" {
			t.Errorf("removed route still resolves: %s", id)
		}
	}
}
