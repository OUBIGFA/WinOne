package interconnect

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"slices"
	"testing"
	"winone/desktopmygo/i18n"
)

// After adding content, both senders give the list the space used by the
// empty drop invitation. These are the real controls, including removal.
func TestSelectedFilesBecomeTheTransferContent(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	for _, cloud := range []bool{false, true} {
		var calls []call
		h := interconnectHost(t, general{}, &calls)
		p := newTransferPanel(h.Env, cloud)
		p.add([]string{`C:\Documents\Quarterly report.pdf`, `D:\Photos\holiday.jpg`})
		tt := ui.NewTester(func(c *ui.Context) { p.composer(c, true, false) }, 560, 1100)
		h.Settle(tt)
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			i18n.SetLocale(locale)
			h.Shots(t, fmt.Sprintf("selected-files-%s-%t", locale, cloud), func(c *ui.Context) { p.composer(c, true, false) }, 900, nil)
		}
		i18n.SetLocale(i18n.Chinese)
		if tt.HasText(tr("drop")) {
			t.Error("empty drop invitation still takes priority over the selected files")
		}
		for _, name := range []string{"Quarterly report.pdf", "holiday.jpg"} {
			if !slices.Contains(tt.Texts(), name) {
				t.Errorf("file name is not the list's primary text: %s", name)
			}
		}
		if err := tt.Click(tr("remove") + " " + `C:\Documents\Quarterly report.pdf`); err != nil {
			t.Fatal(err)
		}
		h.Settle(tt)
		if len(p.form.Paths) != 1 || p.form.Paths[0] != `D:\Photos\holiday.jpg` {
			t.Fatal("removal changed the wrong file", p.form.Paths)
		}
		if err := tt.Click(tr("fileActions")); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(tr("clear")); err != nil {
			t.Fatal(err)
		}
		h.Settle(tt)
		if !tt.HasText(tr("drop")) {
			t.Error("clearing files did not restore the empty drop invitation")
		}
	}
}

// A batch must not push the send action below a tall, unbounded file list.
func TestLargeFileSelectionKeepsSendingWithinReach(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	for _, cloud := range []bool{false, true} {
		var calls []call
		h := interconnectHost(t, general{}, &calls)
		p := newTransferPanel(h.Env, cloud)
		for i := 0; i < 40; i++ {
			p.add([]string{fmt.Sprintf(`C:\Documents\客户资料与长期归档\Quarterly report with a detailed filename and revision %02d.pdf`, i)})
		}
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			i18n.SetLocale(locale)
			tt := ui.NewTester(func(c *ui.Context) { p.composer(c, true, false) }, 560, 900)
			h.Settle(tt)
			label := tr("sendSelection")
			if cloud {
				label = tr("uploadAndShare")
			}
			bounds, ok := tt.Find(label)
			if !ok || bounds.Y+bounds.H > 700 {
				t.Fatalf("batch hides sending: %s %+v", label, bounds)
			}
			for _, text := range tt.Texts() {
				if b, ok := tt.Find(text); ok && (b.X < -.5 || b.X+b.W > 560.5) {
					t.Errorf("file content exceeds window: %q %+v", text, b)
				}
			}
			h.Shots(t, fmt.Sprintf("file-batch-%s-%t", locale, cloud), func(c *ui.Context) { p.composer(c, true, false) }, 900, nil)
		}
	}
}
