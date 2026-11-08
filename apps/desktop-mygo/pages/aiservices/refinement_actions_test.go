package aiservices

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

func TestSearchKeySlotsKeepOrderingSwitchAndExplicitReveal(t *testing.T) {
	_, h, p, _ := setup(t)
	a := newAdvancedPanel(p)
	keys := searchKeySlots("first; second\nfirst\tsecond, first")
	if len(keys) != 2 {
		t.Fatal("repeated pasted key became another slot")
	}
	a.settings().SearchAPIs = []core.Preserved[searchAPI]{{Value: searchAPI{Vendor: "fixture", KeyList: &keys}}}
	tt := ui.NewTester(func(c *ui.Context) { a.searchKeys(c, a.settings().SearchAPIs[0].Value, false) }, 560, 900)
	settle(h, tt)
	if slices.Contains(tt.Texts(), "first") {
		t.Fatal("key visible before reveal")
	}
	name := i18n.T("aiNative.searchKeyNumber", "number", 1)
	if err := tt.Click(i18n.T("ai.copySearchKey", "name", name)); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if tt.Clipboard() != "first" {
		t.Fatal("copied the wrong key")
	}
	if err := tt.Click(nativeT("moveDown") + " " + name); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(name); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	got := *a.settings().SearchAPIs[0].Value.KeyList
	if got[0].Key != "second" || !got[0].Off || got[1].Key != "first" {
		t.Fatalf("key ordering/switch changed credentials: %+v", got)
	}
	for range 2 {
		if err := tt.Click(nativeT("remove") + " " + name); err != nil {
			t.Fatal(err)
		}
		settle(h, tt)
	}
	if list := a.settings().SearchAPIs[0].Value.KeyList; list == nil || len(*list) != 0 || !tt.HasText(nativeT("noSearchKeys")) {
		t.Fatal("last removal restored legacy credentials")
	}
}

func TestMaskingValueActionsCopyAndStageAnException(t *testing.T) {
	_, h, p, _ := setup(t)
	a := newAdvancedPanel(p)
	m := &maskingRecords{owner: a, selected: &maskingRecord{Key: "fixture", Values: []maskingValue{{Placeholder: "MASK", Kind: "EMAIL", Known: true}}}, values: map[string]string{"MASK": "person@example.test"}}
	tt := ui.NewTester(func(c *ui.Context) { m.detailView(c, false) }, 900, 900)
	settle(h, tt)
	if err := tt.Click(nativeT("copy")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if tt.Clipboard() != "person@example.test" {
		t.Fatal("copy did not use original value")
	}
	if err := tt.Click(nativeT("neverMaskValue")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !slices.Contains(a.settings().RedactAllow, "person@example.test") || !a.focusPrivacy || m.selected != nil {
		t.Fatal("exception did not open the editable privacy draft")
	}
	if len(p.saved.Value.magpie().RedactAllow) != 0 {
		t.Fatal("exception bypassed Save")
	}
}
