package capture

import "testing"

func TestCollectionPreferenceSavesAndReloads(t *testing.T) {
	fake, h, p, tt := setup(t, true)
	click(t, tt, "截图时默认连续采集文字")
	h.Env.Rail.Save.Run()
	h.Settle(tt)
	if len(fake.saved) != 1 || fake.saved[0]["multiRegionSelection"] != true {
		t.Fatalf("collection preference not saved: %+v", fake.saved)
	}
	p.reload()
	h.Settle(tt)
	if !p.draft.Value.MultiRegionSelection {
		t.Fatal("collection preference lost on reload")
	}
}
