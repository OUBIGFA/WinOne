package shell

import "testing"

func TestCatalogContainsOnlyRetainedPages(t *testing.T) {
	want := []string{"overview", "capture", "selection", "quick-assistant", "interconnect", "wintray", "convenience", "ai-services", "settings"}
	got := Catalog()
	if len(got) != len(want) {
		t.Fatalf("page count = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("page %d = %s, want %s", i, got[i].ID, id)
		}
	}
	for _, id := range []string{"chat", "onebot", "flash-chat"} {
		if spec(id).id != "overview" {
			t.Errorf("removed route still resolves: %s", id)
		}
	}
}
