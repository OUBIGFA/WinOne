package automations

import "testing"

// A narrow list initially selects its first schedule in the Host snapshot.
// Pressing that same real native row must reveal details; it must not require
// a different schedule to be selected first or issue another history read.
func TestSelectingTheCurrentScheduleRevealsItsNativeNarrowDetails(t *testing.T) {
	_, h, p, tt := setup(t, 560)
	if p.selected != "s1" || p.narrowDetail || tt.HasText(tr("edit")) {
		t.Fatal("the fixture did not start at the production narrow list")
	}
	reads := len(h.Called("chat.automations.runs"))
	if err := tt.Click("Daily report"); err != nil {
		t.Fatal(err)
	}
	h.Settle(tt)
	if !p.narrowDetail || !tt.HasText(tr("edit")) || !tt.HasText("Finished") {
		t.Fatal("pressing the current schedule did not reveal its actual detail/history controls")
	}
	if p.selected != "s1" || p.runsID != "s1" || len(h.Called("chat.automations.runs")) != reads {
		t.Fatal("revealing the current owner changed its history or issued a duplicate read")
	}
}
