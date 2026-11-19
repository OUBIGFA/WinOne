package aiservices

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/desktopmygo/i18n"
)

func TestHeaderPresetKeepsUserValuesAndFillsCaseInsensitiveEmptyRows(t *testing.T) {
	rows := []*headerRow{{Name: "user-agent", Value: "my-agent", JSON: true}, {Name: "  VERSION ", Value: " "}, {}, {Name: "X-Private", Value: "kept"}}
	var codex headerPreset
	for _, preset := range headerPresets {
		if preset.Name == "Codex CLI" {
			codex = preset
		}
	}
	if codex.Name == "" {
		t.Fatal("the original Codex preset is unavailable")
	}
	applyHeaderPreset(&rows, codex)
	if len(rows) != 4 || rows[0].Value != "my-agent" || !rows[0].JSON || rows[1].Value != "0.167.0" || rows[2].Value != "kept" || rows[3].Name != "originator" || rows[3].Value != "codex_cli_rs" {
		t.Fatalf("preset overwrote or omitted headers: %+v", rows)
	}
	applyHeaderPreset(&rows, codex)
	if len(rows) != 4 {
		t.Fatal("applying a preset twice duplicated its headers")
	}
}

func TestHeaderPresetNativeSelectionSavesAndReopens(t *testing.T) {
	f, h, p, _ := setup(t)
	pr := p.current()
	pr.Headers = map[string]string{"User-Agent": "user-value", "X-Private": "keep"}
	e := p.editor
	e.reset(pr)
	tester := ui.NewTester(func(c *ui.Context) { e.headersView(c, pr, false) }, 900, 900)
	for _, label := range []string{i18n.T("aiSync.headerPresets"), "Codex CLI"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		h.Settle(tester)
	}
	if pr.Headers["User-Agent"] != "user-value" || pr.Headers["originator"] != "codex_cli_rs" || pr.Headers["version"] != "0.167.0" || pr.Headers["X-Private"] != "keep" {
		t.Fatalf("native preset was not applied to the draft: %+v", pr.Headers)
	}
	shot(t, tester, "header-preset-codex")
	p.save()
	h.Settle(tester)
	if f.lastSave() == nil {
		t.Fatal("preset change did not save")
	}
	current := p.current()
	if current.Headers["originator"] != "codex_cli_rs" || current.Headers["User-Agent"] != "user-value" {
		t.Fatal("saved preset did not survive reread")
	}
}
